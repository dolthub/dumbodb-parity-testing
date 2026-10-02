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

// $lookup / $graphLookup whose foreign collection is a view join the view's
// output (LKVIEW-01..04).

// lookupFromViewCase seeds orders, inventory, and emps next to |col|, creates
// view "instock" (inventory with qty > 0) and view "empsv" (emps), then runs
// |pipeline| against |source| and returns the aggregated documents.
func lookupFromViewCase(name, source string, pipeline mongo.Pipeline, extraViews func(ctx context.Context, db *mongo.Database) error) harness.TestCase {
	return harness.TestCase{
		Name:    name,
		Support: harness.DumboDBXFail,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			seed := map[string][]interface{}{
				"orders": {
					bson.D{{Key: "_id", Value: 1}, {Key: "item", Value: "apple"}},
					bson.D{{Key: "_id", Value: 2}, {Key: "item", Value: "pear"}},
				},
				"inventory": {
					bson.D{{Key: "_id", Value: 10}, {Key: "sku", Value: "apple"}, {Key: "qty", Value: 5}},
					bson.D{{Key: "_id", Value: 11}, {Key: "sku", Value: "pear"}, {Key: "qty", Value: 0}},
				},
				"emps": {
					bson.D{{Key: "_id", Value: "a"}, {Key: "boss", Value: nil}},
					bson.D{{Key: "_id", Value: "b"}, {Key: "boss", Value: "a"}},
					bson.D{{Key: "_id", Value: "c"}, {Key: "boss", Value: "b"}},
				},
			}
			for coll, docs := range seed {
				if _, err := db.Collection(coll).InsertMany(ctx, docs); err != nil {
					return nil, err
				}
			}
			if err := db.CreateView(ctx, "instock", "inventory", mongo.Pipeline{
				bson.D{{Key: "$match", Value: bson.D{{Key: "qty", Value: bson.D{{Key: "$gt", Value: 0}}}}}},
			}); err != nil {
				return nil, err
			}
			if err := db.CreateView(ctx, "empsv", "emps", mongo.Pipeline{}); err != nil {
				return nil, err
			}
			if extraViews != nil {
				if err := extraViews(ctx, db); err != nil {
					return nil, err
				}
			}

			cur, err := db.Collection(source).Aggregate(ctx, pipeline)
			if err != nil {
				code, _, _ := harness.CommandErrorCode(err)
				return bson.M{"code": code}, nil
			}
			var docs []bson.M
			if err := cur.All(ctx, &docs); err != nil {
				return nil, err
			}
			return docs, nil
		},
	}
}

var lookupInStock = bson.D{{Key: "$lookup", Value: bson.D{
	{Key: "from", Value: "instock"}, {Key: "localField", Value: "item"},
	{Key: "foreignField", Value: "sku"}, {Key: "as", Value: "stock"},
}}}

var countStockByID = mongo.Pipeline{
	bson.D{{Key: "$project", Value: bson.D{{Key: "n", Value: bson.D{{Key: "$size", Value: "$stock"}}}}}},
	bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
}

func TestLookupFromView(t *testing.T) {
	harness.PairTest(t, lookupFromViewCase("LKVIEW-01-aggregate-lookup-from-view", "orders",
		append(mongo.Pipeline{lookupInStock}, countStockByID...), nil))

	harness.PairTest(t, lookupFromViewCase("LKVIEW-02-view-lookup-from-view", "ordersv", countStockByID,
		func(ctx context.Context, db *mongo.Database) error {
			return db.CreateView(ctx, "ordersv", "orders", mongo.Pipeline{lookupInStock})
		}))

	harness.PairTest(t, lookupFromViewCase("LKVIEW-03-lookup-pipeline-form-from-view", "orders", append(mongo.Pipeline{
		bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "instock"},
			{Key: "let", Value: bson.D{{Key: "i", Value: "$item"}}},
			{Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$sku", "$$i"}}}}}}}}},
			{Key: "as", Value: "stock"},
		}}},
	}, countStockByID...), nil))

	harness.PairTest(t, lookupFromViewCase("LKVIEW-04-graphLookup-from-view", "emps", mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: "c"}}}},
		bson.D{{Key: "$graphLookup", Value: bson.D{
			{Key: "from", Value: "empsv"}, {Key: "startWith", Value: "$boss"},
			{Key: "connectFromField", Value: "boss"}, {Key: "connectToField", Value: "_id"}, {Key: "as", Value: "chain"},
		}}},
		bson.D{{Key: "$project", Value: bson.D{{Key: "n", Value: bson.D{{Key: "$size", Value: "$chain"}}}}}},
	}, nil))
}
