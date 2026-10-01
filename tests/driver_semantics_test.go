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

package tests

import (
	"context"
	"sync"
	"testing"

	"github.com/dolthub/dumbodb-parity-testing/harness"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type startedCommandRecorder struct {
	mu      sync.Mutex
	started map[string][]bson.Raw
	replies map[string][]bson.Raw
}

func newStartedCommandRecorder() *startedCommandRecorder {
	return &startedCommandRecorder{
		started: make(map[string][]bson.Raw),
		replies: make(map[string][]bson.Raw),
	}
}

func (r *startedCommandRecorder) monitor() *event.CommandMonitor {
	return &event.CommandMonitor{
		Started: func(_ context.Context, evt *event.CommandStartedEvent) {
			command := append(bson.Raw(nil), evt.Command...)
			r.mu.Lock()
			defer r.mu.Unlock()
			r.started[evt.CommandName] = append(r.started[evt.CommandName], command)
		},
		Succeeded: func(_ context.Context, evt *event.CommandSucceededEvent) {
			reply := append(bson.Raw(nil), evt.Reply...)
			r.mu.Lock()
			defer r.mu.Unlock()
			r.replies[evt.CommandName] = append(r.replies[evt.CommandName], reply)
		},
	}
}

func (r *startedCommandRecorder) lastStarted(name string) bson.Raw {
	r.mu.Lock()
	defer r.mu.Unlock()
	commands := r.started[name]
	if len(commands) == 0 {
		return nil
	}
	return commands[len(commands)-1]
}

func (r *startedCommandRecorder) lastReply(name string) bson.Raw {
	r.mu.Lock()
	defer r.mu.Unlock()
	replies := r.replies[name]
	if len(replies) == 0 {
		return nil
	}
	return replies[len(replies)-1]
}

func (r *startedCommandRecorder) repliesFor(name string) []bson.Raw {
	r.mu.Lock()
	defer r.mu.Unlock()
	replies := r.replies[name]
	result := make([]bson.Raw, len(replies))
	copy(result, replies)
	return result
}

func monitoredClient(ctx context.Context, uri string, recorder *startedCommandRecorder, clientOptions ...*options.ClientOptions) (*mongo.Client, error) {
	opts := options.Client().ApplyURI(uri).SetMonitor(recorder.monitor())
	for _, clientOption := range clientOptions {
		opts = options.MergeClientOptions(opts, clientOption)
	}
	return mongo.Connect(ctx, opts)
}

func TestDriverSemantics_retryableWritesStandaloneOmitsTxnNumber(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "driver_retryable_writes_standalone_omits_txn_number",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			recorder := newStartedCommandRecorder()
			client, err := monitoredClient(ctx, harness.ServerURI(ctx), recorder, options.Client().SetRetryWrites(true))
			if err != nil {
				return nil, err
			}
			defer func() { _ = client.Disconnect(ctx) }()

			monitoredCol := client.Database(col.Database().Name()).Collection(col.Name())
			if _, err := monitoredCol.InsertOne(ctx, bson.D{{Key: "_id", Value: "retryable"}}); err != nil {
				return nil, err
			}
			insertCommand := recorder.lastStarted("insert")
			_, txnNumberErr := insertCommand.LookupErr("txnNumber")
			return bson.D{
				{Key: "insertObserved", Value: insertCommand != nil},
				{Key: "txnNumberPresent", Value: txnNumberErr == nil},
			}, nil
		},
	})
}

func TestDriverSemantics_causalSessionReadUsesOperationTime(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:     "driver_causal_session_read_uses_operation_time",
		Support:  harness.DumboDBFull,
		Topology: harness.TopologyReplicaSet,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			recorder := newStartedCommandRecorder()
			client, err := monitoredClient(ctx, harness.ServerURI(ctx), recorder)
			if err != nil {
				return nil, err
			}
			defer func() { _ = client.Disconnect(ctx) }()

			session, err := client.StartSession(options.Session().SetCausalConsistency(true))
			if err != nil {
				return nil, err
			}
			defer session.EndSession(ctx)
			sessionCtx := mongo.NewSessionContext(ctx, session)
			monitoredCol := client.Database(col.Database().Name()).Collection(col.Name())
			if _, err := monitoredCol.InsertOne(sessionCtx, bson.D{{Key: "_id", Value: "causal"}, {Key: "value", Value: 1}}); err != nil {
				return nil, err
			}
			operationTimeSet := session.OperationTime() != nil
			var found bson.M
			if err := monitoredCol.FindOne(sessionCtx, bson.D{{Key: "_id", Value: "causal"}}).Decode(&found); err != nil {
				return nil, err
			}
			findCommand := recorder.lastStarted("find")
			readConcern, readConcernErr := findCommand.LookupErr("readConcern")
			afterClusterTimePresent := false
			if readConcernErr == nil {
				_, afterClusterTimeErr := readConcern.Document().LookupErr("afterClusterTime")
				afterClusterTimePresent = afterClusterTimeErr == nil
			}
			return bson.D{
				{Key: "operationTimeSet", Value: operationTimeSet},
				{Key: "afterClusterTimePresent", Value: afterClusterTimePresent},
				{Key: "readOwnWrite", Value: found["value"]},
			}, nil
		},
	})
}

func TestDriverSemantics_transactionCommitAndAbortCommands(t *testing.T) {
	// TestTransaction_withTransaction_retries_on_conflict covers the driver's
	// callback retry contract. This test covers the adjacent wire contracts.
	harness.PairTest(t, harness.TestCase{
		Name:     "driver_transaction_commit_and_abort_commands",
		Support:  harness.DumboDBFull,
		Topology: harness.TopologyReplicaSet,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			recorder := newStartedCommandRecorder()
			client, err := monitoredClient(ctx, harness.ServerURI(ctx), recorder)
			if err != nil {
				return nil, err
			}
			defer func() { _ = client.Disconnect(ctx) }()
			monitoredCol := client.Database(col.Database().Name()).Collection(col.Name())

			commitSession, err := client.StartSession()
			if err != nil {
				return nil, err
			}
			defer commitSession.EndSession(ctx)
			if err := commitSession.StartTransaction(); err != nil {
				return nil, err
			}
			commitCtx := mongo.NewSessionContext(ctx, commitSession)
			if _, err := monitoredCol.InsertOne(commitCtx, bson.D{{Key: "_id", Value: "committed"}}); err != nil {
				return nil, err
			}
			if err := commitSession.CommitTransaction(ctx); err != nil {
				return nil, err
			}

			abortSession, err := client.StartSession()
			if err != nil {
				return nil, err
			}
			defer abortSession.EndSession(ctx)
			if err := abortSession.StartTransaction(); err != nil {
				return nil, err
			}
			abortCtx := mongo.NewSessionContext(ctx, abortSession)
			if _, err := monitoredCol.InsertOne(abortCtx, bson.D{{Key: "_id", Value: "aborted"}}); err != nil {
				return nil, err
			}
			if err := abortSession.AbortTransaction(ctx); err != nil {
				return nil, err
			}

			commitCommand := recorder.lastStarted("commitTransaction")
			abortCommand := recorder.lastStarted("abortTransaction")
			commitReply := recorder.lastReply("commitTransaction")
			abortReply := recorder.lastReply("abortTransaction")
			return bson.D{
				{Key: "commitObserved", Value: commitCommand != nil},
				{Key: "commitTxnNumberPresent", Value: rawFieldPresent(commitCommand, "txnNumber")},
				{Key: "commitAutocommitPresent", Value: rawFieldPresent(commitCommand, "autocommit")},
				{Key: "commitOKPresent", Value: rawFieldPresent(commitReply, "ok")},
				{Key: "abortObserved", Value: abortCommand != nil},
				{Key: "abortTxnNumberPresent", Value: rawFieldPresent(abortCommand, "txnNumber")},
				{Key: "abortAutocommitPresent", Value: rawFieldPresent(abortCommand, "autocommit")},
				{Key: "abortOKPresent", Value: rawFieldPresent(abortReply, "ok")},
			}, nil
		},
	})
}

func rawFieldPresent(raw bson.Raw, field string) bool {
	if raw == nil {
		return false
	}
	_, err := raw.LookupErr(field)
	return err == nil
}

func TestDriverSemantics_changeStreamsMongoOnly(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:     "driver_change_streams_mongo_only",
		Support:  harness.DumboDBMongoOnly,
		Topology: harness.TopologyReplicaSet,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// DumboDB does not implement change streams. Keeping this Mongo-only
			// test makes the unsupported driver surface explicit.
			stream, err := col.Watch(ctx, mongo.Pipeline{})
			if err != nil {
				return nil, err
			}
			defer func() { _ = stream.Close(ctx) }()
			return bson.D{{Key: "streamCreated", Value: true}}, nil
		},
	})
}
