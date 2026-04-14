// Parity tests migrated from dolthub/dumbodb/tests/regex_probe_test.go.
// Probes regex, text-search, and $mod behaviour across MongoDB and DumboDB.
package tests

import (
	"context"
	"math"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// insertTextIndex creates a text index on the given fields using the provided name.
func insertTextIndex(ctx context.Context, col *mongo.Collection, fields bson.D, name string) error {
	_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    fields,
		Options: options.Index().SetName(name),
	})
	return err
}

// ─── regex ────────────────────────────────────────────────────────────────────

func TestProbeRegexCI(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ProbeRegexCI",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: int32(1)}, {Key: "name", Value: "Hello World"}},
				bson.D{{Key: "_id", Value: int32(2)}, {Key: "name", Value: "goodbye world"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "name", Value: primitive.Regex{Pattern: "hello", Options: "i"}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestProbeRegexMultiline(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ProbeRegexMultiline",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: int32(1)}, {Key: "text", Value: "line one\nline two"}},
				bson.D{{Key: "_id", Value: int32(2)}, {Key: "text", Value: "single line"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "text", Value: primitive.Regex{Pattern: "^line two", Options: "m"}}}
			return findIDs(ctx, col, filter)
		},
	})
}

// ─── text search ──────────────────────────────────────────────────────────────

func TestProbeTextPhrase(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ProbeTextPhrase",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertTextIndex(ctx, col, bson.D{{Key: "content", Value: "text"}}, "content_text"); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: int32(1)}, {Key: "content", Value: "quick brown fox"}},
				bson.D{{Key: "_id", Value: int32(2)}, {Key: "content", Value: "brown fox quick"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: `"quick brown"`}}}}
			cursor, err := col.Find(ctx, filter)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestProbeTextDiacriticSensitive(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ProbeTextDiacriticSensitive",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertTextIndex(ctx, col, bson.D{{Key: "word", Value: "text"}}, "word_text"); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: int32(1)}, {Key: "word", Value: "café"}},
				bson.D{{Key: "_id", Value: int32(2)}, {Key: "word", Value: "cafe"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{
				{Key: "$text", Value: bson.D{
					{Key: "$search", Value: "cafe"},
					{Key: "$diacriticSensitive", Value: false},
				}},
			}
			cursor, err := col.Find(ctx, filter)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

// TestProbeTextLanguage is skipped (language/stemming NLP support not implemented).
func TestProbeTextLanguage(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ProbeTextLanguage",
		Support: harness.DumboDBMongoOnly,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertTextIndex(ctx, col, bson.D{{Key: "body", Value: "text"}}, "body_text"); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: int32(1)}, {Key: "body", Value: "running quickly"}},
				bson.D{{Key: "_id", Value: int32(2)}, {Key: "body", Value: "run fast"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "$text", Value: bson.D{
				{Key: "$search", Value: "run"},
				{Key: "$language", Value: "en"},
			}}}
			cursor, err := col.Find(ctx, filter)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

// ─── $mod ─────────────────────────────────────────────────────────────────────

func TestProbeModInt32(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ProbeModInt32",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: int32(9)}},
				bson.D{{Key: "_id", Value: int32(2)}, {Key: "v", Value: int32(10)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "v", Value: bson.D{{Key: "$mod", Value: bson.A{3, 0}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestProbeModInt64(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ProbeModInt64",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: int64(9)}},
				bson.D{{Key: "_id", Value: int32(2)}, {Key: "v", Value: int64(10)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "v", Value: bson.D{{Key: "$mod", Value: bson.A{3, 0}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestProbeModNestedField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ProbeModNestedField",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.D{{Key: "b", Value: 9}}}},
				bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: bson.D{{Key: "b", Value: 10}}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "a.b", Value: bson.D{{Key: "$mod", Value: bson.A{3, 0}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestProbeModNonNumericField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ProbeModNonNumericField",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: "hello"}},
				bson.D{{Key: "_id", Value: int32(2)}, {Key: "v", Value: nil}},
				bson.D{{Key: "_id", Value: int32(3)}, {Key: "v", Value: true}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "v", Value: bson.D{{Key: "$mod", Value: bson.A{3, 0}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestProbeModNaNDivisor(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ProbeModNaNDivisor",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: 9}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Find(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$mod", Value: bson.A{math.NaN(), 0}}}}})
			return nil, err
		},
	})
}
