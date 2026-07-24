package harness

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type CompareResult int

const (
	Match        CompareResult = iota
	Diverge
	CompareError
)

type Comparison struct {
	Result CompareResult
	Diff   string
}

// defaultIgnoredFields are document fields omitted from comparison because
// their values are non-deterministic (timestamps, generated IDs) or are
// physical storage metrics that depend on the storage engine rather than
// the data (WiredTiger vs Dolt prolly trees report different on-disk byte
// counts for identical documents). Field names are matched as they appear
// after normalization; the mongo driver marshals result structs with
// lowercased field names (sizeOnDisk -> sizeondisk), so both cases are
// listed for robustness against raw runCommand documents.
var defaultIgnoredFields = map[string]bool{
	"operationTime": true,
	"$clusterTime":  true,
	"insertedId":    true,
	"insertedIds":   true,
	"sizeOnDisk":    true,
	"sizeondisk":    true,
	"totalSize":     true,
	"totalsize":     true,
}

// millisToleranceMS is the maximum amount DumboDB is allowed to be slower
// than MongoDB on a server-side `millis` field before the comparison is
// flagged as a divergence. DumboDB being faster is always accepted; a
// small absolute slack catches sub-tick rounding noise and routine jitter
// while still flagging cases where DumboDB is materially slower (an early
// performance-regression signal).
const millisToleranceMS = 5

// CompareResponses compares a MongoDB response against a DumboDB response,
// handling both result values and errors.
func CompareResponses(mongoResult interface{}, mongoErr error, dumboDBResult interface{}, dumboDBErr error) Comparison {
	if mongoErr != nil || dumboDBErr != nil {
		return compareErrors(mongoErr, dumboDBErr)
	}
	mongo := normalize(mongoResult)
	dumbodb := normalize(dumboDBResult)
	harmonizeMillis(mongo, dumbodb)
	return compareValues(mongo, dumbodb)
}

// harmonizeMillis walks the two normalized values in tandem. At any map
// level where both sides report a numeric "millis" field, the DumboDB
// value is rewritten to the MongoDB value when DumboDB is at most
// millisToleranceMS slower (or any amount faster). The subsequent
// reflect.DeepEqual then treats those millis pairs as matched.
//
// DumboDB slower than MongoDB by more than the tolerance is left
// unmodified and surfaces as a divergence.
func harmonizeMillis(mongo, dumbodb interface{}) {
	switch ma := mongo.(type) {
	case map[string]interface{}:
		da, ok := dumbodb.(map[string]interface{})
		if !ok {
			return
		}
		if mv, ok := ma["millis"]; ok {
			if dv, ok2 := da["millis"]; ok2 {
				mf, ok3 := toFloat64(mv)
				df, ok4 := toFloat64(dv)
				if ok3 && ok4 && df <= mf+float64(millisToleranceMS) {
					da["millis"] = mv
				}
			}
		}
		for k, v := range ma {
			if dv, ok := da[k]; ok {
				harmonizeMillis(v, dv)
			}
		}
	case []interface{}:
		da, ok := dumbodb.([]interface{})
		if !ok || len(da) != len(ma) {
			return
		}
		for i := range ma {
			harmonizeMillis(ma[i], da[i])
		}
	}
}

func compareErrors(mongoErr, dumboDBErr error) Comparison {
	if mongoErr == nil && dumboDBErr == nil {
		return Comparison{Result: Match}
	}
	if mongoErr == nil || dumboDBErr == nil {
		return Comparison{
			Result: Diverge,
			Diff:   fmt.Sprintf("mongo err=%v, dumbodb err=%v", mongoErr, dumboDBErr),
		}
	}
	mCode := errorCode(mongoErr)
	dCode := errorCode(dumboDBErr)
	if mCode != dCode {
		return Comparison{
			Result: Diverge,
			Diff:   fmt.Sprintf("error code mismatch: mongo=%d dumbodb=%d", mCode, dCode),
		}
	}
	// codeName is the stable, human-readable identity of an error. Compare it
	// when both sides provide one; a mismatch here (e.g. same numeric code but
	// different codeName) is a real divergence and yields a clearer diagnostic
	// than the raw message comparison below.
	mName := errorName(mongoErr)
	dName := errorName(dumboDBErr)
	if mName != "" && dName != "" && mName != dName {
		return Comparison{
			Result: Diverge,
			Diff:   fmt.Sprintf("error codeName mismatch: mongo=%q dumbodb=%q (code=%d)", mName, dName, mCode),
		}
	}
	mMsg := mongoErr.Error()
	dMsg := dumboDBErr.Error()
	if mMsg != dMsg {
		return Comparison{
			Result: Diverge,
			Diff:   fmt.Sprintf("error message mismatch:\n  mongo: %s\n  dumbodb: %s", mMsg, dMsg),
		}
	}
	return Comparison{Result: Match}
}

func errorCode(err error) int32 {
	if err == nil {
		return 0
	}
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return int32(cmdErr.Code)
	}
	var writeExc mongo.WriteException
	if errors.As(err, &writeExc) && len(writeExc.WriteErrors) > 0 {
		return int32(writeExc.WriteErrors[0].Code)
	}
	return 0
}

// errorName returns the MongoDB codeName of err when it is a CommandError, or
// the empty string otherwise (WriteError carries no codeName).
func errorName(err error) string {
	if err == nil {
		return ""
	}
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Name
	}
	return ""
}

// normalize converts any value to a stable, comparable representation.
// It strips non-deterministic fields (ObjectIds, timestamps, insertedId)
// and normalizes numeric types so 1 == 1.0.
func normalize(v interface{}) interface{} {
	if v == nil {
		return nil
	}

	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	v = rv.Interface()

	switch val := v.(type) {
	case bson.D:
		return normalizeBSONDoc(val)
	case bson.M:
		m := make(map[string]interface{}, len(val))
		for k, v2 := range val {
			if defaultIgnoredFields[k] {
				continue
			}
			norm := normalize(v2)
			if norm == objectIDSentinel {
				continue
			}
			m[k] = norm
		}
		return m
	case bson.A:
		return normalizeSlice([]interface{}(val))
	case []bson.M:
		s := make([]interface{}, len(val))
		for i, doc := range val {
			s[i] = normalize(doc)
		}
		return s
	case []bson.D:
		// bson.D preserves insertion order; dumbodb's bson-a storage
		// lex-sorts object fields on write. Comparing field order
		// would diverge for every multi-field result. Normalise each
		// element to a map so the comparison is order-insensitive.
		s := make([]interface{}, len(val))
		for i, doc := range val {
			s[i] = normalize(doc)
		}
		return s
	case []interface{}:
		return normalizeSlice(val)
	case primitive.ObjectID:
		return objectIDSentinel
	case primitive.Binary:
		// UUID binaries (subtype 3 or 4) are server-generated and will differ
		// between MongoDB and DumboDB instances. Normalize them to a sentinel so
		// the comparison focuses on structural equality rather than the raw bytes.
		if val.Subtype == 3 || val.Subtype == 4 {
			return "<UUID>"
		}
		return val
	case primitive.DateTime:
		return "<DateTime>"
	case primitive.Timestamp:
		return "<Timestamp>"
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case float32:
		f := float64(val)
		if math.IsNaN(f) {
			return "<NaN>"
		}
		return f
	case float64:
		if math.IsNaN(val) {
			return "<NaN>"
		}
		return val
	case bool, string:
		return val
	default:
		// For struct types (e.g. *mongo.UpdateResult), marshal to BSON then normalize.
		data, err := bson.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		var doc bson.D
		if err := bson.Unmarshal(data, &doc); err != nil {
			return fmt.Sprintf("%v", v)
		}
		return normalizeBSONDoc(doc)
	}
}

const objectIDSentinel = "<ObjectID>"

func normalizeBSONDoc(d bson.D) map[string]interface{} {
	m := make(map[string]interface{}, len(d))
	for _, elem := range d {
		if defaultIgnoredFields[elem.Key] {
			continue
		}
		norm := normalize(elem.Value)
		if norm == objectIDSentinel {
			continue
		}
		m[elem.Key] = norm
	}
	return m
}

func normalizeSlice(s []interface{}) []interface{} {
	out := make([]interface{}, 0, len(s))
	for _, elem := range s {
		norm := normalize(elem)
		out = append(out, norm)
	}
	return out
}

func compareValues(a, b interface{}) Comparison {
	if reflect.DeepEqual(a, b) {
		return Comparison{Result: Match}
	}
	// Numeric tolerance: 1 == 1.0.
	if fa, ok := toFloat64(a); ok {
		if fb, ok := toFloat64(b); ok {
			if math.Abs(fa-fb) < 1e-9 {
				return Comparison{Result: Match}
			}
		}
	}
	return Comparison{
		Result: Diverge,
		Diff:   formatDiff(a, b),
	}
}

func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	}
	return 0, false
}

func formatDiff(a, b interface{}) string {
	aStr := formatValue(a)
	bStr := formatValue(b)
	if aStr == bStr {
		return fmt.Sprintf("values differ in type: mongo=%T dumbodb=%T", a, b)
	}
	return fmt.Sprintf("mongo:  %s\ndumbodb:  %s", aStr, bStr)
}

func formatValue(v interface{}) string {
	switch val := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var sb strings.Builder
		sb.WriteString("{")
		for i, k := range keys {
			if i > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "%q: %s", k, formatValue(val[k]))
		}
		sb.WriteString("}")
		return sb.String()
	case []interface{}:
		var sb strings.Builder
		sb.WriteString("[")
		for i, elem := range val {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(formatValue(elem))
		}
		sb.WriteString("]")
		return sb.String()
	case string:
		return fmt.Sprintf("%q", val)
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", val)
	}
}
