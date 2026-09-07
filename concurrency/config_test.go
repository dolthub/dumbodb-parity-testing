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

import "testing"

func TestConfigValidate(t *testing.T) {
	valid := Config{
		TargetURI:  "mongodb://localhost:27017",
		Operations: 1,
		Workers:    1,
		Database:   "concurrency_test",
		Collection: "documents",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "target", mutate: func(c *Config) { c.TargetURI = "" }},
		{name: "limit", mutate: func(c *Config) { c.Operations = 0 }},
		{name: "workers", mutate: func(c *Config) { c.Workers = 0 }},
		{name: "database", mutate: func(c *Config) { c.Database = "" }},
		{name: "collection", mutate: func(c *Config) { c.Collection = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := valid
			test.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
