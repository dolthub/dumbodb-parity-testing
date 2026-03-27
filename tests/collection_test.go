package tests

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dongo-parity-testing/harness"
)

// colTestDocs is a small dataset for collection-level tests.
var colTestDocs = []interface{}{
	bson.D{{Key: "_id", Value: "ct1"}, {Key: "name", Value: "alpha"}, {Key: "val", Value: int32(1)}},
	bson.D{{Key: "_id", Value: "ct2"}, {Key: "name", Value: "beta"}, {Key: "val", Value: int32(2)}},
	bson.D{{Key: "_id", Value: "ct3"}, {Key: "name", Value: "gamma"}, {Key: "val", Value: int32(3)}},
	bson.D{{Key: "_id", Value: "ct4"}, {Key: "name", Value: "delta"}, {Key: "val", Value: int32(4)}},
	bson.D{{Key: "_id", Value: "ct5"}, {Key: "name", Value: "epsilon"}, {Key: "val", Value: int32(5)}},
}

func insertColTestDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, colTestDocs)
	return err
}

// cursorTestDocs holds 20 documents for cursor iteration and batch tests.
var cursorTestDocs []interface{}

func init() {
	for i := 1; i <= 20; i++ {
		cursorTestDocs = append(cursorTestDocs, bson.D{
			{Key: "_id", Value: fmt.Sprintf("cur%02d", i)},
			{Key: "n", Value: int32(i)},
		})
	}
}

func insertCursorTestDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, cursorTestDocs)
	return err
}

// sortedStrings converts []string to sorted []interface{} for stable comparison.
func sortedStrings(ss []string) []interface{} {
	sort.Strings(ss)
	out := make([]interface{}, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// runCommandDoc executes a RunCommand and returns the decoded bson.D.
func runCommandDoc(ctx context.Context, col *mongo.Collection, cmd interface{}) (interface{}, error) {
	result := col.Database().RunCommand(ctx, cmd)
	var doc bson.D
	if err := result.Decode(&doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// ============================================================
// Collection management
// ============================================================

func TestCollection_ImplicitCreate(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_ImplicitCreate",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Verify the collection exists after implicit creation via insert.
			names, err := col.Database().ListCollectionNames(ctx, bson.D{{Key: "name", Value: col.Name()}})
			if err != nil {
				return nil, err
			}
			return sortedStrings(names), nil
		},
	})
}

func TestCollection_CreateExplicit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_CreateExplicit",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Explicitly create a second collection and verify both appear in the listing.
			if err := col.Database().CreateCollection(ctx, "extra_col"); err != nil {
				return nil, err
			}
			names, err := col.Database().ListCollectionNames(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return sortedStrings(names), nil
		},
	})
}

func TestCollection_CreateCapped(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_CreateCapped",
		Support: harness.DongoFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.CreateCollection().SetCapped(true).SetSizeInBytes(1024 * 1024)
			if err := col.Database().CreateCollection(ctx, "capped_col", opts); err != nil {
				return nil, err
			}
			// Verify it appears in the collection listing.
			names, err := col.Database().ListCollectionNames(ctx, bson.D{{Key: "name", Value: "capped_col"}})
			if err != nil {
				return nil, err
			}
			return sortedStrings(names), nil
		},
	})
}

func TestCollection_CreateCapped_SizeAndMax(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_CreateCapped_SizeAndMax",
		Support: harness.DongoFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Capped collection with both size and max document count.
			opts := options.CreateCollection().
				SetCapped(true).
				SetSizeInBytes(512 * 1024).
				SetMaxDocuments(100)
			if err := col.Database().CreateCollection(ctx, "capped_max_col", opts); err != nil {
				return nil, err
			}
			names, err := col.Database().ListCollectionNames(ctx, bson.D{{Key: "name", Value: "capped_max_col"}})
			if err != nil {
				return nil, err
			}
			return sortedStrings(names), nil
		},
	})
}

func TestCollection_CreateValidator(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_CreateValidator",
		Support: harness.DongoFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Create with a jsonSchema validator that requires a 'name' field.
			validator := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "required", Value: bson.A{"name"}},
				{Key: "properties", Value: bson.D{
					{Key: "name", Value: bson.D{{Key: "bsonType", Value: "string"}}},
				}},
			}}}
			opts := options.CreateCollection().SetValidator(validator)
			if err := col.Database().CreateCollection(ctx, "validated_col", opts); err != nil {
				return nil, err
			}
			// A valid insert should succeed.
			validCol := col.Database().Collection("validated_col")
			_, err := validCol.InsertOne(ctx, bson.D{{Key: "name", Value: "test"}})
			if err != nil {
				return nil, err
			}
			count, err := validCol.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return count, nil
		},
	})
}

func TestCollection_CreateAlreadyExists(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_CreateAlreadyExists",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Creating a collection that already exists should return a NamespaceExists error.
			err := col.Database().CreateCollection(ctx, col.Name())
			if err != nil {
				// Return just the error so the harness compares error codes.
				return nil, err
			}
			return "no error", nil
		},
	})
}

func TestCollection_CreateCollation(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_CreateCollation",
		Support: harness.DongoXFail,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Collection with a default collation (case-insensitive English).
			collation := &options.Collation{Locale: "en", Strength: 2}
			opts := options.CreateCollection().SetCollation(collation)
			if err := col.Database().CreateCollection(ctx, "collation_col", opts); err != nil {
				return nil, err
			}
			names, err := col.Database().ListCollectionNames(ctx, bson.D{{Key: "name", Value: "collation_col"}})
			if err != nil {
				return nil, err
			}
			return sortedStrings(names), nil
		},
	})
}

func TestCollection_Drop(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_Drop",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Drop the collection and verify it no longer appears.
			if err := col.Drop(ctx); err != nil {
				return nil, err
			}
			names, err := col.Database().ListCollectionNames(ctx, bson.D{{Key: "name", Value: col.Name()}})
			if err != nil {
				return nil, err
			}
			return sortedStrings(names), nil
		},
	})
}

func TestCollection_DropNonexistent(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_DropNonexistent",
		Support: harness.DongoFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Dropping a nonexistent collection should not return an error.
			err := col.Database().Collection("no_such_col").Drop(ctx)
			if err != nil {
				return "error", err
			}
			return "ok", nil
		},
	})
}

func TestCollection_Rename(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_Rename",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// renameCollection requires the admin database in MongoDB.
			src := col.Database().Name() + "." + col.Name()
			dst := col.Database().Name() + ".renamed_col"
			result := col.Database().Client().Database("admin").RunCommand(ctx, bson.D{
				{Key: "renameCollection", Value: src},
				{Key: "to", Value: dst},
			})
			var doc bson.D
			if err := result.Decode(&doc); err != nil {
				return nil, err
			}
			// Verify new name appears.
			names, err := col.Database().ListCollectionNames(ctx, bson.D{{Key: "name", Value: "renamed_col"}})
			if err != nil {
				return nil, err
			}
			return sortedStrings(names), nil
		},
	})
}

func TestCollection_ListCollections(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_ListCollections",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// List collections filtered by name — should return just the test collection.
			cursor, err := col.Database().ListCollections(ctx, bson.D{{Key: "name", Value: col.Name()}})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			// Extract only the name field for stable comparison.
			names := make([]interface{}, 0, len(results))
			for _, r := range results {
				for _, e := range r {
					if e.Key == "name" {
						names = append(names, e.Value)
					}
				}
			}
			return names, nil
		},
	})
}

func TestCollection_ListCollectionNames(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_ListCollectionNames",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			names, err := col.Database().ListCollectionNames(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return sortedStrings(names), nil
		},
	})
}

func TestCollection_ListCollectionNamesFilter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_ListCollectionNamesFilter",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Filter to only the known collection name.
			names, err := col.Database().ListCollectionNames(ctx, bson.D{{Key: "name", Value: col.Name()}})
			if err != nil {
				return nil, err
			}
			return sortedStrings(names), nil
		},
	})
}

func TestCollection_ListCollectionsIdIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_ListCollectionsIdIndex",
		Support: harness.DongoXFail,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// ListCollections result includes idIndex field with index spec.
			cursor, err := col.Database().ListCollections(ctx, bson.D{{Key: "name", Value: col.Name()}})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return "not found", nil
			}
			return results[0], nil
		},
	})
}

func TestCollection_DropAndRecreate(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_DropAndRecreate",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Drop, then reinsert — collection should exist again with new docs.
			if err := col.Drop(ctx); err != nil {
				return nil, err
			}
			if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "new1"}, {Key: "val", Value: int32(99)}}); err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return count, nil
		},
	})
}

func TestCollection_CreateTimeSeries(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_CreateTimeSeries",
		Support: harness.DongoXFail,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Time series collections require timeField option.
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("timestamp"),
			)
			if err := col.Database().CreateCollection(ctx, "ts_col", tsOpts); err != nil {
				return nil, err
			}
			names, err := col.Database().ListCollectionNames(ctx, bson.D{{Key: "name", Value: "ts_col"}})
			if err != nil {
				return nil, err
			}
			return sortedStrings(names), nil
		},
	})
}

// ============================================================
// Database-level operations
// ============================================================

func TestDB_ListDatabaseNames(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_ListDatabaseNames",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Filter to just the current test DB name for a deterministic result.
			names, err := col.Database().Client().ListDatabaseNames(ctx,
				bson.D{{Key: "name", Value: col.Database().Name()}})
			if err != nil {
				return nil, err
			}
			return sortedStrings(names), nil
		},
	})
}

func TestDB_ListDatabases(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_ListDatabases",
		Support: harness.DongoXFail,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// List all databases — result structure and system DBs may differ.
			result, err := col.Database().Client().ListDatabases(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return result, nil
		},
	})
}

func TestDB_DropDatabase(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_DropDatabase",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			dbName := col.Database().Name()
			if err := col.Database().Drop(ctx); err != nil {
				return nil, err
			}
			// After drop, the database should not appear in the name list.
			names, err := col.Database().Client().ListDatabaseNames(ctx,
				bson.D{{Key: "name", Value: dbName}})
			if err != nil {
				return nil, err
			}
			return sortedStrings(names), nil
		},
	})
}

func TestDB_RunCommand_Ping(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_RunCommand_Ping",
		Support: harness.DongoFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// ping returns {ok: 1} on both MongoDB and Dongo.
			return runCommandDoc(ctx, col, bson.D{{Key: "ping", Value: 1}})
		},
	})
}

func TestDB_RunCommand_Hello(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_RunCommand_Hello",
		Support: harness.DongoXFail,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// hello returns topology info; structure/values may differ in Dongo.
			return runCommandDoc(ctx, col, bson.D{{Key: "hello", Value: 1}})
		},
	})
}

func TestDB_RunCommand_IsMaster(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_RunCommand_IsMaster",
		Support: harness.DongoXFail,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Legacy isMaster command; topology fields may differ in Dongo.
			return runCommandDoc(ctx, col, bson.D{{Key: "isMaster", Value: 1}})
		},
	})
}

func TestDB_RunCommand_BuildInfo(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_RunCommand_BuildInfo",
		Support: harness.DongoXFail,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// buildInfo includes version strings that differ between MongoDB and Dongo.
			return runCommandDoc(ctx, col, bson.D{{Key: "buildInfo", Value: 1}})
		},
	})
}

func TestDB_RunCommand_ServerStatus(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_RunCommand_ServerStatus",
		Support: harness.DongoXFail,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// serverStatus returns a large document with runtime metrics.
			return runCommandDoc(ctx, col, bson.D{{Key: "serverStatus", Value: 1}})
		},
	})
}

func TestDB_RunCommand_DbStats(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_RunCommand_DbStats",
		Support: harness.DongoXFail,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// dbStats includes storage sizes that differ between engines.
			return runCommandDoc(ctx, col, bson.D{{Key: "dbStats", Value: 1}})
		},
	})
}

func TestDB_RunCommand_CollStats(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_RunCommand_CollStats",
		Support: harness.DongoXFail,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// collStats includes per-collection storage metrics.
			return runCommandDoc(ctx, col, bson.D{{Key: "collStats", Value: col.Name()}})
		},
	})
}

func TestDB_RunCommand_ListIndexes(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_RunCommand_ListIndexes",
		Support: harness.DongoXFail,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// listIndexes returns a cursor-like response with index specs.
			return runCommandDoc(ctx, col, bson.D{{Key: "listIndexes", Value: col.Name()}})
		},
	})
}

func TestDB_RunCommand_Validate(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_RunCommand_Validate",
		Support: harness.DongoXFail,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// validate reports on collection integrity; details differ by engine.
			return runCommandDoc(ctx, col, bson.D{{Key: "validate", Value: col.Name()}})
		},
	})
}

func TestDB_RunCommand_ListCollections(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_RunCommand_ListCollections",
		Support: harness.DongoXFail,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// listCollections via RunCommand returns a cursor response document.
			return runCommandDoc(ctx, col, bson.D{
				{Key: "listCollections", Value: 1},
				{Key: "filter", Value: bson.D{{Key: "name", Value: col.Name()}}},
			})
		},
	})
}

func TestDB_RunCommand_Create(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_RunCommand_Create",
		Support: harness.DongoXFail,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// create command creates a collection.
			return runCommandDoc(ctx, col, bson.D{{Key: "create", Value: "cmd_col"}})
		},
	})
}

func TestDB_RunCommand_Drop(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_RunCommand_Drop",
		Support: harness.DongoXFail,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// drop command via RunCommand.
			return runCommandDoc(ctx, col, bson.D{{Key: "drop", Value: col.Name()}})
		},
	})
}

// ============================================================
// Cursor commands
// ============================================================

func TestCursor_AllDocs(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_AllDocs",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			result := make([]interface{}, len(docs))
			for i, d := range docs {
				result[i] = d
			}
			return result, nil
		},
	})
}

func TestCursor_BatchSize(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_BatchSize",
		Support: harness.DongoFull,
		Setup:   insertCursorTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// BatchSize controls network batching but the final result is the same.
			opts := options.Find().
				SetBatchSize(3).
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			result := make([]interface{}, len(docs))
			for i, d := range docs {
				result[i] = d
			}
			return result, nil
		},
	})
}

func TestCursor_Limit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_Limit",
		Support: harness.DongoFull,
		Setup:   insertCursorTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "n", Value: 1}}).
				SetLimit(5).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			result := make([]interface{}, len(docs))
			for i, d := range docs {
				result[i] = d
			}
			return result, nil
		},
	})
}

func TestCursor_Skip(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_Skip",
		Support: harness.DongoFull,
		Setup:   insertCursorTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "n", Value: 1}}).
				SetSkip(15).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			result := make([]interface{}, len(docs))
			for i, d := range docs {
				result[i] = d
			}
			return result, nil
		},
	})
}

func TestCursor_SkipLimit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_SkipLimit",
		Support: harness.DongoFull,
		Setup:   insertCursorTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "n", Value: 1}}).
				SetSkip(5).
				SetLimit(5).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			result := make([]interface{}, len(docs))
			for i, d := range docs {
				result[i] = d
			}
			return result, nil
		},
	})
}

func TestCursor_Exhaustion(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_Exhaustion",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// After consuming all documents, cursor.Next should return false.
			cursor, err := col.Find(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			var count int32
			for cursor.Next(ctx) {
				count++
			}
			if err := cursor.Err(); err != nil {
				return nil, err
			}
			hasMore := cursor.Next(ctx)
			return bson.D{
				{Key: "count", Value: count},
				{Key: "hasMore", Value: hasMore},
			}, nil
		},
	})
}

func TestCursor_LargeDataset(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_LargeDataset",
		Support: harness.DongoFull,
		Setup:   insertCursorTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// 20 docs with batchSize=3 requires multiple GetMore calls.
			opts := options.Find().
				SetBatchSize(3).
				SetSort(bson.D{{Key: "n", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var count int32
			for cursor.Next(ctx) {
				count++
			}
			if err := cursor.Err(); err != nil {
				return nil, err
			}
			return count, nil
		},
	})
}

func TestCursor_EmptyCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_EmptyCollection",
		Support: harness.DongoFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Find on empty collection returns empty result.
			cursor, err := col.Find(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			return int32(len(docs)), nil
		},
	})
}

func TestCursor_EmptyFilter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_EmptyFilter",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Empty filter matches all documents.
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			result := make([]interface{}, len(docs))
			for i, d := range docs {
				result[i] = d
			}
			return result, nil
		},
	})
}

func TestCursor_Sort(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_Sort",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "val", Value: -1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			result := make([]interface{}, len(docs))
			for i, d := range docs {
				result[i] = d
			}
			return result, nil
		},
	})
}

func TestCursor_CloseEarly(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_CloseEarly",
		Support: harness.DongoFull,
		Setup:   insertCursorTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Close cursor after reading just 3 docs; no error expected.
			cursor, err := col.Find(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			var count int32
			for cursor.Next(ctx) && count < 3 {
				count++
			}
			if err := cursor.Close(ctx); err != nil {
				return nil, err
			}
			return count, nil
		},
	})
}

func TestCursor_MaxTime(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_MaxTime",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// maxTimeMS: Dongo may not honor or recognize this option.
			opts := options.Find().
				SetMaxTime(5000 * 1000000). // 5 seconds in nanoseconds
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			return int32(len(docs)), nil
		},
	})
}

func TestCursor_AllowDiskUse(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_AllowDiskUse",
		Support: harness.DongoFull,
		Setup:   insertCursorTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// allowDiskUse permits spilling to disk for large sorts.
			opts := options.Find().
				SetAllowDiskUse(true).
				SetSort(bson.D{{Key: "n", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			return int32(len(docs)), nil
		},
	})
}

func TestCursor_Hint(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_Hint",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Hint with _id index (always exists); Dongo may not support hint option.
			opts := options.Find().
				SetHint(bson.D{{Key: "_id", Value: 1}}).
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			result := make([]interface{}, len(docs))
			for i, d := range docs {
				result[i] = d
			}
			return result, nil
		},
	})
}

func TestCursor_Comment(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_Comment",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// SetComment attaches a query comment; Dongo may not support this option.
			opts := options.Find().
				SetComment("parity test query").
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			return int32(len(docs)), nil
		},
	})
}

func TestCursor_ShowRecordId(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_ShowRecordId",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// showRecordId adds $recordId to each returned document.
			opts := options.Find().
				SetShowRecordID(true).
				SetLimit(1).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			if len(docs) == 0 {
				return "empty", nil
			}
			// Return whether $recordId field is present.
			for _, elem := range docs[0] {
				if elem.Key == "$recordId" {
					return "has recordId", nil
				}
			}
			return "no recordId", nil
		},
	})
}

func TestCursor_NoCursorTimeout(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_NoCursorTimeout",
		Support: harness.DongoXFail,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// noCursorTimeout prevents idle cursor expiry; Dongo may not support.
			opts := options.Find().
				SetNoCursorTimeout(true).
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			defer cursor.Close(ctx)
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			return int32(len(docs)), nil
		},
	})
}

// ============================================================
// Explain
// ============================================================

func TestExplain_Find_QueryPlanner(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Find_QueryPlanner",
		Support: harness.DongoXFail,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cmd := bson.D{
				{Key: "explain", Value: bson.D{
					{Key: "find", Value: col.Name()},
					{Key: "filter", Value: bson.D{{Key: "val", Value: bson.D{{Key: "$gt", Value: int32(2)}}}}},
				}},
				{Key: "verbosity", Value: "queryPlanner"},
			}
			return runCommandDoc(ctx, col, cmd)
		},
	})
}

func TestExplain_Find_ExecutionStats(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Find_ExecutionStats",
		Support: harness.DongoXFail,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cmd := bson.D{
				{Key: "explain", Value: bson.D{
					{Key: "find", Value: col.Name()},
					{Key: "filter", Value: bson.D{}},
				}},
				{Key: "verbosity", Value: "executionStats"},
			}
			return runCommandDoc(ctx, col, cmd)
		},
	})
}

func TestExplain_Find_AllPlansExecution(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Find_AllPlansExecution",
		Support: harness.DongoXFail,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cmd := bson.D{
				{Key: "explain", Value: bson.D{
					{Key: "find", Value: col.Name()},
					{Key: "filter", Value: bson.D{}},
				}},
				{Key: "verbosity", Value: "allPlansExecution"},
			}
			return runCommandDoc(ctx, col, cmd)
		},
	})
}

func TestExplain_Aggregate(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Aggregate",
		Support: harness.DongoXFail,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cmd := bson.D{
				{Key: "explain", Value: bson.D{
					{Key: "aggregate", Value: col.Name()},
					{Key: "pipeline", Value: bson.A{
						bson.D{{Key: "$match", Value: bson.D{{Key: "val", Value: bson.D{{Key: "$gt", Value: int32(2)}}}}}},
					}},
					{Key: "cursor", Value: bson.D{}},
				}},
				{Key: "verbosity", Value: "queryPlanner"},
			}
			return runCommandDoc(ctx, col, cmd)
		},
	})
}

func TestExplain_Count(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Count",
		Support: harness.DongoXFail,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cmd := bson.D{
				{Key: "explain", Value: bson.D{
					{Key: "count", Value: col.Name()},
					{Key: "query", Value: bson.D{{Key: "val", Value: bson.D{{Key: "$lte", Value: int32(3)}}}}},
				}},
				{Key: "verbosity", Value: "queryPlanner"},
			}
			return runCommandDoc(ctx, col, cmd)
		},
	})
}

func TestExplain_Update(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Update",
		Support: harness.DongoXFail,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cmd := bson.D{
				{Key: "explain", Value: bson.D{
					{Key: "update", Value: col.Name()},
					{Key: "updates", Value: bson.A{bson.D{
						{Key: "q", Value: bson.D{{Key: "val", Value: int32(1)}}},
						{Key: "u", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "val", Value: int32(99)}}}}},
					}}},
				}},
				{Key: "verbosity", Value: "queryPlanner"},
			}
			return runCommandDoc(ctx, col, cmd)
		},
	})
}

func TestExplain_Delete(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Delete",
		Support: harness.DongoXFail,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cmd := bson.D{
				{Key: "explain", Value: bson.D{
					{Key: "delete", Value: col.Name()},
					{Key: "deletes", Value: bson.A{bson.D{
						{Key: "q", Value: bson.D{{Key: "val", Value: int32(5)}}},
						{Key: "limit", Value: int32(1)},
					}}},
				}},
				{Key: "verbosity", Value: "queryPlanner"},
			}
			return runCommandDoc(ctx, col, cmd)
		},
	})
}

func TestExplain_Distinct(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Distinct",
		Support: harness.DongoXFail,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cmd := bson.D{
				{Key: "explain", Value: bson.D{
					{Key: "distinct", Value: col.Name()},
					{Key: "key", Value: "name"},
				}},
				{Key: "verbosity", Value: "queryPlanner"},
			}
			return runCommandDoc(ctx, col, cmd)
		},
	})
}

// ============================================================
// Additional collection operations
// ============================================================

func TestCollection_CountDocuments_NoFilter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_CountDocuments_NoFilter",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return count, nil
		},
	})
}

func TestCollection_CountDocuments_WithFilter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_CountDocuments_WithFilter",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{{Key: "val", Value: bson.D{{Key: "$gte", Value: int32(3)}}}})
			if err != nil {
				return nil, err
			}
			return count, nil
		},
	})
}

func TestCollection_EstimatedCount(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_EstimatedCount",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.EstimatedDocumentCount(ctx)
			if err != nil {
				return nil, err
			}
			return count, nil
		},
	})
}

func TestCollection_Distinct_Simple(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_Distinct_Simple",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			values, err := col.Distinct(ctx, "val", bson.D{})
			if err != nil {
				return nil, err
			}
			// Sort for stable comparison.
			strs := make([]interface{}, len(values))
			copy(strs, values)
			sort.Slice(strs, func(i, j int) bool {
				return fmt.Sprintf("%v", strs[i]) < fmt.Sprintf("%v", strs[j])
			})
			return strs, nil
		},
	})
}

func TestCollection_Distinct_Filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_Distinct_Filter",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			values, err := col.Distinct(ctx, "name", bson.D{{Key: "val", Value: bson.D{{Key: "$lte", Value: int32(3)}}}})
			if err != nil {
				return nil, err
			}
			strs := make([]interface{}, len(values))
			copy(strs, values)
			sort.Slice(strs, func(i, j int) bool {
				return fmt.Sprintf("%v", strs[i]) < fmt.Sprintf("%v", strs[j])
			})
			return strs, nil
		},
	})
}

func TestCollection_Distinct_ArrayField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_Distinct_ArrayField",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "da1"}, {Key: "tags", Value: bson.A{"go", "db"}}},
				bson.D{{Key: "_id", Value: "da2"}, {Key: "tags", Value: bson.A{"go", "nosql"}}},
				bson.D{{Key: "_id", Value: "da3"}, {Key: "tags", Value: bson.A{"db", "sql"}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			values, err := col.Distinct(ctx, "tags", bson.D{})
			if err != nil {
				return nil, err
			}
			strs := make([]interface{}, len(values))
			copy(strs, values)
			sort.Slice(strs, func(i, j int) bool {
				return fmt.Sprintf("%v", strs[i]) < fmt.Sprintf("%v", strs[j])
			})
			return strs, nil
		},
	})
}

func TestCollection_Distinct_Nested(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_Distinct_Nested",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "dn1"}, {Key: "addr", Value: bson.D{{Key: "city", Value: "NYC"}}}},
				bson.D{{Key: "_id", Value: "dn2"}, {Key: "addr", Value: bson.D{{Key: "city", Value: "LA"}}}},
				bson.D{{Key: "_id", Value: "dn3"}, {Key: "addr", Value: bson.D{{Key: "city", Value: "NYC"}}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Distinct on a nested field using dot notation.
			values, err := col.Distinct(ctx, "addr.city", bson.D{})
			if err != nil {
				return nil, err
			}
			strs := make([]interface{}, len(values))
			copy(strs, values)
			sort.Slice(strs, func(i, j int) bool {
				return fmt.Sprintf("%v", strs[i]) < fmt.Sprintf("%v", strs[j])
			})
			return strs, nil
		},
	})
}

func TestCollection_Find_EmptyResult(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_Find_EmptyResult",
		Support: harness.DongoFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Filter that matches no documents.
			cursor, err := col.Find(ctx, bson.D{{Key: "val", Value: int32(999)}})
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			return int32(len(docs)), nil
		},
	})
}
