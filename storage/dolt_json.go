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

package storage

import (
	"context"
	"encoding/json"
	"fmt"
)

// DoltJSONBackend stores each Doc as a JSON-typed column on a
// VARCHAR-pk table:
//
//	CREATE TABLE users (
//	    _id VARCHAR(64) PRIMARY KEY,
//	    doc JSON NOT NULL
//	);
//
// The base row payload is the JSON document itself, isolating "JSON
// includes field names per row" as the only schema-shape variable vs
// DoltBackend. No secondary index -- the parity test is currently
// scoped to base-table storage cost; index parity is deferred until
// DumboDB's index storage path is repaired.
//
// All branching / commit / GC / measurement plumbing is inherited
// from DoltBackend via embedding; only Setup, InsertBatch, and
// UpdateEmail change.
type DoltJSONBackend struct {
	*DoltBackend
}

// NewDoltJSONBackend connects to the same dolt sql-server as
// DoltBackend and provisions a fresh database. The connection
// machinery is identical; only the schema differs.
func NewDoltJSONBackend(ctx context.Context) (*DoltJSONBackend, error) {
	b, err := NewDoltBackend(ctx)
	if err != nil {
		return nil, err
	}
	return &DoltJSONBackend{DoltBackend: b}, nil
}

func (b *DoltJSONBackend) Name() string { return "DoltJSON" }

func (b *DoltJSONBackend) Setup(ctx context.Context) error {
	_, err := b.db.ExecContext(ctx, `
		CREATE TABLE users (
			_id VARCHAR(64) PRIMARY KEY,
			doc JSON        NOT NULL
		)`)
	return err
}

func (b *DoltJSONBackend) InsertBatch(ctx context.Context, docs []Doc) error {
	if len(docs) == 0 {
		return nil
	}
	const batchSize = 500
	for i := 0; i < len(docs); i += batchSize {
		end := i + batchSize
		if end > len(docs) {
			end = len(docs)
		}
		batch := docs[i:end]

		query := "INSERT INTO users (_id, doc) VALUES "
		args := make([]interface{}, 0, len(batch)*2)
		for j, d := range batch {
			if j > 0 {
				query += ","
			}
			query += "(?, ?)"
			payload, err := json.Marshal(map[string]any{
				"email": d.Email,
				"name":  d.Name,
				"age":   d.Age,
			})
			if err != nil {
				return fmt.Errorf("dolt-json: marshal %s: %w", d.ID, err)
			}
			args = append(args, d.ID, string(payload))
		}
		if _, err := b.db.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	return nil
}

func (b *DoltJSONBackend) UpdateEmail(ctx context.Context, id, newEmail string) error {
	// JSON_SET keeps the rest of the document intact and rewrites only
	// the email path; the generated `email` column + idx_email update
	// follows automatically.
	_, err := b.db.ExecContext(ctx,
		"UPDATE users SET doc = JSON_SET(doc, '$.email', ?) WHERE _id=?",
		newEmail, id)
	return err
}
