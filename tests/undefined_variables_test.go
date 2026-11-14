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

// A reference to an undefined variable fails with 17276 in every stage
// (dolthub/dumbodb#246).

func TestUndefinedVariables(t *testing.T) {
	seed := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: int32(1)}})
		return err
	}
	cases := []struct {
		name  string
		stage bson.D
	}{
		{"ProjectField", bson.D{{Key: "$project", Value: bson.D{{Key: "c", Value: "$$nope"}}}}},
		{"ProjectID", bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: "$$nope"}}}}},
		{"Set", bson.D{{Key: "$set", Value: bson.D{{Key: "c", Value: "$$nope"}}}}},
		{"SetPath", bson.D{{Key: "$set", Value: bson.D{{Key: "c", Value: "$$nope.x"}}}}},
		{"SetNested", bson.D{{Key: "$set", Value: bson.D{{Key: "c", Value: bson.D{{Key: "x", Value: "$$nope"}}}}}}},
		{"SetCapitalized", bson.D{{Key: "$set", Value: bson.D{{Key: "c", Value: "$$Nope"}}}}},
		{"SetEmptyName", bson.D{{Key: "$set", Value: bson.D{{Key: "c", Value: "$$"}}}}},
		{"SetSystemVariables", bson.D{{Key: "$set", Value: bson.D{{Key: "c", Value: "$$ROOT.a"}, {Key: "d", Value: "$$CURRENT"}, {Key: "e", Value: "$$REMOVE"}}}}},
		{"SetBoundByLet", bson.D{{Key: "$set", Value: bson.D{{Key: "c", Value: bson.D{{Key: "$let", Value: bson.D{{Key: "vars", Value: bson.D{{Key: "x", Value: int32(1)}}}, {Key: "in", Value: "$$x"}}}}}}}}},
		{"GroupID", bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$$nope"}}}}},
		{"GroupIDRoot", bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$$ROOT"}}}}},
		{"LookupUndefined", bson.D{{Key: "$lookup", Value: bson.D{{Key: "let", Value: bson.D{{Key: "v", Value: "$a"}}},
			{Key: "pipeline", Value: bson.A{bson.D{{Key: "$documents", Value: bson.A{bson.D{{Key: "q", Value: int32(1)}}}}}, bson.D{{Key: "$project", Value: bson.D{{Key: "z", Value: "$$v"}, {Key: "y", Value: "$$w"}}}}}}, {Key: "as", Value: "j"}}}}},
	}
	for _, c := range cases {
		stage := c.stage
		harness.PairTest(t, harness.TestCase{
			Name:    "UndefinedVariables_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   seed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, stage)
			},
		})
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "UndefinedVariables_FindProjection",
		Support: harness.DumboDBFull,
		Setup:   seed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findWithProjection(ctx, col, bson.D{}, bson.D{{Key: "c", Value: "$$nope"}})
		},
	})
}

// mongod checks the field paths and variables of every expression while it
// parses the pipeline, so the errors come even when no document is read.
// Variables are in scope only inside the expression that binds them.
func TestExpressionParseChecks(t *testing.T) {
	stages := map[string]string{
		"GroupKeyOperator":      `{"$group":{"_id":{"$add":["$$nope",1]}}}`,
		"GroupKeyField":         `{"$group":{"_id":{"k":{"$add":["$$nope",1]}}}}`,
		"GroupKeyFieldVariable": `{"$group":{"_id":{"k":"$$nope"}}}`,
		"GroupKeyEmptyPart":     `{"$group":{"_id":"$a..b"}}`,
		"GroupKeyDollarPart":    `{"$group":{"_id":"$a.$b"}}`,
		"GroupKeyPathInOp":      `{"$group":{"_id":{"$add":["$a..b",1]}}}`,
		"GroupKeyCond":          `{"$group":{"_id":{"$cond":[true,"$$nope",1]}}}`,
		"GroupAccumulator":      `{"$group":{"_id":null,"s":{"$sum":"$$nope"}}}`,
		"GroupAccumulatorPath":  `{"$group":{"_id":null,"s":{"$sum":"$a..b"}}}`,
		"LetOutOfScope":         `{"$group":{"_id":{"$let":{"vars":{"x":1},"in":"$$y"}}}}`,
		"LetInScope":            `{"$set":{"c":{"$let":{"vars":{"x":"$a"},"in":"$$x.x"}}}}`,
		"LetBadName":            `{"$set":{"c":{"$let":{"vars":{"Abc":1},"in":1}}}}`,
		"LetSystemName":         `{"$set":{"c":{"$let":{"vars":{"ROOT":1},"in":1}}}}`,
		"FilterDefaultThis":     `{"$set":{"c":{"$filter":{"input":"$arr","cond":{"$gt":["$$this",1]}}}}}`,
		"FilterUndefined":       `{"$group":{"_id":{"$filter":{"input":[1],"cond":"$$nope"}}}}`,
		"FilterAsBadName":       `{"$set":{"c":{"$map":{"input":[1],"as":"Q","in":1}}}}`,
		"MapAs":                 `{"$set":{"c":{"$map":{"input":"$arr","as":"z","in":{"$add":["$$z",1]}}}}}`,
		"ReduceVariables":       `{"$set":{"c":{"$reduce":{"input":"$arr","initialValue":0,"in":{"$add":["$$this","$$value"]}}}}}`,
		"ReduceValueOutside":    `{"$group":{"_id":{"$reduce":{"input":[1],"initialValue":"$$value","in":1}}}}`,
		"ReplaceRoot":           `{"$replaceRoot":{"newRoot":"$$nope"}}`,
		"ReplaceWithDollarPart": `{"$replaceWith":"$a.$b"}`,
		"ReplaceWithEmptyPart":  `{"$replaceWith":"$a..b"}`,
		"SortByCountVariable":   `{"$sortByCount":"$$nope"}`,
		"SortByCountPath":       `{"$sortByCount":"$a.$b"}`,
		"BucketGroupBy":         `{"$bucket":{"groupBy":"$$nope","boundaries":[0,1],"default":"x"}}`,
		"BucketAutoGroupBy":     `{"$bucketAuto":{"groupBy":"$$nope","buckets":2}}`,
		"SetKeep":               `{"$set":{"c":"$$KEEP"}}`,
		"SetKeepInOperator":     `{"$set":{"c":{"$add":["$$KEEP",1]}}}`,
		"SetSearchMeta":         `{"$set":{"c":"$$SEARCH_META"}}`,
		"ReadDollarName":        `{"$set":{"c":"$$$a"}}`,
		"ReadUnderscoreName":    `{"$set":{"c":"$$_x"}}`,
		"ReadDigitName":         `{"$group":{"_id":"$$1a"}}`,
		"ReadDashName":          `{"$set":{"c":{"$let":{"vars":{"a":1},"in":"$$a-b"}}}}`,
		"ReadDollarInName":      `{"$set":{"c":"$$a$b.c"}}`,
		"ReadUpperName":         `{"$set":{"c":"$$Aa"}}`,
		"SetIsMR":               `{"$set":{"c":"$$IS_MR"}}`,
		"SetClusterTime":        `{"$set":{"c":"$$CLUSTER_TIME"}}`,
		"SetUserRoles":          `{"$set":{"c":"$$USER_ROLES"}}`,
		"SetTrailingDot":        `{"$set":{"c":"$a."}}`,
		"SetVariablePath":       `{"$set":{"c":{"$add":["$$nope.x",1]}}}`,
		"ProjectCurrentEmpty":   `{"$project":{"c":"$$CURRENT..x"}}`,
		"ProjectRootDollar":     `{"$project":{"c":"$$ROOT.$x"}}`,
		"Redact":                `{"$redact":{"$cond":[true,"$$KEEP","$$PRUNE"]}}`,
		"RedactUndefined":       `{"$redact":"$$nope"}`,
		"MatchExprVariable":     `{"$match":{"$expr":"$$nope"}}`,
		"MatchExprPath":         `{"$match":{"$expr":{"$eq":["$a..b",1]}}}`,
		"PartitionBy":           `{"$setWindowFields":{"partitionBy":"$$nope","output":{"s":{"$sum":1}}}}`,
		"WindowOutput":          `{"$setWindowFields":{"output":{"s":{"$sum":"$$nope"}}}}`,
		"WindowOutputPath":      `{"$setWindowFields":{"output":{"s":{"$sum":"$a.$b"}}}}`,
		"UnwindEmptyPart":       `{"$unwind":"$a..b"}`,
		"LookupLetVariable":     `{"$lookup":{"from":"x","let":{"v":"$$nope"},"pipeline":[],"as":"j"}}`,
		"LookupLetPath":         `{"$lookup":{"from":"x","let":{"v":"$a..b"},"pipeline":[],"as":"j"}}`,
		"LookupLetBadName":      `{"$lookup":{"from":"x","let":{"V":1},"pipeline":[],"as":"j"}}`,
		"LookupSubPipeline":     `{"$lookup":{"from":"x","pipeline":[{"$set":{"q":"$$nope"}}],"as":"j"}}`,
		"LookupSubPipelineLet":  `{"$lookup":{"from":"x","let":{"v":1},"pipeline":[{"$match":{"$expr":{"$eq":["$$v.x","$$w"]}}}],"as":"j"}}`,
		"LookupSubPipelineOut":  `{"$lookup":{"from":"x","pipeline":[{"$out":"y"}],"as":"j"}}`,
		"LookupSubPipelineBad":  `{"$lookup":{"from":"x","pipeline":[{"$bogus":{}}],"as":"j"}}`,
		"LookupLetDocumentPath": `{"$lookup":{"from":"x","let":{"v":"$$ROOT"},"pipeline":[{"$documents":[{"q":1}]},{"$project":{"_id":0,"q":"$$v.a.x","r":{"$add":["$$v.a.x",1]}}}],"as":"j"}}`,
		"FacetSet":              `{"$facet":{"f":[{"$set":{"q":"$$nope"}}]}}`,
		"WindowOutputDollar":    `{"$setWindowFields":{"output":{"$y":{"$sum":1}}}}`,
		"WindowOutputEmpty":     `{"$setWindowFields":{"output":{"":{"$sum":1}}}}`,
		"WindowOutputConflict":  `{"$setWindowFields":{"output":{"y.$z":{"$sum":1},"y":{"$sum":1}}}}`,
		"LookupDocumentsPath":   `{"$lookup":{"pipeline":[{"$documents":[{"a":"$a..b"}]}],"as":"j"}}`,
		"LookupDocumentsArray":  `{"$lookup":{"pipeline":[{"$documents":"$a.$b"}],"as":"j"}}`,
		"MatchGetFieldNoInput":  `{"$match":{"$expr":{"$getField":{"field":"j"}}}}`,
		"MatchGetFieldUnknown":  `{"$match":{"$expr":{"$getField":{"field":"j","zz":1}}}}`,
		"MatchGetFieldNoField":  `{"$match":{"$expr":{"$eq":[{"$getField":{"input":{}}},1]}}}`,
		"SwitchNotObject":       `{"$set":{"c":{"$switch":5}}}`,
		"SwitchBranchesNotArr":  `{"$set":{"c":{"$switch":{"branches":5}}}}`,
		"SwitchBranchNotObject": `{"$set":{"c":{"$switch":{"branches":[5]}}}}`,
		"SwitchBranchUnknown":   `{"$set":{"c":{"$switch":{"branches":[{"case":true,"then":1,"zz":1}]}}}}`,
		"SwitchBranchNoCase":    `{"$set":{"c":{"$switch":{"branches":[{"then":1}],"zz":1}}}}`,
		"SwitchBranchNoThen":    `{"$set":{"c":{"$switch":{"branches":[{"case":true}]}}}}`,
		"SwitchUnknown":         `{"$set":{"c":{"$switch":{"zz":1,"branches":5}}}}`,
		"SwitchNoBranches":      `{"$set":{"c":{"$switch":{"branches":[],"default":1}}}}`,
		"SwitchNoBranchesExpr":  `{"$match":{"$expr":{"$switch":{"default":1}}}}`,
		"LetNotObject":          `{"$set":{"c":{"$let":5}}}`,
		"LetVarsNotObject":      `{"$set":{"c":{"$let":{"vars":5,"in":1}}}}`,
		"LetUnknown":            `{"$set":{"c":{"$let":{"zz":1,"vars":5}}}}`,
		"LetNoVars":             `{"$set":{"c":{"$let":{"in":1}}}}`,
		"LetNoIn":               `{"$set":{"c":{"$let":{"vars":{}}}}}`,
	}
	for name, js := range stages {
		var stage bson.D
		if err := bson.UnmarshalExtJSON([]byte(js), false, &stage); err != nil {
			t.Fatal(name, err)
		}
		for _, seeded := range []bool{false, true} {
			name := "ExpressionParseChecks_" + name
			if seeded {
				name += "_Seeded"
			}
			seeded := seeded
			harness.PairTest(t, harness.TestCase{
				Name:    name,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					if !seeded {
						return nil
					}
					_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.D{{Key: "x", Value: int32(1)}}}, {Key: "arr", Value: bson.A{int32(1), int32(2)}}})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return fieldOpsAgg(ctx, col, stage)
				},
			})
		}
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "ExpressionParseChecks_FindExpr",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findWithProjection(ctx, col, bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$$nope", int32(1)}}}}}, nil)
		},
	})
}
