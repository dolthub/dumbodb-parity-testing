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

package storage

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dirBytes returns the total byte count of all files under root.
func dirBytes(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}

// settledDirBytes measures the on-disk size of dir after garbage collection,
// re-running gc + measurement until a further pass no longer meaningfully
// shrinks the store (a fixpoint) or maxPasses is reached. A single GC + walk can
// catch a store that has not settled -- an incomplete collection or in-progress
// compaction under load leaves extra bytes on disk -- which is the most likely
// cause of the ~2x run-to-run swing that flaked the storage-budget tests
// (workspace-5fs). Reporting the smallest size a further GC cannot beat gives the
// settled, meaningful number. In the common case the store is already settled and
// this returns after the second, confirming pass; it logs to stderr when it needs
// more (or fails to settle), so a recurrence is diagnosable. It does NOT change
// the GC mode -- the default sweep, kept for parity with the Dolt baseline.
func settledDirBytes(maxPasses int, label string, gc func() error, dir string) (int64, error) {
	prev := int64(-1)
	for pass := 0; pass < maxPasses; pass++ {
		if err := gc(); err != nil {
			return 0, err
		}
		cur, err := dirBytes(dir)
		if err != nil {
			return 0, err
		}
		if prev >= 0 {
			tol := prev / 1000
			if tol < 64 {
				tol = 64
			}
			if cur >= prev-tol { // no meaningful further shrink -> settled
				if cur < prev {
					prev = cur
				}
				if pass > 1 {
					fmt.Fprintf(os.Stderr, "settledDirBytes(%s): settled at %d bytes after %d GC passes\n", label, prev, pass+1)
				}
				return prev, nil
			}
		}
		prev = cur
	}
	fmt.Fprintf(os.Stderr, "settledDirBytes(%s): did NOT settle in %d passes; last=%d bytes (store may not be quiescing -- investigate)\n", label, maxPasses, prev)
	return prev, nil
}

// fmtBytes formats a byte count as a human-readable string.
func fmtBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// printTable logs a formatted table to t using t.Log.
func printTable(t *testing.T, headers []string, rows [][]string) {
	t.Helper()
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	format := func(cells []string) string {
		var sb strings.Builder
		for i, cell := range cells {
			if i > 0 {
				sb.WriteString(" | ")
			}
			sb.WriteString(fmt.Sprintf("%-*s", widths[i], cell))
		}
		return sb.String()
	}

	sep := func() string {
		var sb strings.Builder
		for i, w := range widths {
			if i > 0 {
				sb.WriteString("-+-")
			}
			sb.WriteString(strings.Repeat("-", w))
		}
		return sb.String()
	}

	t.Log(format(headers))
	t.Log(sep())
	for _, row := range rows {
		t.Log(format(row))
	}
}
