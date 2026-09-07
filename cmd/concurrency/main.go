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
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	concurrency "github.com/dolthub/dumbodb-parity-testing/concurrency"
)

func main() {
	var cfg concurrency.Config
	var outputPath string
	flag.StringVar(&cfg.TargetURI, "uri", "mongodb://localhost:27017", "MongoDB 8 target URI")
	flag.DurationVar(&cfg.Duration, "duration", concurrency.DefaultDuration, "maximum run duration")
	flag.Int64Var(&cfg.Operations, "operations", 0, "maximum operations; zero uses duration only")
	flag.IntVar(&cfg.Workers, "workers", 16, "concurrent workers")
	flag.Int64Var(&cfg.Seed, "seed", 1, "deterministic workload seed")
	flag.StringVar(&cfg.Database, "database", fmt.Sprintf("concurrency_%d", time.Now().UnixNano()), "isolated run database")
	flag.StringVar(&cfg.Collection, "collection", "documents", "run collection")
	flag.BoolVar(&cfg.KeepData, "keep-data", false, "retain the run database")
	flag.StringVar(&cfg.Scenario, "scenario", "cas", "scenario: cas, blind-inc, disjoint-set, or same-set")
	flag.IntVar(&cfg.PayloadBytes, "payload-bytes", 0, "padding bytes retained in the contended document")
	flag.StringVar(&outputPath, "output", "", "write indented JSON report to this path")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	scenario, err := concurrency.NewScenario(cfg.Scenario, cfg.Workers, cfg.PayloadBytes)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	result, err := concurrency.Run(ctx, cfg, scenario)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(result.Summary())
	output := os.Stdout
	if outputPath != "" {
		output, err = os.Create(outputPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer output.Close()
	}
	if err := result.WriteJSON(output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if !concurrency.ChecksPassed(result.Checks) {
		os.Exit(1)
	}
}
