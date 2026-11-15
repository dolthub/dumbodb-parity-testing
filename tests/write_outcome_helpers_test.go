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
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// outcomeCode is the error code of err: the first write error's for a write,
// 0 for no error, and -1 for an error without a code.
func outcomeCode(err error) int32 {
	if err == nil {
		return 0
	}
	var we mongo.WriteException
	if errors.As(err, &we) {
		if len(we.WriteErrors) > 0 {
			return int32(we.WriteErrors[0].Code)
		}
		if we.WriteConcernError != nil {
			return int32(we.WriteConcernError.Code)
		}
	}
	var bwe mongo.BulkWriteException
	if errors.As(err, &bwe) && len(bwe.WriteErrors) > 0 {
		return int32(bwe.WriteErrors[0].Code)
	}
	var ce mongo.CommandError
	if errors.As(err, &ce) {
		return ce.Code
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0
	}
	return -1
}

// writeOutcome runs write and returns its error code with the collection's
// documents after it, so a case compares both what failed and what stayed.
func writeOutcome(ctx context.Context, col *mongo.Collection, write func() error) (interface{}, error) {
	code := outcomeCode(write())
	docs, err := sortedDocs(ctx, col)
	if err != nil {
		return nil, err
	}
	return bson.D{{Key: "code", Value: code}, {Key: "docs", Value: docs}}, nil
}

// sortedDocs returns the collection's documents in _id order.
func sortedDocs(ctx context.Context, col *mongo.Collection) ([]bson.D, error) {
	cur, err := col.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var docs []bson.D
	if err = cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

// insertDocs returns a Setup inserting docs.
func insertDocs(docs ...interface{}) func(ctx context.Context, col *mongo.Collection) error {
	return func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, docs)
		return err
	}
}

// sortedIDs returns the _ids of the documents filter matches, in _id order.
func sortedIDs(ctx context.Context, col *mongo.Collection, filter interface{}) (interface{}, error) {
	cur, err := col.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).
		SetProjection(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var docs []bson.D
	if err = cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	ids := bson.A{}
	for _, d := range docs {
		ids = append(ids, d[0].Value)
	}
	return ids, nil
}

// supportFor is DumboDBXFail for a test xfail lists by name and DumboDBFull
// otherwise.
func supportFor(t *testing.T, xfail map[string]bool) harness.DumboDBSupport {
	if xfail[t.Name()] {
		return harness.DumboDBXFail
	}
	return harness.DumboDBFull
}
