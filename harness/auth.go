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
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Auth parity tests run against a SEPARATE, access-control-enabled server pair
// so the existing non-auth suites are unaffected. Enable by setting PARITY_AUTH=1
// and starting both servers with access control on:
//
//	mongod --auth ...            (MongoDB)
//	dumbodb --auth ...           (DumboDB; --auth is a no-op until enforcement lands)
//
// The harness bootstraps the MongoDB admin via the localhost exception on first
// use, then connects to both servers as that admin. Tests that need a specific
// non-admin identity use ConnectAs (see auth_user.go).

const (
	defaultAuthMongoURI  = "mongodb://localhost:27017"
	defaultAuthDumboDBURI = "mongodb://localhost:27018"
	defaultAdminUser     = "admin"
	defaultAdminPassword = "admin-pw"
)

var (
	authClients     *AuthClients
	authClientsOnce sync.Once
	authClientsErr  error
)

// AuthClients holds admin-authenticated connections to both servers.
type AuthClients struct {
	MongoAdmin   *mongo.Client
	DumboDBAdmin *mongo.Client
}

// AuthConfigured reports whether the auth parity suite should run. It is
// opt-in (PARITY_AUTH=1) so ordinary `go test ./...` runs skip auth tests
// rather than failing when no access-control-enabled servers are present.
func AuthConfigured() bool { return os.Getenv("PARITY_AUTH") == "1" }

// RequireAuth skips the calling test unless the auth parity suite is enabled.
func RequireAuth(t *testing.T) {
	t.Helper()
	if !AuthConfigured() {
		t.Skip("auth parity suite disabled; set PARITY_AUTH=1 and start both servers with access control")
	}
}

// AuthMongoBaseURI is the credential-free base URI of the auth-enabled MongoDB.
func AuthMongoBaseURI() string { return envOr("MONGO_AUTH_URI", defaultAuthMongoURI) }

// AuthDumboDBBaseURI is the credential-free base URI of the auth-enabled DumboDB.
func AuthDumboDBBaseURI() string { return envOr("DUMBODB_AUTH_URI", defaultAuthDumboDBURI) }

// AdminUser and AdminPassword are the bootstrap super-user credentials the
// harness creates and connects as.
func AdminUser() string     { return envOr("MONGO_ADMIN_USER", defaultAdminUser) }
func AdminPassword() string { return envOr("MONGO_ADMIN_PWD", defaultAdminPassword) }

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// GetAuthClients returns admin-authenticated clients to both servers,
// connecting (and bootstrapping the MongoDB admin) on first call.
func GetAuthClients(ctx context.Context) (*AuthClients, error) {
	authClientsOnce.Do(func() {
		if err := bootstrapMongoAdmin(ctx); err != nil {
			authClientsErr = fmt.Errorf("bootstrap mongo admin: %w", err)
			return
		}

		cred := options.Credential{
			Username:   AdminUser(),
			Password:   AdminPassword(),
			AuthSource: "admin",
		}

		mc, err := connect(ctx, AuthMongoBaseURI(), &cred)
		if err != nil {
			authClientsErr = fmt.Errorf("connect mongo admin: %w", err)
			return
		}
		// DumboDB's SCRAM handshake is live but user management is stubbed, so
		// no admin user exists to authenticate as; and enforcement is off, so an
		// unauthenticated connection has full access. Connect without
		// credentials. Once DumboDB implements createUser this becomes an
		// admin-credentialed connection like MongoDB's.
		dc, err := connect(ctx, AuthDumboDBBaseURI(), nil)
		if err != nil {
			_ = mc.Disconnect(ctx)
			authClientsErr = fmt.Errorf("connect dumbodb (no auth): %w", err)
			return
		}

		authClients = &AuthClients{MongoAdmin: mc, DumboDBAdmin: dc}
	})
	return authClients, authClientsErr
}

// connect dials uri, applying cred when non-nil, and pings to confirm the
// connection (and authentication) succeeded.
func connect(ctx context.Context, uri string, cred *options.Credential) (*mongo.Client, error) {
	opts := options.Client().ApplyURI(uri)
	if cred != nil {
		opts = opts.SetAuth(*cred)
	}
	c, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, err
	}
	pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := c.Ping(pctx, nil); err != nil {
		_ = c.Disconnect(ctx)
		return nil, err
	}
	return c, nil
}

// bootstrapMongoAdmin creates the admin super-user via the localhost exception.
// It is idempotent: if the admin already exists, createUser fails (the
// exception is consumed once a user exists) and we treat that as success after
// confirming we can authenticate as the admin.
func bootstrapMongoAdmin(ctx context.Context) error {
	noAuth, err := connect(ctx, AuthMongoBaseURI(), nil)
	if err != nil {
		return err
	}
	defer func() { _ = noAuth.Disconnect(ctx) }()

	createErr := noAuth.Database("admin").RunCommand(ctx, bson.D{
		{Key: "createUser", Value: AdminUser()},
		{Key: "pwd", Value: AdminPassword()},
		{Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "root"}, {Key: "db", Value: "admin"}}}},
	}).Err()

	if createErr == nil {
		return nil
	}
	// Already-bootstrapped: the localhost exception is gone once any user
	// exists, so createUser comes back Unauthorized (13) or the admin is a
	// duplicate. Confirm by authenticating as the admin.
	if !isAlreadyBootstrapped(createErr) {
		return createErr
	}
	cred := options.Credential{Username: AdminUser(), Password: AdminPassword(), AuthSource: "admin"}
	c, err := connect(ctx, AuthMongoBaseURI(), &cred)
	if err != nil {
		return fmt.Errorf("admin already present but auth failed: %w (createUser: %v)", err, createErr)
	}
	_ = c.Disconnect(ctx)
	return nil
}

// isAlreadyBootstrapped reports whether a createUser error indicates the admin
// (or some user) already exists, meaning the localhost exception is spent.
func isAlreadyBootstrapped(err error) bool {
	var ce mongo.CommandError
	if errors.As(err, &ce) {
		// 13 Unauthorized (exception consumed), 11000 DuplicateKey / 51003
		// (admin already created).
		return ce.Code == 13 || ce.Code == 11000 || ce.Code == 51003
	}
	return false
}
