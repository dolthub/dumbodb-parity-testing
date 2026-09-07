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

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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
}

func Run(ctx context.Context, cfg Config, scenario Scenario) (LifecycleResult, error) {
	if err := cfg.Validate(); err != nil {
		return LifecycleResult{}, err
	}
	if scenario == nil {
		return LifecycleResult{}, fmt.Errorf("scenario is required")
	}

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.TargetURI))
	if err != nil {
		return LifecycleResult{}, fmt.Errorf("connect: %w", err)
	}
	defer client.Disconnect(context.Background())

	if err := client.Ping(ctx, nil); err != nil {
		return LifecycleResult{}, fmt.Errorf("ping: %w", err)
	}
	identity, err := serverIdentity(ctx, client)
	if err != nil {
		return LifecycleResult{}, err
	}

	database := client.Database(cfg.Database)
	collection := database.Collection(cfg.Collection)
	if !cfg.KeepData {
		defer func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = database.Drop(cleanupCtx)
		}()
	}

	runCtx := ctx
	cancel := func() {}
	if cfg.Duration > 0 {
		runCtx, cancel = context.WithTimeout(ctx, cfg.Duration)
	}
	defer cancel()

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
				case <-runCtx.Done():
					return
				default:
				}
				sequence := issued.Add(1)
				if cfg.Operations > 0 && sequence > cfg.Operations {
					return
				}
				started := time.Now()
				outcome := scenario.Execute(runCtx, collection, id, sequence)
				if err := ledger.Record(outcome, time.Since(started)); err != nil {
					return
				}
				if runCtx.Err() != nil {
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

func serverIdentity(ctx context.Context, client *mongo.Client) (serverInfo, error) {
	var response bson.M
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&response); err != nil {
		return serverInfo{}, fmt.Errorf("buildInfo: %w", err)
	}
	version, _ := response["version"].(string)
	if version == "" {
		return serverInfo{}, fmt.Errorf("buildInfo did not return a version")
	}
	return serverInfo{Product: "MongoDB", Version: version}, nil
}
