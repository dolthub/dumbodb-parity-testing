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
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestWindowSpecChecks: an output specification needs exactly one
// $-prefixed window function mongod knows, and no other field than window;
// ranks, $shift, $derivative, $integral and $expMovingAvg, and bounded
// document and range windows, need the sortBy they rely on.
func TestWindowSpecChecks(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "x", Value: int32(1)}, {Key: "t", Value: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}})
		return err
	}
	outputs := map[string]bson.D{
		"empty":           {},
		"window_only":     {{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{int32(0), int32(0)}}}}},
		"plain_field":     {{Key: "x", Value: int32(1)}},
		"extra_before":    {{Key: "x", Value: int32(1)}, {Key: "$sum", Value: int32(1)}},
		"extra_after":     {{Key: "$sum", Value: int32(1)}, {Key: "x", Value: int32(1)}},
		"two_functions":   {{Key: "$sum", Value: int32(1)}, {Key: "$max", Value: int32(1)}},
		"unknown":         {{Key: "$bogus", Value: "$x"}},
		"merge_objects":   {{Key: "$mergeObjects", Value: "$x"}},
		"documents_bound": {{Key: "$sum", Value: int32(1)}, {Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "current"}}}}},
		"documents_all":   {{Key: "$sum", Value: int32(1)}, {Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "unbounded"}}}}},
		"range":           {{Key: "$sum", Value: int32(1)}, {Key: "window", Value: bson.D{{Key: "range", Value: bson.A{int32(0), int32(1)}}}}},
		"rank":            {{Key: "$rank", Value: bson.D{}}},
		"dense_rank":      {{Key: "$denseRank", Value: bson.D{}}},
		"document_number": {{Key: "$documentNumber", Value: bson.D{}}},
		"shift":           {{Key: "$shift", Value: bson.D{{Key: "output", Value: "$x"}, {Key: "by", Value: int32(1)}}}},
		"derivative":      {{Key: "$derivative", Value: bson.D{{Key: "input", Value: "$x"}}}},
		"integral":        {{Key: "$integral", Value: bson.D{{Key: "input", Value: "$x"}}}},
		"exp_moving_avg":  {{Key: "$expMovingAvg", Value: bson.D{{Key: "input", Value: "$x"}, {Key: "N", Value: int32(2)}}}},
	}
	sortBys := map[string]bson.D{"none": nil, "compound": {{Key: "_id", Value: 1}, {Key: "x", Value: 1}}}
	for name, output := range outputs {
		for sortName, sortBy := range sortBys {
			spec := bson.D{{Key: "output", Value: bson.D{{Key: "m", Value: output}}}}
			if sortBy != nil {
				spec = append(spec, bson.E{Key: "sortBy", Value: sortBy})
			}
			harness.PairTest(t, harness.TestCase{
				Name:    fmt.Sprintf("WindowSpecChecks_%s_%s", name, sortName),
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run:     pipelineRun(bson.A{bson.D{{Key: "$setWindowFields", Value: spec}}}),
			})
		}
	}
}
