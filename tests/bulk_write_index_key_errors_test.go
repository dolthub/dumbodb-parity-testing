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

// bulkWrite reports a document an index cannot hold with the index's own code
// and message, as insert does.
func TestBulkWriteIndexKeyErrors(t *testing.T) {
	d := func(kv ...interface{}) bson.D {
		var out bson.D
		for i := 0; i < len(kv); i += 2 {
			out = append(out, bson.E{Key: kv[i].(string), Value: kv[i+1]})
		}
		return out
	}
	for _, c := range []struct {
		name string
		key  bson.D
		doc  bson.D
	}{
		{"geo", d("loc", "2dsphere"), d("_id", 1, "loc", d("coordinates", bson.A{500, 0}, "type", "Point"))},
		{"parallel_arrays", d("a", 1, "b", 1), d("_id", 1, "a", bson.A{1}, "b", bson.A{2})},
	} {
		t.Run(c.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "BulkWriteIndexKeyErrors_" + c.name,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: c.key})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					var res bson.M
					if err := col.Database().Client().Database("admin").RunCommand(ctx, d(
						"bulkWrite", 1,
						"ops", bson.A{d("insert", 0, "document", c.doc)},
						"nsInfo", bson.A{d("ns", col.Database().Name()+"."+col.Name())},
					)).Decode(&res); err != nil {
						return nil, err
					}
					var out bson.A
					for _, e := range res["cursor"].(bson.M)["firstBatch"].(bson.A) {
						m := e.(bson.M)
						out = append(out, d("ok", m["ok"], "code", m["code"], "errmsg", m["errmsg"]))
					}
					return out, nil
				},
			})
		})
	}
}
