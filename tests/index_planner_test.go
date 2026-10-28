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
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Index-planner parity: whether a query reads its index at runtime, and how
// many documents it fetches, measured with serverStatus counters (scannedBy).
// Exact key counts are not compared: mongod examines one extra key per range
// it scans, which DumboDB does not count.

type plannerCase struct {
	name    string
	support harness.DumboDBSupport
	filter  bson.D
	opts    *options.FindOptions
}

// plannerWork runs the query once for its result size and again under
// scannedBy for the work it did.
func plannerWork(ctx context.Context, col *mongo.Collection, filter bson.D, opts *options.FindOptions) (interface{}, error) {
	var returned int
	op := func() error {
		cur, err := col.Find(ctx, filter, opts)
		if err != nil {
			return err
		}
		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			return err
		}
		returned = len(docs)
		return nil
	}
	scanned, err := scannedBy(ctx, col, op, true)
	if err != nil {
		return nil, err
	}
	keys, _ := scanned[0].Value.(int64)
	return bson.D{
		{Key: "returned", Value: returned},
		{Key: "docsExamined", Value: scanned[1].Value},
		{Key: "readsIndex", Value: keys > 0},
	}, nil
}

func runPlannerCases(t *testing.T, prefix string, setup func(context.Context, *mongo.Collection) error, cases []plannerCase) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    fmt.Sprintf("%s_%s", prefix, tc.name),
				Support: tc.support,
				Setup:   setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return plannerWork(ctx, col, tc.filter, tc.opts)
				},
			})
		})
	}
}

// TestIndexPlanner_CompoundIn: $in on the leading field of a compound index
// (dolthub/dumbodb#131). A limit without a sort still fetches every match
// until index ranges stream (#139, #140).
func TestIndexPlanner_CompoundIn(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 10000)
		for i := 0; i < 10000; i++ {
			docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "bucket", Value: int32(i % 50)}, {Key: "k", Value: int32(i)}})
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "bucket", Value: 1}, {Key: "k", Value: 1}}})
		return err
	}
	in := bson.D{{Key: "bucket", Value: bson.D{{Key: "$in", Value: bson.A{int32(1), int32(3)}}}}}
	sortK := bson.D{{Key: "k", Value: 1}}
	runPlannerCases(t, "IndexPlanner_CompoundIn", setup, []plannerCase{
		{"find", harness.DumboDBFull, in, nil},
		{"find_sort", harness.DumboDBFull, in, options.Find().SetSort(sortK)},
		{"find_limit", harness.DumboDBXFail, in, options.Find().SetLimit(5)},
		{"find_sort_limit", harness.DumboDBFull, in, options.Find().SetSort(sortK).SetLimit(5)},
	})
}

// TestIndexPlanner_BoundShapes: null equality (dolthub/dumbodb#133), $exists
// on a sparse index (#134) and an anchored regex prefix (#135) can each use an
// index. Null on a sparse index cannot: the index omits documents missing the
// field, which {field: null} matches.
func TestIndexPlanner_BoundShapes(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 10000)
		for i := 0; i < 10000; i++ {
			doc := bson.D{{Key: "_id", Value: int32(i)}, {Key: "s", Value: fmt.Sprintf("s%04d", i)}}
			if i%10 == 0 {
				doc = append(doc, bson.E{Key: "n", Value: nil}, bson.E{Key: "sp", Value: int32(i)})
			} else {
				doc = append(doc, bson.E{Key: "n", Value: int32(i)})
			}
			docs = append(docs, doc)
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "n", Value: 1}}},
			{Keys: bson.D{{Key: "sp", Value: 1}}, Options: options.Index().SetSparse(true)},
			{Keys: bson.D{{Key: "s", Value: 1}}},
		})
		return err
	}
	regex := bson.D{{Key: "s", Value: primitive.Regex{Pattern: "^s050"}}}
	runPlannerCases(t, "IndexPlanner_BoundShapes", setup, []plannerCase{
		{"null_eq", harness.DumboDBFull, bson.D{{Key: "n", Value: nil}}, nil},
		{"null_eq_operator", harness.DumboDBFull, bson.D{{Key: "n", Value: bson.D{{Key: "$eq", Value: nil}}}}, nil},
		{"null_on_sparse_index", harness.DumboDBFull, bson.D{{Key: "sp", Value: nil}}, nil},
		{"exists_on_sparse_index", harness.DumboDBFull, bson.D{{Key: "sp", Value: bson.D{{Key: "$exists", Value: true}}}}, nil},
		{"regex_prefix", harness.DumboDBFull, regex, nil},
		{"regex_prefix_operator", harness.DumboDBFull, bson.D{{Key: "s", Value: bson.D{{Key: "$regex", Value: "^s050"}}}}, nil},
	})
}
