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

func lookupSeedJSON(ctx context.Context, col *mongo.Collection, local, foreign []string) error {
	insert := func(c *mongo.Collection, docs []string) error {
		var parsed []interface{}
		for _, js := range docs {
			var d bson.D
			if err := bson.UnmarshalExtJSON([]byte(js), false, &d); err != nil {
				return err
			}
			parsed = append(parsed, d)
		}
		_, err := c.InsertMany(ctx, parsed)
		return err
	}
	if err := insert(col, local); err != nil {
		return err
	}
	return insert(col.Database().Collection(col.Name()+"_f"), foreign)
}

// TestLookupGetFieldMatchThenUnwind: a $match reading the $lookup output
// through $getField stays after the $lookup even when an $unwind of it
// follows, so mongod keeps the documents with two matches.
func TestLookupGetFieldMatchThenUnwind(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "LookupGetFieldMatchThenUnwind",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			return lookupSeedJSON(ctx, col,
				[]string{`{"_id":1,"k":1}`, `{"_id":2,"k":2}`},
				[]string{`{"_id":10,"k":1}`, `{"_id":11,"k":1}`, `{"_id":12,"k":2}`})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col,
				bson.D{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: col.Name() + "_f"}, {Key: "localField", Value: "k"},
					{Key: "foreignField", Value: "k"}, {Key: "as", Value: "j"},
				}}},
				bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{
					bson.D{{Key: "$size", Value: bson.D{{Key: "$getField", Value: "j"}}}}, int32(2)}}}}}}},
				bson.D{{Key: "$unwind", Value: "$j"}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "j._id", Value: 1}}}},
			)
		},
	})
}

// TestLookupLeadingZeroFieldNames: mongod reads "a.00", "a.01" and "k.00"
// as field names in localField and foreignField, never as array indexes.
func TestLookupLeadingZeroFieldNames(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		return lookupSeedJSON(ctx, col,
			[]string{`{"_id":1,"a":{"00":5,"01":6}}`, `{"_id":2,"a":[5,6]}`, `{"_id":3,"a":[{"00":6},{"01":5}]}`},
			[]string{`{"_id":10,"k":{"00":5}}`, `{"_id":11,"k":[5,6]}`, `{"_id":12,"k":{"00":6}}`, `{"_id":13,"k":[{"00":6}]}`, `{"_id":14,"k":[[5]]}`})
	}
	for _, local := range []string{"a.00", "a.01"} {
		harness.PairTest(t, harness.TestCase{
			Name:    "LookupLeadingZeroFieldNames_" + local,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     lookupPathsRun(local, "k.00", "j"),
		})
	}
}

// TestLookupNullLocalThroughForeignArrays: a null or missing local value
// matches a foreign path only where a query for null on it would, so mongod
// skips paths ending at scalar or nested array elements.
func TestLookupNullLocalThroughForeignArrays(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		return lookupSeedJSON(ctx, col,
			[]string{`{"_id":1}`, `{"_id":2,"x":null}`},
			[]string{`{"_id":10,"k":[5,6]}`, `{"_id":11,"k":[]}`, `{"_id":12,"k":[[7]]}`, `{"_id":13,"k":[{"a":1}]}`, `{"_id":14}`})
	}
	for _, foreign := range []string{"k.zz", "k.0", "k.1", "k.0.zz", "k.zz.yy"} {
		harness.PairTest(t, harness.TestCase{
			Name:    "LookupNullLocalThroughForeignArrays_" + foreign,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     lookupPathsRun("x", foreign, "j"),
		})
	}
}
