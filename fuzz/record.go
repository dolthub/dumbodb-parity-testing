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

package fuzz

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"go.mongodb.org/mongo-driver/bson"
)

// Group is every finding with one signature.
type Group struct {
	Signature string
	Class     Class
	Count     int
	First     Finding
	ReproPath string
}

// Recorder writes every finding to findings.jsonl in Dir and, for the first
// finding of each signature, a repro file under Dir/repro.
type Recorder struct {
	Dir string
	Log io.Writer

	mu       sync.Mutex
	jsonl    *os.File
	groups   map[string]*Group
	order    []string
	writeErr error
}

func NewRecorder(dir string, log io.Writer) (*Recorder, error) {
	if err := os.MkdirAll(filepath.Join(dir, "repro"), 0o755); err != nil {
		return nil, err
	}
	f, err := os.Create(filepath.Join(dir, "findings.jsonl"))
	if err != nil {
		return nil, err
	}
	return &Recorder{Dir: dir, Log: log, jsonl: f, groups: map[string]*Group{}}, nil
}

func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.jsonl.Close()
}

func (r *Recorder) IsNew(signature string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.groups[signature] == nil
}

func (r *Recorder) Record(f Finding) {
	r.mu.Lock()
	defer r.mu.Unlock()
	line, _ := json.Marshal(f)
	if _, err := r.jsonl.Write(append(line, '\n')); err != nil && r.writeErr == nil {
		r.writeErr = err
	}
	g := r.groups[f.Signature]
	if g != nil {
		g.Count++
		return
	}
	g = &Group{Signature: f.Signature, Class: f.Class, Count: 1, First: f}
	r.groups[f.Signature] = g
	r.order = append(r.order, f.Signature)
	g.ReproPath = filepath.Join(r.Dir, "repro", fmt.Sprintf("%04d-%s.txt", len(r.order), f.Class))
	if err := os.WriteFile(g.ReproPath, []byte(ReproText(f)), 0o644); err != nil && r.writeErr == nil {
		r.writeErr = err
	}
	fmt.Fprintf(r.Log, "NEW %s -> %s\n", f.Describe(), g.ReproPath)
}

// WriteErr is the first error writing findings, if any.
func (r *Recorder) WriteErr() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.writeErr
}

// Groups returns the groups, most frequent first.
func (r *Recorder) Groups() []Group {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Group, 0, len(r.order))
	for _, sig := range r.order {
		out = append(out, *r.groups[sig])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

// ReproText is a replayable file: comment lines describing f, then one
// canonical Extended JSON command per line.
func ReproText(f Finding) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s %s hint=%s, %s phase\n", f.Class, f.Kind, f.Hint, f.Phase)
	fmt.Fprintf(&b, "# signature: %s\n", f.Signature)
	fmt.Fprintf(&b, "# seed %d, %s, step %d, minimized %v\n", f.Seed, f.Mode, f.Step, f.Minimized)
	fmt.Fprintf(&b, "# diff: %s\n", oneLine(f.Diff))
	fmt.Fprintf(&b, "# command: %s\n", f.Command)
	fmt.Fprintf(&b, "# mongodb: %s\n", oneLine(f.MongoDB))
	fmt.Fprintf(&b, "# dumbodb: %s\n", oneLine(f.DumboDB))
	b.WriteString("# replay: go run ./cmd/fuzz -replay <this file>\n")
	for _, cmd := range f.Repro {
		b.WriteString(extJSON(cmd, true))
		b.WriteByte('\n')
	}
	return b.String()
}

func oneLine(s string) string { return strings.ReplaceAll(s, "\n", " ") }

// ReadRepro parses a repro file, skipping blank and comment lines.
func ReadRepro(path string) ([]bson.D, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cmds []bson.D
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var cmd bson.D
		if err := bson.UnmarshalExtJSON([]byte(line), false, &cmd); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, n, err)
		}
		cmds = append(cmds, cmd)
	}
	return cmds, sc.Err()
}

// Replay runs cmds on both servers, printing each command and how the two
// answers compare, and returns how many diverged. Drop results are not
// compared.
func (r *Runner) Replay(ctx context.Context, cmds []bson.D, w io.Writer) int {
	diverged := 0
	for _, cmd := range cmds {
		res := r.both(ctx, "replay", cmd)
		fmt.Fprintf(w, "> %s\n", extJSON(cmd, false))
		if res.class == Match || (cmd[0].Key == "drop" && res.class != ServerFailure) {
			fmt.Fprintf(w, "  same: %s\n", res.mongo.Render())
			continue
		}
		diverged++
		fmt.Fprintf(w, "  %s: %s\n  mongodb: %s\n  dumbodb: %s\n", strings.ToUpper(string(res.class)), res.diff,
			res.mongo.Render(), res.dumbo.Render())
	}
	return diverged
}
