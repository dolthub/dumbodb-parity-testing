package tests

// Parity families for KeyString encoding soundness (Phase E of
// dumbodb docs/design/secondary-index-structural-sharing.md):
//
//   Index_MixedTypeBrackets_* -- behavior T1 (queries never leak
//                                across MongoDB type brackets)
//   Index_RegexFilter_*       -- regex filters on indexed fields are
//                                pattern matches, not equality probes
//   Index_MultikeyMixedTypes_* / Index_Multikey_* -- behavior T4 and
//                                multikey range dedup

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// idxmMixedBracketSetup spans most type brackets over one indexed field.
func idxmMixedBracketSetup(ctx context.Context, col *mongo.Collection) error {
	docs := []interface{}{
		bson.D{{Key: "_id", Value: "nul"}, {Key: "f", Value: nil}},
		bson.D{{Key: "_id", Value: "n1"}, {Key: "f", Value: int32(1)}},
		bson.D{{Key: "_id", Value: "n2"}, {Key: "f", Value: float64(2.5)}},
		bson.D{{Key: "_id", Value: "sa"}, {Key: "f", Value: "alpha"}},
		bson.D{{Key: "_id", Value: "sz"}, {Key: "f", Value: "zulu"}},
		bson.D{{Key: "_id", Value: "bt"}, {Key: "f", Value: true}},
		bson.D{{Key: "_id", Value: "dt"}, {Key: "f", Value: primitive.NewDateTimeFromTime(time.Unix(1000, 0).UTC())}},
		bson.D{{Key: "_id", Value: "ts"}, {Key: "f", Value: primitive.Timestamp{T: 5, I: 1}}},
	}
	if _, err := col.InsertMany(ctx, docs); err != nil {
		return err
	}
	_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "f", Value: int32(1)}},
	})
	return err
}

func TestIndex_MixedTypeBrackets_NumericRangeExcludesOthers(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_MixedTypeBrackets_NumericRangeExcludesOthers",
		Support: harness.DumboDBFull,
		Setup:   idxmMixedBracketSetup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			out := bson.D{}
			for _, probe := range []struct {
				label  string
				filter interface{}
			}{
				// Open-ended numeric ranges: must match numbers only,
				// never the null / string / bool / date / ts docs.
				{"gt0", bson.D{{Key: "f", Value: bson.D{{Key: "$gt", Value: int32(0)}}}}},
				{"lt10", bson.D{{Key: "f", Value: bson.D{{Key: "$lt", Value: int32(10)}}}}},
			} {
				d, err := idxmProbe(ctx, col, probe.label, probe.filter)
				if err != nil {
					return nil, err
				}
				out = append(out, d...)
			}
			return out, nil
		},
	})
}

func TestIndex_MixedTypeBrackets_StringRangeExcludesOthers(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_MixedTypeBrackets_StringRangeExcludesOthers",
		Support: harness.DumboDBFull,
		Setup:   idxmMixedBracketSetup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			out := bson.D{}
			for _, probe := range []struct {
				label  string
				filter interface{}
			}{
				{"gt_a", bson.D{{Key: "f", Value: bson.D{{Key: "$gt", Value: "a"}}}}},
				{"lt_zz", bson.D{{Key: "f", Value: bson.D{{Key: "$lt", Value: "zz"}}}}},
			} {
				d, err := idxmProbe(ctx, col, probe.label, probe.filter)
				if err != nil {
					return nil, err
				}
				out = append(out, d...)
			}
			return out, nil
		},
	})
}

func TestIndex_MixedTypeBrackets_TimestampRange(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_MixedTypeBrackets_TimestampRange",
		Support: harness.DumboDBFull,
		Setup: idxmSetupNamed(bson.D{
			{Key: "t1", Value: primitive.Timestamp{T: 1, I: 0}},
			{Key: "t5", Value: primitive.Timestamp{T: 5, I: 0}},
			{Key: "t9", Value: primitive.Timestamp{T: 9, I: 0}},
			{Key: "num", Value: int32(3)},
		}, "f"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return idxmProbe(ctx, col, "gt_ts3",
				bson.D{{Key: "f", Value: bson.D{{Key: "$gt", Value: primitive.Timestamp{T: 3, I: 0}}}}})
		},
	})
}

func TestIndex_RegexFilter_OnIndexedField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_RegexFilter_OnIndexedField",
		Support: harness.DumboDBFull,
		Setup: idxmSetupNamed(bson.D{
			{Key: "a1", Value: "apple"},
			{Key: "a2", Value: "apricot"},
			{Key: "b1", Value: "banana"},
			{Key: "nul", Value: nil},
		}, "f"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// A bare regex filter is a pattern match. An index that
			// treats it as an equality probe returns nothing.
			ids, err := idxmFindIDs(ctx, col, bson.D{
				{Key: "f", Value: primitive.Regex{Pattern: "^ap", Options: ""}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "ids", Value: ids}}, nil
		},
	})
}

func TestIndex_Multikey_RangeNoDuplicates(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Multikey_RangeNoDuplicates",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			docs := []interface{}{
				bson.D{{Key: "_id", Value: "m1"}, {Key: "f", Value: bson.A{int32(5), int32(6), int32(7)}}},
				bson.D{{Key: "_id", Value: "m2"}, {Key: "f", Value: bson.A{int32(1), int32(9)}}},
				bson.D{{Key: "_id", Value: "s1"}, {Key: "f", Value: int32(8)}},
			}
			if _, err := col.InsertMany(ctx, docs); err != nil {
				return err
			}
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "f", Value: int32(1)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// m1 has three elements > 4: it must appear once, not three
			// times. Counts go through the count command; multikey range
			// counts must match MongoDB (doc-level, not entry-level).
			return idxmProbe(ctx, col, "gt4",
				bson.D{{Key: "f", Value: bson.D{{Key: "$gt", Value: int32(4)}}}})
		},
	})
}

func TestIndex_MultikeyMixedTypes_ElementLookups(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_MultikeyMixedTypes_ElementLookups",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			docs := []interface{}{
				bson.D{{Key: "_id", Value: "mix"}, {Key: "f", Value: bson.A{int32(5), "five", true}}},
				bson.D{{Key: "_id", Value: "num"}, {Key: "f", Value: bson.A{int32(5), int32(6)}}},
			}
			if _, err := col.InsertMany(ctx, docs); err != nil {
				return err
			}
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "f", Value: int32(1)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			out := bson.D{}
			for _, probe := range []struct {
				label  string
				filter interface{}
			}{
				{"int5", bson.D{{Key: "f", Value: int32(5)}}},
				{"strfive", bson.D{{Key: "f", Value: "five"}}},
				{"booltrue", bson.D{{Key: "f", Value: true}}},
			} {
				d, err := idxmProbe(ctx, col, probe.label, probe.filter)
				if err != nil {
					return nil, err
				}
				out = append(out, d...)
			}
			return out, nil
		},
	})
}
