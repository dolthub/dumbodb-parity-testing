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
	"math"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// $convert and its $to* shorthands follow mongod's conversion table for every
// input type and target type (workspace-wpy).

type convertInput struct {
	name  string
	value interface{}
}

func convertInputs() []convertInput {
	oid, _ := primitive.ObjectIDFromHex("5ab9cbfa31c2ab715d42129e")
	uuid := primitive.Binary{Subtype: 4, Data: []byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0}}
	return []convertInput{
		{"Double", 2.75},
		{"DoubleNegative", -2.75},
		{"DoubleWhole", 42.0},
		{"DoubleNegZero", math.Copysign(0, -1)},
		{"DoubleNaN", math.NaN()},
		{"DoubleInf", math.Inf(1)},
		{"DoubleNegInf", math.Inf(-1)},
		{"DoubleBig", 1e300},
		{"DoubleSmall", 1.5e-7},
		{"DoubleIntMaxPlus", 2147483648.0},
		{"DoubleLongMax", 9223372036854775807.0},
		{"DoubleThird", 1.0 / 3},
		{"DoubleE21", 1e21},
		{"DoubleE15", 123456789012345.0},
		{"Int", int32(7)},
		{"IntNegative", int32(-7)},
		{"IntZero", int32(0)},
		{"Long", int64(9007199254740993)},
		{"LongSmall", int64(12)},
		{"LongMin", int64(math.MinInt64)},
		{"Decimal", mustDecimal("2.75")},
		{"DecimalTrailingZeros", mustDecimal("1.500")},
		{"DecimalNaN", mustDecimal("NaN")},
		{"DecimalInf", mustDecimal("-Infinity")},
		{"DecimalHuge", mustDecimal("1E+400")},
		{"DecimalTiny", mustDecimal("1E-400")},
		{"DecimalZero", mustDecimal("0E-10")},
		{"DecimalNegZero", mustDecimal("-0")},
		{"DecimalInt", mustDecimal("12345678901234567890123")},
		{"StringInt", "123"},
		{"StringNegative", "-45"},
		{"StringPlus", "+45"},
		{"StringSpaces", " 12"},
		{"StringTrailingSpace", "12 "},
		{"StringFraction", "1.5"},
		{"StringExponent", "1e3"},
		{"StringHex", "0x1A"},
		{"StringLeadingZero", "007"},
		{"StringWord", "abc"},
		{"StringEmpty", ""},
		{"StringInfinity", "Infinity"},
		{"StringNaN", "NaN"},
		{"StringIntOverflow", "2147483648"},
		{"StringLongOverflow", "9223372036854775808"},
		{"StringDecimal", "1.23E+5"},
		{"StringDate", "2021-03-04T05:06:07Z"},
		{"StringDateOnly", "2021-03-04"},
		{"StringObjectID", "5ab9cbfa31c2ab715d42129e"},
		{"StringFalse", "false"},
		{"BoolTrue", true},
		{"BoolFalse", false},
		{"Date", primitive.NewDateTimeFromTime(time.Date(2021, 3, 4, 5, 6, 7, 890e6, time.UTC))},
		{"DateBeforeEpoch", primitive.NewDateTimeFromTime(time.Date(1960, 1, 2, 3, 4, 5, 0, time.UTC))},
		{"ObjectID", oid},
		{"Timestamp", primitive.Timestamp{T: 1614834367, I: 3}},
		{"Object", bson.D{{Key: "a", Value: int32(1)}}},
		{"EmptyObject", bson.D{}},
		{"Array", bson.A{int32(1)}},
		{"EmptyArray", bson.A{}},
		{"BinData", primitive.Binary{Subtype: 0, Data: []byte("hi!")}},
		{"UUID", uuid},
		{"Regex", primitive.Regex{Pattern: "a", Options: "i"}},
		{"MinKey", primitive.MinKey{}},
		{"MaxKey", primitive.MaxKey{}},
		{"Null", nil},
		{"Missing", "$nope"},
	}
}

var convertTargets = []interface{}{
	"double", "string", "objectId", "bool", "date", "int", "long", "decimal",
	"binData", "object", "array", "null", "regex", "timestamp", "missing", "minKey",
}

// runConvertExpr evaluates expr against an empty document and returns field r.
func runConvertExpr(ctx context.Context, db *mongo.Database, expr interface{}) (interface{}, error) {
	cur, err := db.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$documents", Value: bson.A{bson.D{}}}},
		{{Key: "$project", Value: bson.D{{Key: "_id", Value: int32(0)}, {Key: "r", Value: expr}}}},
	})
	if err != nil {
		return nil, err
	}
	var out []bson.D
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return docsToSlice(out), nil
}

func convertInputExpr(in convertInput) interface{} {
	if in.name == "Missing" {
		return in.value
	}
	return bson.D{{Key: "$literal", Value: in.value}}
}

func TestConvertMatrix(t *testing.T) {
	for _, in := range convertInputs() {
		for _, to := range convertTargets {
			name := in.name + "_To_" + to.(string)
			t.Run(name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "ConvertMatrix_" + name,
					Support: harness.DumboDBFull,
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						return runConvertExpr(ctx, col.Database(), bson.D{{Key: "$convert", Value: bson.D{
							{Key: "input", Value: convertInputExpr(in)}, {Key: "to", Value: to},
						}}})
					},
				})
			})
		}
	}
}

func TestConvertShorthands(t *testing.T) {
	shorthands := []string{"$toDouble", "$toString", "$toObjectId", "$toBool", "$toDate", "$toInt", "$toLong", "$toDecimal", "$toUUID"}
	for _, in := range convertInputs() {
		for _, op := range shorthands {
			name := in.name + "_" + op[1:]
			t.Run(name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "ConvertShorthand_" + name,
					Support: harness.DumboDBFull,
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						return runConvertExpr(ctx, col.Database(), bson.D{{Key: op, Value: convertInputExpr(in)}})
					},
				})
			})
		}
	}
}

func TestConvertOptions(t *testing.T) {
	conv := func(fields ...bson.E) bson.D { return bson.D{{Key: "$convert", Value: bson.D(fields)}} }
	in := func(v interface{}) bson.E { return bson.E{Key: "input", Value: v} }
	to := func(v interface{}) bson.E { return bson.E{Key: "to", Value: v} }
	uuidString := "12345678-9abc-def0-1234-56789abcdef0"
	for _, tc := range []struct {
		name string
		expr interface{}
	}{
		{"OnErrorUsed", conv(in("abc"), to("int"), bson.E{Key: "onError", Value: "bad"})},
		{"OnErrorNotUsed", conv(in("12"), to("int"), bson.E{Key: "onError", Value: "bad"})},
		{"OnErrorMissing", conv(in("abc"), to("int"), bson.E{Key: "onError", Value: "$nope"})},
		{"OnErrorUnsupported", conv(in(bson.D{{Key: "$literal", Value: bson.D{}}}), to("int"), bson.E{Key: "onError", Value: "bad"})},
		{"OnNullUsed", conv(in(nil), to("int"), bson.E{Key: "onNull", Value: "was null"})},
		{"OnNullMissing", conv(in("$nope"), to("int"), bson.E{Key: "onNull", Value: "was missing"})},
		{"OnNullNotUsed", conv(in("5"), to("int"), bson.E{Key: "onNull", Value: "was null"})},
		{"ToNull", conv(in("5"), to(nil))},
		{"ToMissingField", conv(in("5"), to("$nope"))},
		{"ToFromField", conv(in("5"), to(bson.D{{Key: "$literal", Value: "long"}}))},
		{"ToNumericCode", conv(in("5"), to(int32(18)))},
		{"ToNumericDouble", conv(in("5"), to(16.0))},
		{"ToNumericFraction", conv(in("5"), to(16.5))},
		{"ToNumericInvalid", conv(in("5"), to(int32(100)))},
		{"ToUnknownName", conv(in("5"), to("integer"))},
		{"ToBoolType", conv(in("5"), to(true))},
		{"ToDocumentLong", conv(in("5"), to(bson.D{{Key: "type", Value: "long"}}))},
		{"ToBinDataSubtype", conv(in(uuidString), to(bson.D{{Key: "type", Value: "binData"}, {Key: "subtype", Value: int32(4)}}), bson.E{Key: "format", Value: "uuid"})},
		{"ToBinDataBase64", conv(in("aGkh"), to("binData"), bson.E{Key: "format", Value: "base64"})},
		{"ToBinDataHex", conv(in("68692a"), to("binData"), bson.E{Key: "format", Value: "hex"})},
		{"ToBinDataUtf8", conv(in("hi!"), to("binData"), bson.E{Key: "format", Value: "utf8"})},
		{"ToBinDataNoFormat", conv(in("aGkh"), to("binData"))},
		{"BinDataToStringHex", conv(in(bson.D{{Key: "$literal", Value: primitive.Binary{Data: []byte("hi!")}}}), to("string"), bson.E{Key: "format", Value: "hex"})},
		{"BinDataToStringBadFormat", conv(in(bson.D{{Key: "$literal", Value: primitive.Binary{Data: []byte("hi!")}}}), to("string"), bson.E{Key: "format", Value: "rot13"})},
		{"MissingInput", conv(to("int"))},
		{"MissingTo", conv(in("5"))},
		{"UnknownArgument", conv(in("5"), to("int"), bson.E{Key: "bogus", Value: int32(1)})},
		{"NotObject", bson.D{{Key: "$convert", Value: "5"}}},
		{"ShorthandArray", bson.D{{Key: "$toInt", Value: bson.A{"5"}}}},
		{"ShorthandTwoArgs", bson.D{{Key: "$toInt", Value: bson.A{"5", "6"}}}},
		{"ShorthandNoArgs", bson.D{{Key: "$toInt", Value: bson.A{}}}},
		{"NestedConvert", bson.D{{Key: "$toLong", Value: bson.D{{Key: "$toDate", Value: "2021-03-04"}}}}},
		{"ConstantErrorInUntakenBranch", bson.D{{Key: "$cond", Value: bson.A{false, bson.D{{Key: "$toInt", Value: "abc"}}, int32(1)}}}},
		{"ToBinDataBase64Url", conv(in("aGk-_w"), to("binData"), bson.E{Key: "format", Value: "base64url"})},
		{"ToBinDataBase64UrlPadded", conv(in("aGk-_w=="), to("binData"), bson.E{Key: "format", Value: "base64url"})},
		{"ToBinDataBadBase64", conv(in("a$b"), to("binData"), bson.E{Key: "format", Value: "base64"})},
		{"ToBinDataBadBase64OnError", conv(in("a$b"), to("binData"), bson.E{Key: "format", Value: "base64"}, bson.E{Key: "onError", Value: "bad"})},
		{"ToBinDataUnpaddedBase64", conv(in("aGk"), to("binData"), bson.E{Key: "format", Value: "base64"})},
		{"ToBinDataOddHex", conv(in("abc"), to("binData"), bson.E{Key: "format", Value: "hex"})},
		{"ToBinDataUpperHex", conv(in("ABCDEF"), to("binData"), bson.E{Key: "format", Value: "hex"})},
		{"ToBinDataUUIDWrongSubtype", conv(in(uuidString), to("binData"), bson.E{Key: "format", Value: "uuid"})},
		{"ToBinDataAutoFormat", conv(in("aGkh"), to("binData"), bson.E{Key: "format", Value: "auto"})},
		{"ToBinDataBadUUID", conv(in("not-a-uuid"), to(bson.D{{Key: "type", Value: "binData"}, {Key: "subtype", Value: int32(4)}}), bson.E{Key: "format", Value: "uuid"})},
		{"ToBinDataSubtypeColumn", conv(in("aGkh"), to(bson.D{{Key: "type", Value: "binData"}, {Key: "subtype", Value: int32(7)}}), bson.E{Key: "format", Value: "base64"})},
		{"ToBinDataSubtypeUser", conv(in("aGkh"), to(bson.D{{Key: "type", Value: "binData"}, {Key: "subtype", Value: int32(200)}}), bson.E{Key: "format", Value: "base64"})},
		{"ToBinDataSubtypeInvalid", conv(in("aGkh"), to(bson.D{{Key: "type", Value: "binData"}, {Key: "subtype", Value: int32(50)}}), bson.E{Key: "format", Value: "base64"})},
		{"BinDataSubtypeFraction", conv(in(bson.D{{Key: "$literal", Value: primitive.Binary{Data: []byte("hi!")}}}), to(bson.D{{Key: "type", Value: "binData"}, {Key: "subtype", Value: 1.5}}))},
		{"BinDataSubtypeString", conv(in(bson.D{{Key: "$literal", Value: primitive.Binary{Data: []byte("hi!")}}}), to(bson.D{{Key: "type", Value: "binData"}, {Key: "subtype", Value: "x"}}))},
		{"BinDataSubtypeColumn", conv(in(bson.D{{Key: "$literal", Value: primitive.Binary{Data: []byte("hi!")}}}), to(bson.D{{Key: "type", Value: "binData"}, {Key: "subtype", Value: int32(7)}}))},
		{"ToDocumentNoType", conv(in("5"), to(bson.D{{Key: "subtype", Value: int32(0)}}))},
		{"BinDataToStringBase64Url", conv(in(bson.D{{Key: "$literal", Value: primitive.Binary{Data: []byte{0xfb, 0xff, 0xfe}}}}), to("string"), bson.E{Key: "format", Value: "base64url"})},
		{"BinDataToStringBase64UrlShort", conv(in(bson.D{{Key: "$literal", Value: primitive.Binary{Data: []byte{0xfb}}}}), to("string"), bson.E{Key: "format", Value: "base64url"})},
		{"BinDataToStringBadUtf8", conv(in(bson.D{{Key: "$literal", Value: primitive.Binary{Data: []byte{0xff}}}}), to("string"), bson.E{Key: "format", Value: "utf8"})},
		{"BinDataToStringNotUUID", conv(in(bson.D{{Key: "$literal", Value: primitive.Binary{Data: []byte("hi!")}}}), to("string"), bson.E{Key: "format", Value: "uuid"})},
		{"FormatNotString", conv(in("5"), to("int"), bson.E{Key: "format", Value: int32(1)})},
		{"FormatNull", conv(in("5"), to("int"), bson.E{Key: "format", Value: nil})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ConvertOptions_" + tc.name,
				Support: harness.DumboDBFull,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return runConvertExpr(ctx, col.Database(), tc.expr)
				},
			})
		})
	}
}

// Conversions evaluated per document, so they are not folded while parsing.
func TestConvertFieldInputs(t *testing.T) {
	docs := bson.A{
		bson.D{{Key: "v", Value: "12"}},
		bson.D{{Key: "v", Value: "abc"}},
		bson.D{{Key: "v", Value: nil}},
		bson.D{},
		bson.D{{Key: "v", Value: 2.5}},
		bson.D{{Key: "v", Value: bson.D{{Key: "a", Value: int32(1)}}}},
		bson.D{{Key: "v", Value: "$v"}},
	}
	for _, tc := range []struct {
		name string
		expr interface{}
	}{
		{"ToIntOnError", bson.D{{Key: "$convert", Value: bson.D{{Key: "input", Value: "$v"}, {Key: "to", Value: "int"}, {Key: "onError", Value: "bad"}, {Key: "onNull", Value: "none"}}}}},
		{"ToStringOnErrorMissing", bson.D{{Key: "$convert", Value: bson.D{{Key: "input", Value: "$v"}, {Key: "to", Value: "string"}, {Key: "onError", Value: "$nope"}}}}},
		{"ToBool", bson.D{{Key: "$toBool", Value: "$v"}}},
		{"ToString", bson.D{{Key: "$toString", Value: "$v"}}},
		{"TypeFromField", bson.D{{Key: "$convert", Value: bson.D{{Key: "input", Value: int32(1)}, {Key: "to", Value: bson.D{{Key: "$cond", Value: bson.A{bson.D{{Key: "$eq", Value: bson.A{"$v", "12"}}}, "string", "double"}}}}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ConvertFieldInputs_" + tc.name,
				Support: harness.DumboDBFull,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					cur, err := col.Database().Aggregate(ctx, mongo.Pipeline{
						{{Key: "$documents", Value: docs}},
						{{Key: "$project", Value: bson.D{{Key: "_id", Value: int32(0)}, {Key: "r", Value: tc.expr}}}},
					})
					if err != nil {
						return nil, err
					}
					var out []bson.D
					return docsToSlice(out), cur.All(ctx, &out)
				},
			})
		})
	}
}
