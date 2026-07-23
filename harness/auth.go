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
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Auth parity tests run against a SEPARATE, access-control-enabled server pair
// (see servers.go) so the non-auth suites are unaffected. The harness bootstraps
// each server's admin via the localhost exception on first use, then connects to
// both servers as that admin. Tests that need a specific non-admin identity use
// ConnectAs (see auth_user.go).

const (
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

// AuthMongoBaseURI is the credential-free base URI of the auth-enabled MongoDB
// provisioned for the suite.
func AuthMongoBaseURI() string { return provisioned.authMongoURI }

// AuthDumboDBBaseURI is the credential-free base URI of the auth-enabled DumboDB
// provisioned for the suite.
func AuthDumboDBBaseURI() string { return provisioned.authDumboURI }

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
		if err := bootstrapAdmin(ctx, AuthMongoBaseURI()); err != nil {
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
		if err := bootstrapAdmin(ctx, AuthDumboDBBaseURI()); err != nil {
			_ = mc.Disconnect(ctx)
			authClientsErr = fmt.Errorf("bootstrap dumbodb admin: %w", err)
			return
		}

		dc, err := connect(ctx, AuthDumboDBBaseURI(), &cred)
		if err != nil {
			_ = mc.Disconnect(ctx)
			authClientsErr = fmt.Errorf("connect dumbodb admin: %w", err)
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

// bootstrapAdmin creates the admin super-user on the server at baseURI via the
// localhost exception. It is idempotent: an already-bootstrapped server is
// treated as success once we confirm we can authenticate as the admin.
func bootstrapAdmin(ctx context.Context, baseURI string) error {
	noAuth, err := connect(ctx, baseURI, nil)
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
	if !isAlreadyBootstrapped(createErr) {
		return createErr
	}
	cred := options.Credential{Username: AdminUser(), Password: AdminPassword(), AuthSource: "admin"}
	c, err := connect(ctx, baseURI, &cred)
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
