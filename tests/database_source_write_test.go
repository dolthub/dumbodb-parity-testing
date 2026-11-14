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

// TestDatabaseSourceWrite: an {aggregate: 1} pipeline led by $documents or
// $listClusterCatalog may end in $out or $merge, with $merge's 'on' checks,
// whenMatched pipeline and unique index requirement, and either before the
// last stage fails with 40601.
func TestDatabaseSourceWrite(t *testing.T) {
	cases := map[string]string{
		"DocumentsOut":           `[{"$documents":[{"_id":1,"y":1},{"_id":2}]},{"$out":"w"}]`,
		"DocumentsMerge":         `[{"$documents":[{"_id":1,"y":1},{"_id":3}]},{"$merge":"w"}]`,
		"DocumentsMergePipeline": `[{"$documents":[{"_id":1,"y":2},{"_id":3}]},{"$merge":{"into":"w","whenMatched":[{"$set":{"z":"$$new.y"}}]}}]`,
		"DocumentsMergeNullID":   `[{"$documents":[{"_id":null,"x":1}]},{"$merge":"w"}]`,
		"DocumentsMergeNoIndex":  `[{"$documents":[{"_id":1,"y":1}]},{"$merge":{"into":"w","on":"y"}}]`,
		"DocumentsMergeBadSpec":  `[{"$documents":[{"_id":1}]},{"$merge":{"into":"w","bogus":1}}]`,
		"DocumentsOutNotLast":    `[{"$documents":[{"_id":1}]},{"$out":"w"},{"$match":{}}]`,
		"DocumentsMergeNotLast":  `[{"$documents":[{"_id":1}]},{"$merge":"w"},{"$match":{}}]`,
		"CatalogOut":             `[{"$listClusterCatalog":{}},{"$match":{"type":"collection","ns":{"$not":{"$regex":"\\.(system|tmp)\\."}}}},{"$project":{"_id":{"$substrCP":["$ns",{"$indexOfCP":["$ns","."]},100]},"type":1}},{"$out":"w"}]`,
	}
	for name, js := range cases {
		var pipeline bson.A
		if err := bson.UnmarshalExtJSON([]byte(`{"p":`+js+`}`), false, &struct{ P *bson.A }{&pipeline}); err != nil {
			t.Fatal(name, err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "DatabaseSourceWrite_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.Database().Collection("w").InsertMany(ctx, []interface{}{
					bson.D{{Key: "_id", Value: int32(1)}, {Key: "x", Value: int32(1)}},
					bson.D{{Key: "_id", Value: int32(2)}, {Key: "x", Value: int32(2)}},
				})
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Database().Aggregate(ctx, pipeline)
				res, _ := drainCursor(ctx, cursor, err)
				cur, err := col.Database().Collection("w").Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
				if err != nil {
					return nil, err
				}
				var docs []bson.M
				if err = cur.All(ctx, &docs); err != nil {
					return nil, err
				}
				return bson.M{"result": res, "target": docs}, nil
			},
		})
	}
}
