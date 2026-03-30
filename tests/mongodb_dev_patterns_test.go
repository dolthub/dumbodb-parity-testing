// mongodb_dev_patterns_test.go covers MongoDB Development Patterns tutorials.
// Source: https://www.mongodb.com/docs/manual/tutorial/
// Each test mirrors the data and operations shown on the corresponding tutorial page.
package tests

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dongo-parity-testing/harness"
)

// tutorialCheck verifies that actual matches expected (from tutorial docs).
// It uses CompareResponses so the same normalization rules apply.
func tutorialCheck(t *testing.T, name string, actual interface{}, expected interface{}) {
	t.Helper()
	cmp := harness.CompareResponses(actual, nil, expected, nil)
	if cmp.Result != harness.Match {
		t.Errorf("TUTORIAL %s: result differs from docs expected:\n%s", name, cmp.Diff)
	}
}

// ─── Model Tree Structures with Parent References ──────────────────────────────
// https://www.mongodb.com/docs/manual/tutorial/model-tree-structures-with-parent-references/

// devPatternsCategoriesParentSeed inserts the categories tree from the tutorial.
// Tree: Books → Programming → {Databases → {MongoDB, dbm}, Languages}
func devPatternsCategoriesParentSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: "MongoDB"}, {Key: "name", Value: "MongoDB"}, {Key: "parent", Value: "Databases"}},
		bson.D{{Key: "_id", Value: "dbm"}, {Key: "name", Value: "dbm"}, {Key: "parent", Value: "Databases"}},
		bson.D{{Key: "_id", Value: "Databases"}, {Key: "name", Value: "Databases"}, {Key: "parent", Value: "Programming"}},
		bson.D{{Key: "_id", Value: "Languages"}, {Key: "name", Value: "Languages"}, {Key: "parent", Value: "Programming"}},
		bson.D{{Key: "_id", Value: "Programming"}, {Key: "name", Value: "Programming"}, {Key: "parent", Value: "Books"}},
		bson.D{{Key: "_id", Value: "Books"}, {Key: "name", Value: "Books"}, {Key: "parent", Value: nil}},
	})
	return err
}

func TestDevPatterns_ParentRefs_FindParent(t *testing.T) {
	// "Given a child node, to find its parent query using the _id."
	// db.categories.findOne({ _id: "MongoDB" })
	// Expected: { _id: "MongoDB", name: "MongoDB", parent: "Databases" }
	harness.PairTest(t, harness.TestCase{
		Name:    "DevPatterns_ParentRefs_FindParent",
		Support: harness.DongoFull,
		Setup:   devPatternsCategoriesParentSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var result bson.D
			err := col.FindOne(ctx, bson.D{{Key: "_id", Value: "MongoDB"}}).Decode(&result)
			if err != nil {
				return nil, err
			}
			expected := bson.D{
				{Key: "_id", Value: "MongoDB"},
				{Key: "name", Value: "MongoDB"},
				{Key: "parent", Value: "Databases"},
			}
			tutorialCheck(t, "ParentRefs_FindParent", result, expected)
			return result, nil
		},
	})
}

func TestDevPatterns_ParentRefs_FindChildren(t *testing.T) {
	// "To find all immediate children of 'Databases', query by parent field."
	// db.categories.find({ parent: "Databases" })
	// Expected: MongoDB and dbm documents (sorted by name for determinism).
	harness.PairTest(t, harness.TestCase{
		Name:    "DevPatterns_ParentRefs_FindChildren",
		Support: harness.DongoFull,
		Setup:   devPatternsCategoriesParentSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(
				ctx,
				bson.D{{Key: "parent", Value: "Databases"}},
				options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}),
			)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			expected := []interface{}{
				bson.D{{Key: "_id", Value: "MongoDB"}, {Key: "name", Value: "MongoDB"}, {Key: "parent", Value: "Databases"}},
				bson.D{{Key: "_id", Value: "dbm"}, {Key: "name", Value: "dbm"}, {Key: "parent", Value: "Databases"}},
			}
			iface := make([]interface{}, len(results))
			for i, r := range results {
				iface[i] = r
			}
			tutorialCheck(t, "ParentRefs_FindChildren", iface, expected)
			return int32(len(results)), nil
		},
	})
}

func TestDevPatterns_ParentRefs_IndexOnParent(t *testing.T) {
	// "Create an index on the parent field to support common tree queries."
	// db.categories.createIndex({ parent: 1 })
	harness.PairTest(t, harness.TestCase{
		Name:    "DevPatterns_ParentRefs_IndexOnParent",
		Support: harness.DongoFull,
		Setup:   devPatternsCategoriesParentSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			model := mongo.IndexModel{Keys: bson.D{{Key: "parent", Value: 1}}}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "index_name", Value: name}}, nil
		},
	})
}

// ─── Expire Data (TTL Indexes) ────────────────────────────────────────────────
// https://www.mongodb.com/docs/manual/tutorial/expire-data/

func TestDevPatterns_TTL_ExpireAfterSeconds(t *testing.T) {
	// "Expire after a fixed number of seconds."
	// db.log_events.createIndex({ "createdAt": 1 }, { expireAfterSeconds: 3600 })
	// Documents with createdAt older than 3600 seconds are removed by the TTL monitor.
	harness.PairTest(t, harness.TestCase{
		Name:    "DevPatterns_TTL_ExpireAfterSeconds",
		Support: harness.DongoFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			expireAfter := int32(3600)
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "createdAt", Value: 1}},
				Options: &options.IndexOptions{ExpireAfterSeconds: &expireAfter},
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			// Insert a log event as shown in the tutorial.
			now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
			_, err = col.InsertOne(ctx, bson.D{
				{Key: "createdAt", Value: now},
				{Key: "logEvent", Value: int32(2)},
				{Key: "logMessage", Value: "Success!"},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			tutorialCheck(t, "TTL_ExpireAfterSeconds_count", count, int64(1))
			return bson.D{{Key: "index_name", Value: name}, {Key: "doc_count", Value: count}}, nil
		},
	})
}

func TestDevPatterns_TTL_ExpireAtSpecificTime(t *testing.T) {
	// "Expire at a specific clock time."
	// db.log_events.createIndex({ "expireAt": 1 }, { expireAfterSeconds: 0 })
	// Document expires at the exact datetime stored in the expireAt field.
	harness.PairTest(t, harness.TestCase{
		Name:    "DevPatterns_TTL_ExpireAtSpecificTime",
		Support: harness.DongoFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			expireAfter := int32(0)
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "expireAt", Value: 1}},
				Options: &options.IndexOptions{ExpireAfterSeconds: &expireAfter},
			}
			name, err := col.Indexes().CreateOne(ctx, model)
			if err != nil {
				return nil, err
			}
			// Tutorial example: document expires at a specific future time.
			expireAt := time.Date(2030, 10, 22, 17, 0, 0, 0, time.UTC)
			_, err = col.InsertOne(ctx, bson.D{
				{Key: "expireAt", Value: expireAt},
				{Key: "logEvent", Value: int32(2)},
				{Key: "logMessage", Value: "Success!"},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			tutorialCheck(t, "TTL_ExpireAtSpecificTime_count", count, int64(1))
			return bson.D{{Key: "index_name", Value: name}, {Key: "doc_count", Value: count}}, nil
		},
	})
}

// ─── Auto-Incrementing Sequences (Counters Collection) ───────────────────────
// https://www.mongodb.com/docs/manual/tutorial/create-an-auto-incrementing-field/
//
// Pattern: maintain a "counters" collection; use FindOneAndUpdate with $inc
// to atomically obtain the next sequence value, then use it as _id.

func TestDevPatterns_AutoIncrement_GetNextSequence(t *testing.T) {
	// Initialise counters collection with { _id: "userid", seq: 0 }.
	// Call getNextSequence twice and verify seq increments: 1, then 2.
	harness.PairTest(t, harness.TestCase{
		Name:    "DevPatterns_AutoIncrement_GetNextSequence",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "userid"},
				{Key: "seq", Value: int32(0)},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Simulate getNextSequence("userid") called twice.
			opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
			var r1 bson.D
			err := col.FindOneAndUpdate(
				ctx,
				bson.D{{Key: "_id", Value: "userid"}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "seq", Value: int32(1)}}}},
				opts,
			).Decode(&r1)
			if err != nil {
				return nil, err
			}
			var r2 bson.D
			err = col.FindOneAndUpdate(
				ctx,
				bson.D{{Key: "_id", Value: "userid"}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "seq", Value: int32(1)}}}},
				opts,
			).Decode(&r2)
			if err != nil {
				return nil, err
			}
			// Extract seq values.
			seq1 := seqValue(r1)
			seq2 := seqValue(r2)
			result := bson.D{
				{Key: "first_seq", Value: seq1},
				{Key: "second_seq", Value: seq2},
			}
			tutorialCheck(t, "AutoIncrement_GetNextSequence", result, bson.D{
				{Key: "first_seq", Value: int32(1)},
				{Key: "second_seq", Value: int32(2)},
			})
			return result, nil
		},
	})
}

// seqValue extracts the "seq" field from a bson.D document.
func seqValue(d bson.D) int32 {
	for _, e := range d {
		if e.Key == "seq" {
			switch v := e.Value.(type) {
			case int32:
				return v
			case int64:
				return int32(v)
			}
		}
	}
	return -1
}

func TestDevPatterns_AutoIncrement_InsertWithSequenceID(t *testing.T) {
	// Full pattern: counters collection + users collection.
	// Insert two users with auto-incremented _id values (1 and 2).
	harness.PairTest(t, harness.TestCase{
		Name:    "DevPatterns_AutoIncrement_InsertWithSequenceID",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			// The counters collection is a separate collection; we seed it here
			// in a sibling collection named col.Name()+"_counters".
			db := col.Database()
			counters := db.Collection(col.Name() + "_counters")
			_, err := counters.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "userid"},
				{Key: "seq", Value: int32(0)},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			counters := db.Collection(col.Name() + "_counters")
			opts := options.FindOneAndUpdate().SetReturnDocument(options.After)

			getNextSeq := func() (int32, error) {
				var r bson.D
				err := counters.FindOneAndUpdate(
					ctx,
					bson.D{{Key: "_id", Value: "userid"}},
					bson.D{{Key: "$inc", Value: bson.D{{Key: "seq", Value: int32(1)}}}},
					opts,
				).Decode(&r)
				if err != nil {
					return 0, err
				}
				return seqValue(r), nil
			}

			id1, err := getNextSeq()
			if err != nil {
				return nil, err
			}
			_, err = col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: id1},
				{Key: "name", Value: "Sarah C."},
			})
			if err != nil {
				return nil, err
			}

			id2, err := getNextSeq()
			if err != nil {
				return nil, err
			}
			_, err = col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: id2},
				{Key: "name", Value: "Bob D."},
			})
			if err != nil {
				return nil, err
			}

			// Retrieve inserted users sorted by _id.
			cursor, err := col.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var users []bson.D
			if err := cursor.All(ctx, &users); err != nil {
				return nil, err
			}
			result := bson.D{{Key: "user_count", Value: int32(len(users))}}
			tutorialCheck(t, "AutoIncrement_UserCount", result, bson.D{{Key: "user_count", Value: int32(2)}})
			return result, nil
		},
	})
}
