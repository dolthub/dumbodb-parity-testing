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

// TestMergeIntoView: $merge into a view fails with 166 once a document
// reaches it, and with 51183 at parse for an 'on' other than _id.
func TestMergeIntoView(t *testing.T) {
	cases := map[string]struct {
		database bool
		pipeline string
	}{
		"Default":         {false, `[{"$merge":"vw"}]`},
		"Replace":         {false, `[{"$merge":{"into":"vw","whenMatched":"replace"}}]`},
		"KeepExisting":    {false, `[{"$merge":{"into":"vw","whenMatched":"keepExisting"}}]`},
		"Pipeline":        {false, `[{"$merge":{"into":"vw","whenMatched":[{"$set":{"a":1}}]}}]`},
		"Discard":         {false, `[{"$merge":{"into":"vw","whenNotMatched":"discard"}}]`},
		"NoDocuments":     {false, `[{"$match":{"_id":99}},{"$merge":"vw"}]`},
		"OnOtherField":    {false, `[{"$merge":{"into":"vw","on":"x"}}]`},
		"DocumentsSource": {true, `[{"$documents":[{"_id":1}]},{"$merge":"vw"}]`},
	}
	for name, tc := range cases {
		var pipeline bson.A
		if err := bson.UnmarshalExtJSON([]byte(`{"p":`+tc.pipeline+`}`), false, &struct{ P *bson.A }{&pipeline}); err != nil {
			t.Fatal(name, err)
		}
		database := tc.database
		harness.PairTest(t, harness.TestCase{
			Name:    "MergeIntoView_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				db := col.Database()
				if _, err := col.InsertMany(ctx, []interface{}{
					bson.D{{Key: "_id", Value: int32(1)}, {Key: "x", Value: int32(10)}},
					bson.D{{Key: "_id", Value: int32(2)}, {Key: "x", Value: int32(20)}},
				}); err != nil {
					return err
				}
				if _, err := db.Collection("base").InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}}); err != nil {
					return err
				}
				return db.CreateView(ctx, "vw", "base", bson.A{})
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				var cursor *mongo.Cursor
				var err error
				if database {
					cursor, err = col.Database().Aggregate(ctx, pipeline)
				} else {
					cursor, err = col.Aggregate(ctx, pipeline)
				}
				res, _ := drainCursor(ctx, cursor, err)
				var base []bson.M
				cur, err := col.Database().Collection("base").Find(ctx, bson.D{})
				if err != nil {
					return nil, err
				}
				if err = cur.All(ctx, &base); err != nil {
					return nil, err
				}
				return bson.M{"result": res, "base": base}, nil
			},
		})
	}
}
