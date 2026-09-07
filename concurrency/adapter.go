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

package concurrency

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type WriteResult struct {
	Matched  int64
	Modified int64
}

type Collection interface {
	InsertOne(context.Context, interface{}) error
	FindOne(context.Context, interface{}, interface{}) error
	UpdateOne(context.Context, interface{}, interface{}) (WriteResult, error)
}

type Target interface {
	Ping(context.Context) error
	Identity(context.Context) (serverInfo, error)
	Collection(database, collection string) Collection
	DropDatabase(context.Context, string) error
	Disconnect(context.Context) error
}

type mongoTarget struct {
	client *mongo.Client
}

func connectMongoTarget(ctx context.Context, uri string) (Target, error) {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	return &mongoTarget{client: client}, nil
}

func (t *mongoTarget) Ping(ctx context.Context) error {
	return t.client.Ping(ctx, nil)
}

func (t *mongoTarget) Identity(ctx context.Context) (serverInfo, error) {
	var response bson.M
	if err := t.client.Database("admin").RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&response); err != nil {
		return serverInfo{}, fmt.Errorf("buildInfo: %w", err)
	}
	version, _ := response["version"].(string)
	if version == "" {
		return serverInfo{}, fmt.Errorf("buildInfo did not return a version")
	}
	return serverInfo{Product: "MongoDB", Version: version}, nil
}

func (t *mongoTarget) Collection(database, collection string) Collection {
	return mongoCollection{collection: t.client.Database(database).Collection(collection)}
}

func (t *mongoTarget) DropDatabase(ctx context.Context, database string) error {
	return t.client.Database(database).Drop(ctx)
}

func (t *mongoTarget) Disconnect(ctx context.Context) error {
	return t.client.Disconnect(ctx)
}

type mongoCollection struct {
	collection *mongo.Collection
}

func (c mongoCollection) InsertOne(ctx context.Context, document interface{}) error {
	_, err := c.collection.InsertOne(ctx, document)
	return err
}

func (c mongoCollection) FindOne(ctx context.Context, filter interface{}, result interface{}) error {
	return c.collection.FindOne(ctx, filter).Decode(result)
}

func (c mongoCollection) UpdateOne(ctx context.Context, filter, update interface{}) (WriteResult, error) {
	result, err := c.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return WriteResult{}, err
	}
	return WriteResult{Matched: result.MatchedCount, Modified: result.ModifiedCount}, nil
}
