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
	"sort"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Findings O12-O16 of the #184 security review (workspace-8tj.24 to
// workspace-8tj.28). O11's truncated UTF-8 for $substrCP crashes mongod
// 8.0.28, so it is covered by DumboDB unit tests instead.

func round6Setup(ctx context.Context, col *mongo.Collection) error {
	if _, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "s", Value: "$b"}, {Key: "b", Value: "B"}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "s", Value: "x"}},
	}); err != nil {
		return err
	}
	_, err := col.Database().Collection("secret").InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(9)}},
		bson.D{{Key: "_id", Value: int32(10)}, {Key: "v", Value: "$$v"}},
		bson.D{{Key: "_id", Value: int32(11)}, {Key: "v", Value: "$b"}},
	})
	return err
}

func round6Aggregate(pipeline bson.A) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cur, err := col.Aggregate(ctx, pipeline)
		if err != nil {
			return errOutcome(err), nil
		}
		// Maps print with sorted keys; foreign natural order differs, so the
		// joined arrays are compared as multisets too.
		var out []bson.M
		if err := cur.All(ctx, &out); err != nil {
			return errOutcome(err), nil
		}
		strs := make([]string, len(out))
		for i, d := range out {
			for k, v := range d {
				if arr, ok := v.(bson.A); ok {
					elems := make([]string, len(arr))
					for j, e := range arr {
						elems[j] = fmt.Sprint(e)
					}
					sort.Strings(elems)
					d[k] = elems
				}
			}
			strs[i] = fmt.Sprint(d)
		}
		sort.Strings(strs)
		return strs, nil
	}
}

func TestReviewRound6Aggregations(t *testing.T) {
	cases := map[string]bson.A{
		// O13
		"LookupEmptyFromPipeline": {bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: ""}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{}}}}}, {Key: "as", Value: "x"}}}}},
		"LookupEmptyFromLocal":    {bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: ""}, {Key: "localField", Value: "_id"}, {Key: "foreignField", Value: "_id"}, {Key: "as", Value: "x"}}}}},
		"LookupEmptyFromDocs":     {bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: ""}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$documents", Value: bson.A{}}}}}, {Key: "as", Value: "x"}}}}},
		// O15
		"LookupLetStringValue": {bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "let", Value: bson.D{{Key: "v", Value: "$s"}}},
			{Key: "pipeline", Value: bson.A{bson.D{{Key: "$documents", Value: bson.A{bson.D{{Key: "y", Value: "$$v"}, {Key: "b", Value: "other"}}}}}}},
			{Key: "as", Value: "j"},
		}}}},
		"LookupLetRoot": {bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "let", Value: bson.D{{Key: "r", Value: "$$ROOT"}}},
			{Key: "pipeline", Value: bson.A{bson.D{{Key: "$documents", Value: bson.A{"$$r"}}}}},
			{Key: "as", Value: "j"},
		}}}},
		"LookupLetInMatch": {bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "secret"},
			{Key: "let", Value: bson.D{{Key: "v", Value: "$s"}}},
			{Key: "pipeline", Value: bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "v", Value: "$$v"}}}}}},
			{Key: "as", Value: "j"},
		}}}},
		"LookupLetPlainMatch": {bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "secret"},
			{Key: "let", Value: bson.D{{Key: "v", Value: "$s"}}},
			{Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "v", Value: "$$v"}}}}}},
			{Key: "as", Value: "j"},
		}}}},
		"LookupLetMatchOrExpr": {bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "secret"},
			{Key: "let", Value: bson.D{{Key: "v", Value: "$s"}}},
			{Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "$or", Value: bson.A{
				bson.D{{Key: "v", Value: "$$v"}},
				bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$v", "$$v"}}}}},
			}}}}}}},
			{Key: "as", Value: "j"},
		}}}},
		// O16
		"UnionWithCollAndDocuments":   {bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: "secret"}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$documents", Value: bson.A{}}}}}}}}},
		"LookupFromAndDocuments":      {bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: "secret"}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$documents", Value: bson.A{}}}}}, {Key: "as", Value: "x"}}}}},
		"UnionWithEmptyCollDocuments": {bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: ""}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$documents", Value: bson.A{}}}}}}}}},
	}
	for name, p := range cases {
		pipeline := p
		t.Run(name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ReviewRound6_" + name,
				Support: harness.DumboDBFull,
				Setup:   round6Setup,
				Run:     round6Aggregate(pipeline),
			})
		})
	}
}

// O12: malformed paths in pipeline updates, through update, findAndModify and bulkWrite.
func TestReviewRound6PipelinePaths(t *testing.T) {
	pipelines := map[string]bson.A{
		"SetEmpty":      {bson.D{{Key: "$set", Value: bson.D{{Key: "", Value: int32(1)}}}}},
		"SetDoubleDot":  {bson.D{{Key: "$set", Value: bson.D{{Key: "a..b", Value: int32(1)}}}}},
		"UnsetEmpty":    {bson.D{{Key: "$unset", Value: ""}}},
		"UnsetTrailing": {bson.D{{Key: "$unset", Value: "a."}}},
		"EmptyStage":    {bson.D{}},
		"TwoKeyStage":   {bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: int32(1)}}}, {Key: "$unset", Value: "s"}}},
	}
	for name, p := range pipelines {
		pipeline := p
		for _, via := range []string{"Update", "FindAndModify", "BulkWrite"} {
			v := via
			n := "ReviewRound6_PipelinePath_" + v + "_" + name
			t.Run(v+"_"+name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    n,
					Support: harness.DumboDBXFail,
					Setup:   round6Setup,
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						var err error
						switch v {
						case "Update":
							_, err = col.UpdateOne(ctx, bson.D{{Key: "_id", Value: int32(1)}}, pipeline)
						case "FindAndModify":
							err = col.Database().RunCommand(ctx, bson.D{{Key: "findAndModify", Value: col.Name()}, {Key: "query", Value: bson.D{{Key: "_id", Value: int32(1)}}}, {Key: "update", Value: pipeline}}).Err()
						case "BulkWrite":
							err = col.Database().Client().Database("admin").RunCommand(ctx, bson.D{
								{Key: "bulkWrite", Value: int32(1)},
								{Key: "ops", Value: bson.A{bson.D{{Key: "update", Value: int32(0)}, {Key: "filter", Value: bson.D{{Key: "_id", Value: int32(1)}}}, {Key: "updateMods", Value: pipeline}}}},
								{Key: "nsInfo", Value: bson.A{bson.D{{Key: "ns", Value: col.Database().Name() + "." + col.Name()}}}},
							}).Err()
						}
						var docs []bson.M
						cur, ferr := col.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
						if ferr != nil {
							return nil, ferr
						}
						if ferr = cur.All(ctx, &docs); ferr != nil {
							return nil, ferr
						}
						return bson.M{"outcome": errOutcome(err), "after": docs}, nil
					},
				})
			})
		}
	}
}
