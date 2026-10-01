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

	"github.com/dolthub/dumbodb-parity-testing/harness"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
)

func testReadConcernLevel(t *testing.T, name string, concern *readconcern.ReadConcern) {
	t.Helper()
	harness.PairTest(t, harness.TestCase{
		Name:     name,
		Support:  harness.DumboDBFull,
		Topology: harness.TopologyReplicaSet,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: name}, {Key: "value", Value: int32(1)}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			recorder := newStartedCommandRecorder()
			client, err := monitoredClient(ctx, harness.ServerURI(ctx), recorder)
			if err != nil {
				return nil, err
			}
			defer func() { _ = client.Disconnect(ctx) }()
			readCol := client.Database(col.Database().Name()).Collection(col.Name(), options.Collection().SetReadConcern(concern))
			var doc bson.M
			if err := readCol.FindOne(ctx, bson.D{{Key: "_id", Value: name}}).Decode(&doc); err != nil {
				return nil, err
			}
			findCommand := recorder.lastStarted("find")
			level := ""
			if readConcernValue, err := findCommand.LookupErr("readConcern"); err == nil {
				if levelValue, err := readConcernValue.Document().LookupErr("level"); err == nil {
					level = levelValue.StringValue()
				}
			}
			return bson.D{{Key: "level", Value: level}, {Key: "value", Value: doc["value"]}}, nil
		},
	})
}

func TestReadConcern_local(t *testing.T) {
	testReadConcernLevel(t, "ReadConcern_local", readconcern.Local())
}

func TestReadConcern_majority(t *testing.T) {
	testReadConcernLevel(t, "ReadConcern_majority", readconcern.Majority())
}

func TestReadConcern_snapshotMongoOnly(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:     "ReadConcern_snapshot_mongo_only",
		Support:  harness.DumboDBMongoOnly,
		Topology: harness.TopologyReplicaSet,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "snapshot"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// DumboDB does not advertise snapshot reads or atClusterTime.
			session, err := col.Database().Client().StartSession(options.Session().SetSnapshot(true))
			if err != nil {
				return nil, err
			}
			defer session.EndSession(ctx)
			var doc bson.M
			err = col.FindOne(mongo.NewSessionContext(ctx, session), bson.D{{Key: "_id", Value: "snapshot"}}).Decode(&doc)
			return doc, err
		},
	})
}
