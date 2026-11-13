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

// TestDensifyMinStart: under full bounds, a partition that starts at the
// minimum passes it through and fills from its value, so a long minimum with
// a double step generates nothing below it.
func TestDensifyMinStart(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "p", Value: "a"}, {Key: "x", Value: int64(9007199254740993)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "p", Value: "a"}, {Key: "x", Value: int64(9007199254741001)}},
			bson.D{{Key: "_id", Value: int32(3)}, {Key: "p", Value: "b"}, {Key: "x", Value: int64(9007199254740993)}},
		})
		return err
	}
	for name, partition := range map[string]bool{"single": false, "partitions": true} {
		spec := bson.D{{Key: "field", Value: "x"}, {Key: "range", Value: bson.D{{Key: "step", Value: 4.0}, {Key: "bounds", Value: "full"}}}}
		if partition {
			spec = append(spec, bson.E{Key: "partitionByFields", Value: bson.A{"p"}})
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "DensifyMinStart_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: pipelineRunExtJSON(bson.A{bson.D{{Key: "$densify", Value: spec}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "p", Value: 1}, {Key: "x", Value: 1}}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "p", Value: 1}, {Key: "x", Value: 1}}}}}),
		})
	}
}
