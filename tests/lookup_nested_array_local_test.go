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

// An equality $lookup matches each outer element of an array localField as a
// whole value: an inner array is one value, not a set of candidates.

var lookupNestedForeign = []interface{}{
	bson.D{{Key: "_id", Value: "k1"}, {Key: "k", Value: int32(1)}},
	bson.D{{Key: "_id", Value: "k2"}, {Key: "k", Value: int32(2)}},
	bson.D{{Key: "_id", Value: "k3"}, {Key: "k", Value: int32(3)}},
	bson.D{{Key: "_id", Value: "k12"}, {Key: "k", Value: bson.A{int32(1), int32(2)}}},
	bson.D{{Key: "_id", Value: "kn12"}, {Key: "k", Value: bson.A{bson.A{int32(1), int32(2)}}}},
	bson.D{{Key: "_id", Value: "k4n5"}, {Key: "k", Value: bson.A{int32(4), bson.A{int32(5)}}}},
	bson.D{{Key: "_id", Value: "knull"}, {Key: "k", Value: nil}},
	bson.D{{Key: "_id", Value: "kmissing"}},
	bson.D{{Key: "_id", Value: "kempty"}, {Key: "k", Value: bson.A{}}},
	bson.D{{Key: "_id", Value: "kdoc"}, {Key: "k", Value: bson.D{{Key: "x", Value: int32(1)}}}},
}

type lookupNestedVariant struct {
	suffix       string
	subPipeline  bool
	foreignIndex bool
}

var lookupNestedVariants = []lookupNestedVariant{
	{suffix: "", subPipeline: true},
	{suffix: "_NoPipeline"},
	{suffix: "_ForeignIndexed", foreignIndex: true},
}

func lookupNestedRun(ctx context.Context, col *mongo.Collection, local interface{}, variant lookupNestedVariant) (interface{}, error) {
	foreign := col.Database().Collection(col.Name() + "_foreign")
	if _, err := foreign.InsertMany(ctx, lookupNestedForeign); err != nil {
		return nil, err
	}
	if variant.foreignIndex {
		if _, err := foreign.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "k", Value: 1}}}); err != nil {
			return nil, err
		}
	}
	doc := bson.D{{Key: "_id", Value: int32(1)}}
	if local != nil {
		doc = append(doc, bson.E{Key: "a", Value: local})
	}
	if _, err := col.InsertOne(ctx, doc); err != nil {
		return nil, err
	}
	spec := bson.D{
		{Key: "from", Value: foreign.Name()},
		{Key: "localField", Value: "a"},
		{Key: "foreignField", Value: "k"},
	}
	if variant.subPipeline {
		spec = append(spec, bson.E{Key: "pipeline", Value: bson.A{
			bson.D{{Key: "$project", Value: bson.D{{Key: "k", Value: 0}}}},
		}})
	}
	spec = append(spec, bson.E{Key: "as", Value: "j"})
	return fieldOpsAgg(ctx, col,
		bson.D{{Key: "$lookup", Value: spec}},
		bson.D{{Key: "$project", Value: bson.D{{Key: "ids", Value: bson.D{{Key: "$sortArray", Value: bson.D{
			{Key: "input", Value: "$j._id"},
			{Key: "sortBy", Value: int32(1)},
		}}}}}}},
	)
}

func TestLookupNestedArrayLocalField(t *testing.T) {
	cases := []struct {
		name  string
		local interface{}
	}{
		{"Lookup_NestedLocal_InnerArrayAndScalar", bson.A{bson.A{int32(1), int32(2)}, int32(3)}},
		{"Lookup_NestedLocal_OnlyInnerArray", bson.A{bson.A{int32(1), int32(2)}}},
		{"Lookup_NestedLocal_InnerSingleton", bson.A{bson.A{int32(5)}}},
		{"Lookup_NestedLocal_DeeplyNested", bson.A{bson.A{bson.A{int32(1)}}}},
		{"Lookup_NestedLocal_FlatArray", bson.A{int32(1), int32(4)}},
		{"Lookup_NestedLocal_EmptyArray", bson.A{}},
		{"Lookup_NestedLocal_ArrayWithNull", bson.A{nil, int32(3)}},
		{"Lookup_NestedLocal_ArrayWithEmpty", bson.A{bson.A{}}},
		{"Lookup_NestedLocal_ArrayWithDoc", bson.A{bson.D{{Key: "x", Value: int32(1)}}}},
		{"Lookup_NestedLocal_Scalar", int32(2)},
		{"Lookup_NestedLocal_Missing", nil},
	}
	for _, variant := range lookupNestedVariants {
		for _, c := range cases {
			local, variant := c.local, variant
			harness.PairTest(t, harness.TestCase{
				Name:    c.name + variant.suffix,
				Support: harness.DumboDBFull,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return lookupNestedRun(ctx, col, local, variant)
				},
			})
		}
	}
}
