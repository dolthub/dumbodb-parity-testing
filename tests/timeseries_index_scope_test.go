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

// A time-series collection has no _id index. An {_id: 1} index is a user
// index like any other, and index statistics and dropIndexes see only the
// user indexes. The bucket span options are validated and reported as mongod
// does.

func tsScopeRun(steps ...bson.D) func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		db := col.Database()
		name := col.Name() + "_ts"
		defer db.Collection(name).Drop(ctx)
		if err := db.RunCommand(ctx, bson.D{{Key: "create", Value: name}, {Key: "timeseries", Value: bson.D{
			{Key: "timeField", Value: "t"}, {Key: "metaField", Value: "m"},
		}}}).Err(); err != nil {
			return nil, err
		}
		if _, err := db.Collection(name).InsertOne(ctx, bson.D{
			{Key: "t", Value: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}, {Key: "m", Value: int32(1)}, {Key: "_id", Value: int32(5)},
		}); err != nil {
			return nil, err
		}
		var out bson.A
		for _, step := range steps {
			cmd := append(bson.D{{Key: step[0].Key, Value: name}}, step[1:]...)
			var res bson.D
			if err := db.RunCommand(ctx, cmd).Decode(&res); err != nil {
				out = append(out, bson.D{{Key: "error", Value: fmt.Sprint(errorCode(err))}})
				continue
			}
			out = append(out, tsScopeSummary(res))
		}
		return out, nil
	}
}

// tsScopeSummary keeps the parts of a reply that do not depend on storage.
func tsScopeSummary(res bson.D) bson.D {
	out := bson.D{}
	for _, e := range res {
		switch e.Key {
		case "numIndexesBefore", "numIndexesAfter", "note", "nIndexesWas", "nindexes", "nIndexes", "keysPerIndex":
			out = append(out, e)
		case "cursor":
			for _, ce := range e.Value.(bson.D) {
				if ce.Key != "firstBatch" {
					continue
				}
				var names bson.A
				for _, d := range ce.Value.(bson.A) {
					for _, f := range d.(bson.D) {
						if f.Key == "name" || f.Key == "n" {
							names = append(names, f.Value)
						}
					}
				}
				out = append(out, bson.E{Key: "batch", Value: names})
			}
		}
	}
	return out
}

func errorCode(err error) int32 {
	if ce, ok := err.(mongo.CommandError); ok {
		return ce.Code
	}
	return -1
}

func TestTimeSeriesIndexScope(t *testing.T) {
	createID := bson.D{{Key: "createIndexes"}, {Key: "indexes", Value: bson.A{bson.D{{Key: "key", Value: bson.D{{Key: "_id", Value: 1}}}, {Key: "name", Value: "myid"}}}}}
	listIndexes := bson.D{{Key: "listIndexes"}}
	indexStats := bson.D{{Key: "aggregate"}, {Key: "pipeline", Value: bson.A{
		bson.D{{Key: "$indexStats", Value: bson.D{}}}, bson.D{{Key: "$sort", Value: bson.D{{Key: "name", Value: 1}}}},
	}}, {Key: "cursor", Value: bson.D{}}}
	collStatsN := bson.D{{Key: "aggregate"}, {Key: "pipeline", Value: bson.A{
		bson.D{{Key: "$collStats", Value: bson.D{{Key: "storageStats", Value: bson.D{}}}}},
		bson.D{{Key: "$project", Value: bson.D{{Key: "n", Value: "$storageStats.nindexes"}}}},
	}}, {Key: "cursor", Value: bson.D{}}}
	collStats := bson.D{{Key: "collStats"}}
	drop := func(index interface{}) bson.D { return bson.D{{Key: "dropIndexes"}, {Key: "index", Value: index}} }
	cases := []struct {
		name  string
		steps []bson.D
	}{
		{"CreateIDIndex", []bson.D{createID, listIndexes, createID}},
		{"CreateReservedName", []bson.D{{{Key: "createIndexes"}, {Key: "indexes", Value: bson.A{bson.D{{Key: "key", Value: bson.D{{Key: "_id", Value: 1}}}, {Key: "name", Value: "_id_"}}}}}}},
		{"Stats", []bson.D{indexStats, collStatsN, collStats, createID, indexStats, collStatsN, collStats}},
		{"DropIDName", []bson.D{drop("_id_"), drop(bson.A{"_id_"})}},
		{"DropIDKey", []bson.D{drop(bson.D{{Key: "_id", Value: 1}}), createID, drop(bson.D{{Key: "_id", Value: 1}}), listIndexes}},
		{"DropKeyPrefix", []bson.D{drop(bson.D{{Key: "m", Value: 1}})}},
		{"Validate", []bson.D{{{Key: "validate"}}, createID, {{Key: "validate"}}}},
		{"FindHintIDKey", []bson.D{{{Key: "find"}, {Key: "hint", Value: bson.D{{Key: "_id", Value: 1}}}}}},
		{"FindHintIDName", []bson.D{{{Key: "find"}, {Key: "hint", Value: "_id_"}}}},
		{"FindHintMetaTime", []bson.D{{{Key: "find"}, {Key: "hint", Value: bson.D{{Key: "m", Value: 1}, {Key: "t", Value: 1}}}}}},
		{"FindHintUserIDIndex", []bson.D{createID, {{Key: "find"}, {Key: "hint", Value: bson.D{{Key: "_id", Value: 1}}}}}},
	}
	for _, c := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "TimeSeriesIndexScope_" + c.name,
			Support: harness.DumboDBFull,
			Run:     tsScopeRun(c.steps...),
		})
	}
}

// Dropping by a key that only prefixes a compound index finds nothing.
func TestDropIndexesKeyPrefix(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DropIndexes_KeyPrefixOfCompound",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "a", Value: 1}, {Key: "b", Value: 1}}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return nil, col.Database().RunCommand(ctx, bson.D{{Key: "dropIndexes", Value: col.Name()}, {Key: "index", Value: bson.D{{Key: "a", Value: 1}}}}).Err()
		},
	})
}

func TestTimeSeriesBucketSpan(t *testing.T) {
	cases := []struct {
		name string
		opts bson.D
	}{
		{"RoundingOnly", bson.D{{Key: "bucketRoundingSeconds", Value: int32(100)}}},
		{"MaxSpanOnly", bson.D{{Key: "bucketMaxSpanSeconds", Value: int32(100)}}},
		{"Unequal", bson.D{{Key: "bucketMaxSpanSeconds", Value: int32(100)}, {Key: "bucketRoundingSeconds", Value: int32(50)}}},
		{"Equal", bson.D{{Key: "bucketMaxSpanSeconds", Value: int32(100)}, {Key: "bucketRoundingSeconds", Value: int32(100)}}},
		{"EqualWithMeta", bson.D{{Key: "metaField", Value: "m"}, {Key: "bucketMaxSpanSeconds", Value: int32(60)}, {Key: "bucketRoundingSeconds", Value: int32(60)}}},
		{"Doubles", bson.D{{Key: "bucketMaxSpanSeconds", Value: 100.5}, {Key: "bucketRoundingSeconds", Value: 100.5}}},
		{"Longs", bson.D{{Key: "bucketMaxSpanSeconds", Value: int64(100)}, {Key: "bucketRoundingSeconds", Value: int64(100)}}},
		{"Strings", bson.D{{Key: "bucketMaxSpanSeconds", Value: "100"}, {Key: "bucketRoundingSeconds", Value: "100"}}},
		{"Nulls", bson.D{{Key: "bucketMaxSpanSeconds", Value: nil}, {Key: "bucketRoundingSeconds", Value: nil}}},
		{"Negative", bson.D{{Key: "bucketMaxSpanSeconds", Value: int32(-5)}, {Key: "bucketRoundingSeconds", Value: int32(-5)}}},
		{"TooLarge", bson.D{{Key: "bucketMaxSpanSeconds", Value: int32(31536001)}, {Key: "bucketRoundingSeconds", Value: int32(31536001)}}},
		{"Largest", bson.D{{Key: "bucketMaxSpanSeconds", Value: int32(31536000)}, {Key: "bucketRoundingSeconds", Value: int32(31536000)}}},
		{"GranularityDefaultSpan", bson.D{{Key: "granularity", Value: "minutes"}, {Key: "bucketMaxSpanSeconds", Value: int32(86400)}}},
		{"GranularityOtherSpan", bson.D{{Key: "granularity", Value: "minutes"}, {Key: "bucketMaxSpanSeconds", Value: int32(100)}}},
		{"GranularityWithRounding", bson.D{{Key: "granularity", Value: "seconds"}, {Key: "bucketMaxSpanSeconds", Value: int32(3600)}, {Key: "bucketRoundingSeconds", Value: int32(60)}}},
		{"GranularityNull", bson.D{{Key: "granularity", Value: nil}}},
	}
	for _, c := range cases {
		opts := c.opts
		harness.PairTest(t, harness.TestCase{
			Name:    "TimeSeriesBucketSpan_" + c.name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				db := col.Database()
				name := col.Name() + "_ts"
				defer db.Collection(name).Drop(ctx)
				if err := db.RunCommand(ctx, bson.D{{Key: "create", Value: name}, {Key: "timeseries", Value: append(bson.D{{Key: "timeField", Value: "t"}}, opts...)}}).Err(); err != nil {
					return nil, err
				}
				specs, err := db.ListCollectionSpecifications(ctx, bson.D{{Key: "name", Value: name}})
				if err != nil || len(specs) != 1 {
					return nil, err
				}
				var options bson.D
				if err := bson.Unmarshal(specs[0].Options, &options); err != nil {
					return nil, err
				}
				return options, nil
			},
		})
	}
}
