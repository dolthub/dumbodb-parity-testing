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
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ConnectAs dials baseURI authenticating as (user, password) against authSource
// and pings to force the authentication handshake, so an authentication failure
// is returned here rather than deferred to the first operation. The caller owns
// the returned client and must Disconnect it. On failure the client is already
// disconnected and a nil client plus the error are returned.
func ConnectAs(ctx context.Context, baseURI, user, password, authSource string) (*mongo.Client, error) {
	cred := options.Credential{Username: user, Password: password, AuthSource: authSource}
	return connect(ctx, baseURI, &cred)
}

// ConnectAsMech is ConnectAs with an explicit SCRAM mechanism (e.g.
// "SCRAM-SHA-1" or "SCRAM-SHA-256"). An empty mechanism lets the driver
// negotiate. Used by handshake tests that pin a specific mechanism.
func ConnectAsMech(ctx context.Context, baseURI, user, password, authSource, mechanism string) (*mongo.Client, error) {
	cred := options.Credential{Username: user, Password: password, AuthSource: authSource}
	if mechanism != "" {
		cred.AuthMechanism = mechanism
	}
	return connect(ctx, baseURI, &cred)
}

// ConnectNoAuth dials baseURI without credentials and pings, for exercising the
// pre-authentication state of an access-control-enabled server. The caller owns
// the returned client and must Disconnect it.
func ConnectNoAuth(ctx context.Context, baseURI string) (*mongo.Client, error) {
	return connect(ctx, baseURI, nil)
}

// CommandErrorCode extracts the MongoDB error code and codeName from err. It
// handles both command-level errors (CommandError) and write-level errors
// (WriteException / WriteError), since an authorization failure on a write
// surfaces as the latter. The bool is false when no code can be extracted.
func CommandErrorCode(err error) (code int32, name string, ok bool) {
	if err == nil {
		return 0, "", false
	}
	var ce mongo.CommandError
	if errors.As(err, &ce) {
		return ce.Code, ce.Name, true
	}
	var we mongo.WriteException
	if errors.As(err, &we) && len(we.WriteErrors) > 0 {
		return int32(we.WriteErrors[0].Code), "", true
	}
	return 0, "", false
}
