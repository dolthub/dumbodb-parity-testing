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

// mongod absorbs an $unwind of a $lookup's as field into the $lookup and
// streams the matches, so their total size is not limited. That holds in
// sub-pipelines, after a view's stages and across a $match that does not read
// as.

func lookupUnwindStage(col *mongo.Collection) bson.D {
	return lookupLimitStage(col, nil)
}

var (
	lookupUnwindJ     = bson.D{{Key: "$unwind", Value: "$j"}}
	lookupUnwindCount = bson.D{{Key: "$count", Value: "n"}}
)

func TestLookupUnwindStream_Scopes(t *testing.T) {
	cases := []struct {
		name string
		run  func(ctx context.Context, col *mongo.Collection) (interface{}, error)
	}{
		{"Facet", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col, bson.D{{Key: "$facet", Value: bson.D{{Key: "f", Value: bson.A{
				lookupUnwindStage(col), lookupUnwindJ, lookupUnwindCount,
			}}}}})
		}},
		{"MatchBetween", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col, lookupUnwindStage(col),
				bson.D{{Key: "$match", Value: bson.D{{Key: "k", Value: int32(1)}}}},
				lookupUnwindJ, lookupUnwindCount)
		}},
		{"MatchReadingAs", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col, lookupUnwindStage(col),
				bson.D{{Key: "$match", Value: bson.D{{Key: "j.k", Value: int32(1)}}}},
				lookupUnwindJ, lookupUnwindCount)
		}},
		{"ViewEndsWithLookup", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			view := col.Name() + "_v"
			if err := db.CreateView(ctx, view, col.Name(), mongo.Pipeline{lookupUnwindStage(col)}); err != nil {
				return nil, err
			}
			defer db.Collection(view).Drop(ctx)
			return fieldOpsAgg(ctx, db.Collection(view), lookupUnwindJ, lookupUnwindCount)
		}},
		{"ViewWithUnwind", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			view := col.Name() + "_v"
			if err := db.CreateView(ctx, view, col.Name(), mongo.Pipeline{lookupUnwindStage(col), lookupUnwindJ}); err != nil {
				return nil, err
			}
			defer db.Collection(view).Drop(ctx)
			return fieldOpsAgg(ctx, db.Collection(view), lookupUnwindCount)
		}},
		{"InsideLookupPipeline", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col,
				bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: int32(1)}}}},
				bson.D{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: col.Name()},
					{Key: "pipeline", Value: bson.A{lookupUnwindStage(col), lookupUnwindJ, lookupUnwindCount}},
					{Key: "as", Value: "outer"},
				}}},
			)
		}},
		{"InsideUnionWith", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col,
				bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: int32(-1)}}}},
				bson.D{{Key: "$unionWith", Value: bson.D{
					{Key: "coll", Value: col.Name()},
					{Key: "pipeline", Value: bson.A{lookupUnwindStage(col), lookupUnwindJ, lookupUnwindCount}},
				}}},
			)
		}},
	}
	for _, c := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "LookupUnwindStream_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   lookupLimitSeed,
			Run:     c.run,
		})
	}
}

// A sub-pipeline that fans one foreign document out to about 1GB of matches.
func TestLookupUnwindStream_FanOut(t *testing.T) {
	lookup := func(col *mongo.Collection) bson.D {
		return bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: col.Name() + "_f"},
			{Key: "pipeline", Value: bson.A{
				bson.D{{Key: "$unwind", Value: "$arr"}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "arr", Value: 1}, {Key: "pad", Value: 1}}}},
			}},
			{Key: "as", Value: "j"},
		}}}
	}
	cases := []struct {
		name  string
		after []bson.D
	}{
		{"Limit", []bson.D{lookupUnwindJ, {{Key: "$limit", Value: int32(1)}}, {{Key: "$project", Value: bson.D{{Key: "a", Value: "$j.arr"}}}}}},
		{"Count", []bson.D{lookupUnwindJ, lookupUnwindCount}},
		{"NotUnwound", []bson.D{{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}}}},
	}
	for _, c := range cases {
		after := c.after
		harness.PairTest(t, harness.TestCase{
			Name:    "LookupUnwindStream_FanOut" + c.name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}}); err != nil {
					return err
				}
				arr := make(bson.A, 2000)
				for i := range arr {
					arr[i] = int32(i)
				}
				pad := make(bson.A, 60000)
				for i := range pad {
					pad[i] = int32(i)
				}
				_, err := col.Database().Collection(col.Name()+"_f").InsertOne(ctx,
					bson.D{{Key: "_id", Value: int32(1)}, {Key: "arr", Value: arr}, {Key: "pad", Value: pad}})
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, append([]bson.D{lookup(col)}, after...)...)
			},
		})
	}
}

func lookupUnwindSmallSeed(ctx context.Context, col *mongo.Collection) error {
	if _, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "k", Value: int32(1)}, {Key: "x", Value: bson.A{int32(1), bson.D{{Key: "a", Value: int32(1)}}}}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "k", Value: int32(2)}, {Key: "x", Value: bson.D{{Key: "a", Value: int32(1)}}}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "k", Value: int32(3)}, {Key: "x", Value: int32(5)}},
	}); err != nil {
		return err
	}
	_, err := col.Database().Collection(col.Name()+"_f").InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(10)}, {Key: "k", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(11)}, {Key: "k", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(12)}, {Key: "k", Value: int32(2)}},
	})
	return err
}

func TestLookupUnwindStream_Output(t *testing.T) {
	lookupAs := func(col *mongo.Collection, as string) bson.D {
		return bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: col.Name() + "_f"},
			{Key: "localField", Value: "k"},
			{Key: "foreignField", Value: "k"},
			{Key: "pipeline", Value: bson.A{bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}}},
			{Key: "as", Value: as},
		}}}
	}
	sortOut := bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}, {Key: "i", Value: 1}}}}
	cases := []struct {
		name  string
		build func(col *mongo.Collection) []bson.D
	}{
		{"IndexAndPreserve", func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupAs(col, "j"), {{Key: "$unwind", Value: bson.D{
				{Key: "path", Value: "$j"}, {Key: "includeArrayIndex", Value: "i"}, {Key: "preserveNullAndEmptyArrays", Value: true},
			}}}, sortOut}
		}},
		{"DottedAs", func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupAs(col, "x.j"), sortOut}
		}},
		{"DottedAsUnwound", func(col *mongo.Collection) []bson.D {
			return []bson.D{lookupAs(col, "x.j.y"), {{Key: "$unwind", Value: bson.D{
				{Key: "path", Value: "$x.j.y"}, {Key: "includeArrayIndex", Value: "i"}, {Key: "preserveNullAndEmptyArrays", Value: true},
			}}}, sortOut}
		}},
		{"UnwindIndexOfNonArrays", func(col *mongo.Collection) []bson.D {
			return []bson.D{{{Key: "$unwind", Value: bson.D{
				{Key: "path", Value: "$x"}, {Key: "includeArrayIndex", Value: "i"}, {Key: "preserveNullAndEmptyArrays", Value: true},
			}}}, sortOut}
		}},
	}
	for _, c := range cases {
		build := c.build
		harness.PairTest(t, harness.TestCase{
			Name:    "LookupUnwindStream_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   lookupUnwindSmallSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, build(col)...)
			},
		})
	}
}
