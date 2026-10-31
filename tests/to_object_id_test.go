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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func insertToObjectIDDocs(ctx context.Context, col *mongo.Collection) error {
	oid, _ := primitive.ObjectIDFromHex("5f1d7f6e9c8b4a3d2e1f0a9b")
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(0)}, {Key: "v", Value: "5f1d7f6e9c8b4a3d2e1f0a9b"}, {Key: "ref", Value: oid}},
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: oid}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "v", Value: nil}},
		bson.D{{Key: "_id", Value: int32(3)}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "v", Value: "not an id"}},
	})
	return err
}

func TestToObjectID(t *testing.T) {
	toObjectID := func(arg interface{}) bson.D { return bson.D{{Key: "$toObjectId", Value: arg}} }
	convert := func(input, to interface{}, extra ...bson.E) bson.D {
		return bson.D{{Key: "$convert", Value: append(bson.D{part("input", input), part("to", to)}, extra...)}}
	}
	hex := "5f1d7f6e9c8b4a3d2e1f0a9b"
	oid, _ := primitive.ObjectIDFromHex(hex)

	cases := []mathCase{
		{"Hex", toObjectID(hex)},
		{"UpperHex", toObjectID("5F1D7F6E9C8B4A3D2E1F0A9B")},
		{"Zeros", toObjectID("000000000000000000000000")},
		{"MaxValue", toObjectID("ffffffffffffffffffffffff")},
		{"ObjectID", toObjectID(oid)},
		{"ArrayForm", toObjectID(bson.A{hex})},
		{"Field", toObjectID("$v")},
		{"Expression", toObjectID(bson.D{{Key: "$concat", Value: bson.A{"5f1d7f6e9c8b", "4a3d2e1f0a9b"}}})},

		{"Null", toObjectID(nil)},
		{"Missing", toObjectID("$missing")},
		{"ArrayFormNull", toObjectID(bson.A{nil})},
		{"TooShort", toObjectID(hex[:23])},
		{"TooLong", toObjectID(hex + "0")},
		{"EmptyString", toObjectID("")},
		{"NonHex", toObjectID("zz1d7f6e9c8b4a3d2e1f0a9b")},
		{"Spaces", toObjectID(" 5f1d7f6e9c8b4a3d2e1f0a9")},
		{"Prefix", toObjectID("0x1d7f6e9c8b4a3d2e1f0a9b")},
		{"Unicode", toObjectID("5f1d7f6e9c8b4a3d2e1f0a\u00E9")},
		{"Int", toObjectID(int32(1))},
		{"Long", toObjectID(int64(1))},
		{"Double", toObjectID(1.5)},
		{"Decimal", toObjectID(mustDecimal("1"))},
		{"Bool", toObjectID(true)},
		{"Date", toObjectID(primitive.DateTime(0))},
		{"Timestamp", toObjectID(primitive.Timestamp{T: 1, I: 1})},
		{"Binary", toObjectID(primitive.Binary{Data: oid[:]})},
		{"Document", toObjectID(bson.D{{Key: "a", Value: hex}})},
		{"ArrayOfString", toObjectID(bson.A{bson.A{hex}})},
		{"Regex", toObjectID(primitive.Regex{Pattern: hex})},
		{"MinKey", toObjectID(primitive.MinKey{})},

		{"TooManyArguments", toObjectID(bson.A{hex, hex})},
		{"NoArguments", toObjectID(bson.A{})},

		{"ConvertString", convert(hex, "objectId")},
		{"ConvertTypeNumber", convert(hex, int32(7))},
		{"ConvertTypeDouble", convert(hex, 7.0)},
		{"ConvertObjectID", convert(oid, "objectId")},
		{"ConvertBadString", convert("nope", "objectId")},
		{"ConvertBadStringOnError", convert("nope", "objectId", part("onError", "bad"))},
		{"ConvertNumberOnError", convert(int32(1), "objectId", part("onError", nil))},
		{"ConvertNullOnNull", convert(nil, "objectId", part("onNull", "none"))},
		{"ConvertMissingOnNull", convert("$missing", "objectId", part("onNull", "none"))},
		{"ConvertNull", convert(nil, "objectId")},
		{"ConvertUnsupported", convert(true, "objectId")},
		{"ConvertFromObjectIDToString", convert(oid, "string")},
		{"ConvertFromObjectIDToBool", convert(oid, "bool")},
		{"ConvertFromObjectIDToDate", convert(oid, "date")},
		{"ConvertFromObjectIDToInt", convert(oid, "int")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ToObjectId_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   insertToObjectIDDocs,
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
			bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: toObjectID("$v")}}}},
		}},
		{"AllDocs", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$lt", Value: int32(4)}}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: toObjectID("$v")}}}},
		}},
		{"AllDocsOnError", bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: convert("$v", "objectId", part("onError", "bad"), part("onNull", "none"))}}}},
		}},
		{"MatchExprEquality", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$lt", Value: int32(4)}}}}}},
			bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{toObjectID("$v"), oid}}}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
		}},
		{"LookupByConvertedID", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: int32(0)}}}},
			bson.D{{Key: "$addFields", Value: bson.D{{Key: "oid", Value: toObjectID("$v")}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "same", Value: bson.D{{Key: "$eq", Value: bson.A{"$oid", "$ref"}}}}}}},
		}},
	}
	for _, p := range pipelines {
		t.Run(p.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ToObjectId_" + p.name,
				Support: harness.DumboDBFull,
				Setup:   insertToObjectIDDocs,
				Run:     pipelineRunExtJSON(p.pipeline),
			})
		})
	}
}
