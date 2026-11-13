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
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// mongod parses a pipeline's stages in order, sub-pipelines included, so the
// first stage that fails to parse names the error, before any check of where
// a stage may run.
func TestPipelineParseOrder(t *testing.T) {
	pipelines := map[string]string{
		"LookupUnknownBeforeChangeStream":    `[{"$lookup":{"from":"other","pipeline":[{"$bogus":{}},{"$changeStream":{}}],"as":"j"}}]`,
		"UnionWithUnknownBeforeChangeStream": `[{"$unionWith":{"coll":"other","pipeline":[{"$bogus":{}},{"$changeStream":{}}]}}]`,
		"UnknownBeforeChangeStream":          `[{"$bogus":{}},{"$changeStream":{}}]`,
		"BadDensifyBeforeChangeStream":       `[{"$densify":{"field":1}},{"$changeStream":{}}]`,
		"DensifyFieldTypeBeforeMissingRange": `[{"$densify":{"field":1}}]`,
		"MatchBeforeChangeStream":            `[{"$match":{}},{"$changeStream":{}}]`,
		"LookupFacetChangeStream":            `[{"$match":{"_id":-1}},{"$lookup":{"from":"COLL","pipeline":[{"$facet":{"a":[{"$changeStream":{}}]}}],"as":"j"}}]`,
		"NestedSearch":                       `[{"$match":{"_id":-1}},{"$lookup":{"from":"COLL","pipeline":[{"$lookup":{"from":"COLL","pipeline":[{"$unionWith":{"coll":"COLL","pipeline":[{"$search":{}}]}}],"as":"k"}}],"as":"j"}}]`,
		"LookupUnknownOnEmptyInput":          `[{"$match":{"_id":-1}},{"$lookup":{"from":"COLL","pipeline":[{"$bogus":{}}],"as":"j"}}]`,
		"LookupBadDensifyOnEmptyInput":       `[{"$match":{"_id":-1}},{"$lookup":{"from":"COLL","pipeline":[{"$densify":{"field":"x","range":{"step":"a","bounds":"full"}}}],"as":"j"}}]`,
		"UnionWithNestedUnknown":             `[{"$unionWith":{"coll":"COLL","pipeline":[{"$lookup":{"from":"COLL","pipeline":[{"$bogus":{}}],"as":"k"}}]}}]`,
		"FacetBadDensify":                    `[{"$facet":{"f":[{"$match":{}},{"$densify":{"field":1}}]}}]`,
		"EmptyProjectBeforeBadSkip":          `[{"$project":{}},{"$skip":-1}]`,
		"BadSetBeforeBadSkip":                `[{"$set":{"$x":1}},{"$skip":"x"}]`,
		"UndefinedBeforeBadSkip":             `[{"$project":{"x":"$$bogus"}},{"$skip":-1}]`,
		"BadUnsetBeforeBadSkip":              `[{"$unset":1},{"$skip":-1}]`,
		"ProjectBeforeBadSkip":               `[{"$project":{"x":1}},{"$skip":-1}]`,
	}
	for name, js := range pipelines {
		js := js
		harness.PairTest(t, harness.TestCase{
			Name:    "PipelineParseOrder_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}})
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				var stages []bson.D
				if err := bson.UnmarshalExtJSON([]byte(`{"p":`+strings.ReplaceAll(js, "COLL", col.Name())+`}`), false, &struct{ P *[]bson.D }{&stages}); err != nil {
					return nil, err
				}
				return fieldOpsAgg(ctx, col, stages...)
			},
		})
	}

	views := map[string]string{
		"OutThenUnknown":     `[{"$out":"y"},{"$bogus":{}}]`,
		"UnknownThenOut":     `[{"$bogus":{}},{"$out":"y"}]`,
		"OutInLookup":        `[{"$lookup":{"from":"COLL","pipeline":[{"$out":"y"}],"as":"j"}}]`,
		"BadDensify":         `[{"$densify":{"field":"x","range":{"step":"a","bounds":"full"}}}]`,
		"UndefinedInLookup":  `[{"$set":{"a":1}},{"$lookup":{"from":"COLL","pipeline":[{"$set":{"b":"$$nope"}}],"as":"j"}}]`,
		"UnknownInUnionWith": `[{"$unionWith":{"coll":"COLL","pipeline":[{"$bogus":{}}]}}]`,
		"UnknownInFacet":     `[{"$facet":{"f":[{"$bogus":{}}]}}]`,
		"LookupThenSet":      `[{"$lookup":{"from":"COLL","localField":"a","foreignField":"b","as":"j"}},{"$set":{"x":1}}]`,
		"BadUnsetBadSkip":    `[{"$unset":1},{"$skip":-1}]`,
	}
	for name, js := range views {
		js := js
		harness.PairTest(t, harness.TestCase{
			Name:    "PipelineParseOrder_View" + name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				var stages bson.A
				if err := bson.UnmarshalExtJSON([]byte(strings.ReplaceAll(js, "COLL", col.Name())), false, &stages); err != nil {
					return nil, err
				}
				view := col.Name() + "_v"
				defer col.Database().Collection(view).Drop(ctx)
				return nil, col.Database().CreateView(ctx, view, col.Name(), stages)
			},
		})
	}
}

// $queryStats returns no statistics from the admin database, after mongod's
// specification, position and namespace checks. $listCatalog is not
// implemented, but its specification and position are checked.
func TestQueryStatsStage(t *testing.T) {
	type cmd struct {
		db    string
		coll  interface{}
		stage string
	}
	cases := map[string]cmd{
		"Admin":                  {"admin", 1, `[{"$queryStats":{}}]`},
		"AdminThenMatch":         {"admin", 1, `[{"$queryStats":{}},{"$match":{"a":1}}]`},
		"AdminThenUnknown":       {"admin", 1, `[{"$queryStats":{}},{"$bogus":{}}]`},
		"UnknownField":           {"admin", 1, `[{"$queryStats":{"zz":1}}]`},
		"NotObject":              {"admin", 1, `[{"$queryStats":5}]`},
		"HmacWithoutKey":         {"admin", 1, `[{"$queryStats":{"transformIdentifiers":{"algorithm":"hmac-sha-256"}}}]`},
		"BadAlgorithm":           {"admin", 1, `[{"$queryStats":{"transformIdentifiers":{"algorithm":"bad"}}}]`},
		"TransformNotObject":     {"admin", 1, `[{"$queryStats":{"transformIdentifiers":5}}]`},
		"HmacWithKey":            {"admin", 1, `[{"$queryStats":{"transformIdentifiers":{"algorithm":"hmac-sha-256","hmacKey":{"$binary":{"base64":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","subType":"08"}}}}}]`},
		"AfterDocuments":         {"admin", 1, `[{"$documents":[]},{"$queryStats":{}}]`},
		"Twice":                  {"admin", 1, `[{"$queryStats":{}},{"$queryStats":{}}]`},
		"InFacet":                {"admin", 1, `[{"$facet":{"a":[{"$queryStats":{}}]}}]`},
		"OtherDatabase":          {"", 1, `[{"$queryStats":{}}]`},
		"Collection":             {"", "c", `[{"$queryStats":{}}]`},
		"CollectionUnknownField": {"", "c", `[{"$queryStats":{"zz":1}}]`},
		"InLookup":               {"", "c", `[{"$lookup":{"from":"c","pipeline":[{"$queryStats":{}}],"as":"j"}}]`},
		"ListCatalogSpec":        {"admin", 1, `[{"$listCatalog":{"zz":1}}]`},
		"ListCatalogThenUnknown": {"admin", 1, `[{"$listCatalog":{}},{"$bogus":{}}]`},
		"ListCatalogNotFirst":    {"admin", 1, `[{"$documents":[]},{"$listCatalog":{}}]`},
		"ListCatalogCollSpec":    {"", "c", `[{"$listCatalog":{"zz":1}}]`},
		"HmacShortKey":           {"admin", 1, `[{"$queryStats":{"transformIdentifiers":{"algorithm":"hmac-sha-256","hmacKey":{"$binary":{"base64":"AAAA","subType":"08"}}}}}]`},
		"HmacGeneralSubtype":     {"admin", 1, `[{"$queryStats":{"transformIdentifiers":{"algorithm":"hmac-sha-256","hmacKey":{"$binary":{"base64":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","subType":"00"}}}}}]`},
		"TransformNull":          {"admin", 1, `[{"$queryStats":{"transformIdentifiers":null}}]`},
		"SettingsNotObject":      {"admin", 1, `[{"$querySettings":5}]`},
		"SettingsUnknownField":   {"admin", 1, `[{"$querySettings":{"zz":1}}]`},
		"SettingsDebugNotBool":   {"admin", 1, `[{"$querySettings":{"showDebugQueryShape":1}}]`},
		"SettingsDebugBool":      {"admin", 1, `[{"$querySettings":{"showDebugQueryShape":true}}]`},
		"SettingsCollNotObject":  {"", "c", `[{"$querySettings":5}]`},
		"InhibitNotObject":       {"", "c", `[{"$_internalInhibitOptimization":5}]`},
		"InhibitNotEmpty":        {"", "c", `[{"$_internalInhibitOptimization":{"zz":1}}]`},
	}
	for name, c := range cases {
		c := c
		harness.PairTest(t, harness.TestCase{
			Name:    "QueryStatsStage_" + name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				var pipeline bson.A
				if err := bson.UnmarshalExtJSON([]byte(c.stage), false, &pipeline); err != nil {
					return nil, err
				}
				db := col.Database()
				if c.db != "" {
					db = col.Database().Client().Database(c.db)
				}
				var res bson.M
				err := db.RunCommand(ctx, bson.D{{Key: "aggregate", Value: c.coll}, {Key: "pipeline", Value: pipeline}, {Key: "cursor", Value: bson.D{}}}).Decode(&res)
				if err != nil {
					return nil, err
				}
				return res["cursor"].(bson.M)["firstBatch"], nil
			},
		})
	}
}

// Stages a standalone mongod refuses, or runs only on {aggregate: 1}, fail
// with mongod's codes rather than as unknown stages.
func TestStandaloneStageErrors(t *testing.T) {
	names := []string{
		"$querySettings", "$changeStreamSplitLargeEvent", "$listSampledQueries", "$setVariableFromSubPipeline",
		"$listCachedAndActiveUsers", "$_internalInhibitOptimization", "$listClusterCatalog",
	}
	for _, name := range names {
		for _, first := range []bool{true, false} {
			for _, onCollection := range []bool{true, false} {
				name, first, onCollection := name, first, onCollection
				if name == "$listClusterCatalog" && first && !onCollection {
					continue
				}
				label := "StandaloneStageErrors_" + strings.TrimPrefix(name, "$")
				if !first {
					label += "_AfterMatch"
				}
				if !onCollection {
					label += "_Database"
				}
				harness.PairTest(t, harness.TestCase{
					Name:    label,
					Support: harness.DumboDBFull,
					Setup: func(ctx context.Context, col *mongo.Collection) error {
						_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}})
						return err
					},
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						pipeline := bson.A{bson.D{{Key: name, Value: bson.D{}}}}
						if !first {
							pipeline = append(bson.A{bson.D{{Key: "$match", Value: bson.D{}}}}, pipeline...)
						}
						var target interface{} = col.Name()
						if !onCollection {
							target = 1
						}
						var res bson.M
						if err := col.Database().RunCommand(ctx, bson.D{{Key: "aggregate", Value: target}, {Key: "pipeline", Value: pipeline}, {Key: "cursor", Value: bson.D{}}}).Decode(&res); err != nil {
							return nil, err
						}
						return res["cursor"].(bson.M)["firstBatch"], nil
					},
				})
			}
		}
	}
}
