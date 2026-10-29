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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func insertBinarySizeDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(0)}, {Key: "v", Value: "café"}},
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: primitive.Binary{Subtype: 0, Data: []byte{1, 2, 3}}}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "v", Value: nil}},
		bson.D{{Key: "_id", Value: int32(3)}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "v", Value: int32(5)}},
	})
	return err
}

func TestBinarySize(t *testing.T) {
	binarySize := func(arg interface{}) bson.D { return bson.D{{Key: "$binarySize", Value: arg}} }
	uuid := primitive.Binary{Subtype: 4, Data: []byte("0123456789abcdef")}

	cases := []mathCase{
		{"ASCII", binarySize("abc")},
		{"Empty", binarySize("")},
		{"TwoByteUTF8", binarySize("café")},
		{"ThreeByteUTF8", binarySize("€")},
		{"FourByteUTF8", binarySize("\U0001F600")},
		{"EmbeddedNull", binarySize("a\x00b")},
		{"Long", binarySize(strings.Repeat("x", 100000))},
		{"BinaryGeneric", binarySize(primitive.Binary{Subtype: 0, Data: []byte{1, 2, 3}})},
		{"BinaryEmpty", binarySize(primitive.Binary{Subtype: 0, Data: []byte{}})},
		{"BinaryFunction", binarySize(primitive.Binary{Subtype: 1, Data: []byte{1, 2}})},
		{"BinaryOld", binarySize(primitive.Binary{Subtype: 2, Data: []byte{1, 2, 3, 4, 5}})},
		{"BinaryUUIDOld", binarySize(primitive.Binary{Subtype: 3, Data: []byte("0123456789abcdef")})},
		{"BinaryUUID", binarySize(uuid)},
		{"BinaryMD5", binarySize(primitive.Binary{Subtype: 5, Data: []byte("0123456789abcdef")})},
		{"BinaryUserDefined", binarySize(primitive.Binary{Subtype: 0x80, Data: []byte{9}})},
		{"BinaryLarge", binarySize(primitive.Binary{Data: make([]byte, 70000)})},
		{"ArrayForm", binarySize(bson.A{"abc"})},
		{"Field", binarySize("$v")},
		{"Expression", binarySize(bson.D{{Key: "$concat", Value: bson.A{"ab", "é"}}})},

		{"Null", binarySize(nil)},
		{"Missing", binarySize("$missing")},
		{"ArrayFormNull", binarySize(bson.A{nil})},
		{"Number", binarySize(int32(5))},
		{"Bool", binarySize(true)},
		{"Document", binarySize(bson.D{{Key: "a", Value: "b"}})},
		{"ArrayOfStrings", binarySize(bson.A{bson.A{"a"}})},
		{"ObjectID", binarySize(primitive.NewObjectID())},
		{"Regex", binarySize(primitive.Regex{Pattern: "a"})},
		{"Symbol", binarySize(primitive.Symbol("abc"))},
		{"JavaScript", binarySize(primitive.JavaScript("x"))},
		{"Date", binarySize(primitive.DateTime(0))},

		{"TooManyArguments", binarySize(bson.A{"a", "b"})},
		{"NoArguments", binarySize(bson.A{})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "BinarySize_" + tc.name,
				Support: harness.DumboDBXFail,
				Setup:   insertBinarySizeDocs,
				Run:     projectTyped(tc.expr),
			})
		})
	}

	pipelines := []struct {
		name     string
		pipeline bson.A
	}{
		{"AllDocsTypeError", bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: binarySize("$v")}}}},
		}},
		{"AllDocs", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$lt", Value: int32(4)}}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: binarySize("$v")}}}},
		}},
		{"MatchExpr", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$lt", Value: int32(4)}}}}}},
			bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$gt", Value: bson.A{binarySize("$v"), int32(3)}}}}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
		}},
		{"GroupSum", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$lt", Value: int32(4)}}}}}},
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "bytes", Value: bson.D{{Key: "$sum", Value: binarySize("$v")}}}}}},
		}},
	}
	for _, p := range pipelines {
		t.Run(p.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "BinarySize_" + p.name,
				Support: harness.DumboDBXFail,
				Setup:   insertBinarySizeDocs,
				Run:     pipelineRunExtJSON(p.pipeline),
			})
		})
	}
}
