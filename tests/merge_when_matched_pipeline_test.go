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
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// mergePipelineRun seeds the source collection and, unless target is nil, a
// target collection, runs [$sort by _id, $merge spec] with the target's name
// filled in as "into" where spec has none, and returns the outcome and the
// target's documents sorted by _id.
func mergePipelineRun(specJSON string, target []string) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		var spec bson.D
		if err := bson.UnmarshalExtJSON([]byte(specJSON), false, &spec); err != nil {
			return nil, err
		}
		tgt := col.Database().Collection(col.Name() + "_tgt")
		if err := tgt.Drop(ctx); err != nil {
			return nil, err
		}
		if target != nil {
			docs := make([]interface{}, 0, len(target))
			for _, js := range target {
				var d bson.D
				if err := bson.UnmarshalExtJSON([]byte(js), false, &d); err != nil {
					return nil, err
				}
				docs = append(docs, d)
			}
			if len(docs) == 0 {
				if err := col.Database().CreateCollection(ctx, tgt.Name()); err != nil {
					return nil, err
				}
			} else if _, err := tgt.InsertMany(ctx, docs); err != nil {
				return nil, err
			}
		}
		hasInto := false
		for _, e := range spec {
			hasInto = hasInto || e.Key == "into"
		}
		if !hasInto {
			spec = append(bson.D{{Key: "into", Value: tgt.Name()}}, spec...)
		}
		outcome := bson.M{"ok": true}
		cursor, err := col.Aggregate(ctx, bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$merge", Value: spec}},
		})
		if err == nil {
			err = cursor.All(ctx, &[]bson.M{})
		}
		if err != nil {
			outcome = errOutcome(err)
		}
		cur, err := tgt.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
		if err != nil {
			return nil, err
		}
		var docs []bson.M
		if err = cur.All(ctx, &docs); err != nil {
			return nil, err
		}
		return bson.M{"outcome": outcome, "target": docs}, nil
	}
}

// TestMergeWhenMatchedPipeline: a whenMatched pipeline updates each matched
// target document with $addFields, $set, $project, $unset, $replaceRoot or
// $replaceWith, reading the source document as $$new and the let variables,
// which are evaluated against the source document. The pipeline is parsed for
// each document that reaches $merge, so a stage an update refuses or an
// undefined variable fails only then, and the batch's other documents are
// still written.
func TestMergeWhenMatchedPipeline(t *testing.T) {
	target := []string{`{"_id":1,"x":1,"z":100}`, `{"_id":2,"x":2,"z":200}`}
	cases := map[string]struct {
		spec   string
		target []string
	}{
		"SetNew":              {`{"whenMatched":[{"$set":{"x":"$$new.x","w":{"$add":["$z","$$new.x"]}}}]}`, target},
		"AddFieldsLet":        {`{"let":{"nx":"$x","c":5},"whenMatched":[{"$addFields":{"nx":"$$nx","c":"$$c"}}]}`, target},
		"ProjectDiscard":      {`{"whenMatched":[{"$project":{"z":1}}],"whenNotMatched":"discard"}`, target},
		"ProjectDropID":       {`{"whenMatched":[{"$project":{"_id":0,"k":"$$new.y"}}]}`, target},
		"Unset":               {`{"whenMatched":[{"$unset":"z"}]}`, target},
		"UnsetArray":          {`{"whenMatched":[{"$unset":["z","x"]}]}`, target},
		"ReplaceRoot":         {`{"whenMatched":[{"$replaceRoot":{"newRoot":{"_id":"$_id","r":"$$new"}}}]}`, target},
		"ReplaceWith":         {`{"whenMatched":[{"$replaceWith":{"q":"$$new.y"}}]}`, target},
		"ReplaceWithNew":      {`{"whenMatched":[{"$replaceWith":"$$new"}]}`, target},
		"TwoStages":           {`{"whenMatched":[{"$set":{"s":{"$add":["$x","$$new.x"]}}},{"$unset":"x"}]}`, target},
		"Empty":               {`{"whenMatched":[]}`, target},
		"LetNewRoot":          {`{"let":{"new":"$$ROOT"},"whenMatched":[{"$set":{"m":"$$new"}}]}`, target},
		"LetNull":             {`{"let":null,"whenMatched":[{"$set":{"m":"$$new.x"}}]}`, target},
		"LetRootCurrent":      {`{"let":{"b":"$$ROOT.x","c":"$$CURRENT.y"},"whenMatched":[{"$set":{"b":"$$b","c":"$$c"}}]}`, target},
		"LetMissingValue":     {`{"let":{"v":"$y"},"whenMatched":[{"$set":{"v":"$$v"}}]}`, target},
		"NewMissingPath":      {`{"whenMatched":[{"$set":{"m":"$$new.nosuch","n":{"$type":"$$new.nosuch"}}}]}`, target},
		"LetShadowedNew":      {`{"whenMatched":[{"$set":{"a":{"$let":{"vars":{"new":1},"in":"$$new"}}}}]}`, target},
		"LiteralNew":          {`{"whenMatched":[{"$set":{"a":{"$literal":"$$new"}}}]}`, target},
		"Remove":              {`{"whenMatched":[{"$set":{"x":"$$REMOVE"}}]}`, target},
		"NotMatchedFailAll":   {`{"whenMatched":[{"$set":{"a":1}}],"whenNotMatched":"fail"}`, []string{`{"_id":1}`, `{"_id":2}`, `{"_id":3}`}},
		"NoTarget":            {`{"whenMatched":[{"$set":{"a":1}}]}`, nil},
		"NoTargetDiscard":     {`{"whenMatched":[{"$set":{"a":1}}],"whenNotMatched":"discard"}`, nil},
		"NoTargetFail":        {`{"whenMatched":[{"$set":{"a":1}}],"whenNotMatched":"fail"}`, nil},
		"EmptyTarget":         {`{"whenMatched":[{"$set":{"a":1}}]}`, []string{}},
		"ChangeID":            {`{"whenMatched":[{"$set":{"_id":5}}]}`, target},
		"KeepID":              {`{"whenMatched":[{"$set":{"_id":"$_id","s":"$$new.x"}}]}`, target},
		"ReplaceWithNumber":   {`{"whenMatched":[{"$replaceWith":5}]}`, target},
		"DivideByZero":        {`{"whenMatched":[{"$set":{"d":{"$divide":[1,0]}}}]}`, target},
		"DivideByString":      {`{"whenMatched":[{"$set":{"d":{"$divide":[1,"$$new.y"]}}}]}`, target},
		"LetDivideByZero":     {`{"let":{"q":{"$divide":[1,0]}},"whenMatched":[{"$set":{"d":1}}]}`, target},
		"LetClusterTime":      {`{"let":{"v":"$$CLUSTER_TIME"},"whenMatched":[{"$set":{"a":"$$v"}}]}`, target},
		"ClusterTime":         {`{"whenMatched":[{"$set":{"b":"$$CLUSTER_TIME"}}]}`, target},
		"MatchStage":          {`{"whenMatched":[{"$match":{}}]}`, target},
		"MatchStageDiscard":   {`{"whenMatched":[{"$match":{}}],"whenNotMatched":"discard"}`, nil},
		"SortStage":           {`{"whenMatched":[{"$sort":{"a":1}}]}`, target},
		"LookupStage":         {`{"whenMatched":[{"$lookup":{"from":"x","as":"j","pipeline":[]}}]}`, target},
		"UndefinedVar":        {`{"whenMatched":[{"$set":{"a":"$$undefinedVar"}}]}`, target},
		"UndefinedVarWithLet": {`{"let":{"v":1},"whenMatched":[{"$set":{"a":"$$nosuch"}}]}`, target},
		"BadExpression":       {`{"whenMatched":[{"$set":{"a":{"$bad":1}}}]}`, target},
		"DollarField":         {`{"whenMatched":[{"$set":{"$a":1}}]}`, target},
	}
	for name, tc := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "MergeWhenMatchedPipeline_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertMany(ctx, []interface{}{
					bson.D{{Key: "_id", Value: int32(1)}, {Key: "x", Value: int32(10)}, {Key: "y", Value: "a"}},
					bson.D{{Key: "_id", Value: int32(2)}, {Key: "x", Value: int32(20)}, {Key: "y", Value: "b"}},
					bson.D{{Key: "_id", Value: int32(3)}, {Key: "x", Value: int32(30)}},
				})
				return err
			},
			Run: mergePipelineRun(tc.spec, tc.target),
		})
	}
}

// TestMergeSpecErrors: $merge checks its specification fields in order with
// mongod's IDL codes, then the whenMatched pipeline's stage names and shapes,
// then its let variables: only with a whenMatched pipeline, new only as
// $$ROOT, each value parsed before its name is checked.
func TestMergeSpecErrors(t *testing.T) {
	cases := map[string]string{
		"Empty":                   `{}`,
		"MissingInto":             `{"on":"_id"}`,
		"IntoInt":                 `{"into":1}`,
		"IntoNull":                `{"into":null}`,
		"IntoEmpty":               `{"into":""}`,
		"IntoEmptyThenBogus":      `{"into":"","bogus":1}`,
		"IntoIntThenBogus":        `{"into":1,"bogus":1}`,
		"BogusThenIntoInt":        `{"bogus":1,"into":1}`,
		"IntoDocEmpty":            `{"into":{}}`,
		"IntoDocNoColl":           `{"into":{"db":"x"}}`,
		"IntoCollInt":             `{"into":{"coll":1}}`,
		"IntoCollEmpty":           `{"into":{"coll":""}}`,
		"IntoCollNull":            `{"into":{"coll":null}}`,
		"IntoDBInt":               `{"into":{"coll":"t","db":1}}`,
		"IntoUnknown":             `{"into":{"coll":"t","bogus":1}}`,
		"Unknown":                 `{"into":"t","bogus":1}`,
		"OnInt":                   `{"into":"t","on":1}`,
		"OnNull":                  `{"into":"t","on":null}`,
		"OnEmptyArray":            `{"into":"t","on":[]}`,
		"OnArrayInt":              `{"into":"t","on":[1]}`,
		"OnDuplicate":             `{"into":"t","on":["_id","_id"]}`,
		"OnDuplicateThenBadMode":  `{"into":"t","on":["a","a"],"whenNotMatched":1}`,
		"OnEmptyString":           `{"into":"t","on":""}`,
		"OnDollar":                `{"into":"t","on":"$a"}`,
		"OnDollarThenBadMatched":  `{"into":"t","on":"$a","whenMatched":1}`,
		"OnIntThenBadMatched":     `{"into":"t","on":1,"whenMatched":1}`,
		"BadMatchedThenOnInt":     `{"into":"t","whenMatched":1,"on":1}`,
		"MatchedInt":              `{"into":"t","whenMatched":1}`,
		"MatchedNull":             `{"into":"t","whenMatched":null}`,
		"MatchedBogus":            `{"into":"t","whenMatched":"bogus"}`,
		"MatchedPipelineName":     `{"into":"t","whenMatched":"pipeline"}`,
		"MatchedThenBogus":        `{"into":"t","whenMatched":1,"bogus":1}`,
		"NotMatchedInt":           `{"into":"t","whenNotMatched":1}`,
		"NotMatchedNull":          `{"into":"t","whenNotMatched":null}`,
		"NotMatchedBogus":         `{"into":"t","whenNotMatched":"bogus"}`,
		"BothBogusNotMatched":     `{"into":"t","whenNotMatched":"bogus","whenMatched":"bogus"}`,
		"BothBogusMatched":        `{"into":"t","whenMatched":"bogus","whenNotMatched":"bogus"}`,
		"PipelineNonObject":       `{"into":"t","whenMatched":[1]}`,
		"PipelineNonObjectFirst":  `{"into":"t","whenMatched":[1],"whenNotMatched":"bogus"}`,
		"PipelineTwoFields":       `{"into":"t","whenMatched":[{"$set":{"a":1},"$unset":"b"}]}`,
		"PipelineTwoFieldsLater":  `{"into":"t","whenMatched":[{"a":1,"b":1}],"whenNotMatched":"bogus"}`,
		"PipelineEmptyStage":      `{"into":"t","whenMatched":[{}]}`,
		"PipelineEmptyStageLater": `{"into":"t","whenMatched":[{}],"whenNotMatched":1}`,
		"PipelineUnknownStage":    `{"into":"t","whenMatched":[{"$bogus":1}]}`,
		"PipelineUnknownThenMode": `{"into":"t","whenMatched":[{"$bogus":1}],"whenNotMatched":"bogus"}`,
		"PipelineUnknownBadLet":   `{"into":"t","whenMatched":[{"$bogus":1}],"let":{"A":1}}`,
		"LetInt":                  `{"into":"t","whenMatched":[{"$set":{"a":1}}],"let":1}`,
		"LetIntBeforePipeline":    `{"into":"t","let":1,"whenMatched":[1]}`,
		"LetWithMerge":            `{"into":"t","whenMatched":"merge","let":{"a":1}}`,
		"LetWithDefault":          `{"into":"t","let":{"a":1}}`,
		"LetEmptyWithDefault":     `{"into":"t","let":{}}`,
		"LetWithReplace":          `{"into":"t","whenMatched":"replace","let":{"a":1}}`,
		"LetWithKeepExisting":     `{"into":"t","whenMatched":"keepExisting","let":{}}`,
		"LetWithFail":             `{"into":"t","whenMatched":"fail","let":{"a":1}}`,
		"LetBadWithMerge":         `{"into":"t","whenMatched":"merge","let":{"a":{"$bad":1}}}`,
		"LetBadModeThenLetInt":    `{"into":"t","whenMatched":"bogus","let":1}`,
		"LetUpperName":            `{"into":"t","whenMatched":[{"$set":{"a":1}}],"let":{"A":1}}`,
		"LetEmptyName":            `{"into":"t","whenMatched":[{"$set":{"a":1}}],"let":{"":1}}`,
		"LetRootName":             `{"into":"t","whenMatched":[{"$set":{"a":1}}],"let":{"ROOT":1}}`,
		"LetDottedName":           `{"into":"t","whenMatched":[{"$set":{"a":1}}],"let":{"a.b":1}}`,
		"LetNewInt":               `{"into":"t","whenMatched":[{"$set":{"a":1}}],"let":{"new":1}}`,
		"LetNewField":             `{"into":"t","whenMatched":[{"$set":{"a":1}}],"let":{"new":"$y"}}`,
		"LetNewCurrent":           `{"into":"t","whenMatched":[{"$set":{"a":1}}],"let":{"new":"$$CURRENT"}}`,
		"LetNewLiteral":           `{"into":"t","whenMatched":[{"$set":{"a":1}}],"let":{"new":{"$literal":"$$ROOT"}}}`,
		"LetNewAfterBadName":      `{"into":"t","whenMatched":[{"$set":{"a":1}}],"let":{"A":1,"new":1}}`,
		"LetBadExpression":        `{"into":"t","whenMatched":[{"$set":{"a":1}}],"let":{"a":{"$bad":1}}}`,
		"LetBadExprThenBadName":   `{"into":"t","whenMatched":[{"$set":{"a":1}}],"let":{"a":{"$bad":1},"A":1}}`,
		"LetBadNameThenBadExpr":   `{"into":"t","whenMatched":[{"$set":{"a":1}}],"let":{"A":1,"a":{"$bad":1}}}`,
		"LetUndefinedVar":         `{"into":"t","whenMatched":[{"$set":{"a":1}}],"let":{"b":"$$c"}}`,
		"LetReadsNew":             `{"into":"t","whenMatched":[{"$set":{"a":1}}],"let":{"new":"$$ROOT","b":"$$new"}}`,
		"LetArgCount":             `{"into":"t","whenMatched":[{"$set":{"a":1}}],"let":{"a":{"$substr":[1]}}}`,
		"KeepExistingDiscard":     `{"into":"t","whenMatched":"keepExisting","whenNotMatched":"discard"}`,
		"KeepExistingFail":        `{"into":"t","whenMatched":"keepExisting","whenNotMatched":"fail"}`,
		"FailDiscard":             `{"into":"t","whenMatched":"fail","whenNotMatched":"discard"}`,
		"FailFail":                `{"into":"t","whenMatched":"fail","whenNotMatched":"fail"}`,
		"FailFailBadOn":           `{"into":"t","whenMatched":"fail","whenNotMatched":"fail","on":"$a"}`,
		"FailFailLet":             `{"into":"t","whenMatched":"fail","whenNotMatched":"fail","let":{"a":1}}`,
		"ReplaceFail":             `{"into":"t","whenMatched":"replace","whenNotMatched":"fail"}`,
		"PipelineDiscard":         `{"into":"t","whenMatched":[],"whenNotMatched":"discard"}`,
		"LetBadExprThenBadMode":   `{"into":"t","whenMatched":[{"$set":{"a":1}}],"let":{"a":{"$bad":1}},"whenNotMatched":"bogus"}`,
	}
	for name, js := range cases {
		var spec bson.D
		if err := bson.UnmarshalExtJSON([]byte(js), false, &spec); err != nil {
			t.Fatal(name, err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "MergeSpecErrors_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}})
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Aggregate(ctx, bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: int32(99)}}}}, bson.D{{Key: "$merge", Value: spec}}})
				return drainCursor(ctx, cursor, err)
			},
		})
	}
}
