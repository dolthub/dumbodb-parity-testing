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
