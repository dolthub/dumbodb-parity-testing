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

// CompareResult indicates the outcome of a comparison.
type CompareResult int

const (
	Match        CompareResult = iota
	Diverge
	CompareError
)

// Comparison holds the result of comparing two responses.
type Comparison struct {
	Result CompareResult
	Diff   string
}

// defaultIgnoredFields are document fields omitted from comparison because
// their values are non-deterministic (timestamps, generated IDs).
var defaultIgnoredFields = map[string]bool{
	"operationTime": true,
	"$clusterTime":  true,
	"insertedId":    true,
	"insertedIds":   true,
}

// CompareResponses compares a MongoDB response against a Dongo response,
// handling both result values and errors.
func CompareResponses(mongoResult interface{}, mongoErr error, dongoResult interface{}, dongoErr error) Comparison {
	if mongoErr != nil || dongoErr != nil {
		return compareErrors(mongoErr, dongoErr)
	}
	return compareValues(normalize(mongoResult), normalize(dongoResult))
}

func compareErrors(mongoErr, dongoErr error) Comparison {
	if mongoErr == nil && dongoErr == nil {
		return Comparison{Result: Match}
	}
	if mongoErr == nil || dongoErr == nil {
		return Comparison{
			Result: Diverge,
			Diff:   fmt.Sprintf("mongo err=%v, dongo err=%v", mongoErr, dongoErr),
		}
	}
	mCode := errorCode(mongoErr)
	dCode := errorCode(dongoErr)
	if mCode != dCode {
		return Comparison{
			Result: Diverge,
			Diff:   fmt.Sprintf("error code mismatch: mongo=%d dongo=%d", mCode, dCode),
		}
	}
	mMsg := mongoErr.Error()
	dMsg := dongoErr.Error()
	if mMsg != dMsg {
		return Comparison{
			Result: Diverge,
			Diff:   fmt.Sprintf("error message mismatch:\n  mongo: %s\n  dongo: %s", mMsg, dMsg),
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

// normalize converts any value to a stable, comparable representation.
// It strips non-deterministic fields (ObjectIds, timestamps, insertedId)
// and normalizes numeric types so 1 == 1.0.
func normalize(v interface{}) interface{} {
	if v == nil {
		return nil
	}

	// Dereference pointers.
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
	case []interface{}:
		return normalizeSlice(val)
	case primitive.ObjectID:
		return objectIDSentinel
	case primitive.DateTime:
		return "<DateTime>"
	case primitive.Timestamp:
		return "<Timestamp>"
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case float32:
		return float64(val)
	case float64:
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

// objectIDSentinel is a placeholder used to detect ObjectID values during normalization.
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
		return fmt.Sprintf("values differ in type: mongo=%T dongo=%T", a, b)
	}
	return fmt.Sprintf("mongo:  %s\ndongo:  %s", aStr, bStr)
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
