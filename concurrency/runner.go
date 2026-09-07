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
	Product            string
	Version            string
	Scenario           string
	StartedAt          time.Time
	FinishedAt         time.Time
	WorkloadStartedAt  time.Time
	WorkloadFinishedAt time.Time
	Ledger             LedgerSnapshot
	Checks             []Check
	Config             RunConfig
	Statistics         RunStatistics
	Truncated          bool
	StopReason         string
	target             Target
	database           string
	keepData           bool
}

func (r *LifecycleResult) Finalize(ctx context.Context, preserve bool) error {
	if r.target == nil {
		return nil
	}
	var dropErr error
	if !preserve && !r.keepData {
		dropErr = r.target.DropDatabase(ctx, r.database)
	}
	disconnectErr := r.target.Disconnect(ctx)
	r.target = nil
	if dropErr != nil {
		return dropErr
	}
	return disconnectErr
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
	if err := target.Ping(ctx); err != nil {
		_ = target.Disconnect(context.Background())
		return LifecycleResult{}, fmt.Errorf("ping: %w", err)
	}
	identity, err := target.Identity(ctx)
	if err != nil {
		_ = target.Disconnect(context.Background())
		return LifecycleResult{}, err
	}

	collection := target.Collection(cfg.Database, cfg.Collection)

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
			CASDelay:     cfg.CASDelay.String(),
			LatencyScope: scenarioLatencyScope(scenario.Name()),
		},
		target:   target,
		database: cfg.Database,
		keepData: cfg.KeepData,
	}
	if err := scenario.Setup(ctx, collection); err != nil {
		return result, fmt.Errorf("setup %s: %w", scenario.Name(), err)
	}
	var issued atomic.Int64
	ledger := &Ledger{}
	var workers sync.WaitGroup
	var recordErr error
	var recordErrOnce sync.Once
	stop := make(chan struct{})
	result.WorkloadStartedAt = time.Now().UTC()
	var deadline time.Time
	if cfg.Duration > 0 {
		deadline = result.WorkloadStartedAt.Add(cfg.Duration)
	}
	workers.Add(cfg.Workers)
	for workerID := 0; workerID < cfg.Workers; workerID++ {
		go func(id int) {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case <-stop:
					return
				default:
				}
				if !deadline.IsZero() && time.Now().After(deadline) {
					return
				}
				sequence, ok := reserveOperation(&issued, cfg.Operations)
				if !ok {
					return
				}
				started := time.Now()
				outcome := scenario.Execute(ctx, collection, id, sequence)
				if err := ledger.Record(outcome, time.Since(started)); err != nil {
					recordErrOnce.Do(func() {
						recordErr = fmt.Errorf("record operation %d: %w", sequence, err)
						close(stop)
					})
					return
				}
				if ctx.Err() != nil {
					return
				}
			}
		}(workerID)
	}
	workers.Wait()
	result.WorkloadFinishedAt = time.Now().UTC()
	result.Truncated = ctx.Err() != nil
	if result.Truncated {
		result.StopReason = ctx.Err().Error()
	}

	if recordErr != nil {
		return result, recordErr
	}
	result.Ledger = ledger.Snapshot()
	if reserved := issued.Load(); reserved != result.Ledger.Attempts {
		return result, fmt.Errorf(
			"reserved operations %d != recorded attempts %d",
			reserved,
			result.Ledger.Attempts,
		)
	}
	result.Statistics = calculateStatistics(result.Ledger)
	if err := result.Ledger.Validate(); err != nil {
		return result, err
	}
	verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer verifyCancel()
	if result.Truncated {
		timer := time.NewTimer(time.Second)
		select {
		case <-timer.C:
		case <-verifyCtx.Done():
			timer.Stop()
			return result, verifyCtx.Err()
		}
	}
	checks, err := scenario.Verify(verifyCtx, collection, result.Ledger)
	if err != nil {
		return result, fmt.Errorf("verify %s: %w", scenario.Name(), err)
	}
	result.Checks = checks
	result.FinishedAt = time.Now().UTC()
	return result, nil
}

func scenarioLatencyScope(name string) string {
	if name == "cas" || name == "uuid-cas" {
		return "read-and-update"
	}
	return "update"
}

func reserveOperation(issued *atomic.Int64, limit int64) (int64, bool) {
	if limit <= 0 {
		return issued.Add(1), true
	}
	for {
		current := issued.Load()
		if current >= limit {
			return 0, false
		}
		if issued.CompareAndSwap(current, current+1) {
			return current + 1, true
		}
	}
}

type serverInfo struct {
	Product string
	Version string
}
