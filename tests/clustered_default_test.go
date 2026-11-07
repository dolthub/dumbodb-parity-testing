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
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Every DumboDB collection is clustered by _id; a default MongoDB collection
// is not. These cases compare against a plain mongod collection and record
// where users see the difference.

func TestClusteredDefault_Deviates(t *testing.T) {
	ids := []interface{}{int32(3), int32(1), int32(2)}
	insertInOrder := func(ctx context.Context, col *mongo.Collection) error {
		for _, id := range ids {
			if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: id}}); err != nil {
				return err
			}
		}
		return nil
	}
	cases := []struct {
		name      string
		run       func(context.Context, *mongo.Collection) (interface{}, error)
		mongoWant interface{}
		dumboWant interface{}
	}{
		{
			name:      "natural_order",
			run:       cluFindIDs(bson.D{}),
			mongoWant: bson.A{int32(3), int32(1), int32(2)},
			dumboWant: bson.A{int32(1), int32(2), int32(3)},
		},
		{
			name:      "list_collections_options",
			run:       defaultCollectionOptions,
			mongoWant: bson.M{"idIndex": true, "options": bson.M{}},
			dumboWant: bson.M{"idIndex": false, "options": bson.M{"clusteredIndex": bson.M{
				"key": bson.M{"_id": int32(1)}, "name": "_id_", "unique": true, "v": int32(2),
			}}},
		},
		{
			name:      "list_indexes_id",
			run:       defaultIDIndexSpec,
			mongoWant: bson.M{"key": bson.M{"_id": int32(1)}, "name": "_id_", "v": int32(2)},
			dumboWant: bson.M{"key": bson.M{"_id": int32(1)}, "name": "_id_", "v": int32(2), "unique": true, "clustered": true},
		},
		{
			name:      "explain_id_equality",
			run:       defaultExplainStages(bson.D{{Key: "_id", Value: int32(2)}}),
			mongoWant: []string{"EXPRESS_IXSCAN"},
			dumboWant: []string{"EXPRESS_CLUSTERED_IXSCAN"},
		},
		{
			name:      "explain_id_range",
			run:       defaultExplainStages(bson.D{{Key: "_id", Value: bson.D{{Key: "$gt", Value: int32(1)}}}}),
			mongoWant: []string{"FETCH", "IXSCAN"},
			dumboWant: []string{"CLUSTERED_IXSCAN"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:        "ClusteredDefault_" + tc.name,
				Support:     harness.DumboDBDeviates,
				Setup:       insertInOrder,
				Run:         tc.run,
				MongoExpect: expectResult("mongo", tc.mongoWant),
				DumboExpect: expectResult("dumbodb", tc.dumboWant),
			})
		})
	}
}

func expectResult(server string, want interface{}) func(*testing.T, interface{}, error) {
	return func(t *testing.T, got interface{}, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", server, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %#v, want %#v", server, got, want)
		}
	}
}

// defaultCollectionOptions returns the collection's listCollections options
// and whether it reports an idIndex.
func defaultCollectionOptions(ctx context.Context, col *mongo.Collection) (interface{}, error) {
	cur, err := col.Database().ListCollections(ctx, bson.D{{Key: "name", Value: col.Name()}})
	if err != nil {
		return nil, err
	}
	var specs []bson.M
	if err := cur.All(ctx, &specs); err != nil {
		return nil, err
	}
	if len(specs) != 1 {
		return nil, fmt.Errorf("listCollections returned %d collections", len(specs))
	}
	_, hasIDIndex := specs[0]["idIndex"]
	return bson.M{"options": specs[0]["options"], "idIndex": hasIDIndex}, nil
}

// defaultIDIndexSpec returns the _id_ entry of listIndexes.
func defaultIDIndexSpec(ctx context.Context, col *mongo.Collection) (interface{}, error) {
	cur, err := col.Indexes().List(ctx)
	if err != nil {
		return nil, err
	}
	var specs []bson.M
	if err := cur.All(ctx, &specs); err != nil {
		return nil, err
	}
	for _, spec := range specs {
		if spec["name"] == "_id_" {
			return spec, nil
		}
	}
	return nil, fmt.Errorf("no _id_ index")
}

// defaultExplainStages returns the winning plan's stage names, outermost first.
func defaultExplainStages(filter bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		var res bson.M
		cmd := bson.D{{Key: "explain", Value: bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: filter}}}, {Key: "verbosity", Value: "queryPlanner"}}
		if err := col.Database().RunCommand(ctx, cmd).Decode(&res); err != nil {
			return nil, err
		}
		planner, _ := res["queryPlanner"].(bson.M)
		stage, _ := planner["winningPlan"].(bson.M)
		if query, ok := stage["queryPlan"].(bson.M); ok {
			stage = query
		}
		var stages []string
		for stage != nil {
			name, _ := stage["stage"].(string)
			stages = append(stages, name)
			stage, _ = stage["inputStage"].(bson.M)
		}
		return stages, nil
	}
}
