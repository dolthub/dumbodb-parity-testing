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

// Parity coverage for document-validator ENFORCEMENT across every write path
// (workspace-pui.2). The pre-existing validator parity tests only checked
// create/collMod round-trip and a single coarse insert rejection; these assert
// that update / findAndModify / bulkWrite reject an invalid result with the same
// error code as MongoDB, and that validationAction:"warn" allows the write.

import (
	"context"
	"errors"
	"testing"

	"github.com/dolthub/dumbodb-parity-testing/harness"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// veNonNegAge is a query-expression validator requiring age >= 0.
var veNonNegAge = bson.D{{Key: "age", Value: bson.D{{Key: "$gte", Value: int32(0)}}}}

// classifyWrite normalizes a write outcome to a comparable value so PairTest
// asserts the two servers agree on rejection AND error code (rather than raw
// message text). Handles CommandError, WriteException, and BulkWriteException.
func classifyWrite(err error) bson.D {
	if err == nil {
		return bson.D{{Key: "rejected", Value: false}}
	}
	code, name, ok := harness.CommandErrorCode(err)
	if !ok {
		var bwe mongo.BulkWriteException
		if errors.As(err, &bwe) && len(bwe.WriteErrors) > 0 {
			code = int32(bwe.WriteErrors[0].Code)
		}
	}
	return bson.D{
		{Key: "rejected", Value: true},
		{Key: "code", Value: code},
		{Key: "codeName", Value: name},
	}
}

// veValidatedColl creates a fresh collection with the non-negative-age validator
// at the given level/action and seeds one valid document (_id:1, age:5).
func veValidatedColl(ctx context.Context, col *mongo.Collection, suffix, level, action string) (*mongo.Collection, error) {
	name := col.Name() + suffix
	opts := options.CreateCollection().SetValidator(veNonNegAge)
	if level != "" {
		opts.SetValidationLevel(level)
	}
	if action != "" {
		opts.SetValidationAction(action)
	}
	if err := col.Database().CreateCollection(ctx, name, opts); err != nil {
		return nil, err
	}
	vc := col.Database().Collection(name)
	if _, err := vc.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "age", Value: int32(5)}}); err != nil {
		return nil, err
	}
	return vc, nil
}

func TestValidatorEnforce_Update_InvalidRejected(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ValidatorEnforce_Update_InvalidRejected",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			vc, err := veValidatedColl(ctx, col, "_upd", "strict", "error")
			if err != nil {
				return nil, err
			}
			_, updErr := vc.UpdateOne(ctx, bson.D{{Key: "_id", Value: 1}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "age", Value: int32(-5)}}}})
			return classifyWrite(updErr), nil
		},
	})
}

func TestValidatorEnforce_UpdateUpsert_InvalidRejected(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ValidatorEnforce_UpdateUpsert_InvalidRejected",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			vc, err := veValidatedColl(ctx, col, "_ups", "strict", "error")
			if err != nil {
				return nil, err
			}
			_, upErr := vc.UpdateOne(ctx, bson.D{{Key: "_id", Value: 99}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "age", Value: int32(-1)}}}},
				options.Update().SetUpsert(true))
			return classifyWrite(upErr), nil
		},
	})
}

func TestValidatorEnforce_FindAndModify_InvalidRejected(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ValidatorEnforce_FindAndModify_InvalidRejected",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			vc, err := veValidatedColl(ctx, col, "_fam", "strict", "error")
			if err != nil {
				return nil, err
			}
			famErr := vc.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: 1}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "age", Value: int32(-3)}}}}).Err()
			return classifyWrite(famErr), nil
		},
	})
}

func TestValidatorEnforce_BulkWrite_InvalidRejected(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ValidatorEnforce_BulkWrite_InvalidRejected",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			vc, err := veValidatedColl(ctx, col, "_bw", "strict", "error")
			if err != nil {
				return nil, err
			}
			_, updErr := vc.BulkWrite(ctx, []mongo.WriteModel{
				mongo.NewUpdateOneModel().
					SetFilter(bson.D{{Key: "_id", Value: 1}}).
					SetUpdate(bson.D{{Key: "$set", Value: bson.D{{Key: "age", Value: int32(-1)}}}}),
			})
			updClass := classifyWrite(updErr)

			_, insErr := vc.BulkWrite(ctx, []mongo.WriteModel{
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: 2}, {Key: "age", Value: int32(-9)}}),
			})
			return bson.D{{Key: "update", Value: updClass}, {Key: "insert", Value: classifyWrite(insErr)}}, nil
		},
	})
}

func TestValidatorEnforce_Bypass_AllowsInvalid(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ValidatorEnforce_Bypass_AllowsInvalid",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			vc, err := veValidatedColl(ctx, col, "_byp", "strict", "error")
			if err != nil {
				return nil, err
			}
			_, insErr := vc.InsertOne(ctx, bson.D{{Key: "_id", Value: 2}, {Key: "age", Value: int32(-9)}},
				options.InsertOne().SetBypassDocumentValidation(true))
			_, updErr := vc.UpdateOne(ctx, bson.D{{Key: "_id", Value: 1}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "age", Value: int32(-1)}}}},
				options.Update().SetBypassDocumentValidation(true))
			return bson.D{
				{Key: "insert", Value: classifyWrite(insErr)},
				{Key: "update", Value: classifyWrite(updErr)},
			}, nil
		},
	})
}

func TestValidatorEnforce_Warn_AllowsInvalid(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ValidatorEnforce_Warn_AllowsInvalid",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			vc, err := veValidatedColl(ctx, col, "_warn", "strict", "warn")
			if err != nil {
				return nil, err
			}
			_, updErr := vc.UpdateOne(ctx, bson.D{{Key: "_id", Value: 1}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "age", Value: int32(-5)}}}})
			// Both servers must allow the write; also confirm it applied.
			var got bson.M
			readErr := vc.FindOne(ctx, bson.D{{Key: "_id", Value: 1}}).Decode(&got)
			return bson.D{
				{Key: "write", Value: classifyWrite(updErr)},
				{Key: "age", Value: got["age"]},
				{Key: "readOK", Value: readErr == nil},
			}, nil
		},
	})
}
