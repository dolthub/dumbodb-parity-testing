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

func runAdminCommand(ctx context.Context, col *mongo.Collection, cmd bson.D) (interface{}, error) {
	result := col.Database().Client().Database("admin").RunCommand(ctx, cmd)
	var doc bson.D
	if err := result.Decode(&doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func TestGetParameter_AllUnknown(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetParameter_AllUnknown",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runAdminCommand(ctx, col, bson.D{
				{Key: "getParameter", Value: int32(1)},
				{Key: "internalQueryFacetBufferSizeBytes_notReal", Value: int32(1)},
			})
		},
	})
}

func TestGetParameter_Quiet(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetParameter_Quiet",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runAdminCommand(ctx, col, bson.D{
				{Key: "getParameter", Value: int32(1)},
				{Key: "quiet", Value: int32(1)},
			})
		},
	})
}

func TestGetParameter_KnownPlusUnknownSilentlyDropsUnknown(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetParameter_KnownPlusUnknownSilentlyDropsUnknown",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runAdminCommand(ctx, col, bson.D{
				{Key: "getParameter", Value: int32(1)},
				{Key: "quiet", Value: int32(1)},
				{Key: "totallyBogusXYZ", Value: int32(1)},
			})
		},
	})
}

func TestGetParameter_NonAdminRejected(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetParameter_NonAdminRejected",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			result := col.Database().RunCommand(ctx, bson.D{
				{Key: "getParameter", Value: int32(1)},
				{Key: "quiet", Value: int32(1)},
			})
			var doc bson.D
			if err := result.Decode(&doc); err != nil {
				return nil, err
			}
			return doc, nil
		},
	})
}
