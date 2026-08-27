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
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// MongoDB caps a database name at 63 bytes. DumboDB splits "<db>@<rootish>"
// and bounds the two parts separately, so a rootish no longer counts against
// the database name -- that separation, not the larger cap, is what makes a
// revision addressable. DumboDB previously bounded the whole string at 63 and
// counted characters rather than bytes.
//
// Raising the base name to 128 bytes is a convenience on top of the split, and
// is the deviation these cases pin.
const (
	mongoMaxDBNameBytes   = 63
	dumboDBMaxDBNameBytes = 128
)

// dbNameOfLength builds a database name of exactly n bytes that is unique to
// this test run, so concurrent cases cannot collide.
func dbNameOfLength(ns string, n int) string {
	if len(ns) >= n {
		return ns[:n]
	}

	return ns + strings.Repeat("d", n-len(ns))
}

// multibyteDBNameOfBytes is dbNameOfLength padded with U+00E9 instead of ASCII.
// Each pad character is two bytes, so the result has roughly half as many
// characters as bytes; n is still the byte length.
func multibyteDBNameOfBytes(ns string, n int) string {
	if len(ns) >= n {
		return ns[:n]
	}

	name := ns + strings.Repeat("\u00e9", (n-len(ns))/2)
	if (n-len(ns))%2 == 1 {
		name += "d"
	}

	return name
}

// insertInto returns a Run that writes one document into the named database,
// which is what forces the server to validate the namespace.
func insertInto(name string) func(context.Context, harness.AuthTarget) (interface{}, error) {
	return func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		defer func() { _ = tgt.Admin.Database(name).Drop(ctx) }()

		_, err := tgt.Admin.Database(name).Collection("items").
			InsertOne(ctx, bson.D{{Key: "_id", Value: 1}})

		return nil, err
	}
}

func permitted(server string) func(t *testing.T, res interface{}, err error) {
	return func(t *testing.T, _ interface{}, err error) {
		t.Helper()
		if err != nil {
			t.Errorf("%s: expected the database name to be accepted, got %v", server, err)
		}
	}
}

func rejectedAsInvalidNamespace(server string) func(t *testing.T, res interface{}, err error) {
	return func(t *testing.T, _ interface{}, err error) {
		t.Helper()
		code, _, ok := harness.CommandErrorCode(err)
		if !ok {
			t.Errorf("%s: expected an InvalidNamespace error, got %v", server, err)
			return
		}
		if code != 73 {
			t.Errorf("%s: expected code 73 (InvalidNamespace), got %d (err=%v)", server, code, err)
		}
	}
}

// TestDBNameLengthDeviates pins DumboDB's deliberate deviation from MongoDB's
// 63-byte database name limit. See dumbodb/docs/verify/rootish.md.
func TestDBNameLengthDeviates(t *testing.T) {
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "DBNAME-01-over-mongo-limit",
		Support: harness.DumboDBDeviates,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			return insertInto(dbNameOfLength(tgt.NS, mongoMaxDBNameBytes+1))(ctx, tgt)
		},
		MongoExpect: rejectedAsInvalidNamespace("MongoDB"),
		DumboExpect: permitted("DumboDB"),
	})

	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "DBNAME-02-at-dumbodb-limit",
		Support: harness.DumboDBDeviates,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			return insertInto(dbNameOfLength(tgt.NS, dumboDBMaxDBNameBytes))(ctx, tgt)
		},
		MongoExpect: rejectedAsInvalidNamespace("MongoDB"),
		DumboExpect: permitted("DumboDB"),
	})

	// Past DumboDB's own limit both servers reject, but for different reasons
	// and with different messages, so each outcome is asserted on its own.
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "DBNAME-03-over-dumbodb-limit",
		Support: harness.DumboDBDeviates,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			return insertInto(dbNameOfLength(tgt.NS, dumboDBMaxDBNameBytes+1))(ctx, tgt)
		},
		MongoExpect: rejectedAsInvalidNamespace("MongoDB"),
		DumboExpect: rejectedAsInvalidNamespace("DumboDB"),
	})

	// The pad character is two bytes, so this name has fewer characters than
	// bytes. A server counting characters would accept it; both servers count
	// bytes, so MongoDB rejects it and DumboDB takes it only because it is
	// exactly at the cap.
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "DBNAME-04-multibyte-counts-as-bytes",
		Support: harness.DumboDBDeviates,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			return insertInto(multibyteDBNameOfBytes(tgt.NS, dumboDBMaxDBNameBytes))(ctx, tgt)
		},
		MongoExpect: rejectedAsInvalidNamespace("MongoDB"),
		DumboExpect: permitted("DumboDB"),
	})
}
