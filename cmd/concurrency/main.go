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

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	concurrency "github.com/dolthub/dumbodb-parity-testing/concurrency"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func main() {
	var cfg concurrency.Config
	flag.StringVar(&cfg.TargetURI, "uri", "mongodb://localhost:27017", "MongoDB 8 target URI")
	flag.DurationVar(&cfg.Duration, "duration", concurrency.DefaultDuration, "maximum run duration")
	flag.Int64Var(&cfg.Operations, "operations", 0, "maximum operations; zero uses duration only")
	flag.IntVar(&cfg.Workers, "workers", 16, "concurrent workers")
	flag.Int64Var(&cfg.Seed, "seed", 1, "deterministic workload seed")
	flag.StringVar(&cfg.Database, "database", fmt.Sprintf("concurrency_%d", time.Now().UnixNano()), "isolated run database")
	flag.StringVar(&cfg.Collection, "collection", "documents", "run collection")
	flag.BoolVar(&cfg.KeepData, "keep-data", false, "retain the run database")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	result, err := concurrency.Run(ctx, cfg, pingOperation)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func pingOperation(ctx context.Context, collection *mongo.Collection, _ int, _ int64) concurrency.Outcome {
	err := collection.Database().RunCommand(ctx, bson.D{{Key: "ping", Value: 1}}).Err()
	if err != nil {
		return concurrency.Outcome{Kind: concurrency.OutcomeClientError, Err: err}
	}
	return concurrency.Outcome{Kind: concurrency.OutcomeMatched}
}
