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
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name string, n int) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), make([]byte, n), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSettledDirBytes_StableStore(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", 1000)
	calls := 0
	got, err := settledDirBytes(4, "test", func() error { calls++; return nil }, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1000 {
		t.Fatalf("got %d, want 1000", got)
	}
	if calls != 2 { // one measure + one confirming pass
		t.Fatalf("calls=%d, want 2 (settles on the confirming pass)", calls)
	}
}

// The anomaly the fix targets: the first measurement catches an unsettled store
// (extra bytes), a further GC settles it. settledDirBytes must report the
// settled size, not the transient one.
func TestSettledDirBytes_DiscardsUnsettledFirstMeasurement(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a", 1000)
	writeFile(t, dir, "unsettled", 900) // first walk sees 1900
	calls := 0
	gc := func() error {
		calls++
		if calls >= 2 {
			_ = os.Remove(filepath.Join(dir, "unsettled")) // GC settles to 1000
		}
		return nil
	}
	got, err := settledDirBytes(4, "test", gc, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1000 {
		t.Fatalf("got %d, want settled 1000 (not the 1900 transient)", got)
	}
}

func TestSettledDirBytes_GCErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	if _, err := settledDirBytes(4, "test", func() error { return errors.New("boom") }, dir); err == nil {
		t.Fatal("want error from gc, got nil")
	}
}
