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
	flag.StringVar(&cfg.Scenario, "scenario", "cas", "scenario: cas, uuid-cas, blind-inc, disjoint-set, or same-set")
	flag.IntVar(&cfg.PayloadBytes, "payload-bytes", 0, "padding bytes retained in the contended document")
	flag.DurationVar(&cfg.CASDelay, "cas-delay", 0, "maximum deterministic delay between CAS read and update")
	flag.StringVar(&outputPath, "output", "", "write indented JSON report to this path")
	flag.Parse()
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	scenario, err := concurrency.NewScenarioWithWorkload(cfg.Scenario, cfg.Workers, cfg.PayloadBytes, cfg.Seed, cfg.CASDelay)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	result, runErr := concurrency.Run(ctx, cfg, scenario)
	if result.Product == "" {
		fmt.Fprintln(os.Stderr, runErr)
		os.Exit(1)
	}
	fmt.Println(result.Summary())
	output := os.Stdout
	var outputFile *os.File
	if outputPath != "" {
		var outputErr error
		outputFile, outputErr = os.Create(outputPath)
		if outputErr != nil {
			fmt.Fprintln(os.Stderr, outputErr)
			finalize(&result, true)
			os.Exit(1)
		}
		output = outputFile
	}
	reportErr := result.WriteJSON(output)
	if outputFile != nil {
		if closeErr := outputFile.Close(); reportErr == nil {
			reportErr = closeErr
		}
	}
	if reportErr != nil {
		fmt.Fprintln(os.Stderr, reportErr)
		finalize(&result, true)
		os.Exit(1)
	}
	preserve := cfg.KeepData || result.Truncated || runErr != nil || !result.Passed()
	finalize(&result, preserve)
	if runErr != nil {
		fmt.Fprintln(os.Stderr, runErr)
		os.Exit(1)
	}
	if !result.Passed() {
		os.Exit(1)
	}
}

func finalize(result *concurrency.LifecycleResult, preserve bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := result.Finalize(ctx, preserve); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
