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

package concurrency

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type LifecycleResult struct {
	Product    string
	Version    string
	Scenario   string
	StartedAt  time.Time
	FinishedAt time.Time
	Attempts   int64
	Ledger     LedgerSnapshot
	Checks     []Check
	Config     RunConfig
	Statistics RunStatistics
}

func Run(ctx context.Context, cfg Config, scenario Scenario) (LifecycleResult, error) {
	if err := cfg.Validate(); err != nil {
		return LifecycleResult{}, err
	}
	if scenario == nil {
		return LifecycleResult{}, fmt.Errorf("scenario is required")
	}

	target, err := connectMongoTarget(ctx, cfg.TargetURI)
	if err != nil {
		return LifecycleResult{}, fmt.Errorf("connect: %w", err)
	}
	return RunWithTarget(ctx, cfg, scenario, target)
}

func RunWithTarget(ctx context.Context, cfg Config, scenario Scenario, target Target) (LifecycleResult, error) {
	if err := cfg.Validate(); err != nil {
		return LifecycleResult{}, err
	}
	if scenario == nil {
		return LifecycleResult{}, fmt.Errorf("scenario is required")
	}
	if target == nil {
		return LifecycleResult{}, fmt.Errorf("target is required")
	}
	defer target.Disconnect(context.Background())

	if err := target.Ping(ctx); err != nil {
		return LifecycleResult{}, fmt.Errorf("ping: %w", err)
	}
	identity, err := target.Identity(ctx)
	if err != nil {
		return LifecycleResult{}, err
	}

	collection := target.Collection(cfg.Database, cfg.Collection)
	if !cfg.KeepData {
		defer func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = target.DropDatabase(cleanupCtx, cfg.Database)
		}()
	}

	var deadline time.Time
	if cfg.Duration > 0 {
		deadline = time.Now().Add(cfg.Duration)
	}

	result := LifecycleResult{
		Product:   identity.Product,
		Version:   identity.Version,
		Scenario:  scenario.Name(),
		StartedAt: time.Now().UTC(),
		Config: RunConfig{
			Target:       sanitizedTarget(cfg.TargetURI),
			Duration:     cfg.Duration.String(),
			Operations:   cfg.Operations,
			Workers:      cfg.Workers,
			Seed:         cfg.Seed,
			Database:     cfg.Database,
			Collection:   cfg.Collection,
			PayloadBytes: cfg.PayloadBytes,
		},
	}
	if err := scenario.Setup(ctx, collection); err != nil {
		return LifecycleResult{}, fmt.Errorf("setup %s: %w", scenario.Name(), err)
	}
	var issued atomic.Int64
	ledger := &Ledger{}
	var workers sync.WaitGroup
	workers.Add(cfg.Workers)
	for workerID := 0; workerID < cfg.Workers; workerID++ {
		go func(id int) {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				if !deadline.IsZero() && time.Now().After(deadline) {
					return
				}
				sequence := issued.Add(1)
				if cfg.Operations > 0 && sequence > cfg.Operations {
					return
				}
				started := time.Now()
				outcome := scenario.Execute(ctx, collection, id, sequence)
				if err := ledger.Record(outcome, time.Since(started)); err != nil {
					return
				}
				if ctx.Err() != nil {
					return
				}
			}
		}(workerID)
	}
	workers.Wait()

	result.Attempts = issued.Load()
	if cfg.Operations > 0 && result.Attempts > cfg.Operations {
		result.Attempts = cfg.Operations
	}
	result.Ledger = ledger.Snapshot()
	result.Statistics = calculateStatistics(result.Ledger)
	if err := result.Ledger.Validate(); err != nil {
		return LifecycleResult{}, err
	}
	checks, err := scenario.Verify(ctx, collection, result.Ledger)
	if err != nil {
		return LifecycleResult{}, fmt.Errorf("verify %s: %w", scenario.Name(), err)
	}
	result.Checks = checks
	result.FinishedAt = time.Now().UTC()
	return result, nil
}

type serverInfo struct {
	Product string
	Version string
}
