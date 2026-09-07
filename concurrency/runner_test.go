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
	"reflect"
	"sync"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

type memoryCollection struct {
	mu       sync.Mutex
	document bson.M
}

func (c *memoryCollection) InsertOne(_ context.Context, document interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	decoded, err := documentMap(document)
	if err != nil {
		return err
	}
	c.document = decoded
	return nil
}

func (c *memoryCollection) FindOne(_ context.Context, filter interface{}, result interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	matches, err := matchesFilter(c.document, filter)
	if err != nil {
		return err
	}
	if !matches {
		return fmt.Errorf("document not found")
	}
	encoded, err := bson.Marshal(c.document)
	if err != nil {
		return err
	}
	return bson.Unmarshal(encoded, result)
}

func (c *memoryCollection) UpdateOne(_ context.Context, filter, update interface{}) (WriteResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	matches, err := matchesFilter(c.document, filter)
	if err != nil || !matches {
		return WriteResult{}, err
	}
	updateDocument, err := documentMap(update)
	if err != nil {
		return WriteResult{}, err
	}
	if values, ok := updateDocument["$set"]; ok {
		fields, err := documentMap(values)
		if err != nil {
			return WriteResult{}, err
		}
		for name, value := range fields {
			c.document[name] = value
		}
	}
	if values, ok := updateDocument["$inc"]; ok {
		fields, err := documentMap(values)
		if err != nil {
			return WriteResult{}, err
		}
		for name, value := range fields {
			current, _ := numericInt64(c.document[name])
			increment, valid := numericInt64(value)
			if !valid {
				return WriteResult{}, fmt.Errorf("increment %s is not numeric", name)
			}
			c.document[name] = current + increment
		}
	}
	return WriteResult{Matched: 1, Modified: 1}, nil
}

func (c *memoryCollection) corrupt(mutate func(bson.M)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	mutate(c.document)
}

func documentMap(value interface{}) (bson.M, error) {
	encoded, err := bson.Marshal(value)
	if err != nil {
		return nil, err
	}
	var document bson.M
	if err := bson.Unmarshal(encoded, &document); err != nil {
		return nil, err
	}
	return document, nil
}

func matchesFilter(document bson.M, filter interface{}) (bool, error) {
	fields, err := documentMap(filter)
	if err != nil {
		return false, err
	}
	for name, want := range fields {
		if !reflect.DeepEqual(document[name], want) {
			return false, nil
		}
	}
	return true, nil
}

type corruptingScenario struct {
	Scenario
	corrupt func(bson.M)
}

func (s corruptingScenario) Verify(ctx context.Context, collection Collection, ledger LedgerSnapshot) ([]Check, error) {
	collection.(*memoryCollection).corrupt(s.corrupt)
	return s.Scenario.Verify(ctx, collection, ledger)
}

func TestEveryScenarioRunsThroughConcurrentLifecycle(t *testing.T) {
	for _, name := range []string{"cas", "uuid-cas", "blind-inc", "disjoint-set", "same-set"} {
		t.Run(name, func(t *testing.T) {
			result := runMemoryScenario(t, name, nil)
			if !result.Passed() {
				t.Fatalf("scenario failed: %+v", result.Checks)
			}
			if result.Ledger.Attempts != 100 {
				t.Fatalf("attempts=%d, want 100", result.Ledger.Attempts)
			}
		})
	}
}

func TestEveryScenarioRejectsCorruptedFinalState(t *testing.T) {
	corruptions := map[string]func(bson.M){
		"cas": func(document bson.M) {
			document["version"] = int64(101)
		},
		"uuid-cas": func(document bson.M) {
			document["applied"] = int64(101)
		},
		"blind-inc": func(document bson.M) {
			document["version"] = int64(101)
		},
		"disjoint-set": func(document bson.M) {
			document["worker_0"] = int64(-1)
		},
		"same-set": func(document bson.M) {
			document["value"] = int64(101)
		},
	}
	for name, corrupt := range corruptions {
		t.Run(name, func(t *testing.T) {
			result := runMemoryScenario(t, name, corrupt)
			if result.Passed() {
				t.Fatalf("corrupted final state passed: %+v", result.Checks)
			}
		})
	}
}

func runMemoryScenario(t *testing.T, name string, corrupt func(bson.M)) LifecycleResult {
	t.Helper()
	collection := &memoryCollection{}
	target := &fakeTarget{collection: collection}
	scenario, err := NewScenario(name, 8, 32)
	if err != nil {
		t.Fatal(err)
	}
	if corrupt != nil {
		scenario = corruptingScenario{Scenario: scenario, corrupt: corrupt}
	}
	result, err := RunWithTarget(context.Background(), Config{
		TargetURI:  "mongodb://unused",
		Operations: 100,
		Workers:    8,
		Database:   "test",
		Collection: "documents",
		Scenario:   name,
	}, scenario, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Finalize(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	return result
}
