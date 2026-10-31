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
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// maxTimeMS must stop a pipeline whose stages are slow per document, not
// only one that is slow to read (workspace-8tj.12). mongod 8.0.28 does not
// reliably interrupt $addFields or $group mid-stream, so only stages it
// does interrupt are compared.

func insertSlowRegexDocs(ctx context.Context, col *mongo.Collection) error {
	docs := make([]interface{}, 0, 30)
	for i := 0; i < 30; i++ {
		docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "s", Value: strings.Repeat("a", 30000)}})
	}
	_, err := col.InsertMany(ctx, docs)
	return err
}

func TestMaxTimeMS_SlowExpressionStages(t *testing.T) {
	matches := bson.D{{Key: "$size", Value: bson.D{{Key: "$regexFindAll", Value: bson.D{
		{Key: "input", Value: "$s"}, {Key: "regex", Value: "a"},
	}}}}}
	for _, tc := range []struct {
		name     string
		pipeline mongo.Pipeline
	}{
		{"Project", mongo.Pipeline{{{Key: "$project", Value: bson.D{{Key: "n", Value: matches}}}}}},
		{"MatchExpr", mongo.Pipeline{{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$lt", Value: bson.A{matches, int32(0)}}}}}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "MaxTimeMS_SlowExpressionStages_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   insertSlowRegexDocs,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					cur, err := col.Aggregate(ctx, tc.pipeline, options.Aggregate().SetMaxTime(300*time.Millisecond))
					if err != nil {
						return nil, err
					}
					var out []bson.D
					return len(out), cur.All(ctx, &out)
				},
			})
		})
	}
}

func TestMaxTimeMS_SlowFindProjection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "MaxTimeMS_SlowFindProjection",
		Support: harness.DumboDBFull,
		Setup:   insertSlowRegexDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			projection := bson.D{{Key: "n", Value: bson.D{{Key: "$size", Value: bson.D{{Key: "$regexFindAll", Value: bson.D{
				{Key: "input", Value: "$s"}, {Key: "regex", Value: "a"},
			}}}}}}}
			cur, err := col.Find(ctx, bson.D{}, options.Find().SetProjection(projection).SetMaxTime(300*time.Millisecond))
			if err != nil {
				return nil, err
			}
			var out []bson.D
			return len(out), cur.All(ctx, &out)
		},
	})
}
