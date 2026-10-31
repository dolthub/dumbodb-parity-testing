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

// mongod aborts a transaction when one of its statements fails, so nothing
// it wrote is committed. DumboDB kept the transaction open and committed a
// failed statement's partial write (workspace-8tj.19).
func TestTransactionStatementError(t *testing.T) {
	statements := []struct {
		name string
		run  func(sc context.Context, col *mongo.Collection) error
	}{
		{"FindAndModifyProjectionError", func(sc context.Context, col *mongo.Collection) error {
			return col.Database().RunCommand(sc, bson.D{
				{Key: "findAndModify", Value: col.Name()},
				{Key: "query", Value: bson.D{{Key: "_id", Value: "a"}}},
				{Key: "update", Value: bson.D{{Key: "$inc", Value: bson.D{{Key: "n", Value: int32(1)}}}}},
				{Key: "fields", Value: bson.D{{Key: "x", Value: bson.D{{Key: "$add", Value: bson.A{"$s", int32(1)}}}}}},
			}).Err()
		}},
		{"DuplicateKeyInsert", func(sc context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(sc, bson.D{{Key: "_id", Value: "a"}})
			return err
		}},
		{"UpdateTypeError", func(sc context.Context, col *mongo.Collection) error {
			_, err := col.UpdateOne(sc, bson.D{{Key: "_id", Value: "a"}}, bson.D{{Key: "$inc", Value: bson.D{{Key: "s", Value: int32(1)}}}})
			return err
		}},
		{"UpdateManyPartial", func(sc context.Context, col *mongo.Collection) error {
			_, err := col.UpdateMany(sc, bson.D{}, bson.D{{Key: "$inc", Value: bson.D{{Key: "n", Value: int32(1)}}}})
			return err
		}},
		{"AggregateRuntimeError", func(sc context.Context, col *mongo.Collection) error {
			cur, err := col.Aggregate(sc, mongo.Pipeline{{{Key: "$project", Value: bson.D{{Key: "x", Value: bson.D{{Key: "$add", Value: bson.A{"$s", int32(1)}}}}}}}})
			if err == nil {
				err = cur.All(sc, &[]bson.M{})
			}
			return err
		}},
		{"FindBadFilter", func(sc context.Context, col *mongo.Collection) error {
			cur, err := col.Find(sc, bson.D{{Key: "n", Value: bson.D{{Key: "$bogus", Value: int32(1)}}}})
			if err == nil {
				err = cur.All(sc, &[]bson.M{})
			}
			return err
		}},
	}
	for _, st := range statements {
		run := st.run
		name := "TransactionStatementError_" + st.name
		t.Run(st.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:     name,
				Support:  harness.DumboDBFull,
				Topology: harness.TopologyReplicaSet,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					_, err := col.InsertMany(ctx, []interface{}{
						bson.D{{Key: "_id", Value: "a"}, {Key: "n", Value: int32(1)}, {Key: "s", Value: "x"}},
						bson.D{{Key: "_id", Value: "b"}, {Key: "n", Value: "text"}},
					})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					sess, err := col.Database().Client().StartSession()
					if err != nil {
						return nil, err
					}
					defer sess.EndSession(ctx)

					startErr := sess.StartTransaction()
					sc := mongo.NewSessionContext(ctx, sess)
					_, firstErr := col.InsertOne(sc, bson.D{{Key: "_id", Value: "before"}})
					stmtErr := run(sc, col)
					_, afterErr := col.InsertOne(sc, bson.D{{Key: "_id", Value: "after"}})
					commitErr := sess.CommitTransaction(ctx)

					cur, err := col.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
					if err != nil {
						return nil, err
					}
					var docs []bson.M
					if err := cur.All(ctx, &docs); err != nil {
						return nil, err
					}
					info := func(err error) bson.D {
						i := errInfo(err)
						return bson.D{{Key: "code", Value: i.code}, {Key: "labels", Value: i.labels}}
					}
					return bson.D{
						{Key: "start", Value: info(startErr)},
						{Key: "first", Value: info(firstErr)},
						{Key: "statement", Value: info(stmtErr)},
						{Key: "after", Value: info(afterErr)},
						{Key: "commit", Value: info(commitErr)},
						{Key: "docs", Value: docs},
					}, nil
				},
			})
		})
	}
}
