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

// CommandErrorCode extracts the MongoDB error code and codeName from err, if it
// is a driver CommandError. The bool is false for non-command errors.
func CommandErrorCode(err error) (code int32, name string, ok bool) {
	if ce, isCE := err.(mongo.CommandError); isCE {
		return ce.Code, ce.Name, true
	}
	return 0, "", false
}
