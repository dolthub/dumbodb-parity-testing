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
			return bson.D{
				{Key: "gotError", Value: err != nil},
				{Key: "errCode", Value: errCode(err)},
			}, nil
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
			return bson.D{
				{Key: "gotError", Value: err != nil},
				{Key: "errCode", Value: errCode(err)},
			}, nil
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
			return bson.D{
				{Key: "gotError", Value: err != nil},
				{Key: "errCode", Value: errCode(err)},
			}, nil
		},
	})
}

