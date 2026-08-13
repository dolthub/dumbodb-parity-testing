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

// Locale coverage: every collation locale MongoDB 8.0 accepts, each probed at
// the ordering-relevant strengths against a live MongoDB. This is the O(locales)
// companion to TestCollationMatrix (which is O(options) on a few locales): the
// option surface behaves the same regardless of locale, so it is not re-run per
// locale here. Because MongoDB links ICU 57 and DumboDB links ICU 78, a locale
// whose CLDR collation rules changed between those versions legitimately
// diverges; such cells are recorded in the golden file as the known, reviewed
// inventory of the "modern Unicode" tradeoff -- not silent failures.
//
// Scope note: the shared corpus is diacritic-rich Latin, so this catches
// Latin-script ordering drift (where most of the 109 locales and most CLDR
// changes live). Script-specific tailoring (CJK/Thai/Arabic collators) is not
// exercised by this corpus and would need script-specific rows to probe.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

const sweepDB = "parity_collation_locale_sweep"

// sweepLocales mirrors dumbodb internal/collation/locales.go: the 109 ICU locale
// IDs MongoDB 8.0 accepts for collation.
var sweepLocales = []string{
	"af", "sq", "am", "ar", "hy", "as", "az", "bn", "be", "bs", "bs_Cyrl",
	"bg", "my", "ca", "chr", "zh", "zh_Hant", "hr", "cs", "da", "nl", "dz",
	"en", "en_US", "en_US_POSIX", "eo", "et", "ee", "fo", "fil", "fi", "fr",
	"fr_CA", "gl", "ka", "de", "de_AT", "el", "gu", "ha", "haw", "he", "hi",
	"hu", "is", "ig", "smn", "id", "ga", "it", "ja", "kl", "kn", "kk", "km",
	"kok", "ko", "ky", "lkt", "lo", "lv", "ln", "lt", "dsb", "lb", "mk", "ms",
	"ml", "mt", "mr", "mn", "ne", "se", "nb", "nn", "or", "om", "ps", "fa",
	"fa_AF", "pl", "pt", "pa", "ro", "ru", "sr", "sr_Latn", "si", "sk", "sl",
	"es", "sw", "sv", "ta", "te", "th", "bo", "to", "tr", "uk", "hsb", "ur",
	"ug", "vi", "wae", "cy", "yi", "yo", "zu",
}

// sweepStrengths are the ordering-relevant levels: primary (letter order),
// secondary (+ accents), tertiary (+ case, the default).
var sweepStrengths = []int{1, 2, 3}

func sweepDivergenceFile() string {
	return filepath.Join("testdata", "collation_locale_sweep_divergences.txt")
}

func TestCollationLocaleSweep(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	clients, err := harness.GetClients(ctx)
	if err != nil {
		t.Fatalf("clients: %v", err)
	}
	mcol := clients.Mongo.Database(sweepDB).Collection("corpus")
	dcol := clients.DumboDB.Database(sweepDB).Collection("corpus")
	_ = clients.Mongo.Database(sweepDB).Drop(ctx)
	_ = clients.DumboDB.Database(sweepDB).Drop(ctx)
	defer func() {
		_ = clients.Mongo.Database(sweepDB).Drop(context.Background())
		_ = clients.DumboDB.Database(sweepDB).Drop(context.Background())
	}()

	docs := corpusDocs()
	if _, err := mcol.InsertMany(ctx, docs); err != nil {
		t.Fatalf("seed mongo: %v", err)
	}
	if _, err := dcol.InsertMany(ctx, docs); err != nil {
		t.Fatalf("seed dumbodb: %v", err)
	}

	// diverged maps "locale|sN" -> a short reason ("order", "eq", "order+eq",
	// or "error-asymmetry"). Both servers agreeing (including both rejecting a
	// locale) is a match and is not recorded.
	diverged := map[string]string{}
	var checked, matched int
	for _, loc := range sweepLocales {
		for _, s := range sweepStrengths {
			spec := &options.Collation{Locale: loc, Strength: s}
			key := fmt.Sprintf("%s|s%d", loc, s)
			checked++

			mOrder, mErr := orderSig(ctx, mcol, spec)
			dOrder, dErr := orderSig(ctx, dcol, spec)
			if mErr != nil || dErr != nil {
				// Both erroring means both reject the collation: agreement.
				if (mErr == nil) != (dErr == nil) {
					diverged[key] = "error-asymmetry"
				} else {
					matched++
				}
				continue
			}
			mEq, _ := eqSig(ctx, mcol, spec)
			dEq, _ := eqSig(ctx, dcol, spec)

			var reasons []string
			if mOrder != dOrder {
				reasons = append(reasons, "order")
			}
			if mEq != dEq {
				reasons = append(reasons, "eq")
			}
			if len(reasons) == 0 {
				matched++
				continue
			}
			diverged[key] = strings.Join(reasons, "+")
		}
	}

	t.Logf("locale sweep: %d (locale,strength) cells over %d locales | %d match, %d diverge",
		checked, len(sweepLocales), matched, len(diverged))

	keys := make([]string, 0, len(diverged))
	for k := range diverged {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("  diverge %-16s %s", k, diverged[k])
	}

	if os.Getenv("UPDATE_SWEEP") == "1" {
		writeSweepDivergences(t, diverged, keys)
		t.Logf("wrote %s with %d diverging cells", sweepDivergenceFile(), len(keys))
		return
	}

	expected := loadSweepDivergences(t)
	var regressions, promotions []string
	for k := range diverged {
		if _, ok := expected[k]; !ok {
			regressions = append(regressions, k)
		}
	}
	for k := range expected {
		if _, ok := diverged[k]; !ok {
			promotions = append(promotions, k)
		}
	}
	sort.Strings(regressions)
	sort.Strings(promotions)
	if len(regressions) > 0 {
		t.Errorf("NEW divergences not in golden (rerun with UPDATE_SWEEP=1 to record): %v", regressions)
	}
	if len(promotions) > 0 {
		t.Errorf("golden lists divergences that no longer occur (rerun with UPDATE_SWEEP=1): %v", promotions)
	}
}

func writeSweepDivergences(t *testing.T, diverged map[string]string, sortedKeys []string) {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "# Collation locale-sweep divergences: (locale|strength) cells where DumboDB (ICU 78)\n")
	fmt.Fprintf(&b, "# differs from MongoDB (ICU 57) on order and/or equality of the shared corpus.\n")
	fmt.Fprintf(&b, "# Generated by TestCollationLocaleSweep with UPDATE_SWEEP=1. %d diverging cells.\n", len(sortedKeys))
	fmt.Fprintf(&b, "# Format: locale|strength<TAB>reason(order|eq|order+eq|error-asymmetry)\n")
	for _, k := range sortedKeys {
		fmt.Fprintf(&b, "%s\t%s\n", k, diverged[k])
	}
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatalf("mkdir testdata: %v", err)
	}
	if err := os.WriteFile(sweepDivergenceFile(), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("writing sweep divergence file: %v", err)
	}
}

func loadSweepDivergences(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	data, err := os.ReadFile(sweepDivergenceFile())
	if err != nil {
		if os.IsNotExist(err) {
			return out
		}
		t.Fatalf("reading sweep divergence file: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key := line
		if i := strings.IndexByte(line, '\t'); i >= 0 {
			key = line[:i]
		}
		out[key] = true
	}
	return out
}
