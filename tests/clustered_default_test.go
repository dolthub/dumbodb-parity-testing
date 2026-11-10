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
	"go.mongodb.org/mongo-driver/mongo/options"

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
			name:      "show_record_id_type",
			run:       defaultRecordIDTypes(nil),
			mongoWant: []string{"long", "long", "long"},
			dumboWant: []string{"binData", "binData", "binData"},
		},
		{
			name:      "meta_record_id_type",
			run:       defaultRecordIDTypes(bson.D{{Key: "r", Value: bson.D{{Key: "$meta", Value: "recordId"}}}}),
			mongoWant: []string{"long", "long", "long"},
			dumboWant: []string{"binData", "binData", "binData"},
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

// defaultRecordIDTypes returns the BSON type of each document's RecordId, from
// showRecordId when projection is nil, else from the projection's r field.
func defaultRecordIDTypes(projection bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})
		field := "$recordId"
		if projection == nil {
			opts.SetShowRecordID(true)
		} else {
			opts.SetProjection(projection)
			field = "r"
		}
		cur, err := col.Find(ctx, bson.D{}, opts)
		if err != nil {
			return nil, err
		}
		var docs []bson.Raw
		if err := cur.All(ctx, &docs); err != nil {
			return nil, err
		}
		types := make([]string, len(docs))
		for i, d := range docs {
			v, err := d.LookupErr(field)
			if err != nil {
				types[i] = "missing"
				continue
			}
			switch v.Type {
			case bson.TypeInt64:
				types[i] = "long"
			case bson.TypeBinary:
				types[i] = "binData"
			default:
				types[i] = v.Type.String()
			}
		}
		return types, nil
	}
}

// TestExplain_MultiFieldSort: a sort on two fields walks the compound index
// they lead, in sort order, so a limit stops the walk.
func TestExplain_MultiFieldSort(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 10)
		for i := range docs {
			docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "a", Value: int32(i + 1)}, {Key: "b", Value: int32(10 - i)}}
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "a", Value: 1}}},
			{Keys: bson.D{{Key: "b", Value: 1}, {Key: "a", Value: 1}}, Options: options.Index().SetName("b_a")},
		})
		return err
	}
	run := func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		find := bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: int32(5)}}}}},
			{Key: "sort", Value: bson.D{{Key: "b", Value: 1}, {Key: "a", Value: 1}}}, {Key: "limit", Value: int32(2)}}
		var res bson.M
		if err := col.Database().RunCommand(ctx, bson.D{{Key: "explain", Value: find}, {Key: "verbosity", Value: "executionStats"}}).Decode(&res); err != nil {
			return nil, err
		}
		planner, _ := res["queryPlanner"].(bson.M)
		stage, _ := planner["winningPlan"].(bson.M)
		var stages []string
		for stage != nil {
			name, _ := stage["stage"].(string)
			if index, ok := stage["indexName"].(string); ok {
				name += " " + index
			}
			stages = append(stages, name)
			stage, _ = stage["inputStage"].(bson.M)
		}
		stats, _ := res["executionStats"].(bson.M)
		return bson.D{{Key: "stages", Value: stages}, {Key: "keys", Value: stats["totalKeysExamined"]}, {Key: "returned", Value: stats["nReturned"]}}, nil
	}
	harness.PairTest(t, harness.TestCase{
		Name:      "Explain_MultiFieldSort",
		Support:   harness.DumboDBFull,
		Clustered: true,
		Setup:     setup,
		Run:       run,
	})
}

// TestExplain_CompoundSortWalks: which compound sorts an index walk serves,
// and the documents each returns.
func TestExplain_CompoundSortWalks(t *testing.T) {
	setup := func(index bson.D) func(context.Context, *mongo.Collection) error {
		return func(ctx context.Context, col *mongo.Collection) error {
			docs := make([]interface{}, 30)
			for i := range docs {
				docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "g", Value: int32(i % 2)}, {Key: "a", Value: int32(i % 4)}, {Key: "b", Value: int32(i % 5)}}
			}
			if _, err := col.InsertMany(ctx, docs); err != nil {
				return err
			}
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: index, Options: options.Index().SetName("ix")})
			return err
		}
	}
	run := func(filter, sort bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			find := bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: filter}, {Key: "sort", Value: sort}, {Key: "limit", Value: int32(4)}}
			var res bson.M
			if err := col.Database().RunCommand(ctx, bson.D{{Key: "explain", Value: find}, {Key: "verbosity", Value: "queryPlanner"}}).Decode(&res); err != nil {
				return nil, err
			}
			planner, _ := res["queryPlanner"].(bson.M)
			stage, _ := planner["winningPlan"].(bson.M)
			var stages []string
			for stage != nil {
				name, _ := stage["stage"].(string)
				stages = append(stages, name)
				stage, _ = stage["inputStage"].(bson.M)
			}
			cur, err := col.Find(ctx, filter, options.Find().SetSort(sort).SetLimit(4))
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cur.All(ctx, &docs); err != nil {
				return nil, err
			}
			return bson.D{{Key: "stages", Value: stages}, {Key: "docs", Value: docs}}, nil
		}
	}
	asc := bson.D{{Key: "b", Value: 1}, {Key: "a", Value: 1}, {Key: "_id", Value: 1}}
	desc := bson.D{{Key: "b", Value: -1}, {Key: "a", Value: -1}, {Key: "_id", Value: -1}}
	mixed := bson.D{{Key: "b", Value: 1}, {Key: "a", Value: -1}, {Key: "_id", Value: 1}}
	cases := []struct {
		name    string
		index   bson.D
		filter  bson.D
		sort    bson.D
		support harness.DumboDBSupport
	}{
		{"forward", bson.D{{Key: "b", Value: 1}, {Key: "a", Value: 1}, {Key: "_id", Value: 1}}, bson.D{}, asc, harness.DumboDBFull},
		{"backward", bson.D{{Key: "b", Value: 1}, {Key: "a", Value: 1}, {Key: "_id", Value: 1}}, bson.D{}, desc, harness.DumboDBFull},
		{"equality_prefix", bson.D{{Key: "g", Value: 1}, {Key: "b", Value: 1}, {Key: "a", Value: 1}, {Key: "_id", Value: 1}}, bson.D{{Key: "g", Value: int32(1)}}, asc, harness.DumboDBFull},
		{"mixed_on_ascending_index", bson.D{{Key: "b", Value: 1}, {Key: "a", Value: 1}, {Key: "_id", Value: 1}}, bson.D{}, mixed, harness.DumboDBFull},
		// mongod walks an index declared with the sort's mixed directions.
		// DumboDB stores every key ascending, so no walk yields a mixed order;
		// it returns the same documents from an in-memory sort.
		{"mixed_on_matching_index", bson.D{{Key: "b", Value: 1}, {Key: "a", Value: -1}, {Key: "_id", Value: 1}}, bson.D{}, mixed, harness.DumboDBXFail},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Explain_CompoundSortWalks_" + tc.name,
				Support:   tc.support,
				Clustered: true,
				Setup:     setup(tc.index),
				Run:       run(tc.filter, tc.sort),
			})
		})
	}
}
