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
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// serverStatus metrics.queryExecutor.scanned and scannedObjects count the index
// keys and documents queries read, so their change across an operation shows
// the work it really did, unlike explain, which plans separately.

func queryExecutorCounters(ctx context.Context, col *mongo.Collection) (keys, docs int64, err error) {
	var res struct {
		Metrics struct {
			QueryExecutor struct {
				Scanned        int64 `bson:"scanned"`
				ScannedObjects int64 `bson:"scannedObjects"`
			} `bson:"queryExecutor"`
		} `bson:"metrics"`
	}
	err = col.Database().Client().Database("admin").RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}}).Decode(&res)
	return res.Metrics.QueryExecutor.Scanned, res.Metrics.QueryExecutor.ScannedObjects, err
}

// scannedBy returns how far op moves the counters. A read is measured three
// times and the smallest change kept, so background server work in one window
// does not count against it.
func scannedBy(ctx context.Context, col *mongo.Collection, op func() error, repeatable bool) (bson.D, error) {
	runs := 1
	if repeatable {
		runs = 3
	}
	var bestKeys, bestDocs int64 = -1, -1
	for range runs {
		k0, d0, err := queryExecutorCounters(ctx, col)
		if err != nil {
			return nil, err
		}
		if err := op(); err != nil {
			return nil, err
		}
		k1, d1, err := queryExecutorCounters(ctx, col)
		if err != nil {
			return nil, err
		}
		if bestKeys < 0 || k1-k0 < bestKeys {
			bestKeys = k1 - k0
		}
		if bestDocs < 0 || d1-d0 < bestDocs {
			bestDocs = d1 - d0
		}
	}
	return bson.D{{Key: "scanned", Value: bestKeys}, {Key: "scannedObjects", Value: bestDocs}}, nil
}

func drainFind(ctx context.Context, col *mongo.Collection, filter bson.D, opts *options.FindOptions) error {
	cur, err := col.Find(ctx, filter, opts)
	if err != nil {
		return err
	}
	var docs []bson.M
	return cur.All(ctx, &docs)
}

func TestServerStatus_QueryExecutorScanned(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 1000)
		for i := 0; i < 1000; i++ {
			docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "i", Value: int32(i)}, {Key: "grp", Value: int32(i % 10)}})
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "i", Value: 1}}},
			{Keys: bson.D{{Key: "grp", Value: 1}}},
		})
		return err
	}
	find := func(filter bson.D, opts *options.FindOptions) func(context.Context, *mongo.Collection) error {
		return func(ctx context.Context, col *mongo.Collection) error { return drainFind(ctx, col, filter, opts) }
	}
	for _, tc := range []struct {
		name       string
		op         func(context.Context, *mongo.Collection) error
		repeatable bool
		support    harness.DumboDBSupport
	}{
		{"CollectionScan", find(bson.D{{Key: "x", Value: 1}}, nil), true, harness.DumboDBFull},
		{"IndexEqualityOne", find(bson.D{{Key: "i", Value: int32(5)}}, nil), true, harness.DumboDBFull},
		{"IndexEqualityMany", find(bson.D{{Key: "grp", Value: int32(5)}}, nil), true, harness.DumboDBFull},
		{"IndexRange", find(bson.D{{Key: "i", Value: bson.D{{Key: "$gte", Value: int32(100)}, {Key: "$lt", Value: int32(200)}}}}, nil), true, harness.DumboDBFull},
		{"IndexSortLimit", find(bson.D{}, options.Find().SetSort(bson.D{{Key: "i", Value: -1}}).SetLimit(10)), true, harness.DumboDBFull},
		{"IDEquality", find(bson.D{{Key: "_id", Value: int32(5)}}, nil), true, harness.DumboDBFull},
		{"IDMiss", find(bson.D{{Key: "_id", Value: int32(5000)}}, nil), true, harness.DumboDBFull},
		{"IndexCount", func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.CountDocuments(ctx, bson.D{{Key: "grp", Value: int32(5)}})
			return err
		}, true, harness.DumboDBFull},
		{"IndexUpdateMany", func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.UpdateMany(ctx, bson.D{{Key: "grp", Value: int32(5)}}, bson.D{{Key: "$set", Value: bson.D{{Key: "t", Value: 1}}}})
			return err
		}, false, harness.DumboDBFull},
		{"IndexDeleteOne", func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.DeleteOne(ctx, bson.D{{Key: "i", Value: int32(7)}})
			return err
		}, false, harness.DumboDBFull},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      fmt.Sprintf("ServerStatus_QueryExecutorScanned_%s", tc.name),
				Support:   tc.support,
				Clustered: true,
				Setup:     setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return scannedBy(ctx, col, func() error { return tc.op(ctx, col) }, tc.repeatable)
				},
			})
		})
	}
}
