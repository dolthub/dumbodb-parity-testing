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
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestMergeBatchUnordered: $merge writes a batch unordered, so a document
// that fails whenMatched: fail, whenNotMatched: fail or an _id change is
// skipped while the batch's other documents are written, and the first
// failure is reported.
func TestMergeBatchUnordered(t *testing.T) {
	cases := map[string]struct {
		spec   string
		target []string
	}{
		"NotMatchedFailMerge":    {`{"whenNotMatched":"fail"}`, []string{`{"_id":1}`, `{"_id":3}`}},
		"NotMatchedFailReplace":  {`{"whenMatched":"replace","whenNotMatched":"fail"}`, []string{`{"_id":1}`, `{"_id":3}`}},
		"NotMatchedFailPipeline": {`{"whenMatched":[{"$set":{"a":1}}],"whenNotMatched":"fail"}`, []string{`{"_id":1,"x":1}`, `{"_id":2,"x":2}`}},
		"MatchedFail":            {`{"whenMatched":"fail"}`, []string{`{"_id":2}`}},
	}
	for name, tc := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "MergeBatchUnordered_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertMany(ctx, []interface{}{
					bson.D{{Key: "_id", Value: int32(1)}, {Key: "x", Value: int32(10)}},
					bson.D{{Key: "_id", Value: int32(2)}, {Key: "x", Value: int32(20)}},
					bson.D{{Key: "_id", Value: int32(3)}, {Key: "x", Value: int32(30)}},
				})
				return err
			},
			Run: mergePipelineRun(tc.spec, tc.target),
		})
	}
}

// TestMergeBatchUniqueConflict: a $merge document whose insert or update
// would duplicate a unique key of the target is skipped, the batch's other
// documents are written, and the duplicate key error is reported.
func TestMergeBatchUniqueConflict(t *testing.T) {
	cases := map[string]struct {
		spec   string
		target []bson.D
	}{
		"Insert":         {`{}`, []bson.D{{{Key: "_id", Value: int32(10)}, {Key: "k", Value: int32(2)}}}},
		"InsertReplace":  {`{"whenMatched":"replace"}`, []bson.D{{{Key: "_id", Value: int32(10)}, {Key: "k", Value: int32(2)}}}},
		"UpdateMerge":    {`{"whenMatched":"merge"}`, nil},
		"UpdateReplace":  {`{"whenMatched":"replace"}`, nil},
		"UpdatePipeline": {`{"whenMatched":[{"$set":{"k":"$$new.k"}}]}`, nil},
	}
	updateTarget := []bson.D{
		{{Key: "_id", Value: int32(10)}, {Key: "k", Value: int32(2)}},
		{{Key: "_id", Value: int32(1)}, {Key: "k", Value: int32(9)}},
		{{Key: "_id", Value: int32(3)}, {Key: "k", Value: int32(8)}},
	}
	for name, tc := range cases {
		target := tc.target
		if target == nil {
			target = updateTarget
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "MergeBatchUniqueConflict_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertMany(ctx, []interface{}{
					bson.D{{Key: "_id", Value: int32(1)}, {Key: "k", Value: int32(1)}},
					bson.D{{Key: "_id", Value: int32(2)}, {Key: "k", Value: int32(2)}},
					bson.D{{Key: "_id", Value: int32(3)}, {Key: "k", Value: int32(3)}},
				})
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				var spec bson.D
				if err := bson.UnmarshalExtJSON([]byte(tc.spec), false, &spec); err != nil {
					return nil, err
				}
				tgt := col.Database().Collection(col.Name() + "_tgt")
				docs := make([]interface{}, len(target))
				for i, d := range target {
					docs[i] = d
				}
				if _, err := tgt.InsertMany(ctx, docs); err != nil {
					return nil, err
				}
				if _, err := tgt.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "k", Value: 1}}, Options: options.Index().SetUnique(true)}); err != nil {
					return nil, err
				}
				cursor, err := col.Aggregate(ctx, bson.A{
					bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
					bson.D{{Key: "$merge", Value: append(bson.D{{Key: "into", Value: tgt.Name()}}, spec...)}},
				})
				res, _ := drainCursor(ctx, cursor, err)
				found, err := tgt.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
				docsOut, _ := drainCursor(ctx, found, err)
				return bson.M{"result": res, "target": docsOut}, nil
			},
		})
	}
}
