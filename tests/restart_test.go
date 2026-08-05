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
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func TestRestartableServersPrimitive(t *testing.T) {
	srv := harness.StartRestartableServers(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	dial := func(uri string) *mongo.Client {
		c, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
		if err != nil {
			t.Fatalf("connect %s: %v", uri, err)
		}
		return c
	}

	const dbName = "restart_durability"
	const collName = "docs"
	doc := bson.D{{Key: "_id", Value: 1}, {Key: "v", Value: "before-restart"}}

	for _, srvURI := range []struct {
		name string
		uri  string
	}{{"mongo", srv.MongoURI}, {"dumbodb", srv.DumboDBURI}} {
		c := dial(srvURI.uri)
		if _, err := c.Database(dbName).Collection(collName).InsertOne(ctx, doc); err != nil {
			t.Fatalf("%s: insert before restart: %v", srvURI.name, err)
		}
		_ = c.Disconnect(ctx)
	}

	srv.Restart(t)

	for _, srvURI := range []struct {
		name string
		uri  string
	}{{"mongo", srv.MongoURI}, {"dumbodb", srv.DumboDBURI}} {
		c := dial(srvURI.uri)
		var got bson.M
		err := c.Database(dbName).Collection(collName).FindOne(ctx, bson.D{{Key: "_id", Value: 1}}).Decode(&got)
		if err != nil {
			t.Fatalf("%s: document missing after restart: %v", srvURI.name, err)
		}
		if got["v"] != "before-restart" {
			t.Fatalf("%s: document changed across restart: got %v", srvURI.name, got["v"])
		}
		_ = c.Disconnect(ctx)
	}
}
