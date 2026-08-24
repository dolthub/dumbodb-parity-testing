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

// Reported-settings parity: for every accepted locale, create a collection with
// just {locale} and compare the collation document each server reports back
// (getCollectionInfos -> options.collation), field by field except `version`
// (which intentionally differs: DumboDB reports its real ICU version, MongoDB
// pins "57.1"). This is the reported-settings axis the order/equality sweeps do
// not cover: a locale's CLDR tailoring (e.g. fr_CA backwards=on, da caseFirst=
// upper) must be surfaced in the reported spec exactly as MongoDB surfaces it.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

const reportedSettingsDB = "parity_collation_reported_settings"

// reportedCollation creates a collection defaulted to {locale} and returns the
// collation document the server reports for it.
func reportedCollation(ctx context.Context, db *mongo.Database, locale string) (bson.M, error) {
	_ = db.Collection("probe").Drop(ctx)
	if err := db.CreateCollection(ctx, "probe",
		options.CreateCollection().SetCollation(&options.Collation{Locale: locale})); err != nil {
		return nil, err
	}
	cur, err := db.ListCollections(ctx, bson.D{{Key: "name", Value: "probe"}})
	if err != nil {
		return nil, err
	}
	var infos []bson.M
	if err := cur.All(ctx, &infos); err != nil {
		return nil, err
	}
	if len(infos) != 1 {
		return nil, fmt.Errorf("expected 1 collection info, got %d", len(infos))
	}
	opts, _ := infos[0]["options"].(bson.M)
	coll, _ := opts["collation"].(bson.M)
	return coll, nil
}

// diffExceptVersion reports the fields on which two collation documents differ,
// ignoring `version`.
func diffExceptVersion(m, d bson.M) []string {
	keys := map[string]struct{}{}
	for k := range m {
		keys[k] = struct{}{}
	}
	for k := range d {
		keys[k] = struct{}{}
	}
	var diffs []string
	for k := range keys {
		if k == "version" {
			continue
		}
		if fmt.Sprintf("%v", m[k]) != fmt.Sprintf("%v", d[k]) {
			diffs = append(diffs, fmt.Sprintf("%s(mongo=%v dumbo=%v)", k, m[k], d[k]))
		}
	}
	sort.Strings(diffs)
	return diffs
}

func TestCollationReportedSettingsSweep(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	clients, err := harness.GetClients(ctx)
	if err != nil {
		t.Fatalf("clients: %v", err)
	}
	mdb := clients.Mongo.Database(reportedSettingsDB)
	ddb := clients.DumboDB.Database(reportedSettingsDB)
	_ = mdb.Drop(ctx)
	_ = ddb.Drop(ctx)
	defer func() { _ = mdb.Drop(context.Background()); _ = ddb.Drop(context.Background()) }()

	var mismatches []string
	for _, loc := range sweepLocales {
		mColl, mErr := reportedCollation(ctx, mdb, loc)
		dColl, dErr := reportedCollation(ctx, ddb, loc)
		if mErr != nil || dErr != nil {
			mismatches = append(mismatches, fmt.Sprintf("%s: create/read error mongo=%v dumbo=%v", loc, mErr, dErr))
			continue
		}
		if diffs := diffExceptVersion(mColl, dColl); len(diffs) > 0 {
			mismatches = append(mismatches, fmt.Sprintf("%s: %s", loc, strings.Join(diffs, " ")))
		}
	}

	t.Logf("reported-settings sweep: %d locales | %d match, %d differ",
		len(sweepLocales), len(sweepLocales)-len(mismatches), len(mismatches))
	if len(mismatches) > 0 {
		sort.Strings(mismatches)
		t.Errorf("reported collation differs from MongoDB (ignoring version) for %d locales:\n%s",
			len(mismatches), strings.Join(mismatches, "\n"))
	}
}
