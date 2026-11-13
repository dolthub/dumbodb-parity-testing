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
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// ErrServerFailure stops a run: a server stopped answering.
var ErrServerFailure = errors.New("a server stopped answering")

// Runner sends each case to both servers and hands divergences to Record.
type Runner struct {
	Mongo          *mongo.Database
	DumboDB        *mongo.Database
	CommandTimeout time.Duration
	// Minimize shrinks the dataset of a finding with a new signature before
	// it is recorded.
	Minimize bool
	Record   func(Finding)
	// IsNew reports whether no finding with this signature was recorded yet.
	IsNew func(signature string) bool
}

// Finding is one command whose DumboDB result differs from MongoDB's, with
// what it takes to reproduce it.
type Finding struct {
	Signature string   `json:"signature"`
	Class     Class    `json:"class"`
	Seed      uint64   `json:"seed"`
	Mode      Mode     `json:"mode"`
	Step      int      `json:"step"`
	Kind      string   `json:"kind"`
	Hint      string   `json:"hint"`
	Phase     string   `json:"phase"`
	Diff      string   `json:"diff"`
	Command   string   `json:"command"`
	MongoDB   string   `json:"mongodb"`
	DumboDB   string   `json:"dumbodb"`
	Minimized bool     `json:"minimized"`
	Repro     []bson.D `json:"-"`
}

// pairResult is a command's outcome on both servers and how they compare.
type pairResult struct {
	class        Class
	diff         string
	mongo, dumbo Outcome
	phase        string
	command      bson.D
}

func (r *Runner) both(ctx context.Context, phase string, cmd bson.D) pairResult {
	cctx, cancel := context.WithTimeout(ctx, r.CommandTimeout)
	defer cancel()
	m := Execute(cctx, r.Mongo, cmd)
	d := Execute(cctx, r.DumboDB, cmd)
	class, diff := Compare(m, d)
	return pairResult{class: class, diff: diff, mongo: m, dumbo: d, phase: phase, command: cmd}
}

// setupCommands drop and rebuild the collection.
func setupCommands(indexes, docs []bson.D) []bson.D {
	cmds := []bson.D{{{Key: "drop", Value: Collection}}}
	if len(indexes) > 0 {
		cmds = append(cmds, bson.D{{Key: "createIndexes", Value: Collection}, {Key: "indexes", Value: toArray(indexes)}})
	}
	return append(cmds, insertCommands(docs)...)
}

// resetCommands restore the documents without touching the indexes.
func resetCommands(docs []bson.D) []bson.D {
	cmds := []bson.D{{{Key: "delete", Value: Collection}, {Key: "deletes", Value: bson.A{
		bson.D{{Key: "q", Value: bson.D{}}, {Key: "limit", Value: 0}}}}}}
	return append(cmds, insertCommands(docs)...)
}

func insertCommands(docs []bson.D) []bson.D {
	if len(docs) == 0 {
		return nil
	}
	return []bson.D{{{Key: "insert", Value: Collection}, {Key: "documents", Value: toArray(docs)}}}
}

func toArray(ds []bson.D) bson.A {
	out := make(bson.A, len(ds))
	for i, d := range ds {
		out[i] = d
	}
	return out
}

// prepare runs setup commands. A drop of a missing collection fails on some
// servers and not others, so drop outcomes are not compared.
func (r *Runner) prepare(ctx context.Context, phase string, cmds []bson.D) (pairResult, bool) {
	for _, cmd := range cmds {
		res := r.both(ctx, phase, cmd)
		if cmd[0].Key == "drop" && res.class != ServerFailure {
			continue
		}
		if res.class != Match {
			return res, false
		}
	}
	return pairResult{}, true
}

// runStep runs one step on prepared data and returns its first divergence.
func (r *Runner) runStep(ctx context.Context, mode Mode, docs []bson.D, step Step) pairResult {
	if mode == Writes {
		if res, ok := r.prepare(ctx, "reset", resetCommands(docs)); !ok {
			return res
		}
	}
	res := r.both(ctx, "result", step.Command)
	if res.class != Match || step.Check == nil {
		return res
	}
	return r.both(ctx, "state", step.Check)
}

// RunCase runs every step of c. It returns ErrServerFailure, after recording
// it, when a server stops answering, and ctx's error once ctx ends; other
// divergences are only recorded. A divergence seen as ctx ends is dropped,
// since a cancelled command looks like a server failure.
func (r *Runner) RunCase(ctx context.Context, c Case) error {
	if err := r.setup(ctx, c); err != nil {
		return err
	}
	for i, step := range c.Steps {
		res := r.runStep(ctx, c.Mode, c.Docs, step)
		if err := ctx.Err(); err != nil {
			return err
		}
		if res.class == Match {
			continue
		}
		if res.class == ServerFailure {
			r.record(c, i, step, res, reproCommands(c.Indexes, c.Docs, step), false)
			return ErrServerFailure
		}
		indexes, docs, minimized := c.Indexes, c.Docs, false
		isNew := r.Minimize && r.IsNew(Signature(res.class, step))
		if isNew {
			if mi, md, ok := r.minimize(ctx, c.Indexes, c.Docs, step, res.class); ok {
				if again := r.replayStep(ctx, mi, md, step); again.class == res.class {
					indexes, docs, res, minimized = mi, md, again, true
				}
			}
		}
		r.record(c, i, step, res, reproCommands(indexes, docs, step), minimized)
		if isNew {
			if err := r.setup(ctx, c); err != nil {
				return err
			}
		}
	}
	return nil
}

// errSetupDiverged ends a case whose data differs between the servers.
var errSetupDiverged = errors.New("setup diverged")

// setup rebuilds c's data on both servers. A divergence is recorded and ends
// the case with ErrServerFailure or errSetupDiverged.
func (r *Runner) setup(ctx context.Context, c Case) error {
	res, ok := r.prepare(ctx, "setup", setupCommands(c.Indexes, c.Docs))
	if err := ctx.Err(); err != nil {
		return err
	}
	if ok {
		return nil
	}
	r.record(c, -1, Step{Kind: "setup"}, res, setupCommands(c.Indexes, c.Docs), false)
	if res.class == ServerFailure {
		return ErrServerFailure
	}
	return errSetupDiverged
}

func (r *Runner) record(c Case, i int, step Step, res pairResult, repro []bson.D, minimized bool) {
	r.Record(Finding{
		Signature: Signature(res.class, step),
		Class:     res.class,
		Seed:      c.Seed,
		Mode:      c.Mode,
		Step:      i,
		Kind:      step.Kind,
		Hint:      step.Hint,
		Phase:     res.phase,
		Diff:      res.diff,
		Command:   extJSON(res.command, false),
		MongoDB:   res.mongo.Render(),
		DumboDB:   res.dumbo.Render(),
		Minimized: minimized,
		Repro:     repro,
	})
}

// reproCommands rebuild the data and run the step, so replaying them alone
// shows the finding.
func reproCommands(indexes, docs []bson.D, step Step) []bson.D {
	cmds := setupCommands(indexes, docs)
	cmds = append(cmds, step.Command)
	if step.Check != nil {
		cmds = append(cmds, step.Check)
	}
	return cmds
}

// replayStep rebuilds the data and runs step, returning its divergence.
func (r *Runner) replayStep(ctx context.Context, indexes, docs []bson.D, step Step) pairResult {
	if res, ok := r.prepare(ctx, "setup", setupCommands(indexes, docs)); !ok {
		return res
	}
	res := r.both(ctx, "result", step.Command)
	if res.class != Match || step.Check == nil {
		return res
	}
	return r.both(ctx, "state", step.Check)
}

// maxMinimizeTrials bounds the time one new finding spends shrinking.
const maxMinimizeTrials = 300

// minimize keeps only the hinted index when that still reproduces want, then
// drops documents one at a time while the divergence keeps its class. It
// gives up when a server stops answering.
func (r *Runner) minimize(ctx context.Context, indexes, docs []bson.D, step Step, want Class) ([]bson.D, []bson.D, bool) {
	trials := 0
	failed := false
	reproduces := func(ix, ds []bson.D) bool {
		if failed || ctx.Err() != nil {
			trials = maxMinimizeTrials
			return false
		}
		trials++
		class := r.replayStep(ctx, ix, ds, step).class
		failed = class == ServerFailure
		return class == want
	}
	changed := false
	for _, ix := range indexes {
		if ix.Map()["name"] == step.Hint && len(indexes) > 1 {
			if only := []bson.D{ix}; reproduces(only, docs) {
				indexes, changed = only, true
			}
			break
		}
	}
	for progress := true; progress && trials < maxMinimizeTrials; {
		progress = false
		for i := 0; i < len(docs) && trials < maxMinimizeTrials; {
			trial := append(append([]bson.D{}, docs[:i]...), docs[i+1:]...)
			if reproduces(indexes, trial) {
				docs, progress, changed = trial, true, true
				continue
			}
			i++
		}
	}
	return indexes, docs, changed && !failed
}

// Describe is a one-line summary of f for logs.
func (f Finding) Describe() string {
	return fmt.Sprintf("seed %d %s step %d: %s %s hint=%s (%s)", f.Seed, f.Mode, f.Step, f.Class, f.Kind, f.Hint, f.Phase)
}
