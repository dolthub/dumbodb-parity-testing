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
	"errors"
	"time"
)

const DefaultDuration = 30 * time.Minute

type Config struct {
	TargetURI    string
	Duration     time.Duration
	Operations   int64
	Workers      int
	Seed         int64
	Database     string
	Collection   string
	KeepData     bool
	Scenario     string
	PayloadBytes int
	CASDelay     time.Duration
}

func (c Config) Validate() error {
	if c.TargetURI == "" {
		return errors.New("target URI is required")
	}
	if c.Duration <= 0 && c.Operations <= 0 {
		return errors.New("duration or operations must be positive")
	}
	if c.Workers <= 0 {
		return errors.New("workers must be positive")
	}
	if c.Database == "" {
		return errors.New("database is required")
	}
	if c.Collection == "" {
		return errors.New("collection is required")
	}
	if c.Scenario == "" {
		return errors.New("scenario is required")
	}
	if c.PayloadBytes < 0 {
		return errors.New("payload bytes cannot be negative")
	}
	if c.CASDelay < 0 {
		return errors.New("CAS delay cannot be negative")
	}
	return nil
}
