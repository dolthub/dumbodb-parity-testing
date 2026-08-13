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

// Combinatorial collation parity harness (design doc section 8).
//
// It enumerates the collation option space (with the proved no-op/validity prunes of
// section 8.3, each guarded by a witness in TestCollationMatrixPruningWitnesses),
// and for every cell diffs two observables against MongoDB: the total sort order
// and a curated equality signature. Both are read through find/find-sort, the
// only collation-carrying operations DumboDB accepts today (distinct and
// aggregate return NotImplemented), so a divergence reflects real behavior.
//
// The set of diverging cells is the contract: it is compared against the
// checked-in testdata/collation_matrix_divergences.txt. A cell that newly
// diverges (regression) or newly matches (an implemented option -- promote it)
// both fail the test until the file is regenerated with UPDATE_MATRIX=1.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

const matrixDB = "parity_collation_matrix"

// matrixLocales is en (baseline) plus tailored locales that move letter order or
// equality: de/fr/tr for accent and case tailoring, and es/da/sv for distinct-
// letter alphabets (Spanish n-tilde sorts after n; Danish/Swedish a-ring, ae,
// o-slash, o-umlaut sort after z). A full per-locale sweep of all 109 accepted
// locales is a separate task; this exercises the option surface plus the
// highest-signal letter-order tailorings.
var matrixLocales = []string{"en", "de", "fr", "tr", "es", "da", "sv"}

// matrixProbes are the equality-signature probe values, one per phenomenon whose
// equality classes an option can move (section 8.2). Each yields the set of ids
// that compare equal to it under the cell's collation.
var matrixProbes = []string{"a", "cafe", "resume", "ae", "black-bird", "cote"}

// corpusDocs is stored as explicit code points to keep the source 7-bit ASCII.
func corpusDocs() []interface{} {
	rows := []struct {
		id int
		s  string
	}{
		{1, "a"}, {2, "A"}, {3, "b"}, {4, "B"},
		{5, "cafe"}, {6, "CAFE"}, {7, "caf\u00e9"}, // cafe, CAFE, cafe-acute
		{8, "resume"}, {9, "RESUME"}, {10, "r\u00e9sum\u00e9"}, // resume, RESUME, resume-acute
		{11, "ae"}, {12, "\u00e4"}, {13, "AE"}, // ae, a-umlaut, AE
		{14, "black-bird"}, {15, "blackbird"}, {16, "black bird"},
		{17, "a1"}, {18, "a2"}, {19, "a10"},
		{20, "cote"}, {21, "cot\u00e9"}, {22, "c\u00f4te"}, {23, "c\u00f4t\u00e9"}, // cote quartet
		{24, "z"}, {25, "n"}, {26, "o"}, // anchors: Nordic letters sort after z, Spanish n-tilde after n
		{27, "\u00f1"}, {28, "nz"}, // n-tilde, nz -- Spanish: nz < n-tilde (distinct letter after n)
		{29, "\u00e5"}, {30, "aa"}, // a-ring, aa -- Danish: aa == a-ring, both after z
		{31, "\u00e6"}, {32, "\u00f8"}, // ae-ligature, o-slash -- Danish/Norwegian: after z
		{33, "\u00f6"}, // o-umlaut -- Swedish: after z; German: near o
	}
	docs := make([]interface{}, len(rows))
	for i, r := range rows {
		docs[i] = bson.D{{Key: "_id", Value: r.id}, {Key: "s", Value: r.s}}
	}
	return docs
}

// ccell is one point in the option grid.
type ccell struct {
	locale        string
	strength      int32
	caseLevel     bool
	caseFirst     string
	numeric       bool
	alternate     string
	maxVariable   string // "" unless alternate == shifted
	normalization bool
	backwards     bool
}

func (c ccell) collation() *options.Collation {
	col := &options.Collation{
		Locale:          c.locale,
		Strength:        int(c.strength),
		CaseLevel:       c.caseLevel,
		CaseFirst:       c.caseFirst,
		NumericOrdering: c.numeric,
		Alternate:       c.alternate,
		Normalization:   c.normalization,
		Backwards:       c.backwards,
	}
	if c.alternate == "shifted" {
		col.MaxVariable = c.maxVariable
	}
	return col
}

func (c ccell) key() string {
	mv := c.maxVariable
	if mv == "" {
		mv = "-"
	}
	return fmt.Sprintf("%s|s%d|cl%t|cf%s|no%t|al%s|mv%s|nm%t|bw%t",
		c.locale, c.strength, c.caseLevel, c.caseFirst, c.numeric,
		c.alternate, mv, c.normalization, c.backwards)
}

// generateGrid emits the pruned option grid (section 8.3). Every prune is a
// no-op or validity rule guarded by a witness test.
func generateGrid(locales []string) []ccell {
	bools := []bool{false, true}
	allCaseFirst := []string{"off", "upper", "lower"}
	alternates := []string{"non-ignorable", "shifted"}

	var cells []ccell
	for _, loc := range locales {
		for _, str := range []int32{1, 2, 3, 4, 5} {
			for _, cl := range bools {
				// caseFirst is invalid at strength 1-2 unless caseLevel is on
				// (MongoDB BadValue); enumerate it only where valid.
				caseFirsts := []string{"off"}
				if str >= 3 || cl {
					caseFirsts = allCaseFirst
				}
				for _, cf := range caseFirsts {
					for _, num := range bools {
						for _, alt := range alternates {
							// maxVariable is inert unless alternate is shifted.
							maxVars := []string{""}
							if alt == "shifted" {
								maxVars = []string{"punct", "space"}
							}
							for _, mv := range maxVars {
								for _, nm := range bools {
									// backwards is invalid at strength 1 (MongoDB BadValue);
									// enumerate it only at strength >= 2.
									bwOpts := []bool{false}
									if str >= 2 {
										bwOpts = bools
									}
									for _, bw := range bwOpts {
										cells = append(cells, ccell{
											locale: loc, strength: str, caseLevel: cl,
											caseFirst: cf, numeric: num, alternate: alt,
											maxVariable: mv, normalization: nm, backwards: bw,
										})
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return cells
}

func toInt(v interface{}) int {
	switch n := v.(type) {
	case int32:
		return int(n)
	case int64:
		return int(n)
	case int:
		return n
	case float64:
		return int(n)
	default:
		return -1
	}
}

// orderSig is the total sort order under the collation, with _id as a stable
// tie-break so equal-key runs are deterministic on both servers.
func orderSig(ctx context.Context, col *mongo.Collection, spec *options.Collation) (string, error) {
	cur, err := col.Find(ctx, bson.D{},
		options.Find().SetCollation(spec).SetSort(bson.D{{Key: "s", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return "", err
	}
	var docs []bson.D
	if err := cur.All(ctx, &docs); err != nil {
		return "", err
	}
	ids := make([]string, 0, len(docs))
	for _, d := range docs {
		ids = append(ids, fmt.Sprint(toInt(d.Map()["_id"])))
	}
	return strings.Join(ids, ","), nil
}

// eqSig is the equality signature: for each probe, the sorted set of ids that
// compare equal to it under the collation.
func eqSig(ctx context.Context, col *mongo.Collection, spec *options.Collation) (string, error) {
	var parts []string
	for _, v := range matrixProbes {
		cur, err := col.Find(ctx, bson.D{{Key: "s", Value: v}}, options.Find().SetCollation(spec))
		if err != nil {
			return "", err
		}
		var docs []bson.D
		if err := cur.All(ctx, &docs); err != nil {
			return "", err
		}
		ids := make([]int, 0, len(docs))
		for _, d := range docs {
			ids = append(ids, toInt(d.Map()["_id"]))
		}
		sort.Ints(ids)
		parts = append(parts, fmt.Sprintf("%s=%v", v, ids))
	}
	return strings.Join(parts, ";"), nil
}

// cellDims diffs the two observables independently (section 8.4 treats them as
// separate: equality is the high-severity one that drives find/unique, order
// drives sort). Each returns true when DumboDB matches MongoDB. A DumboDB error
// counts as a divergence on that dimension.
func cellDims(t *testing.T, ctx context.Context, mcol, dcol *mongo.Collection, spec *options.Collation) (orderMatch, eqMatch bool) {
	t.Helper()
	mOrder, err := orderSig(ctx, mcol, spec)
	if err != nil {
		t.Fatalf("mongo order (oracle) errored: %v", err)
	}
	mEq, err := eqSig(ctx, mcol, spec)
	if err != nil {
		t.Fatalf("mongo eq (oracle) errored: %v", err)
	}
	dOrder, dErr1 := orderSig(ctx, dcol, spec)
	dEq, dErr2 := eqSig(ctx, dcol, spec)
	orderMatch = dErr1 == nil && mOrder == dOrder
	eqMatch = dErr2 == nil && mEq == dEq
	return orderMatch, eqMatch
}

func matrixDivergenceFile() string {
	return filepath.Join("testdata", "collation_matrix_divergences.txt")
}

func loadExpectedDivergences(t *testing.T) map[string]bool {
	t.Helper()
	set := map[string]bool{}
	data, err := os.ReadFile(matrixDivergenceFile())
	if err != nil {
		if os.IsNotExist(err) {
			return set
		}
		t.Fatalf("reading divergence file: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		set[line] = true
	}
	return set
}

func writeExpectedDivergences(t *testing.T, keys []string) {
	t.Helper()
	sort.Strings(keys)
	var b strings.Builder
	fmt.Fprintf(&b, "# Collation matrix divergences: (cell, observable) pairs where DumboDB differs from MongoDB.\n")
	fmt.Fprintf(&b, "# Generated by TestCollationMatrix with UPDATE_MATRIX=1. %d diverging (cell, observable) pairs.\n", len(keys))
	fmt.Fprintf(&b, "# Key: locale|strength|caseLevel|caseFirst|numericOrdering|alternate|maxVariable|normalization|backwards|observable(order|eq)\n")
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("\n")
	}
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatalf("mkdir testdata: %v", err)
	}
	if err := os.WriteFile(matrixDivergenceFile(), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("writing divergence file: %v", err)
	}
}

func TestCollationMatrix(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	clients, err := harness.GetClients(ctx)
	if err != nil {
		t.Fatalf("clients: %v", err)
	}
	mcol := clients.Mongo.Database(matrixDB).Collection("corpus")
	dcol := clients.DumboDB.Database(matrixDB).Collection("corpus")
	_ = clients.Mongo.Database(matrixDB).Drop(ctx)
	_ = clients.DumboDB.Database(matrixDB).Drop(ctx)
	defer func() {
		_ = clients.Mongo.Database(matrixDB).Drop(context.Background())
		_ = clients.DumboDB.Database(matrixDB).Drop(context.Background())
	}()

	docs := corpusDocs()
	if _, err := mcol.InsertMany(ctx, docs); err != nil {
		t.Fatalf("seed mongo: %v", err)
	}
	if _, err := dcol.InsertMany(ctx, docs); err != nil {
		t.Fatalf("seed dumbodb: %v", err)
	}

	cells := generateGrid(matrixLocales)
	diverged := map[string]bool{} // entries are "KEY|order" and/or "KEY|eq"
	var fullMatch, orderDiv, eqDiv int
	for _, c := range cells {
		orderMatch, eqMatch := cellDims(t, ctx, mcol, dcol, c.collation())
		if !orderMatch {
			diverged[c.key()+"|order"] = true
			orderDiv++
		}
		if !eqMatch {
			diverged[c.key()+"|eq"] = true
			eqDiv++
		}
		if orderMatch && eqMatch {
			fullMatch++
		}
	}
	t.Logf("matrix: %d cells over %d locales | %d fully match, %d diverge on order, %d diverge on equality",
		len(cells), len(matrixLocales), fullMatch, orderDiv, eqDiv)

	divergedKeys := make([]string, 0, len(diverged))
	for k := range diverged {
		divergedKeys = append(divergedKeys, k)
	}

	if os.Getenv("UPDATE_MATRIX") == "1" {
		writeExpectedDivergences(t, divergedKeys)
		t.Logf("wrote %s with %d diverging cells", matrixDivergenceFile(), len(divergedKeys))
		return
	}

	expected := loadExpectedDivergences(t)
	var regressions, promotions []string
	for k := range diverged {
		if !expected[k] {
			regressions = append(regressions, k)
		}
	}
	for k := range expected {
		if _, still := diverged[k]; !still {
			promotions = append(promotions, k)
		}
	}
	sort.Strings(regressions)
	sort.Strings(promotions)

	if len(regressions) > 0 {
		t.Errorf("%d cell(s) newly diverge from MongoDB (regression). Re-run with UPDATE_MATRIX=1 if intended.\n  %s",
			len(regressions), strings.Join(regressions, "\n  "))
	}
	if len(promotions) > 0 {
		t.Errorf("%d cell(s) now MATCH MongoDB -- an option was implemented; regenerate with UPDATE_MATRIX=1 to promote them to Full.\n  %s",
			len(promotions), strings.Join(promotions, "\n  "))
	}
}

// TestCollationMatrixPruningWitnesses proves, against MongoDB, that each option
// the grid prunes is genuinely a no-op in the pruned region, so the grid is
// complete despite not enumerating those combinations.
func TestCollationMatrixPruningWitnesses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	clients, err := harness.GetClients(ctx)
	if err != nil {
		t.Fatalf("clients: %v", err)
	}
	col := clients.Mongo.Database(matrixDB + "_witness").Collection("corpus")
	_ = clients.Mongo.Database(matrixDB + "_witness").Drop(ctx)
	defer func() { _ = clients.Mongo.Database(matrixDB + "_witness").Drop(context.Background()) }()
	if _, err := col.InsertMany(ctx, corpusDocs()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// sameOrder proves a prune is a genuine no-op: MongoDB accepts both specs and
	// orders them identically.
	sameOrder := func(name string, a, b *options.Collation) {
		oa, ea := orderSig(ctx, col, a)
		ob, eb := orderSig(ctx, col, b)
		if ea != nil || eb != nil {
			t.Errorf("%s: MongoDB errored (a=%v b=%v)", name, ea, eb)
			return
		}
		if oa != ob {
			t.Errorf("%s: prune assumed a no-op but MongoDB order differs:\n a=%s\n b=%s", name, oa, ob)
		}
	}
	// rejected proves a prune is justified because the combination is invalid:
	// MongoDB rejects it, so enumerating it would be wrong.
	rejected := func(name string, spec *options.Collation) {
		if _, err := orderSig(ctx, col, spec); err == nil {
			t.Errorf("%s: expected MongoDB to reject this spec, but it accepted it", name)
		}
	}
	// accepted proves the combination the grid DOES enumerate is valid.
	accepted := func(name string, spec *options.Collation) {
		if _, err := orderSig(ctx, col, spec); err != nil {
			t.Errorf("%s: expected MongoDB to accept this spec, but: %v", name, err)
		}
	}

	// maxVariable is inert unless alternate == shifted.
	sameOrder("maxVariable-inert-nonignorable",
		&options.Collation{Locale: "en", Strength: 3, Alternate: "non-ignorable", MaxVariable: "punct"},
		&options.Collation{Locale: "en", Strength: 3, Alternate: "non-ignorable", MaxVariable: "space"})

	// caseFirst is invalid (not merely inert) at strength 1-2 without caseLevel,
	// so the grid excludes it there and enumerates it only where valid.
	rejected("caseFirst-invalid-strength2-nocaselevel",
		&options.Collation{Locale: "en", Strength: 2, CaseFirst: "upper"})
	accepted("caseFirst-valid-strength2-caselevel",
		&options.Collation{Locale: "en", Strength: 2, CaseLevel: true, CaseFirst: "upper"})
	accepted("caseFirst-valid-strength3",
		&options.Collation{Locale: "en", Strength: 3, CaseFirst: "upper"})

	// backwards is invalid (not merely inert) at strength 1, so the grid excludes
	// it there and enumerates it only at strength >= 2.
	rejected("backwards-invalid-strength1",
		&options.Collation{Locale: "en", Strength: 1, Backwards: true})
	accepted("backwards-valid-strength2",
		&options.Collation{Locale: "en", Strength: 2, Backwards: true})
}
