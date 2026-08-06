// Copyright 2026 Dolthub, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tests

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// unknownFieldCode appends a bogus top-level field to cmd, runs it, and returns
// the resulting command-error code. MongoDB rejects an unknown top-level field
// during IDL parsing (before executing the command) with IDLUnknownField
// (40415), so cmd need only be well-formed enough to reach the parse, not to
// succeed. DumboDB must match once its handler adopts common.RejectUnknownFields.
func unknownFieldCode(ctx context.Context, col *mongo.Collection, cmd bson.D) (interface{}, error) {
	full := make(bson.D, len(cmd), len(cmd)+1)
	copy(full, cmd)
	full = append(full, bson.E{Key: "nonExistentField42", Value: int32(1)})

	err := col.Database().RunCommand(ctx, full).Err()
	code, _, _ := harness.CommandErrorCode(err)
	return bson.D{{Key: "unknownFieldCode", Value: code}}, nil
}

// ufRejectionCase asserts a command rejects an unknown top-level field
// identically on both servers. build receives the test collection's name so the
// command can target a real namespace.
func ufRejectionCase(t *testing.T, name string, support harness.DumboDBSupport, build func(coll string) bson.D) {
	harness.PairTest(t, harness.TestCase{
		Name:    name,
		Support: support,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return unknownFieldCode(ctx, col, build(col.Name()))
		},
	})
}

// ufRejectionCaseAdmin is ufRejectionCase for admin-scoped commands (run against
// the admin database). cmd is fixed (no per-collection target needed).
func ufRejectionCaseAdmin(t *testing.T, name string, support harness.DumboDBSupport, cmd bson.D) {
	harness.PairTest(t, harness.TestCase{
		Name:    name,
		Support: support,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			full := make(bson.D, len(cmd), len(cmd)+1)
			copy(full, cmd)
			full = append(full, bson.E{Key: "nonExistentField42", Value: int32(1)})
			err := col.Database().Client().Database("admin").RunCommand(ctx, full).Err()
			code, _, _ := harness.CommandErrorCode(err)
			return bson.D{{Key: "unknownFieldCode", Value: code}}, nil
		},
	})
}

// TestUnknownField_Session covers the strict cursor/session commands (ei1 Phase
// 5 subset). hello/isMaster/buildInfo/whatsmyuri are NOT included: they are
// legacy non-strict commands MongoDB accepts unknown fields on.
func TestUnknownField_Session(t *testing.T) {
	ufRejectionCaseAdmin(t, "UnknownField_ping", harness.DumboDBFull, bson.D{{Key: "ping", Value: int32(1)}})
	ufRejectionCaseAdmin(t, "UnknownField_logout", harness.DumboDBFull, bson.D{{Key: "logout", Value: int32(1)}})
	ufRejectionCaseAdmin(t, "UnknownField_endSessions", harness.DumboDBFull, bson.D{{Key: "endSessions", Value: bson.A{}}})
	ufRejectionCaseAdmin(t, "UnknownField_connectionStatus", harness.DumboDBFull, bson.D{{Key: "connectionStatus", Value: int32(1)}})
	ufRejectionCase(t, "UnknownField_killCursors", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "killCursors", Value: c}, {Key: "cursors", Value: bson.A{}}}
	})
	ufRejectionCase(t, "UnknownField_getMore", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "getMore", Value: int64(1)}, {Key: "collection", Value: c}}
	})
}

// TestUnknownField_ServerIntrospection covers the strict server-introspection
// commands (ei1 Phase 6 subset). serverStatus/listCommands/currentOp/
// getParameter/debugError/getFreeMonitoringStatus are non-strict, excluded.
func TestUnknownField_ServerIntrospection(t *testing.T) {
	ufRejectionCaseAdmin(t, "UnknownField_hostInfo", harness.DumboDBFull, bson.D{{Key: "hostInfo", Value: int32(1)}})
	ufRejectionCaseAdmin(t, "UnknownField_getLog", harness.DumboDBFull, bson.D{{Key: "getLog", Value: "global"}})
	ufRejectionCaseAdmin(t, "UnknownField_getCmdLineOpts", harness.DumboDBFull, bson.D{{Key: "getCmdLineOpts", Value: int32(1)}})
	ufRejectionCaseAdmin(t, "UnknownField_listDatabases", harness.DumboDBFull, bson.D{{Key: "listDatabases", Value: int32(1)}})
}

// TestUnknownField_UserRole covers the user & role management family (ei1 Phase
// 4). All are strict in MongoDB; the allow-lists were validated against Mongo to
// include fields DumboDB does not model (customData, digestPassword, filter,
// showAuthenticationRestrictions) so valid commands are not over-restricted.
func TestUnknownField_UserRole(t *testing.T) {
	target := map[string]interface{}{
		"createUser": "u", "updateUser": "u", "dropUser": "u",
		"createRole": "r", "updateRole": "r", "dropRole": "r",
		"grantRolesToUser": "u", "revokeRolesFromUser": "u",
		"grantRolesToRole": "r", "revokeRolesFromRole": "r",
		"grantPrivilegesToRole": "r", "revokePrivilegesFromRole": "r",
		"dropAllUsersFromDatabase": int32(1), "dropAllRolesFromDatabase": int32(1),
		"usersInfo": int32(1), "rolesInfo": int32(1),
	}
	order := []string{
		"createUser", "updateUser", "dropUser", "dropAllUsersFromDatabase", "usersInfo",
		"grantRolesToUser", "revokeRolesFromUser",
		"createRole", "updateRole", "dropRole", "dropAllRolesFromDatabase", "rolesInfo",
		"grantRolesToRole", "revokeRolesFromRole", "grantPrivilegesToRole", "revokePrivilegesFromRole",
	}
	for _, cmd := range order {
		cmd, tgt := cmd, target[cmd]
		ufRejectionCase(t, "UnknownField_"+cmd, harness.DumboDBFull, func(string) bson.D {
			return bson.D{{Key: cmd, Value: tgt}}
		})
	}
}

// TestUnknownField_AggregateExplain covers aggregate and explain (ei1 Phase 7,
// top-level fields only). The positive case asserts aggregate's full optional
// field set is accepted (no over-restriction).
func TestUnknownField_AggregateExplain(t *testing.T) {
	ufRejectionCase(t, "UnknownField_aggregate", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "aggregate", Value: c}, {Key: "pipeline", Value: bson.A{}}, {Key: "cursor", Value: bson.D{}}}
	})
	ufRejectionCase(t, "UnknownField_explain", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "explain", Value: bson.D{{Key: "find", Value: c}}}, {Key: "verbosity", Value: "queryPlanner"}}
	})

	// Positive: aggregate's optional fields are NOT treated as unknown. Limited to
	// options DumboDB implements; collation/let are allowed by RejectUnknownFields
	// but separately unimplemented in aggregate (a pre-existing gap, not ei1's).
	harness.PairTest(t, harness.TestCase{
		Name:    "UnknownField_aggregateOptionsAccepted",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			err := col.Database().RunCommand(ctx, bson.D{
				{Key: "aggregate", Value: col.Name()},
				{Key: "pipeline", Value: bson.A{}},
				{Key: "cursor", Value: bson.D{}},
				{Key: "allowDiskUse", Value: true},
				{Key: "bypassDocumentValidation", Value: false},
				{Key: "hint", Value: bson.D{}},
			}).Err()
			code, _, _ := harness.CommandErrorCode(err)
			return bson.D{{Key: "unknownFieldCode", Value: code}}, nil
		},
	})
}

// TestUnknownField_WriteAndTxn covers bulkWrite and the transaction commands
// (strict). startSession/setParameter/convertToCapped are non-strict, excluded.
func TestUnknownField_WriteAndTxn(t *testing.T) {
	ufRejectionCaseAdmin(t, "UnknownField_bulkWrite", harness.DumboDBFull,
		bson.D{{Key: "bulkWrite", Value: int32(1)}, {Key: "ops", Value: bson.A{}}, {Key: "nsInfo", Value: bson.A{}}})
	ufRejectionCaseAdmin(t, "UnknownField_abortTransaction", harness.DumboDBFull,
		bson.D{{Key: "abortTransaction", Value: int32(1)}})
	ufRejectionCaseAdmin(t, "UnknownField_commitTransaction", harness.DumboDBFull,
		bson.D{{Key: "commitTransaction", Value: int32(1)}})
}

// TestUnknownField_DDLExtended covers create/createIndexes/listCollections/
// listIndexes/validate/compact (ei1 Phases 2-3 follow-up). Their allow-lists
// cover MongoDB's full field set (validated by per-field probing) so valid
// commands are not over-restricted.
func TestUnknownField_DDLExtended(t *testing.T) {
	ufRejectionCase(t, "UnknownField_create", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "create", Value: c + "_new"}}
	})
	ufRejectionCase(t, "UnknownField_createIndexes", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "createIndexes", Value: c}, {Key: "indexes", Value: bson.A{}}}
	})
	ufRejectionCase(t, "UnknownField_listCollections", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "listCollections", Value: int32(1)}}
	})
	ufRejectionCase(t, "UnknownField_listIndexes", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "listIndexes", Value: c}}
	})
	ufRejectionCase(t, "UnknownField_compact", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "compact", Value: c}}
	})

	// Positive: create's optional fields are not treated as unknown. Limited to
	// fields DumboDB accepts (storageEngine/indexOptionDefaults are ignored);
	// capped/size/max hit a separate pre-existing create-option gap.
	harness.PairTest(t, harness.TestCase{
		Name:    "UnknownField_createOptionsAccepted",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			err := col.Database().RunCommand(ctx, bson.D{
				{Key: "create", Value: col.Name() + "_opts"},
				{Key: "storageEngine", Value: bson.D{}},
				{Key: "indexOptionDefaults", Value: bson.D{}},
			}).Err()
			code, _, _ := harness.CommandErrorCode(err)
			return bson.D{{Key: "unknownFieldCode", Value: code}}, nil
		},
	})
}

// TestUnknownField_ValidateLegacy documents that validate is a non-strict
// command: MongoDB accepts an unknown field on an existing collection, so
// DumboDB must not reject it either.
func TestUnknownField_ValidateLegacy(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UnknownField_validate_notStrict",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "x", Value: int32(1)}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			err := col.Database().RunCommand(ctx, bson.D{
				{Key: "validate", Value: col.Name()},
				{Key: "nonExistentField42", Value: int32(1)},
			}).Err()
			code, _, _ := harness.CommandErrorCode(err)
			return bson.D{{Key: "unknownFieldCode", Value: code}}, nil
		},
	})
}

// TestUnknownField_AuthAdmin covers saslStart/saslContinue/autoCompact (ei1
// Phase 5-6 tail). The reject runs before the SASL exchange / compaction.
func TestUnknownField_AuthAdmin(t *testing.T) {
	ufRejectionCaseAdmin(t, "UnknownField_saslStart", harness.DumboDBFull,
		bson.D{{Key: "saslStart", Value: int32(1)}, {Key: "mechanism", Value: "SCRAM-SHA-256"}})
	ufRejectionCaseAdmin(t, "UnknownField_saslContinue", harness.DumboDBFull,
		bson.D{{Key: "saslContinue", Value: int32(1)}, {Key: "conversationId", Value: int32(1)}})
}

// TestUnknownField_CRUD locks in the already-strict CRUD commands: MongoDB and
// DumboDB both reject an unknown top-level field with IDLUnknownField (40415).
// This is the ei1 Phase 0 CRUD regression guard. Later family phases add their
// commands here, flipping Support from XFail to Full as each lands.
func TestUnknownField_CRUD(t *testing.T) {
	ufRejectionCase(t, "UnknownField_find", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "find", Value: c}}
	})
	ufRejectionCase(t, "UnknownField_count", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "count", Value: c}}
	})
	ufRejectionCase(t, "UnknownField_distinct", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "distinct", Value: c}, {Key: "key", Value: "x"}}
	})
	ufRejectionCase(t, "UnknownField_insert", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "insert", Value: c}, {Key: "documents", Value: bson.A{bson.D{{Key: "x", Value: int32(1)}}}}}
	})
	ufRejectionCase(t, "UnknownField_update", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "update", Value: c}, {Key: "updates", Value: bson.A{bson.D{
			{Key: "q", Value: bson.D{}}, {Key: "u", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "a", Value: int32(1)}}}}},
		}}}}
	})
	ufRejectionCase(t, "UnknownField_delete", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "delete", Value: c}, {Key: "deletes", Value: bson.A{bson.D{
			{Key: "q", Value: bson.D{}}, {Key: "limit", Value: int32(1)},
		}}}}
	})
	ufRejectionCase(t, "UnknownField_findAndModify", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "findAndModify", Value: c}, {Key: "remove", Value: true}}
	})
}

// TestUnknownField_DDL covers the DDL family (ei1 Phase 2). renameCollection is
// admin-scoped and handled separately below.
func TestUnknownField_DDL(t *testing.T) {
	ufRejectionCase(t, "UnknownField_drop", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "drop", Value: c}}
	})
	ufRejectionCase(t, "UnknownField_dropDatabase", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "dropDatabase", Value: int32(1)}}
	})
	ufRejectionCase(t, "UnknownField_dropIndexes", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "dropIndexes", Value: c}, {Key: "index", Value: "*"}}
	})
	ufRejectionCase(t, "UnknownField_collMod", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "collMod", Value: c}}
	})
}

// TestUnknownField_RenameCollection covers renameCollection, which runs against
// the admin database with fully-qualified namespaces.
func TestUnknownField_RenameCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UnknownField_renameCollection",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			src := col.Database().Name() + "." + col.Name()
			err := col.Database().Client().Database("admin").RunCommand(ctx, bson.D{
				{Key: "renameCollection", Value: src},
				{Key: "to", Value: src + "_renamed"},
				{Key: "nonExistentField42", Value: int32(1)},
			}).Err()
			code, _, _ := harness.CommandErrorCode(err)
			return bson.D{{Key: "unknownFieldCode", Value: code}}, nil
		},
	})
}

// TestUnknownField_Introspection covers the safe introspection commands (ei1
// Phase 3 subset). dataSize takes a namespace value; top is admin-scoped.
func TestUnknownField_Introspection(t *testing.T) {
	ufRejectionCase(t, "UnknownField_dbStats", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "dbStats", Value: int32(1)}}
	})
	ufRejectionCase(t, "UnknownField_collStats", harness.DumboDBFull, func(c string) bson.D {
		return bson.D{{Key: "collStats", Value: c}}
	})

	harness.PairTest(t, harness.TestCase{
		Name:    "UnknownField_dataSize",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			ns := col.Database().Name() + "." + col.Name()
			return unknownFieldCode(ctx, col, bson.D{{Key: "dataSize", Value: ns}})
		},
	})

	// top is a legacy diagnostic command that is NOT strict-IDL in MongoDB: it
	// accepts (ignores) an unknown field rather than rejecting it. DumboDB must
	// match by NOT rejecting -- both return no IDLUnknownField (code 0).
	harness.PairTest(t, harness.TestCase{
		Name:    "UnknownField_top_notStrict",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			err := col.Database().Client().Database("admin").RunCommand(ctx, bson.D{
				{Key: "top", Value: int32(1)},
				{Key: "nonExistentField42", Value: int32(1)},
			}).Err()
			code, _, _ := harness.CommandErrorCode(err)
			return bson.D{{Key: "unknownFieldCode", Value: code}}, nil
		},
	})
}

// TestUnknownField_EnvelopeAccepted proves the protocol envelope is NOT treated
// as unknown: a command carrying the standard driver-appended fields succeeds
// (no IDLUnknownField) on both servers.
func TestUnknownField_EnvelopeAccepted(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UnknownField_EnvelopeAccepted",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			err := col.Database().RunCommand(ctx, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "comment", Value: "envelope"},
				{Key: "maxTimeMS", Value: int32(5000)},
				{Key: "readConcern", Value: bson.D{{Key: "level", Value: "local"}}},
			}).Err()
			code, _, _ := harness.CommandErrorCode(err)
			return bson.D{{Key: "unknownFieldCode", Value: code}}, nil
		},
	})
}
