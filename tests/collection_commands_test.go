package tests

// collection_commands_test.go covers collection and database administration commands
// not yet tested in collection_test.go:
//   collMod, compact, autoCompact (8.0), convertToCapped, dataSize
// plus error-case variants for existing commands.

import (
	"context"
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func TestCollMod_AddValidator(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "CollMod_AddValidator",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Add a JSON Schema validator to an existing collection.
			return runCommandDoc(ctx, col, bson.D{
				{Key: "collMod", Value: col.Name()},
				{Key: "validator", Value: bson.D{
					{Key: "$jsonSchema", Value: bson.D{
						{Key: "bsonType", Value: "object"},
						{Key: "required", Value: bson.A{"name"}},
						{Key: "properties", Value: bson.D{
							{Key: "name", Value: bson.D{{Key: "bsonType", Value: "string"}}},
						}},
					}},
				}},
				{Key: "validationLevel", Value: "moderate"},
				{Key: "validationAction", Value: "warn"},
			})
		},
	})
}

func TestCollMod_ChangeValidationLevel(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "CollMod_ChangeValidationLevel",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Change validation level only (no validator change).
			return runCommandDoc(ctx, col, bson.D{
				{Key: "collMod", Value: col.Name()},
				{Key: "validationLevel", Value: "off"},
			})
		},
	})
}

func TestCollMod_ChangeStreamPreAndPostImages(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "CollMod_ChangeStreamPreAndPostImages",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Enable pre/post images for change streams (MongoDB 6.0+).
			return runCommandDoc(ctx, col, bson.D{
				{Key: "collMod", Value: col.Name()},
				{Key: "changeStreamPreAndPostImages", Value: bson.D{
					{Key: "enabled", Value: true},
				}},
			})
		},
	})
}

func TestCollMod_NonExistentCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "CollMod_NonExistentCollection",
		Support: harness.DumboDBFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// collMod on a collection that does not exist must return an error.
			return runCommandDoc(ctx, col, bson.D{
				{Key: "collMod", Value: "no_such_collection_xyz"},
				{Key: "validationLevel", Value: "off"},
			})
		},
	})
}

func TestCollMod_InvalidOption(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "CollMod_InvalidOption",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Passing an unrecognised option; MongoDB returns an error.
			return runCommandDoc(ctx, col, bson.D{
				{Key: "collMod", Value: col.Name()},
				{Key: "unknownOption", Value: true},
			})
		},
	})
}

func TestCompact_BasicCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Compact_BasicCollection",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// compact rewrites and defragments data; returns {ok: 1} on success.
			return runCommandDoc(ctx, col, bson.D{
				{Key: "compact", Value: col.Name()},
			})
		},
	})
}

func TestCompact_WithForceOption(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Compact_WithForceOption",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// compact with force:true runs even on a primary.
			return runCommandDoc(ctx, col, bson.D{
				{Key: "compact", Value: col.Name()},
				{Key: "force", Value: true},
			})
		},
	})
}

func TestCompact_EmptyCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Compact_EmptyCollection",
		Support: harness.DumboDBFull,
		Setup:   nil, // leave collection empty
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// compact on an empty collection should still succeed.
			return runCommandDoc(ctx, col, bson.D{
				{Key: "compact", Value: col.Name()},
			})
		},
	})
}

func TestCompact_NonExistentCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Compact_NonExistentCollection",
		Support: harness.DumboDBFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// compact on a non-existent collection returns NamespaceNotFound.
			return runCommandDoc(ctx, col, bson.D{
				{Key: "compact", Value: "no_such_collection_xyz"},
			})
		},
	})
}

// TestAutoCompact_DumboDBUnsupported documents a deliberate deviation: MongoDB
// supports autoCompact (returns ok:1), but DumboDB has no background auto-
// compaction on its Dolt-backed storage and reports NotImplemented (238) rather
// than a misleading success. This is not a PairTest -- the servers intentionally
// differ.
func TestAutoCompact_DumboDBUnsupported(t *testing.T) {
	ctx := context.Background()

	clients, err := harness.GetClients(ctx)
	if err != nil {
		t.Fatalf("get clients: %v", err)
	}

	mcol, dcol, cleanup, err := clients.TestDB(ctx, "AutoCompact_DumboDBUnsupported")
	if err != nil {
		t.Fatalf("allocate test DB: %v", err)
	}
	defer cleanup()

	cmd := bson.D{{Key: "autoCompact", Value: true}, {Key: "freeSpaceTargetMB", Value: int32(100)}}

	// autoCompact is admin-only in MongoDB, where it succeeds.
	if merr := mcol.Database().Client().Database("admin").RunCommand(ctx, cmd).Err(); merr != nil {
		t.Errorf("MongoDB autoCompact should succeed on admin, got: %v", merr)
	}

	derr := dcol.Database().Client().Database("admin").RunCommand(ctx, cmd).Err()
	if derr == nil {
		t.Fatalf("DumboDB autoCompact must report unsupported, got success")
	}
	if code, _, _ := harness.CommandErrorCode(derr); code != 238 {
		t.Errorf("DumboDB autoCompact should return NotImplemented (238), got %d: %v", code, derr)
	}
}

func TestConvertToCapped_Basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ConvertToCapped_Basic",
		Support: harness.DumboDBMongoOnly,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// convertToCapped converts a regular collection to a capped collection.
			return runCommandDoc(ctx, col, bson.D{
				{Key: "convertToCapped", Value: col.Name()},
				{Key: "size", Value: int64(1024 * 1024)}, // 1 MB
			})
		},
	})
}

func TestConvertToCapped_VerifyCapped(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ConvertToCapped_VerifyCapped",
		Support: harness.DumboDBMongoOnly,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Convert and then verify via listCollections that options.capped is set.
			_, err := col.Database().RunCommand(ctx, bson.D{
				{Key: "convertToCapped", Value: col.Name()},
				{Key: "size", Value: int64(1024 * 1024)},
			}).DecodeBytes()
			if err != nil {
				return nil, fmt.Errorf("convertToCapped: %w", err)
			}
			// List collections and check capped flag.
			return runCommandDoc(ctx, col, bson.D{
				{Key: "listCollections", Value: int32(1)},
				{Key: "filter", Value: bson.D{{Key: "name", Value: col.Name()}}},
			})
		},
	})
}

func TestConvertToCapped_NonExistentCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ConvertToCapped_NonExistentCollection",
		Support: harness.DumboDBMongoOnly,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// convertToCapped on a non-existent collection returns NamespaceNotFound.
			return runCommandDoc(ctx, col, bson.D{
				{Key: "convertToCapped", Value: "no_such_collection_xyz"},
				{Key: "size", Value: int64(1024 * 1024)},
			})
		},
	})
}

func TestConvertToCapped_ZeroSize(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ConvertToCapped_ZeroSize",
		Support: harness.DumboDBMongoOnly,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// size of 0 is invalid; MongoDB returns an error.
			return runCommandDoc(ctx, col, bson.D{
				{Key: "convertToCapped", Value: col.Name()},
				{Key: "size", Value: int64(0)},
			})
		},
	})
}

func TestConvertToCapped_MissingSize(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ConvertToCapped_MissingSize",
		Support: harness.DumboDBMongoOnly,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Omitting size should return an error.
			return runCommandDoc(ctx, col, bson.D{
				{Key: "convertToCapped", Value: col.Name()},
			})
		},
	})
}

func TestDataSize_BasicCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DataSize_BasicCollection",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// dataSize returns the size of data in a collection or key range.
			ns := fmt.Sprintf("%s.%s", col.Database().Name(), col.Name())
			return runCommandDoc(ctx, col, bson.D{
				{Key: "dataSize", Value: ns},
			})
		},
	})
}

func TestDataSize_EmptyCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DataSize_EmptyCollection",
		Support: harness.DumboDBFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// dataSize on an empty collection should return size 0.
			ns := fmt.Sprintf("%s.%s", col.Database().Name(), col.Name())
			return runCommandDoc(ctx, col, bson.D{
				{Key: "dataSize", Value: ns},
			})
		},
	})
}

func TestDataSize_WithKeyRange(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DataSize_WithKeyRange",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// dataSize with min/max key range (typically for sharding ranges).
			ns := fmt.Sprintf("%s.%s", col.Database().Name(), col.Name())
			return runCommandDoc(ctx, col, bson.D{
				{Key: "dataSize", Value: ns},
				{Key: "keyPattern", Value: bson.D{{Key: "_id", Value: int32(1)}}},
				{Key: "min", Value: bson.D{{Key: "_id", Value: "ct1"}}},
				{Key: "max", Value: bson.D{{Key: "_id", Value: "ct3"}}},
			})
		},
	})
}

func TestDataSize_NonExistentCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DataSize_NonExistentCollection",
		Support: harness.DumboDBFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// dataSize on a non-existent namespace should return an error or empty result.
			ns := fmt.Sprintf("%s.no_such_collection_xyz", col.Database().Name())
			return runCommandDoc(ctx, col, bson.D{
				{Key: "dataSize", Value: ns},
			})
		},
	})
}

func TestCollStats_NonExistentCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "CollStats_NonExistentCollection",
		Support: harness.DumboDBFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// collStats on a non-existent collection returns NamespaceNotFound.
			return runCommandDoc(ctx, col, bson.D{
				{Key: "collStats", Value: "no_such_collection_xyz"},
			})
		},
	})
}

func TestValidate_NonExistentCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Validate_NonExistentCollection",
		Support: harness.DumboDBFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// validate on a non-existent collection returns NamespaceNotFound.
			return runCommandDoc(ctx, col, bson.D{
				{Key: "validate", Value: "no_such_collection_xyz"},
			})
		},
	})
}

func TestValidate_Full(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Validate_Full",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// validate with full:true performs a more thorough check.
			doc, err := runCommandDoc(ctx, col, bson.D{
				{Key: "validate", Value: col.Name()},
				{Key: "full", Value: true},
			})
			if err != nil {
				return nil, err
			}
			// Compare the portable validation result. Omit "warnings" and
			// "indexDetails": under full:true WiredTiger emits non-deterministic
			// transient warnings ("could not complete validation ... actively in
			// use") that DumboDB has no analog for. The top-level valid flag and
			// keysPerIndex already assert index integrity.
			return pickFields(doc.(bson.D),
				"valid", "errors", "corruptRecords", "missingIndexEntries",
				"extraIndexEntries", "keysPerIndex", "nIndexes", "nInvalidDocuments",
				"nNonCompliantDocuments", "nrecords", "ns", "ok", "repaired", "uuid",
			), nil
		},
	})
}

func TestValidate_Repair(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Validate_Repair",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// validate with repair:true attempts to fix inconsistencies (requires standalone).
			return runCommandDoc(ctx, col, bson.D{
				{Key: "validate", Value: col.Name()},
				{Key: "repair", Value: true},
			})
		},
	})
}

func TestRenameCollection_NonExistentSource(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "RenameCollection_NonExistentSource",
		Support: harness.DumboDBFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Renaming a non-existent collection should return NamespaceNotFound.
			admin := col.Database().Client().Database("admin")
			src := fmt.Sprintf("%s.no_such_col_xyz", col.Database().Name())
			dst := fmt.Sprintf("%s.renamed_dst", col.Database().Name())
			result := admin.RunCommand(ctx, bson.D{
				{Key: "renameCollection", Value: src},
				{Key: "to", Value: dst},
			})
			var doc bson.D
			if err := result.Decode(&doc); err != nil {
				return nil, err
			}
			return doc, nil
		},
	})
}

func TestRenameCollection_DropTarget(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "RenameCollection_DropTarget",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			// Create both source and target so we can test dropTarget option.
			if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "src1"}}); err != nil {
				return err
			}
			target := col.Database().Collection(col.Name() + "_target")
			_, err := target.InsertOne(ctx, bson.D{{Key: "_id", Value: "tgt1"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// renameCollection with dropTarget:true overwrites an existing target.
			admin := col.Database().Client().Database("admin")
			src := fmt.Sprintf("%s.%s", col.Database().Name(), col.Name())
			dst := fmt.Sprintf("%s.%s_target", col.Database().Name(), col.Name())
			result := admin.RunCommand(ctx, bson.D{
				{Key: "renameCollection", Value: src},
				{Key: "to", Value: dst},
				{Key: "dropTarget", Value: true},
			})
			var doc bson.D
			if err := result.Decode(&doc); err != nil {
				return nil, err
			}
			return doc, nil
		},
	})
}

func TestDbStats_ScaleOption(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DbStats_ScaleOption",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// dbStats with scale option divides size fields by the scale factor.
			doc, err := runCommandDoc(ctx, col, bson.D{
				{Key: "dbStats", Value: int32(1)},
				{Key: "scale", Value: int32(1024)},
			})
			if err != nil {
				return nil, err
			}
			// Compare the scale-relevant portable fields: scaleFactor is echoed
			// and dataSize/avgObjSize are logical sizes divided by the scale.
			// Omit engine-specific physical metrics (storageSize, indexSize,
			// fsUsedSize, fsTotalSize) that differ between WiredTiger and Dolt.
			return pickFields(doc.(bson.D),
				"db", "collections", "views", "objects", "indexes", "ok",
				"scaleFactor", "dataSize", "avgObjSize",
			), nil
		},
	})
}

func TestDbStats_NonExistentDatabase(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DbStats_NonExistentDatabase",
		Support: harness.DumboDBFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// dbStats on a database that has never had data written returns empty stats.
			ghostDB := col.Database().Client().Database("ghost_db_xyz_never_exists")
			result := ghostDB.RunCommand(ctx, bson.D{{Key: "dbStats", Value: int32(1)}})
			var doc bson.D
			if err := result.Decode(&doc); err != nil {
				return nil, err
			}
			return doc, nil
		},
	})
}

func TestDropDatabase_NonExistent(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DropDatabase_NonExistent",
		Support: harness.DumboDBFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// dropDatabase on a database that does not exist should succeed silently.
			// We run it against the current (empty) test DB.
			return runCommandDoc(ctx, col, bson.D{{Key: "dropDatabase", Value: int32(1)}})
		},
	})
}

func TestListCollections_Filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ListCollections_Filter",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// listCollections with a name filter that matches nothing.
			cursor, err := col.Database().ListCollections(ctx,
				bson.D{{Key: "name", Value: "no_match_xyz"}})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return int32(len(results)), nil
		},
	})
}

func TestListCollections_TypeFilter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ListCollections_TypeFilter",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// listCollections filtering by type:"collection".
			cursor, err := col.Database().ListCollections(ctx,
				bson.D{{Key: "type", Value: "collection"}})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			// Return count only — names vary by test isolation DB.
			return int32(len(results)), nil
		},
	})
}

func TestCount_Deprecated_RunCommand(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Count_Deprecated_RunCommand",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// The legacy "count" command (deprecated in 4.0, still accepted).
			return runCommandDoc(ctx, col, bson.D{
				{Key: "count", Value: col.Name()},
				{Key: "query", Value: bson.D{{Key: "val", Value: bson.D{{Key: "$gt", Value: int32(2)}}}}},
			})
		},
	})
}

func TestCount_Deprecated_NoFilter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Count_Deprecated_NoFilter",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// count command without a filter — counts all documents.
			return runCommandDoc(ctx, col, bson.D{
				{Key: "count", Value: col.Name()},
			})
		},
	})
}

func TestCount_Deprecated_NonExistentCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Count_Deprecated_NonExistentCollection",
		Support: harness.DumboDBFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// count on a non-existent collection returns n:0 (not an error).
			return runCommandDoc(ctx, col, bson.D{
				{Key: "count", Value: "no_such_collection_xyz"},
			})
		},
	})
}

func TestDistinct_NonExistentCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Distinct_NonExistentCollection",
		Support: harness.DumboDBFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// distinct on a non-existent collection returns an empty values array.
			return runCommandDoc(ctx, col, bson.D{
				{Key: "distinct", Value: "no_such_collection_xyz"},
				{Key: "key", Value: "field"},
			})
		},
	})
}

func TestDistinct_WithQuery(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Distinct_WithQuery",
		Support: harness.DumboDBFull,
		Setup:   insertColTestDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// distinct via RunCommand with a query filter.
			return runCommandDoc(ctx, col, bson.D{
				{Key: "distinct", Value: col.Name()},
				{Key: "key", Value: "name"},
				{Key: "query", Value: bson.D{{Key: "val", Value: bson.D{{Key: "$gte", Value: int32(3)}}}}},
			})
		},
	})
}

func TestServerStatus_ReplicationField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name: "ServerStatus_ReplicationField",
		// MongoOnly: serverStatus is a large MongoDB-internal telemetry surface
		// (MongoDB returns ~49 sections; DumboDB emits a deliberate ~11-field
		// subset). It is not a viable full-document parity target, and the
		// "repl" section this case targets does not exist on a standalone
		// server (only within a replica set). DumboDB also does not honor
		// serverStatus section include/exclude filters. Run on MongoDB only.
		Support: harness.DumboDBMongoOnly,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// serverStatus with include filter for specific sections.
			return runCommandDoc(ctx, col, bson.D{
				{Key: "serverStatus", Value: int32(1)},
				{Key: "repl", Value: int32(1)},
				{Key: "metrics", Value: int32(0)},
				{Key: "locks", Value: int32(0)},
			})
		},
	})
}

func TestPing_ExplicitValue(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Ping_ExplicitValue",
		Support: harness.DumboDBFull,
		Setup:   nil,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// ping always returns {ok: 1} regardless of the argument value.
			return runCommandDoc(ctx, col, bson.D{{Key: "ping", Value: int32(42)}})
		},
	})
}
