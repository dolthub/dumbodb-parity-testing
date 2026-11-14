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

// TestSetEmptyObjectThenDottedPath: mongod treats y:{} beside "y.z" as an
// empty sub-spec, so an existing y keeps its fields and gains z, and each
// object element of an array y gains z while a scalar element becomes {z:2}.
func TestSetEmptyObjectThenDottedPath(t *testing.T) {
	seeds := map[string]string{
		"Object": `{"_id":1,"y":{"q":1}}`,
		"Array":  `{"_id":1,"y":[{"a":1},2]}`,
	}
	specs := map[string]string{
		"EmptyFirst":  `{"y":{},"y.z":2}`,
		"DottedFirst": `{"y.z":2,"y":{}}`,
	}
	for seedName, seedJS := range seeds {
		var seed bson.D
		if err := bson.UnmarshalExtJSON([]byte(seedJS), false, &seed); err != nil {
			t.Fatal(err)
		}
		setup := func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, seed)
			return err
		}
		for specName, specJS := range specs {
			var spec bson.D
			if err := bson.UnmarshalExtJSON([]byte(specJS), false, &spec); err != nil {
				t.Fatal(err)
			}
			for _, stage := range []string{"$set", "$addFields"} {
				harness.PairTest(t, harness.TestCase{
					Name:    "SetEmptyObjectThenDottedPath_" + seedName + "_" + specName + "_" + stage[1:],
					Support: harness.DumboDBFull,
					Setup:   setup,
					Run:     pipelineRun(bson.A{bson.D{{Key: stage, Value: spec}}}),
				})
			}
			harness.PairTest(t, harness.TestCase{
				Name:    "SetEmptyObjectThenDottedPath_" + seedName + "_" + specName + "_updatePipeline",
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					_, err := col.UpdateOne(ctx, bson.D{{Key: "_id", Value: int32(1)}}, mongo.Pipeline{{{Key: "$set", Value: spec}}})
					if err != nil {
						return errOutcome(err), nil
					}
					cursor, err := col.Find(ctx, bson.D{})
					return drainCursor(ctx, cursor, err)
				},
			})
		}
	}
}
