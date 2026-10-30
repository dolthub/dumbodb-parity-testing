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
	"math"
	"testing"

	"github.com/dolthub/dumbodb-parity-testing/harness"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// resetLockTimeout returns a closure that restores
// maxTransactionLockRequestTimeoutMillis to the documented MongoDB 8.0 default
// of 5ms. setParameter mutates server-wide state, so every test that changes
// it must restore it on exit even when the test body errors.
func resetLockTimeout(ctx context.Context, client *mongo.Client) func() {
	return func() {
		_ = client.Database("admin").RunCommand(ctx, bson.D{
			{Key: "setParameter", Value: 1},
			{Key: "maxTransactionLockRequestTimeoutMillis", Value: int32(5)},
		}).Err()
	}
}

func TestSetParameter_RoundTrip(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetParameter_RoundTrip",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			client := col.Database().Client()
			admin := client.Database("admin")
			defer resetLockTimeout(ctx, client)()

			var setDoc bson.M
			if err := admin.RunCommand(ctx, bson.D{
				{Key: "setParameter", Value: 1},
				{Key: "maxTransactionLockRequestTimeoutMillis", Value: int32(5000)},
			}).Decode(&setDoc); err != nil {
				return nil, err
			}

			var getDoc bson.M
			if err := admin.RunCommand(ctx, bson.D{
				{Key: "getParameter", Value: 1},
				{Key: "maxTransactionLockRequestTimeoutMillis", Value: 1},
			}).Decode(&getDoc); err != nil {
				return nil, err
			}

			return bson.D{
				{Key: "setWas", Value: setDoc["was"]},
				{Key: "setOK", Value: setDoc["ok"]},
				{Key: "getValue", Value: getDoc["maxTransactionLockRequestTimeoutMillis"]},
				{Key: "getOK", Value: getDoc["ok"]},
			}, nil
		},
	})
}

func TestSetParameter_UnknownParameter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetParameter_UnknownParameter",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			err := col.Database().Client().Database("admin").RunCommand(ctx, bson.D{
				{Key: "setParameter", Value: 1},
				{Key: "notARealParameter_xyz", Value: int32(1)},
			}).Err()
			return nil, err
		},
	})
}

func TestSetParameter_NotRuntimeSettable(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetParameter_NotRuntimeSettable",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			err := col.Database().Client().Database("admin").RunCommand(ctx, bson.D{
				{Key: "setParameter", Value: 1},
				{Key: "featureCompatibilityVersion", Value: "8.0"},
			}).Err()
			return nil, err
		},
	})
}

func TestSetParameter_NonAdminDB(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetParameter_NonAdminDB",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			err := col.Database().RunCommand(ctx, bson.D{
				{Key: "setParameter", Value: 1},
				{Key: "maxTransactionLockRequestTimeoutMillis", Value: int32(1)},
			}).Err()
			return nil, err
		},
	})
}

// TestSetParameter_ValueTypes sets parameters with values of other BSON
// types and reads back what the server stored (workspace-3g8).
func TestSetParameter_ValueTypes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		param string
		value interface{}
	}{
		{"Int_FromInt64", "maxTransactionLockRequestTimeoutMillis", int64(7)},
		{"Int_FromWholeDouble", "maxTransactionLockRequestTimeoutMillis", float64(7)},
		{"Int_FromFractionalDouble", "maxTransactionLockRequestTimeoutMillis", 7.5},
		{"Int_FromDecimal", "maxTransactionLockRequestTimeoutMillis", mustDecimal("7")},
		{"Int_FromString", "maxTransactionLockRequestTimeoutMillis", "7"},
		{"Int_FromBool", "maxTransactionLockRequestTimeoutMillis", true},
		{"Int_FromOutOfRangeInt64", "maxTransactionLockRequestTimeoutMillis", int64(1) << 40},
		{"Bool_FromInt", "quiet", int32(1)},
		{"Bool_FromZeroDouble", "quiet", float64(0)},
		{"Bool_FromString", "quiet", "true"},
		{"Bool_FromFalseString", "quiet", "false"},
		{"Bool_FromEmptyString", "quiet", ""},
		{"Bool_FromNull", "quiet", nil},
		{"Int_FromNegativeFraction", "maxTransactionLockRequestTimeoutMillis", -7.9},
		{"Int_FromNaN", "maxTransactionLockRequestTimeoutMillis", math.NaN()},
		{"Int_FromHugeDouble", "maxTransactionLockRequestTimeoutMillis", 1e300},
		{"Int_FromNull", "maxTransactionLockRequestTimeoutMillis", nil},
		{"Int_FromObject", "maxTransactionLockRequestTimeoutMillis", bson.D{{Key: "a", Value: int32(1)}}},
		{"Int_FromFractionalDecimal", "maxTransactionLockRequestTimeoutMillis", mustDecimal("7.9")},
		{"AuthSchemaVersion_FromInt64", "authSchemaVersion", int64(5)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "SetParameter_ValueTypes_" + tc.name,
				Support: harness.DumboDBFull,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					client := col.Database().Client()
					admin := client.Database("admin")
					defer resetLockTimeout(ctx, client)()
					defer func() {
						_ = admin.RunCommand(ctx, bson.D{{Key: "setParameter", Value: 1}, {Key: "quiet", Value: false}}).Err()
					}()

					var setDoc bson.M
					if err := admin.RunCommand(ctx, bson.D{
						{Key: "setParameter", Value: 1},
						{Key: tc.param, Value: tc.value},
					}).Decode(&setDoc); err != nil {
						return nil, err
					}

					var getDoc bson.M
					if err := admin.RunCommand(ctx, bson.D{
						{Key: "getParameter", Value: 1},
						{Key: tc.param, Value: 1},
					}).Decode(&getDoc); err != nil {
						return nil, err
					}

					return bson.D{
						{Key: "was", Value: setDoc["was"]},
						{Key: "value", Value: getDoc[tc.param]},
					}, nil
				},
			})
		})
	}
}
