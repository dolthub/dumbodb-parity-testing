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

// A field name with an embedded null byte cannot be encoded in BSON. The
// operators that build field names from strings refuse one, so nothing can
// store it, and $arrayToObject names the element types it finds.
func TestNulByteKeys(t *testing.T) {
	stages := map[string]string{
		"ArrayToObjectPair":      `[{"$replaceWith":{"$arrayToObject":[[["a\u0000b",1]]]}}]`,
		"ArrayToObjectKV":        `[{"$replaceWith":{"$arrayToObject":[[{"k":"a\u0000b","v":1}]]}}]`,
		"ArrayToObjectFromField": `[{"$set":{"o":{"$arrayToObject":"$pairs"}}}]`,
		"ArrayToObjectThenOut":   `[{"$replaceWith":{"$arrayToObject":[[["_id",7],["x\u0000y",1]]]}},{"$out":"nul_out"}]`,
		"ArrayToObjectThenMerge": `[{"$project":{"o":{"$arrayToObject":[[{"k":"a\u0000b","v":1}]]}}},{"$merge":"nul_merge"}]`,
		"SetField":               `[{"$replaceWith":{"$setField":{"field":"p\u0000q","input":"$$ROOT","value":1}}}]`,
		"UnsetField":             `[{"$replaceWith":{"$unsetField":{"field":"p\u0000q","input":"$$ROOT"}}}]`,
		"GetField":               `[{"$project":{"x":{"$getField":{"field":"p\u0000q","input":"$$ROOT"}}}}]`,
		"ScalarAfterPair":        `[{"$replaceWith":{"$arrayToObject":{"$literal":[["a",1],5]}}}]`,
		"ScalarAfterObject":      `[{"$replaceWith":{"$arrayToObject":{"$literal":[{"k":"a","v":1},5]}}}]`,
		"ObjectAfterPair":        `[{"$replaceWith":{"$arrayToObject":{"$literal":[["a",1],{"k":"a","v":1}]}}}]`,
		"ScalarFirst":            `[{"$replaceWith":{"$arrayToObject":{"$literal":[5]}}}]`,
	}
	for name, js := range stages {
		var pipeline []bson.D
		if err := bson.UnmarshalExtJSON([]byte(`{"p":`+js+`}`), false, &struct{ P *[]bson.D }{&pipeline}); err != nil {
			t.Fatal(name, err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "NulByteKeys_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "pairs", Value: bson.A{bson.A{"a\x00b", int32(1)}}}})
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				defer col.Database().Collection("nul_out").Drop(ctx)
				defer col.Database().Collection("nul_merge").Drop(ctx)
				return fieldOpsAgg(ctx, col, pipeline...)
			},
		})
	}
}

// $merge refuses a document whose 'on' field is missing, null or reached
// through an array (51132), and one whose 'on' field is an array (51185).
func TestMergeOnFieldValues(t *testing.T) {
	values := map[string]interface{}{
		"Missing": nil,
		"Null":    bson.D{{Key: "$literal", Value: nil}},
		"Array":   bson.D{{Key: "$literal", Value: bson.A{int32(5)}}},
		"Scalar":  int32(5),
	}
	for name, v := range values {
		for _, on := range []string{"k", "k.m"} {
			v, on, name := v, on, name
			set := bson.D{{Key: "k", Value: v}}
			if v == nil {
				set = bson.D{{Key: "k", Value: "$nothing"}}
			}
			if on == "k.m" {
				set = bson.D{{Key: "k", Value: bson.D{{Key: "$literal", Value: bson.A{bson.D{{Key: "m", Value: int32(1)}}}}}}}
				if name != "Missing" {
					continue
				}
				name = "ThroughArray"
			}
			harness.PairTest(t, harness.TestCase{
				Name:    "MergeOnFieldValues_" + name,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}}); err != nil {
						return err
					}
					_, err := col.Database().Collection(col.Name()+"_into").Indexes().CreateOne(ctx, mongo.IndexModel{
						Keys: bson.D{{Key: on, Value: 1}}, Options: options.Index().SetUnique(true),
					})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					into := col.Database().Collection(col.Name() + "_into")
					defer into.Drop(ctx)
					if _, err := fieldOpsAgg(ctx, col,
						bson.D{{Key: "$set", Value: set}},
						bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}}}},
						bson.D{{Key: "$merge", Value: bson.D{{Key: "into", Value: into.Name()}, {Key: "on", Value: on}, {Key: "whenNotMatched", Value: "insert"}}}},
					); err != nil {
						return nil, err
					}
					return findWithProjection(ctx, into, bson.D{}, bson.D{{Key: "_id", Value: 0}})
				},
			})
		}
	}
}

// Stage-level field names with a null byte are refused when the stage is
// parsed.
func TestNulByteStageFieldNames(t *testing.T) {
	cases := map[string]bson.D{
		"Count":            {{Key: "$count", Value: "p\x00q"}},
		"LookupAs":         {{Key: "$lookup", Value: bson.D{{Key: "from", Value: "x"}, {Key: "localField", Value: "a"}, {Key: "foreignField", Value: "a"}, {Key: "as", Value: "p\x00q"}}}},
		"UnwindArrayIndex": {{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$arr"}, {Key: "includeArrayIndex", Value: "i\x00j"}}}},
	}
	for name, stage := range cases {
		stage := stage
		harness.PairTest(t, harness.TestCase{
			Name:    "NulByteStageFieldNames_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "arr", Value: bson.A{int32(1)}}})
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, stage)
			},
		})
	}
}

// The $unwind path is a field path, never a variable, and includeArrayIndex
// names a field.
func TestUnwindPathChecks(t *testing.T) {
	cases := map[string]bson.D{
		"Variable":         {{Key: "$unwind", Value: "$$nope"}},
		"DollarAlone":      {{Key: "$unwind", Value: "$"}},
		"Root":             {{Key: "$unwind", Value: "$$ROOT"}},
		"CurrentPath":      {{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$$CURRENT.arr"}}}},
		"RootPath":         {{Key: "$unwind", Value: "$$ROOT.arr"}},
		"IndexDollar":      {{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$arr"}, {Key: "includeArrayIndex", Value: "$a"}}}},
		"IndexDollarPart":  {{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$arr"}, {Key: "includeArrayIndex", Value: "a.$b"}}}},
		"IndexEmptyPart":   {{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$arr"}, {Key: "includeArrayIndex", Value: "a..b"}}}},
		"IndexTrailingDot": {{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$arr"}, {Key: "includeArrayIndex", Value: "a."}}}},
		"IndexEmpty":       {{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$arr"}, {Key: "includeArrayIndex", Value: ""}}}},
		"IndexDotted":      {{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$arr"}, {Key: "includeArrayIndex", Value: "a.b"}}}},
	}
	for name, stage := range cases {
		stage := stage
		harness.PairTest(t, harness.TestCase{
			Name:    "UnwindPathChecks_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "arr", Value: bson.A{int32(1), int32(2)}}})
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, stage)
			},
		})
	}
}
