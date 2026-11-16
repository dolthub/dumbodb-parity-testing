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

// writeOutcome runs write and returns its error code with the collection's
// documents after it, so a case compares both what failed and what stayed.
func writeOutcome(ctx context.Context, col *mongo.Collection, write func() error) (interface{}, error) {
	code := errCode(write())
	docs, err := allDocsByID(ctx, col)
	if err != nil {
		return nil, err
	}
	return bson.D{{Key: "code", Value: code}, {Key: "docs", Value: docs}}, nil
}

// supportFor is DumboDBXFail for a test xfail lists by name and DumboDBFull
// otherwise.
func supportFor(t *testing.T, xfail map[string]bool) harness.DumboDBSupport {
	if xfail[t.Name()] {
		return harness.DumboDBXFail
	}
	return harness.DumboDBFull
}
