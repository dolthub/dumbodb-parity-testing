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
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// $clusterTime and operationTime belong to a server that has a replica set
// configuration. A standalone mongod returns neither, because there is no
// cluster time to gossip, and a client talking to one should not be told it
// is part of a cluster.
//
// ASSERTED DIRECTLY RATHER THAN THROUGH CompareResponses, on purpose. The
// shared comparison drops both field names from each side before comparing,
// so it cannot see a field that one server emits and the other does not;
// that is workspace-1pr. Routing this case through it would reproduce the
// blindness that let the divergence reach a human in the first place.
func TestLogicalTime_AbsentOnStandalone(t *testing.T) {
	ctx := context.Background()

	for _, s := range []struct {
		name string
		uri  string
	}{
		{"mongod", harness.MongoURI()},
		{"dumbodb", harness.DumboDBURI()},
	} {
		t.Run(s.name, func(t *testing.T) {
			if s.uri == "" {
				t.Skipf("no URI for %s", s.name)
			}
			cli, err := mongo.Connect(ctx, options.Client().ApplyURI(s.uri))
			if err != nil {
				t.Fatalf("connecting to %s: %v", s.name, err)
			}
			defer func() { _ = cli.Disconnect(context.Background()) }()

			for _, cmd := range []string{"ping", "buildInfo", "hello"} {
				var raw bson.Raw
				if err := cli.Database("admin").RunCommand(ctx,
					bson.D{{Key: cmd, Value: 1}}).Decode(&raw); err != nil {
					t.Fatalf("%s on %s: %v", cmd, s.name, err)
				}
				for _, field := range []string{"$clusterTime", "operationTime"} {
					if _, err := raw.LookupErr(field); err == nil {
						t.Errorf("%s returned %s from %s on a standalone server; "+
							"that field says the server is part of a replica set",
							s.name, field, cmd)
					}
				}
			}
		})
	}
}
