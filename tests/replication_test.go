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

// The walking skeleton: the smallest workload that exercises the whole
// apparatus at once -- provision a set, join DumboDB, write, wait for
// convergence, compare stored state.
//
// Its purpose is to prove the apparatus before coverage is written against it.
// The expected outcome while replication is being built is that the control
// converges and matches (the harness works) and the subject does not (reported
// as XFAIL, not as a build failure). Both halves are the deliverable.
func TestReplication_TenDocuments(t *testing.T) {
	harness.ReplicaTest(t, harness.ReplicaCase{
		Name:    "Replication_TenDocuments",
		Support: harness.DumboDBXFail,
		Workload: func(ctx context.Context, primary *mongo.Client) error {
			coll := primary.Database("skeleton").Collection("docs")
			docs := make([]interface{}, 0, 10)
			for i := 0; i < 10; i++ {
				docs = append(docs, bson.D{
					{Key: "_id", Value: int32(i)},
					{Key: "name", Value: "doc"},
					{Key: "n", Value: int64(i)},
				})
			}
			_, err := coll.InsertMany(ctx, docs)
			return err
		},
	})
}

// Same apparatus with the subject skipped. This is the control on its own: if
// this fails, the harness is broken independently of anything DumboDB does.
func TestReplication_ControlOnly(t *testing.T) {
	harness.ReplicaTest(t, harness.ReplicaCase{
		Name:    "Replication_ControlOnly",
		Support: harness.DumboDBMongoOnly,
		Workload: func(ctx context.Context, primary *mongo.Client) error {
			coll := primary.Database("control").Collection("docs")
			_, err := coll.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: "x"}})
			return err
		},
		Assert: func(t *testing.T, res harness.ReplicaResult) {
			if res.Reference == nil {
				t.Fatal("no reference state captured")
			}
			db, ok := res.Reference.Databases["control"]
			if !ok {
				t.Fatal("the reference secondary did not receive the control database")
			}
			coll, ok := db.Collections["docs"]
			if !ok {
				t.Fatal("the reference secondary did not receive the control collection")
			}
			if len(coll.Documents) != 1 {
				t.Errorf("reference holds %d documents, want 1", len(coll.Documents))
			}
		},
	})
}
