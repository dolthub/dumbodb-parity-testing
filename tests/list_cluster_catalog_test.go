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
	"fmt"
	"regexp"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// listClusterCatalogSetup creates a clustered collection with options, a view
// with a collation, and a time-series collection beside the test collection.
func listClusterCatalogSetup(ctx context.Context, col *mongo.Collection) error {
	db := col.Database()
	cmds := []bson.D{
		{{Key: "create", Value: "cc"}, {Key: "clusteredIndex", Value: bson.D{{Key: "key", Value: bson.D{{Key: "_id", Value: int32(1)}}}, {Key: "unique", Value: true}}},
			{Key: "collation", Value: bson.D{{Key: "locale", Value: "fr"}}}, {Key: "validator", Value: bson.D{{Key: "x", Value: bson.D{{Key: "$gt", Value: int32(1)}}}}},
			{Key: "validationLevel", Value: "moderate"}},
		{{Key: "create", Value: "vv"}, {Key: "viewOn", Value: "cc"}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "x", Value: int32(1)}}}}}},
			{Key: "collation", Value: bson.D{{Key: "locale", Value: "de"}}}},
		{{Key: "create", Value: "ts"}, {Key: "timeseries", Value: bson.D{{Key: "timeField", Value: "t"}, {Key: "granularity", Value: "hours"}}}},
	}
	for _, cmd := range cmds {
		if err := db.RunCommand(ctx, cmd).Err(); err != nil {
			return err
		}
	}
	return nil
}

// listClusterCatalogRun runs {aggregate: 1} with stage on dbName (the test
// database when empty), keeping the setup's entries without their uuids and
// ICU collation versions, sorted by namespace.
func listClusterCatalogRun(dbName string, stage interface{}) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		testDB := col.Database().Name()
		db := col.Database()
		if dbName != "" {
			db = col.Database().Client().Database(dbName)
		}
		pipeline := bson.A{
			bson.D{{Key: "$listClusterCatalog", Value: stage}},
			bson.D{{Key: "$match", Value: bson.D{
				{Key: "db", Value: testDB},
				{Key: "ns", Value: bson.D{{Key: "$in", Value: bson.A{testDB + ".cc", testDB + ".vv", testDB + ".ts"}}}},
			}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "info.uuid", Value: 0}, {Key: "options.collation.version", Value: 0}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "ns", Value: 1}}}},
		}
		cursor, err := db.Aggregate(ctx, pipeline)
		return drainCursor(ctx, cursor, err)
	}
}

// TestListClusterCatalog: on a standalone, $listClusterCatalog lists the
// database's collections and views (every database's on admin) with their
// listCollections fields, the namespace, and sharded: false; shards adds an
// empty shard list, and balancingConfiguration adds nothing.
func TestListClusterCatalog(t *testing.T) {
	cases := []struct {
		name  string
		db    string
		stage bson.D
	}{
		{"Default", "", bson.D{}},
		{"Shards", "", bson.D{{Key: "shards", Value: true}}},
		{"ShardsFalse", "", bson.D{{Key: "shards", Value: false}}},
		{"Balancing", "", bson.D{{Key: "balancingConfiguration", Value: true}}},
		{"Both", "", bson.D{{Key: "shards", Value: true}, {Key: "balancingConfiguration", Value: true}}},
		{"Admin", "admin", bson.D{}},
		{"AdminShards", "admin", bson.D{{Key: "shards", Value: true}}},
	}
	for _, tc := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "ListClusterCatalog_" + tc.name,
			Support: harness.DumboDBFull,
			Setup:   listClusterCatalogSetup,
			Run:     listClusterCatalogRun(tc.db, tc.stage),
		})
	}

	harness.PairTest(t, harness.TestCase{
		Name:    "ListClusterCatalog_OtherDatabaseHidden",
		Support: harness.DumboDBFull,
		Setup:   listClusterCatalogSetup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			other := col.Database().Client().Database(fmt.Sprintf("lcc_other_%d", time.Now().UnixNano()))
			if err := other.Drop(ctx); err != nil {
				return nil, err
			}
			if err := other.CreateCollection(ctx, "empty"); err != nil {
				return nil, err
			}
			defer other.Drop(ctx)
			cursor, err := col.Database().Aggregate(ctx, bson.A{
				bson.D{{Key: "$listClusterCatalog", Value: bson.D{}}},
				bson.D{{Key: "$match", Value: bson.D{{Key: "ns", Value: primitive.Regex{Pattern: "^" + regexp.QuoteMeta(other.Name()) + `\.`}}}}},
				bson.D{{Key: "$count", Value: "n"}},
			})
			return drainCursor(ctx, cursor, err)
		},
	})

	harness.PairTest(t, harness.TestCase{
		Name:    "ListClusterCatalog_EmptyDatabase",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Database().Client().Database(fmt.Sprintf("lcc_none_%d", time.Now().UnixNano())).Aggregate(ctx,
				bson.A{bson.D{{Key: "$listClusterCatalog", Value: bson.D{}}}})
			return drainCursor(ctx, cursor, err)
		},
	})

	harness.PairTest(t, harness.TestCase{
		Name:    "ListClusterCatalog_LaterStages",
		Support: harness.DumboDBFull,
		Setup:   listClusterCatalogSetup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Database().Aggregate(ctx, bson.A{
				bson.D{{Key: "$listClusterCatalog", Value: bson.D{}}},
				bson.D{{Key: "$match", Value: bson.D{{Key: "type", Value: bson.D{{Key: "$in", Value: bson.A{"view", "timeseries"}}}}}}},
				bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$type"}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return drainCursor(ctx, cursor, err)
		},
	})
}

// TestListClusterCatalogErrors: $listClusterCatalog checks its specification
// field by field with mongod's IDL codes, must be first, is refused in a
// $facet of {aggregate: 1}, and is refused on a collection before its
// specification is checked.
func TestListClusterCatalogErrors(t *testing.T) {
	cases := map[string]string{
		"NotObject":          `[{"$listClusterCatalog":1}]`,
		"Array":              `[{"$listClusterCatalog":[]}]`,
		"Null":               `[{"$listClusterCatalog":null}]`,
		"ShardsInt":          `[{"$listClusterCatalog":{"shards":1}}]`,
		"ShardsNull":         `[{"$listClusterCatalog":{"shards":null}}]`,
		"BalancingString":    `[{"$listClusterCatalog":{"balancingConfiguration":"x"}}]`,
		"Unknown":            `[{"$listClusterCatalog":{"bogus":1}}]`,
		"UnknownBeforeBad":   `[{"$listClusterCatalog":{"bogus":1,"shards":1}}]`,
		"BadBeforeUnknown":   `[{"$listClusterCatalog":{"shards":1,"bogus":1}}]`,
		"Duplicate":          `[{"$listClusterCatalog":{"shards":true,"shards":false}}]`,
		"Twice":              `[{"$listClusterCatalog":{}},{"$listClusterCatalog":{}}]`,
		"SecondBadSpec":      `[{"$listClusterCatalog":{}},{"$listClusterCatalog":{"bogus":1}}]`,
		"AfterDocuments":     `[{"$documents":[]},{"$listClusterCatalog":{}}]`,
		"AfterDocumentsBad":  `[{"$documents":[]},{"$listClusterCatalog":1}]`,
		"AfterMatch":         `[{"$match":{}},{"$listClusterCatalog":{}}]`,
		"ThenDocuments":      `[{"$listClusterCatalog":{}},{"$documents":[]}]`,
		"ThenBogusStage":     `[{"$listClusterCatalog":{"bogus":1}},{"$bogus":1}]`,
		"ThenBadMatch":       `[{"$listClusterCatalog":{"bogus":1}},{"$match":{"$bad":1}}]`,
		"ThenBadMatchOK":     `[{"$listClusterCatalog":{}},{"$match":{"$bad":1}}]`,
		"InFacet":            `[{"$documents":[]},{"$facet":{"a":[{"$listClusterCatalog":{}}]}}]`,
		"InFacetLater":       `[{"$documents":[]},{"$facet":{"a":[{"$match":{}},{"$listClusterCatalog":{}}]}}]`,
		"InFacetBadSpec":     `[{"$documents":[]},{"$facet":{"a":[{"$listClusterCatalog":1}]}}]`,
		"ThenFacet":          `[{"$listClusterCatalog":{}},{"$facet":{"a":[{"$listClusterCatalog":{}}]}}]`,
		"InLookup":           `[{"$documents":[]},{"$lookup":{"from":"x","pipeline":[{"$listClusterCatalog":1}],"as":"j"}}]`,
		"InUnionWith":        `[{"$documents":[]},{"$unionWith":{"coll":"x","pipeline":[{"$listClusterCatalog":{}}]}}]`,
		"ThenQueryStatsHere": `[{"$listClusterCatalog":{}},{"$queryStats":{}}]`,
	}
	collectionCases := map[string]string{
		"OnCollection":         `[{"$listClusterCatalog":{}}]`,
		"OnCollectionBadSpec":  `[{"$listClusterCatalog":1}]`,
		"OnCollectionUnknown":  `[{"$listClusterCatalog":{"bogus":1}}]`,
		"OnCollectionInFacet":  `[{"$facet":{"a":[{"$listClusterCatalog":1}]}}]`,
		"OnCollectionInLookup": `[{"$lookup":{"from":"x","pipeline":[{"$listClusterCatalog":{}}],"as":"j"}}]`,
	}
	for _, group := range []struct {
		cases        map[string]string
		onCollection bool
	}{{cases, false}, {collectionCases, true}} {
		for name, js := range group.cases {
			var pipeline bson.A
			if err := bson.UnmarshalExtJSON([]byte(`{"p":`+js+`}`), false, &struct{ P *bson.A }{&pipeline}); err != nil {
				t.Fatal(name, err)
			}
			onCollection := group.onCollection
			harness.PairTest(t, harness.TestCase{
				Name:    "ListClusterCatalogErrors_" + name,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					var target interface{} = int32(1)
					if onCollection {
						target = col.Name()
					}
					res := col.Database().RunCommand(ctx, bson.D{{Key: "aggregate", Value: target}, {Key: "pipeline", Value: pipeline}, {Key: "cursor", Value: bson.D{}}})
					if err := res.Err(); err != nil {
						return errOutcome(err), nil
					}
					return bson.M{"ok": true}, nil
				},
			})
		}
	}
}
