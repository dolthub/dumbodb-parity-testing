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

package harness

import (
	"time"

	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type Corruption struct {
	Name      string
	Rationale string
	Apply     func(state *ServerState) error
}

func Corruptions() []Corruption {
	return []Corruption{
		{
			Name:      "changed scalar",
			Rationale: "the baseline case; if this is missed nothing else matters",
			Apply:     mutateFirstDocument(func(d bson.D) bson.D { return setField(d, "payload", "corrupted") }),
		},
		{
			Name:      "int32 changed to int64",
			Rationale: "BSON type fidelity; CompareResponses collapses both to float64 and reports a match",
			Apply:     mutateFirstDocument(func(d bson.D) bson.D { return retypeInt(d, "n") }),
		},
		{
			Name:      "changed _id",
			Rationale: "document identity; CompareResponses drops ObjectIDs from comparison entirely",
			Apply:     mutateFirstDocument(func(d bson.D) bson.D { return setField(d, "_id", "displaced") }),
		},
		{
			Name:      "changed date",
			Rationale: "CompareResponses collapses every date to a single sentinel",
			Apply: mutateFirstDocument(func(d bson.D) bson.D {
				return setField(d, "when", primitive.NewDateTimeFromTime(time.Unix(915148800, 0).UTC()))
			}),
		},
		{
			Name:      "missing document",
			Rationale: "a dropped write is the most likely real replication defect",
			Apply: func(s *ServerState) error {
				return withFirstCollection(s, func(c *CollectionState) error {
					for k := range c.Documents {
						delete(c.Documents, k)
						return nil
					}
					return fmt.Errorf("no documents to remove")
				})
			},
		},
		{
			Name:      "extra document",
			Rationale: "a double-applied oplog entry shows up as an extra document",
			Apply: func(s *ServerState) error {
				return withFirstCollection(s, func(c *CollectionState) error {
					doc, err := bson.Marshal(bson.D{{Key: "_id", Value: "phantom"}})
					if err != nil {
						return err
					}
					c.Documents["phantom"] = doc
					return nil
				})
			},
		},
		{
			Name:      "missing collection",
			Rationale: "a catalog operation lost in the oplog stream",
			Apply: func(s *ServerState) error {
				for _, db := range s.Databases {
					for name := range db.Collections {
						delete(db.Collections, name)
						return nil
					}
				}
				return fmt.Errorf("no collections to remove")
			},
		},
		{
			Name:      "missing database",
			Rationale: "an entire namespace never cloned during initial sync",
			Apply: func(s *ServerState) error {
				for name := range s.Databases {
					delete(s.Databases, name)
					return nil
				}
				return fmt.Errorf("no databases to remove")
			},
		},
		{
			Name:      "missing index",
			Rationale: "indexes are cloned separately from documents and can be lost on their own",
			Apply: func(s *ServerState) error {
				return withFirstCollection(s, func(c *CollectionState) error {
					for name := range c.Indexes {
						delete(c.Indexes, name)
						return nil
					}
					return fmt.Errorf("no indexes to remove")
				})
			},
		},
		{
			Name:      "changed index key",
			Rationale: "an index that exists but indexes the wrong field still answers queries wrongly",
			Apply: func(s *ServerState) error {
				return withFirstCollection(s, func(c *CollectionState) error {
					for name, idx := range c.Indexes {
						replaced, err := replaceField(idx, "key", bson.D{{Key: "wrongField", Value: int32(1)}})
						if err != nil {
							return err
						}
						c.Indexes[name] = replaced
						return nil
					}
					return fmt.Errorf("no indexes to change")
				})
			},
		},
		{
			Name:      "changed collection options",
			Rationale: "validators travel with collection options and are enforced on every write path",
			Apply: func(s *ServerState) error {
				return withFirstCollection(s, func(c *CollectionState) error {
					replaced, err := replaceField(c.Options, "validationLevel", "off")
					if err != nil {
						return err
					}
					c.Options = replaced
					return nil
				})
			},
		},
	}
}

func withFirstCollection(s *ServerState, fn func(*CollectionState) error) error {
	for _, dbName := range sortedKeys(s.Databases) {
		db := s.Databases[dbName]
		for _, collName := range sortedKeys(db.Collections) {
			return fn(db.Collections[collName])
		}
	}
	return fmt.Errorf("captured state has no collections to corrupt")
}

func mutateFirstDocument(fn func(bson.D) bson.D) func(*ServerState) error {
	return func(s *ServerState) error {
		return withFirstCollection(s, func(c *CollectionState) error {
			for key, doc := range c.Documents {
				var d bson.D
				if err := bson.Unmarshal(doc, &d); err != nil {
					return err
				}
				raw, err := bson.Marshal(fn(d))
				if err != nil {
					return err
				}
				c.Documents[key] = raw
				return nil
			}
			return fmt.Errorf("no documents to corrupt")
		})
	}
}

func setField(d bson.D, key string, value interface{}) bson.D {
	for i := range d {
		if d[i].Key == key {
			d[i].Value = value
			return d
		}
	}
	return append(d, bson.E{Key: key, Value: value})
}

func retypeInt(d bson.D, key string) bson.D {
	for i := range d {
		if d[i].Key != key {
			continue
		}
		switch v := d[i].Value.(type) {
		case int32:
			d[i].Value = int64(v)
		case int64:
			d[i].Value = int32(v)
		}
		return d
	}
	return d
}

func replaceField(raw bson.Raw, key string, value interface{}) (bson.Raw, error) {
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	return bson.Marshal(setField(d, key, value))
}

func SeedCorruptionCorpus(ctx context.Context, cli *mongo.Client, dbName string) error {
	db := cli.Database(dbName)

	err := db.RunCommand(ctx, bson.D{
		{Key: "create", Value: "items"},
		{Key: "validator", Value: bson.D{{Key: "n", Value: bson.D{{Key: "$exists", Value: true}}}}},
		{Key: "validationLevel", Value: "strict"},
	}).Err()
	if err != nil {
		return fmt.Errorf("create items: %w", err)
	}

	coll := db.Collection("items")
	docs := make([]interface{}, 0, 5)
	for i := 0; i < 5; i++ {
		docs = append(docs, bson.D{
			{Key: "_id", Value: int32(i)},
			{Key: "n", Value: int32(i)},
			{Key: "payload", Value: "original"},
			{Key: "when", Value: primitive.NewDateTimeFromTime(time.Unix(1767225600, 0).UTC())},
		})
	}
	if _, err := coll.InsertMany(ctx, docs); err != nil {
		return fmt.Errorf("seed items: %w", err)
	}

	_, err = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "n", Value: int32(1)}},
	})
	if err != nil {
		return fmt.Errorf("create index: %w", err)
	}
	return nil
}
