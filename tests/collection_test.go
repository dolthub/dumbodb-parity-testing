package tests

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
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

// pickFields returns a new bson.D containing only the named keys, preserving order.
func pickFields(doc bson.D, keys ...string) bson.D {
	keep := make(map[string]bool, len(keys))
	for _, k := range keys {
		keep[k] = true
	}
	result := make(bson.D, 0, len(keys))
	for _, elem := range doc {
		if keep[elem.Key] {
			result = append(result, elem)
		}
	}
	return result
}

// omitFields returns a new bson.D with the named keys removed, preserving order.
func omitFields(doc bson.D, keys ...string) bson.D {
	drop := make(map[string]bool, len(keys))
	for _, k := range keys {
		drop[k] = true
	}
	result := make(bson.D, 0, len(doc))
	for _, elem := range doc {
		if !drop[elem.Key] {
			result = append(result, elem)
		}
	}
	return result
}

func TestCollection_ImplicitCreate(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_ImplicitCreate",
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBMongoOnly,
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
		Support: harness.DumboDBMongoOnly,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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

func TestDB_ListDatabaseNames(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_ListDatabaseNames",
		Support: harness.DumboDBFull,
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
		Name: "DB_ListDatabases",
		// XFail: structural divergence. MongoDB reports the config and local
		// system databases (which DumboDB intentionally does not implement)
		// and storage-engine-specific sizeondisk/empty values. DumboDB lists
		// only real user databases plus admin, with its own sizes.
		Support: harness.DumboDBXFail,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// ping returns {ok: 1} on both MongoDB and DumboDB.
			return runCommandDoc(ctx, col, bson.D{{Key: "ping", Value: 1}})
		},
	})
}

func TestDB_RunCommand_Hello(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_RunCommand_Hello",
		Support: harness.DumboDBFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// connectionId is server-assigned and differs between instances.
			// topologyVersion is deliberately omitted by DumboDB: advertising
			// it implies awaitable ("streaming") hello monitoring, which
			// DumboDB does not implement. Omit both.
			doc, err := runCommandDoc(ctx, col, bson.D{{Key: "hello", Value: 1}})
			if err != nil {
				return nil, err
			}
			return omitFields(doc.(bson.D), "connectionId", "topologyVersion"), nil
		},
	})
}

func TestDB_RunCommand_IsMaster(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_RunCommand_IsMaster",
		Support: harness.DumboDBFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// connectionId is server-assigned and differs between instances.
			// topologyVersion is deliberately omitted by DumboDB: advertising
			// it implies awaitable ("streaming") hello monitoring, which
			// DumboDB does not implement. Omit both.
			doc, err := runCommandDoc(ctx, col, bson.D{{Key: "isMaster", Value: 1}})
			if err != nil {
				return nil, err
			}
			return omitFields(doc.(bson.D), "connectionId", "topologyVersion"), nil
		},
	})
}

func TestDB_RunCommand_BuildInfo(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_RunCommand_BuildInfo",
		Support: harness.DumboDBFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Omit MongoDB-internal fields: allocator, javascriptEngine, openssl,
			// storageEngines — these are not applicable to DumboDB.
			doc, err := runCommandDoc(ctx, col, bson.D{{Key: "buildInfo", Value: 1}})
			if err != nil {
				return nil, err
			}
			return pickFields(doc.(bson.D),
				"version", "sysInfo", "bits", "debug",
				"maxBsonObjectSize", "ok",
			), nil
		},
	})
}

func TestDB_RunCommand_ServerStatus(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_RunCommand_ServerStatus",
		Support: harness.DumboDBFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// serverStatus returns a large document; compare only dumbodb-supported fields.
			// Skipped: asserts, batchedDeletes, electionMetrics, flowControl, globalLock,
			// repl, sharding, storageEngine, tcmalloc, version (dumbodb reports a stub version),
			// host (differs between Docker/native deployments),
			// uptime/connections/opcounters (vary or absent in dumbodb).
			doc, err := runCommandDoc(ctx, col, bson.D{{Key: "serverStatus", Value: 1}})
			if err != nil {
				return nil, err
			}
			return pickFields(doc.(bson.D), "ok"), nil
		},
	})
}

func TestDB_RunCommand_DbStats(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_RunCommand_DbStats",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Omit engine-specific storage metrics: storageSize, freeStorageSize,
			// fsUsedSize, fsTotalSize, dataSize, indexSize, avgObjSize — these
			// differ between WiredTiger and Dolt's prolly tree storage.
			doc, err := runCommandDoc(ctx, col, bson.D{{Key: "dbStats", Value: 1}})
			if err != nil {
				return nil, err
			}
			return pickFields(doc.(bson.D), "db", "collections", "views", "objects", "ok"), nil
		},
	})
}

func TestDB_RunCommand_CollStats(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_RunCommand_CollStats",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// collStats includes per-collection storage metrics; compare only dumbodb-supported fields.
			// Skipped: wiredTiger (WiredTiger-specific), indexDetails (WiredTiger per-index stats),
			// size/avgObjSize/storageSize/totalSize (dumbodb computes these differently from WiredTiger).
			doc, err := runCommandDoc(ctx, col, bson.D{{Key: "collStats", Value: col.Name()}})
			if err != nil {
				return nil, err
			}
			return pickFields(doc.(bson.D),
				"ns", "count", "nindexes", "ok", "capped",
				"freeStorageSize", "numOrphanDocs", "indexBuilds", "scaleFactor",
			), nil
		},
	})
}

func TestDB_RunCommand_ListIndexes(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DB_RunCommand_ListIndexes",
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// drop command via RunCommand.
			return runCommandDoc(ctx, col, bson.D{{Key: "drop", Value: col.Name()}})
		},
	})
}

func TestCursor_AllDocs(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_AllDocs",
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// maxTimeMS: DumboDB may not honor or recognize this option.
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Hint with _id index (always exists); DumboDB may not support hint option.
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
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// SetComment attaches a query comment; DumboDB may not support this option.
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// noCursorTimeout prevents idle cursor expiry; DumboDB may not support.
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

// Explain
//
// Explain responses contain server-version-specific noise (serverInfo, host,
// port, gitVersion, plannerVersion, timing fields, planCacheKey, etc.) that
// will never match between MongoDB and DumboDB. Rather than compare the full
// response, these tests extract the high-signal "what plan was chosen?"
// fields — the parsed query, winning plan stage, selected index — and compare
// only those. See explainCritical below.

// lookupBSONField returns the value for key in d, or (nil, false).
func lookupBSONField(d bson.D, key string) (interface{}, bool) {
	for _, e := range d {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}

// findQueryPlanner locates the queryPlanner sub-document in an explain
// response. For find/count/update/delete/distinct it is at the top level;
// for aggregate it is nested inside stages[0].$cursor.
func findQueryPlanner(doc bson.D) bson.D {
	if v, ok := lookupBSONField(doc, "queryPlanner"); ok {
		if d, ok := v.(bson.D); ok {
			return d
		}
	}
	if stages, ok := lookupBSONField(doc, "stages"); ok {
		if arr, ok := stages.(bson.A); ok && len(arr) > 0 {
			if first, ok := arr[0].(bson.D); ok {
				if cursor, ok := lookupBSONField(first, "$cursor"); ok {
					if cd, ok := cursor.(bson.D); ok {
						return findQueryPlanner(cd)
					}
				}
			}
		}
	}
	return nil
}

// findExecutionStats locates the executionStats sub-document. Like
// findQueryPlanner, it descends into aggregate's stages[0].$cursor.
func findExecutionStats(doc bson.D) bson.D {
	if v, ok := lookupBSONField(doc, "executionStats"); ok {
		if d, ok := v.(bson.D); ok {
			return d
		}
	}
	if stages, ok := lookupBSONField(doc, "stages"); ok {
		if arr, ok := stages.(bson.A); ok && len(arr) > 0 {
			if first, ok := arr[0].(bson.D); ok {
				if cursor, ok := lookupBSONField(first, "$cursor"); ok {
					if cd, ok := cursor.(bson.D); ok {
						return findExecutionStats(cd)
					}
				}
			}
		}
	}
	return nil
}

// extractPlan recursively pulls stage/indexName from a winning plan tree,
// preserving the inputStage chain so a SORT->FETCH->IXSCAN nest is visible.
func extractPlan(wp bson.D) bson.D {
	out := bson.D{}
	if v, ok := lookupBSONField(wp, "stage"); ok {
		out = append(out, bson.E{Key: "stage", Value: v})
	}
	if v, ok := lookupBSONField(wp, "indexName"); ok {
		out = append(out, bson.E{Key: "indexName", Value: v})
	}
	if v, ok := lookupBSONField(wp, "inputStage"); ok {
		if isD, ok := v.(bson.D); ok {
			out = append(out, bson.E{Key: "inputStage", Value: extractPlan(isD)})
		}
	}
	return out
}

// explainCritical extracts the high-signal fields from an explain response,
// stripping server-specific noise so MongoDB and DumboDB responses can be
// compared structurally. Only fields present in the input are emitted.
func explainCritical(doc bson.D) bson.D {
	out := bson.D{}
	if qp := findQueryPlanner(doc); qp != nil {
		if pq, ok := lookupBSONField(qp, "parsedQuery"); ok {
			out = append(out, bson.E{Key: "parsedQuery", Value: pq})
		}
		if wp, ok := lookupBSONField(qp, "winningPlan"); ok {
			if wpD, ok := wp.(bson.D); ok {
				out = append(out, bson.E{Key: "winningPlan", Value: extractPlan(wpD)})
			}
		}
		if rp, ok := lookupBSONField(qp, "rejectedPlans"); ok {
			if arr, ok := rp.(bson.A); ok {
				out = append(out, bson.E{Key: "rejectedPlansCount", Value: int32(len(arr))})
			}
		}
	}
	if es := findExecutionStats(doc); es != nil {
		esOut := bson.D{}
		for _, k := range []string{"nReturned", "totalDocsExamined", "totalKeysExamined"} {
			if v, ok := lookupBSONField(es, k); ok {
				esOut = append(esOut, bson.E{Key: k, Value: v})
			}
		}
		if len(esOut) > 0 {
			out = append(out, bson.E{Key: "executionStats", Value: esOut})
		}
	}
	return out
}

// runExplain executes an explain command and returns only the critical
// fields, suitable for parity comparison.
func runExplain(ctx context.Context, col *mongo.Collection, cmd interface{}) (interface{}, error) {
	var doc bson.D
	if err := col.Database().RunCommand(ctx, cmd).Decode(&doc); err != nil {
		return nil, err
	}
	return explainCritical(doc), nil
}

func TestExplain_Find_QueryPlanner(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Find_QueryPlanner",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cmd := bson.D{
				{Key: "explain", Value: bson.D{
					{Key: "find", Value: col.Name()},
					{Key: "filter", Value: bson.D{{Key: "val", Value: bson.D{{Key: "$gt", Value: int32(2)}}}}},
				}},
				{Key: "verbosity", Value: "queryPlanner"},
			}
			return runExplain(ctx, col, cmd)
		},
	})
}

func TestExplain_Find_ExecutionStats(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Find_ExecutionStats",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cmd := bson.D{
				{Key: "explain", Value: bson.D{
					{Key: "find", Value: col.Name()},
					{Key: "filter", Value: bson.D{}},
				}},
				{Key: "verbosity", Value: "executionStats"},
			}
			return runExplain(ctx, col, cmd)
		},
	})
}

func TestExplain_Find_AllPlansExecution(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Find_AllPlansExecution",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cmd := bson.D{
				{Key: "explain", Value: bson.D{
					{Key: "find", Value: col.Name()},
					{Key: "filter", Value: bson.D{}},
				}},
				{Key: "verbosity", Value: "allPlansExecution"},
			}
			return runExplain(ctx, col, cmd)
		},
	})
}

func TestExplain_Aggregate(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Aggregate",
		Support: harness.DumboDBFull,
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
			return runExplain(ctx, col, cmd)
		},
	})
}

func TestExplain_Count(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Count",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cmd := bson.D{
				{Key: "explain", Value: bson.D{
					{Key: "count", Value: col.Name()},
					{Key: "query", Value: bson.D{{Key: "val", Value: bson.D{{Key: "$lte", Value: int32(3)}}}}},
				}},
				{Key: "verbosity", Value: "queryPlanner"},
			}
			return runExplain(ctx, col, cmd)
		},
	})
}

func TestExplain_Update(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Update",
		Support: harness.DumboDBFull,
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
			return runExplain(ctx, col, cmd)
		},
	})
}

func TestExplain_Delete(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Delete",
		Support: harness.DumboDBFull,
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
			return runExplain(ctx, col, cmd)
		},
	})
}

func TestExplain_Distinct(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Distinct",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cmd := bson.D{
				{Key: "explain", Value: bson.D{
					{Key: "distinct", Value: col.Name()},
					{Key: "key", Value: "name"},
				}},
				{Key: "verbosity", Value: "queryPlanner"},
			}
			return runExplain(ctx, col, cmd)
		},
	})
}

// Index-aware explain tests: build an index, then verify the planner picks
// IXSCAN on a query that covers it. The point is that explain must reflect
// actual query planning — a COLLSCAN here would be a parity bug.

func TestExplain_Find_IXSCAN_AfterIndexCreated(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Find_IXSCAN_AfterIndexCreated",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertColTestDocs(ctx, col); err != nil {
				return err
			}
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "name", Value: 1}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cmd := bson.D{
				{Key: "explain", Value: bson.D{
					{Key: "find", Value: col.Name()},
					{Key: "filter", Value: bson.D{{Key: "name", Value: "alpha"}}},
				}},
				{Key: "verbosity", Value: "queryPlanner"},
			}
			return runExplain(ctx, col, cmd)
		},
	})
}

func TestExplain_Count_IXSCAN_AfterIndexCreated(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Count_IXSCAN_AfterIndexCreated",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertColTestDocs(ctx, col); err != nil {
				return err
			}
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "val", Value: 1}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cmd := bson.D{
				{Key: "explain", Value: bson.D{
					{Key: "count", Value: col.Name()},
					{Key: "query", Value: bson.D{{Key: "val", Value: int32(3)}}},
				}},
				{Key: "verbosity", Value: "queryPlanner"},
			}
			return runExplain(ctx, col, cmd)
		},
	})
}

func TestExplain_Aggregate_IXSCAN_AfterIndexCreated(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Aggregate_IXSCAN_AfterIndexCreated",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertColTestDocs(ctx, col); err != nil {
				return err
			}
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "val", Value: 1}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cmd := bson.D{
				{Key: "explain", Value: bson.D{
					{Key: "aggregate", Value: col.Name()},
					{Key: "pipeline", Value: bson.A{
						bson.D{{Key: "$match", Value: bson.D{{Key: "val", Value: int32(3)}}}},
					}},
					{Key: "cursor", Value: bson.D{}},
				}},
				{Key: "verbosity", Value: "queryPlanner"},
			}
			return runExplain(ctx, col, cmd)
		},
	})
}

func TestCollection_CountDocuments_NoFilter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collection_CountDocuments_NoFilter",
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
