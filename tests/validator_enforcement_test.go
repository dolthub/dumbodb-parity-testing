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
	"sort"
	"testing"

	"github.com/dolthub/dumbodb-parity-testing/harness"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

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

var veReqName = bson.D{{Key: "$jsonSchema", Value: bson.D{
	{Key: "bsonType", Value: "object"},
	{Key: "required", Value: bson.A{"name"}},
	{Key: "properties", Value: bson.D{{Key: "name", Value: bson.D{{Key: "bsonType", Value: "string"}}}}},
}}}

func veJsonSchemaColl(ctx context.Context, col *mongo.Collection, suffix string) (*mongo.Collection, error) {
	name := col.Name() + suffix
	if err := col.Database().CreateCollection(ctx, name, options.CreateCollection().SetValidator(veReqName)); err != nil {
		return nil, err
	}
	vc := col.Database().Collection(name)
	if _, err := vc.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "name", Value: "ok"}}); err != nil {
		return nil, err
	}
	return vc, nil
}

func TestValidatorEnforce_JsonSchema_Update_InvalidRejected(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ValidatorEnforce_JsonSchema_Update_InvalidRejected",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			vc, err := veJsonSchemaColl(ctx, col, "_jsupd")
			if err != nil {
				return nil, err
			}
			_, updErr := vc.UpdateOne(ctx, bson.D{{Key: "_id", Value: 1}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "name", Value: int32(5)}}}})
			return classifyWrite(updErr), nil
		},
	})
}

func TestValidatorEnforce_JsonSchema_FindAndModify_InvalidRejected(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ValidatorEnforce_JsonSchema_FindAndModify_InvalidRejected",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			vc, err := veJsonSchemaColl(ctx, col, "_jsfam")
			if err != nil {
				return nil, err
			}
			famErr := vc.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: 1}},
				bson.D{{Key: "$unset", Value: bson.D{{Key: "name", Value: ""}}}}).Err()
			return classifyWrite(famErr), nil
		},
	})
}

func TestValidatorEnforce_JsonSchema_BulkWrite_InvalidRejected(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ValidatorEnforce_JsonSchema_BulkWrite_InvalidRejected",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			vc, err := veJsonSchemaColl(ctx, col, "_jsbw")
			if err != nil {
				return nil, err
			}
			_, updErr := vc.BulkWrite(ctx, []mongo.WriteModel{
				mongo.NewUpdateOneModel().
					SetFilter(bson.D{{Key: "_id", Value: 1}}).
					SetUpdate(bson.D{{Key: "$set", Value: bson.D{{Key: "name", Value: int32(9)}}}}),
			})
			_, insErr := vc.BulkWrite(ctx, []mongo.WriteModel{
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: 2}, {Key: "other", Value: "x"}}),
			})
			return bson.D{{Key: "update", Value: classifyWrite(updErr)}, {Key: "insert", Value: classifyWrite(insErr)}}, nil
		},
	})
}

func veLevelSetup(ctx context.Context, col *mongo.Collection, suffix, level string) (*mongo.Collection, error) {
	name := col.Name() + suffix
	if err := col.Database().CreateCollection(ctx, name); err != nil {
		return nil, err
	}
	vc := col.Database().Collection(name)
	if _, err := vc.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: 1}, {Key: "age", Value: int32(-5)}},
		bson.D{{Key: "_id", Value: 2}, {Key: "age", Value: int32(10)}},
	}); err != nil {
		return nil, err
	}
	if err := vc.Database().RunCommand(ctx, bson.D{
		{Key: "collMod", Value: name},
		{Key: "validator", Value: veNonNegAge},
		{Key: "validationLevel", Value: level},
		{Key: "validationAction", Value: "error"},
	}).Err(); err != nil {
		return nil, err
	}
	return vc, nil
}

func TestValidatorLevel_Strict_ValidatesGrandfathered(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ValidatorLevel_Strict_ValidatesGrandfathered",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			vc, err := veLevelSetup(ctx, col, "_strict", "strict")
			if err != nil {
				return nil, err
			}
			_, uErr := vc.UpdateOne(ctx, bson.D{{Key: "_id", Value: 1}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "note", Value: "x"}}}})
			return classifyWrite(uErr), nil
		},
	})
}

// TestValidatorLevel_Moderate_GrandfathersInvalid: under moderate, an update to an
// already-invalid document is allowed (its pre-image failed the validator), while
// an update turning a valid document invalid is still rejected.
func TestValidatorLevel_Moderate_GrandfathersInvalid(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ValidatorLevel_Moderate_GrandfathersInvalid",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			vc, err := veLevelSetup(ctx, col, "_moderate", "moderate")
			if err != nil {
				return nil, err
			}
			_, grandErr := vc.UpdateOne(ctx, bson.D{{Key: "_id", Value: 1}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "note", Value: "x"}}}})
			_, validErr := vc.UpdateOne(ctx, bson.D{{Key: "_id", Value: 2}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "age", Value: int32(-1)}}}})
			return bson.D{
				{Key: "grandfathered", Value: classifyWrite(grandErr)},
				{Key: "validToInvalid", Value: classifyWrite(validErr)},
			}, nil
		},
	})
}

// TestValidatorLevel_Moderate_JsonSchema_GrandfathersInvalid: a moderate-level
// $jsonSchema validator skips an update to a document that already failed the
// schema (grandfathered), while still rejecting an update that turns a
// schema-valid document invalid.
func TestValidatorLevel_Moderate_JsonSchema_GrandfathersInvalid(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ValidatorLevel_Moderate_JsonSchema_GrandfathersInvalid",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			name := col.Name() + "_jsmod"
			if err := col.Database().CreateCollection(ctx, name); err != nil {
				return nil, err
			}
			vc := col.Database().Collection(name)
			if _, err := vc.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "other", Value: "x"}},
				bson.D{{Key: "_id", Value: 2}, {Key: "name", Value: "ok"}},
			}); err != nil {
				return nil, err
			}
			if err := vc.Database().RunCommand(ctx, bson.D{
				{Key: "collMod", Value: name},
				{Key: "validator", Value: veReqName},
				{Key: "validationLevel", Value: "moderate"},
				{Key: "validationAction", Value: "error"},
			}).Err(); err != nil {
				return nil, err
			}

			_, grandErr := vc.UpdateOne(ctx, bson.D{{Key: "_id", Value: 1}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "other", Value: "y"}}}})
			_, validErr := vc.UpdateOne(ctx, bson.D{{Key: "_id", Value: 2}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "name", Value: int32(5)}}}})
			return bson.D{
				{Key: "grandfathered", Value: classifyWrite(grandErr)},
				{Key: "validToInvalid", Value: classifyWrite(validErr)},
			}, nil
		},
	})
}

// veAuditIDs returns the integer _ids matching an audit query, sorted. Installing
// a validator never retro-checks existing documents, so the idiom for finding
// pre-existing offenders is find({$nor:[<validator>]}).
func veAuditIDs(ctx context.Context, vc *mongo.Collection, filter bson.D) ([]int32, error) {
	cur, err := vc.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var ids []int32
	for cur.Next(ctx) {
		var doc struct {
			ID int32 `bson:"_id"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		ids = append(ids, doc.ID)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func TestValidatorAudit_QueryExpr_FindsNonConforming(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ValidatorAudit_QueryExpr_FindsNonConforming",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			name := col.Name() + "_auditqe"
			if err := col.Database().CreateCollection(ctx, name); err != nil {
				return nil, err
			}
			vc := col.Database().Collection(name)
			if _, err := vc.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "age", Value: int32(-5)}},
				bson.D{{Key: "_id", Value: 2}, {Key: "age", Value: int32(10)}},
				bson.D{{Key: "_id", Value: 3}, {Key: "age", Value: int32(-1)}},
				bson.D{{Key: "_id", Value: 4}, {Key: "age", Value: int32(0)}},
			}); err != nil {
				return nil, err
			}
			if err := vc.Database().RunCommand(ctx, bson.D{
				{Key: "collMod", Value: name},
				{Key: "validator", Value: veNonNegAge},
			}).Err(); err != nil {
				return nil, err
			}
			total, err := vc.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			offenders, err := veAuditIDs(ctx, vc, bson.D{{Key: "$nor", Value: bson.A{veNonNegAge}}})
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "totalAfterInstall", Value: total},
				{Key: "nonConforming", Value: offenders},
			}, nil
		},
	})
}

func TestValidatorAudit_JsonSchema_FindsNonConforming(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ValidatorAudit_JsonSchema_FindsNonConforming",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			name := col.Name() + "_auditjs"
			if err := col.Database().CreateCollection(ctx, name); err != nil {
				return nil, err
			}
			vc := col.Database().Collection(name)
			if _, err := vc.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "other", Value: "x"}},
				bson.D{{Key: "_id", Value: 2}, {Key: "name", Value: "ok"}},
				bson.D{{Key: "_id", Value: 3}, {Key: "name", Value: int32(5)}},
				bson.D{{Key: "_id", Value: 4}, {Key: "name", Value: "good"}},
			}); err != nil {
				return nil, err
			}
			if err := vc.Database().RunCommand(ctx, bson.D{
				{Key: "collMod", Value: name},
				{Key: "validator", Value: veReqName},
			}).Err(); err != nil {
				return nil, err
			}
			offenders, err := veAuditIDs(ctx, vc, bson.D{{Key: "$nor", Value: bson.A{veReqName}}})
			if err != nil {
				return nil, err
			}
			var survived bson.M
			readErr := vc.FindOne(ctx, bson.D{{Key: "_id", Value: 1}}).Decode(&survived)
			return bson.D{
				{Key: "nonConforming", Value: offenders},
				{Key: "offenderStillReadable", Value: readErr == nil},
			}, nil
		},
	})
}

// veListValidatorOptions reads a collection's validator/validationLevel/
// validationAction from listCollections. MongoDB does NOT materialize the
// level/action defaults there (they are absent, decoded as empty strings) and
// reports only what was explicitly set.
func veListValidatorOptions(ctx context.Context, db *mongo.Database, name string) (interface{}, error) {
	var res struct {
		Cursor struct {
			FirstBatch []struct {
				Options struct {
					Validator        bson.D `bson:"validator"`
					ValidationLevel  string `bson:"validationLevel"`
					ValidationAction string `bson:"validationAction"`
				} `bson:"options"`
			} `bson:"firstBatch"`
		} `bson:"cursor"`
	}
	if err := db.RunCommand(ctx, bson.D{
		{Key: "listCollections", Value: 1},
		{Key: "filter", Value: bson.D{{Key: "name", Value: name}}},
	}).Decode(&res); err != nil {
		return nil, err
	}
	if len(res.Cursor.FirstBatch) != 1 {
		return bson.D{{Key: "found", Value: false}}, nil
	}
	o := res.Cursor.FirstBatch[0].Options
	return bson.D{
		{Key: "validator", Value: o.Validator},
		{Key: "validationLevel", Value: o.ValidationLevel},
		{Key: "validationAction", Value: o.ValidationAction},
	}, nil
}

func TestValidatorOptions_ValidatorOnly(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ValidatorOptions_ValidatorOnly",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			name := col.Name() + "_optdefault"
			if err := col.Database().CreateCollection(ctx, name,
				options.CreateCollection().SetValidator(veNonNegAge)); err != nil {
				return nil, err
			}
			return veListValidatorOptions(ctx, col.Database(), name)
		},
	})
}

func TestValidatorOptions_ExplicitPreserved(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ValidatorOptions_ExplicitPreserved",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			name := col.Name() + "_optexplicit"
			if err := col.Database().CreateCollection(ctx, name, options.CreateCollection().
				SetValidator(veNonNegAge).SetValidationLevel("moderate").SetValidationAction("warn")); err != nil {
				return nil, err
			}
			return veListValidatorOptions(ctx, col.Database(), name)
		},
	})
}

func TestValidatorOptions_JsonSchemaValidatorOnly(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ValidatorOptions_JsonSchemaValidatorOnly",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			name := col.Name() + "_optjsonschema"
			if err := col.Database().CreateCollection(ctx, name,
				options.CreateCollection().SetValidator(veReqName)); err != nil {
				return nil, err
			}
			return veListValidatorOptions(ctx, col.Database(), name)
		},
	})
}
