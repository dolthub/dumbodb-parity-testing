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

// $collStats can start a $unionWith or $lookup sub-pipeline, and only start
// it (dolthub/dumbodb#239).

func TestCollStatsSubPipeline(t *testing.T) {
	seed := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{bson.D{{Key: "_id", Value: int32(1)}}, bson.D{{Key: "_id", Value: int32(2)}}})
		return err
	}
	count := bson.D{{Key: "$collStats", Value: bson.D{{Key: "count", Value: bson.D{}}}}}
	keep := bson.D{{Key: "$project", Value: bson.D{{Key: "count", Value: 1}, {Key: "ns", Value: 1}}}}
	cases := []struct {
		name   string
		stages func(col *mongo.Collection) []bson.D
	}{
		{"UnionWith", func(col *mongo.Collection) []bson.D {
			return []bson.D{{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: col.Name()}, {Key: "pipeline", Value: bson.A{count, keep}}}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}}
		}},
		{"Lookup", func(col *mongo.Collection) []bson.D {
			return []bson.D{{{Key: "$lookup", Value: bson.D{{Key: "from", Value: col.Name()}, {Key: "pipeline", Value: bson.A{count, keep}}, {Key: "as", Value: "j"}}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}}
		}},
		{"UnionWithNotFirst", func(col *mongo.Collection) []bson.D {
			return []bson.D{{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: col.Name()}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{}}}, count}}}}}}
		}},
		{"LookupNotFirst", func(col *mongo.Collection) []bson.D {
			return []bson.D{{{Key: "$lookup", Value: bson.D{{Key: "from", Value: col.Name()}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$limit", Value: int32(1)}}, count}}, {Key: "as", Value: "j"}}}}}
		}},
		{"MissingCollection", func(col *mongo.Collection) []bson.D {
			return []bson.D{{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: col.Name() + "_none"}, {Key: "pipeline", Value: bson.A{count}}}}}}
		}},
	}
	for _, c := range cases {
		build := c.stages
		harness.PairTest(t, harness.TestCase{
			Name:    "CollStatsSubPipeline_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   seed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, build(col)...)
			},
		})
	}
}
