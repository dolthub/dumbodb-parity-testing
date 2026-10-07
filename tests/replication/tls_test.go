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

//go:build replication

package replication

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Replication over TLS, with and without internal membership authentication.
//
// Every other case in this package runs the set in plaintext, so nothing here
// held the encrypted path until these two. The pair exists because the
// difference between them is one flag and the outcomes are opposite.

const tlsReplicationDocuments = 20

// TestReplication_TLSWithKeyFile is the shipped configuration: requireTLS on
// every member plus a shared keyfile, so members encrypt their traffic and
// authenticate to each other as __system.
//
// Measured working by hand on 2026-10-06 and again on 2026-10-07, which is
// precisely why it is written down. A feature verified only by someone
// remembering to try it is a feature that regresses quietly.
func TestReplication_TLSWithKeyFile(t *testing.T) {
	t.Parallel()
	fixture := harness.NewTLSFixture(t)
	harness.ReplicaTest(t, harness.ReplicaCase{
		Name:     "Replication_TLSWithKeyFile",
		Support:  harness.DumboDBFull,
		TLS:      fixture,
		KeyFile:  harness.NewKeyFile(t, fixture.Dir),
		Workload: insertTLSDocuments,
		Assert:   assertSubjectHoldsTLSDocuments,
	})
}

// TestReplication_TLSWithoutKeyFile is the same set with no keyfile, so the
// members encrypt but never authenticate to each other. mongod supports this,
// so DumboDB must too.
//
// XFail for workspace-3y6.27, a regression in ea23347 that is not about TLS
// at all: DumboDB cannot join ANY no-keyfile set, plaintext included, because
// observeLogicalTime refuses an unsigned $clusterTime and a set with no
// internal authentication never produces a signed one. This case is that bug
// seen through TLS, and it will pass when 3y6.27 does.
//
// Keeping it is still worth it. The whole of tests/replication fails on the
// branch today, so if the fix restored only the plaintext path nothing else
// here would notice.
//
// Promote to DumboDBFull when it passes; the XPASS is the signal.
func TestReplication_TLSWithoutKeyFile(t *testing.T) {
	t.Parallel()
	harness.ReplicaTest(t, harness.ReplicaCase{
		Name:    "Replication_TLSWithoutKeyFile",
		Support: harness.DumboDBXFail,
		TLS:     harness.NewTLSFixture(t),
		// The keyfile case above joins in under ten seconds, so ninety is a
		// wide margin for a member that is going to join. Waiting the default
		// three minutes twice over to confirm a known failure costs six
		// minutes of every CI run and proves nothing the first ninety seconds
		// did not. Raise this, do not delete it, if a slow runner ever makes
		// the margin look thin.
		SubjectWait: 90 * time.Second,
		Workload:    insertTLSDocuments,
		Assert:      assertSubjectHoldsTLSDocuments,
	})
}

func insertTLSDocuments(ctx context.Context, primary *mongo.Client) error {
	docs := make([]interface{}, 0, tlsReplicationDocuments)
	for i := 0; i < tlsReplicationDocuments; i++ {
		docs = append(docs, bson.D{
			{Key: "_id", Value: int32(i)},
			{Key: "over", Value: "tls"},
		})
	}
	_, err := primary.Database("tlsrepl").Collection("docs").InsertMany(ctx, docs)
	return err
}

// assertSubjectHoldsTLSDocuments checks that the subject received the
// documents, rather than only that it did not differ from the reference.
//
// Those are not the same claim. A subject that replicated nothing and a
// reference captured as empty would agree, and the state comparison alone
// would call that a pass. Counting the documents is what stops this case
// passing vacuously.
func assertSubjectHoldsTLSDocuments(t *testing.T, res harness.ReplicaResult) {
	if res.Subject == nil {
		// The expected shape of the XFail case: grading never got as far as
		// capturing the subject. gradeSubject has already recorded why.
		return
	}
	db, ok := res.Subject.Databases["tlsrepl"]
	if !ok {
		t.Error("the subject has no tlsrepl database, so nothing replicated over TLS")
		return
	}
	coll, ok := db.Collections["docs"]
	if !ok {
		t.Error("the subject has no tlsrepl.docs collection, so nothing replicated over TLS")
		return
	}
	if len(coll.Documents) != tlsReplicationDocuments {
		t.Errorf("the subject holds %d documents in tlsrepl.docs, want %d",
			len(coll.Documents), tlsReplicationDocuments)
	}
}
