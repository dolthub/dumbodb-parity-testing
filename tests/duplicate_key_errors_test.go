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

// TestDuplicateKeyErrors: a repeated key gets the error of the place that
// reads it as a path or sort key: a write error 40 in an update modifier,
// 40176 in $set and $addFields at any depth or as a dotted path, 31250 in
// $project and find projections, 7472500 in $sort and find sorts.
func TestDuplicateKeyErrors(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "y", Value: int32(1)}})
		return err
	}
	dup := bson.D{{Key: "y", Value: int32(1)}, {Key: "y", Value: int32(2)}}
	nested := bson.D{{Key: "y", Value: bson.D{{Key: "q", Value: int32(1)}, {Key: "q", Value: int32(2)}}}}
	dotted := bson.D{{Key: "y.z", Value: int32(1)}, {Key: "y.z", Value: int32(2)}}
	sortDup := bson.D{{Key: "y", Value: int32(1)}, {Key: "y", Value: int32(-1)}}
	commands := map[string]func(name string) bson.D{
		"update_set": func(c string) bson.D {
			return bson.D{{Key: "update", Value: c}, {Key: "updates", Value: bson.A{bson.D{{Key: "q", Value: bson.D{{Key: "_id", Value: 1}}}, {Key: "u", Value: bson.D{{Key: "$set", Value: dup}}}}}}}
		},
		"update_inc": func(c string) bson.D {
			return bson.D{{Key: "update", Value: c}, {Key: "updates", Value: bson.A{bson.D{{Key: "q", Value: bson.D{{Key: "_id", Value: 1}}}, {Key: "u", Value: bson.D{{Key: "$inc", Value: dup}}}}}}}
		},
		"find_and_modify_set": func(c string) bson.D {
			return bson.D{{Key: "findAndModify", Value: c}, {Key: "query", Value: bson.D{{Key: "_id", Value: 1}}}, {Key: "update", Value: bson.D{{Key: "$set", Value: dup}}}}
		},
		"find_and_modify_pipeline": func(c string) bson.D {
			return bson.D{{Key: "findAndModify", Value: c}, {Key: "query", Value: bson.D{{Key: "_id", Value: 1}}}, {Key: "update", Value: bson.A{bson.D{{Key: "$set", Value: dup}}}}}
		},
		"find_sort": func(c string) bson.D {
			return bson.D{{Key: "find", Value: c}, {Key: "sort", Value: sortDup}}
		},
		"find_projection": func(c string) bson.D {
			return bson.D{{Key: "find", Value: c}, {Key: "projection", Value: bson.D{{Key: "y", Value: 1}, {Key: "y", Value: 0}}}}
		},
		"find_projection_nested": func(c string) bson.D {
			return bson.D{{Key: "find", Value: c}, {Key: "projection", Value: nested}}
		},
	}
	stages := map[string]bson.D{
		"set_nested":         {{Key: "$set", Value: nested}},
		"set_dotted":         {{Key: "$set", Value: dotted}},
		"add_fields_deep":    {{Key: "$addFields", Value: bson.D{{Key: "y", Value: bson.D{{Key: "q", Value: bson.D{{Key: "r", Value: 1}, {Key: "r", Value: 2}}}}}}}},
		"project_nested":     {{Key: "$project", Value: nested}},
		"project_dotted":     {{Key: "$project", Value: dotted}},
		"sort":               {{Key: "$sort", Value: sortDup}},
		"facet_set":          {{Key: "$facet", Value: bson.D{{Key: "a", Value: bson.A{bson.D{{Key: "$set", Value: dup}}}}}}},
		"lookup_project":     {{Key: "$lookup", Value: bson.D{{Key: "from", Value: "c"}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$project", Value: dup}}}}, {Key: "as", Value: "j"}}}},
		"window_output_same": {{Key: "$setWindowFields", Value: bson.D{{Key: "output", Value: bson.D{{Key: "y", Value: bson.D{{Key: "$sum", Value: 1}}}, {Key: "y", Value: bson.D{{Key: "$sum", Value: 2}}}}}}}},
	}
	for name, cmd := range commands {
		harness.PairTest(t, harness.TestCase{
			Name:    "DuplicateKeyErrors_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return runCommandDoc(ctx, col, cmd(col.Name()))
			},
		})
	}
	for name, stage := range stages {
		harness.PairTest(t, harness.TestCase{
			Name:    "DuplicateKeyErrors_stage_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Aggregate(ctx, bson.A{stage})
				return drainCursor(ctx, cursor, err)
			},
		})
	}
}
