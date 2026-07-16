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
	"strings"
	"testing"

	"github.com/xdg-go/scram"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/dolthub/dumbodb-parity-testing/harness"
	"github.com/dolthub/dumbodb-parity-testing/wire"
)

// Auth parity area B/C, wire level: SCRAM and speculativeAuthenticate cases that
// need raw SASL frames the Go driver hides (SCRAM-07,11-17,20; SPEC-01/02/03/07).
// A single wire.Conn carries the whole conversation, since SASL state is
// per-connection. Cases that need a valid ClientProof drive xdg-go/scram as the
// client. A MongoDB-side check validates the expected behavior (and that the
// wire SCRAM client itself works).

func binPayload(s string) primitive.Binary { return primitive.Binary{Subtype: 0, Data: []byte(s)} }

func payloadStr(reply bson.M) string {
	if b, ok := reply["payload"].(primitive.Binary); ok {
		return string(b.Data)
	}
	return ""
}

func replyCode(reply bson.M) int32 {
	switch v := reply["code"].(type) {
	case int32:
		return v
	case int64:
		return int32(v)
	case float64:
		return int32(v)
	}
	return 0
}

func replyOK(reply bson.M) bool {
	switch v := reply["ok"].(type) {
	case float64:
		return v == 1
	case int32:
		return v == 1
	}
	return false
}

func replyBool(reply bson.M, key string) bool { b, _ := reply[key].(bool); return b }

func saslStart(conn *wire.Conn, mechanism, payload, db string, opts bson.D) (bson.M, error) {
	cmd := bson.D{
		{Key: "saslStart", Value: 1},
		{Key: "mechanism", Value: mechanism},
		{Key: "payload", Value: binPayload(payload)},
	}
	if opts != nil {
		cmd = append(cmd, bson.E{Key: "options", Value: opts})
	}
	cmd = append(cmd, bson.E{Key: "$db", Value: db})
	return conn.RunCommand(cmd)
}

func saslContinue(conn *wire.Conn, convID interface{}, payload, db string) (bson.M, error) {
	return conn.RunCommand(bson.D{
		{Key: "saslContinue", Value: 1},
		{Key: "conversationId", Value: convID},
		{Key: "payload", Value: binPayload(payload)},
		{Key: "$db", Value: db},
	})
}

// scramClient returns a SCRAM-SHA-256 conversation and its client-first message.
func scramClient(user, pwd string) (*scram.ClientConversation, string, error) {
	cl, err := scram.SHA256.NewClient(user, pwd, "")
	if err != nil {
		return nil, "", err
	}
	conv := cl.NewConversation()
	first, err := conv.Step("")
	return conv, first, err
}

// wireCase creates a SCRAM-SHA-256 user (when needUser), runs fn over a raw wire
// connection, and compares MongoDB vs DumboDB. mongoCheck asserts the expected
// MongoDB behavior on the result.
func wireCase(t *testing.T, name string, needUser bool,
	fn func(conn *wire.Conn, db, user, pwd string) (bson.M, error),
	mongoCheck func(t *testing.T, res bson.M)) harness.AuthCase {
	return authCase(name, func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, user, pwd := "wire_"+tgt.NS, "u_"+tgt.NS, "pw-"+tgt.NS
		if needUser {
			defer cleanupUser(ctx, tgt, db, user)
			if err := createUserMech(ctx, tgt.Admin, db, user, pwd, rwRole(db), []string{"SCRAM-SHA-256"}); err != nil {
				return nil, err
			}
		}
		conn, err := wire.Dial(tgt.BaseURI)
		if err != nil {
			return nil, err
		}
		defer func() { _ = conn.Close() }()
		res, err := fn(conn, db, user, pwd)
		if err != nil {
			return nil, err
		}
		if tgt.BaseURI == harness.AuthMongoBaseURI() && mongoCheck != nil {
			mongoCheck(t, res)
		}
		return res, nil
	})
}

func TestAuthScramWire(t *testing.T) {
	// SCRAM-07: an unsupported mechanism -> MechanismUnavailable (334).
	harness.AuthPairTest(t, wireCase(t, "SCRAM-07-unsupported-mechanism", false,
		func(conn *wire.Conn, db, user, pwd string) (bson.M, error) {
			r, err := saslStart(conn, "SCRAM-SHA-999", "n,,n=x,r=abc", "admin", nil)
			return bson.M{"ok": replyOK(r), "code": replyCode(r)}, err
		},
		func(t *testing.T, res bson.M) {
			if res["ok"] != false || res["code"] != int32(334) {
				t.Errorf("SCRAM-07: want ok=false code=334, got %v", res)
			}
		}))

	// SCRAM-11: a malformed client-first payload is rejected.
	harness.AuthPairTest(t, wireCase(t, "SCRAM-11-malformed-client-first", false,
		func(conn *wire.Conn, db, user, pwd string) (bson.M, error) {
			r, err := saslStart(conn, "SCRAM-SHA-256", "this is not scram", "admin", nil)
			return bson.M{"ok": replyOK(r)}, err
		},
		func(t *testing.T, res bson.M) {
			if res["ok"] != false {
				t.Errorf("SCRAM-11: malformed client-first should be rejected, got %v", res)
			}
		}))

	// SCRAM-13: saslContinue with no prior saslStart is rejected.
	harness.AuthPairTest(t, wireCase(t, "SCRAM-13-continue-without-start", false,
		func(conn *wire.Conn, db, user, pwd string) (bson.M, error) {
			r, err := saslContinue(conn, int32(1), "", "admin")
			return bson.M{"ok": replyOK(r)}, err
		},
		func(t *testing.T, res bson.M) {
			if res["ok"] != false {
				t.Errorf("SCRAM-13: saslContinue without saslStart should be rejected, got %v", res)
			}
		}))

	// SCRAM-12: saslContinue with the wrong conversationId is rejected.
	harness.AuthPairTest(t, wireCase(t, "SCRAM-12-wrong-conversationId", true,
		func(conn *wire.Conn, db, user, pwd string) (bson.M, error) {
			_, first, err := scramClient(user, pwd)
			if err != nil {
				return nil, err
			}
			if _, err := saslStart(conn, "SCRAM-SHA-256", first, db, nil); err != nil {
				return nil, err
			}
			r, err := saslContinue(conn, int32(99999), "c=biws,r=x,p=x", db)
			return bson.M{"continueOK": replyOK(r)}, err
		},
		func(t *testing.T, res bson.M) {
			if res["continueOK"] != false {
				t.Errorf("SCRAM-12: wrong conversationId should be rejected, got %v", res)
			}
		}))

	// SCRAM-14: a tampered nonce in client-final is rejected.
	harness.AuthPairTest(t, wireCase(t, "SCRAM-14-tampered-nonce", true,
		func(conn *wire.Conn, db, user, pwd string) (bson.M, error) {
			_, first, err := scramClient(user, pwd)
			if err != nil {
				return nil, err
			}
			start, err := saslStart(conn, "SCRAM-SHA-256", first, db, nil)
			if err != nil {
				return nil, err
			}
			r, err := saslContinue(conn, start["conversationId"], "c=biws,r=tamperednonce,p=AAAA", db)
			return bson.M{"authOK": replyOK(r)}, err
		},
		func(t *testing.T, res bson.M) {
			if res["authOK"] != false {
				t.Errorf("SCRAM-14: tampered nonce should fail auth, got %v", res)
			}
		}))

	// SCRAM-17: server-first echoes the stored iterationCount (i=15000 for
	// SCRAM-SHA-256).
	harness.AuthPairTest(t, wireCase(t, "SCRAM-17-iterationCount-in-server-first", true,
		func(conn *wire.Conn, db, user, pwd string) (bson.M, error) {
			_, first, err := scramClient(user, pwd)
			if err != nil {
				return nil, err
			}
			start, err := saslStart(conn, "SCRAM-SHA-256", first, db, nil)
			if err != nil {
				return nil, err
			}
			return bson.M{"hasIter15000": strings.Contains(payloadStr(start), "i=15000")}, nil
		},
		func(t *testing.T, res bson.M) {
			if res["hasIter15000"] != true {
				t.Errorf("SCRAM-17: server-first should carry i=15000, got %v", res)
			}
		}))

	// SCRAM-15: a full handshake with skipEmptyExchange authenticates and
	// completes with done:true in a single saslContinue.
	harness.AuthPairTest(t, wireCase(t, "SCRAM-15-skipEmptyExchange", true,
		func(conn *wire.Conn, db, user, pwd string) (bson.M, error) {
			res, err := fullHandshake(conn, db, user, pwd, true)
			return res, err
		},
		func(t *testing.T, res bson.M) {
			if res["authOK"] != true || res["done"] != true {
				t.Errorf("SCRAM-15: skipEmptyExchange should authenticate with done:true, got %v", res)
			}
		}))

	// SCRAM-16: a full handshake without skipEmptyExchange still authenticates
	// (via the classic exchange).
	harness.AuthPairTest(t, wireCase(t, "SCRAM-16-classic-exchange", true,
		func(conn *wire.Conn, db, user, pwd string) (bson.M, error) {
			res, err := fullHandshake(conn, db, user, pwd, false)
			return res, err
		},
		func(t *testing.T, res bson.M) {
			if res["authOK"] != true {
				t.Errorf("SCRAM-16: classic handshake should authenticate, got %v", res)
			}
		}))
}

// fullHandshake runs a complete SCRAM-SHA-256 handshake over conn and returns
// whether authentication succeeded and the final done flag. When the server
// leaves done:false after the client-final (classic exchange), an extra empty
// saslContinue is sent.
func fullHandshake(conn *wire.Conn, db, user, pwd string, skipEmpty bool) (bson.M, error) {
	conv, first, err := scramClient(user, pwd)
	if err != nil {
		return nil, err
	}
	var opts bson.D
	if skipEmpty {
		opts = bson.D{{Key: "skipEmptyExchange", Value: true}}
	}
	start, err := saslStart(conn, "SCRAM-SHA-256", first, db, opts)
	if err != nil {
		return nil, err
	}
	if !replyOK(start) {
		return bson.M{"authOK": false, "done": false}, nil
	}
	final, err := conv.Step(payloadStr(start))
	if err != nil {
		return nil, err
	}
	cont, err := saslContinue(conn, start["conversationId"], final, db)
	if err != nil {
		return nil, err
	}
	if replyOK(cont) && !replyBool(cont, "done") {
		// Classic exchange: consume the server-final with an empty saslContinue.
		if _, err := conv.Step(payloadStr(cont)); err == nil {
			cont, err = saslContinue(conn, start["conversationId"], "", db)
			if err != nil {
				return nil, err
			}
		}
	}
	return bson.M{"authOK": replyOK(cont), "done": replyBool(cont, "done")}, nil
}

// helloSpec runs a hello carrying a speculativeAuthenticate saslStart for the
// given mechanism/client-first against authDb, over a raw connection.
func helloSpec(conn *wire.Conn, mechanism, clientFirst, authDb string) (bson.M, error) {
	return conn.RunCommand(bson.D{
		{Key: "hello", Value: 1},
		{Key: "speculativeAuthenticate", Value: bson.D{
			{Key: "saslStart", Value: 1},
			{Key: "mechanism", Value: mechanism},
			{Key: "payload", Value: binPayload(clientFirst)},
			{Key: "db", Value: authDb},
		}},
		{Key: "$db", Value: "admin"},
	})
}

func TestAuthSpeculativeWire(t *testing.T) {
	// SPEC-01: a valid speculativeAuthenticate is answered in the hello reply.
	harness.AuthPairTest(t, wireCase(t, "SPEC-01-valid-speculative", true,
		func(conn *wire.Conn, db, user, pwd string) (bson.M, error) {
			_, first, err := scramClient(user, pwd)
			if err != nil {
				return nil, err
			}
			r, err := helloSpec(conn, "SCRAM-SHA-256", first, db)
			if err != nil {
				return nil, err
			}
			_, hasSpec := r["speculativeAuthenticate"]
			return bson.M{"helloOK": replyOK(r), "hasSpeculative": hasSpec}, nil
		},
		func(t *testing.T, res bson.M) {
			if res["helloOK"] != true || res["hasSpeculative"] != true {
				t.Errorf("SPEC-01: valid speculativeAuthenticate should be answered, got %v", res)
			}
		}))

	// SPEC-02: a speculativeAuthenticate for a mechanism the user lacks is
	// swallowed; the hello still succeeds without the field.
	harness.AuthPairTest(t, wireCase(t, "SPEC-02-wrong-mechanism-swallowed", true,
		func(conn *wire.Conn, db, user, pwd string) (bson.M, error) {
			r, err := helloSpec(conn, "SCRAM-SHA-1", "n,,n="+user+",r=abcdefgh", db)
			if err != nil {
				return nil, err
			}
			_, hasSpec := r["speculativeAuthenticate"]
			return bson.M{"helloOK": replyOK(r), "hasSpeculative": hasSpec}, nil
		},
		func(t *testing.T, res bson.M) {
			if res["helloOK"] != true || res["hasSpeculative"] != false {
				t.Errorf("SPEC-02: wrong-mechanism speculative should be swallowed, got %v", res)
			}
		}))

	// SPEC-03: a speculativeAuthenticate for an unknown user is swallowed.
	harness.AuthPairTest(t, wireCase(t, "SPEC-03-unknown-user-swallowed", false,
		func(conn *wire.Conn, db, user, pwd string) (bson.M, error) {
			_, first, err := scramClient("nobody_x", "whatever")
			if err != nil {
				return nil, err
			}
			r, err := helloSpec(conn, "SCRAM-SHA-256", first, db)
			if err != nil {
				return nil, err
			}
			_, hasSpec := r["speculativeAuthenticate"]
			return bson.M{"helloOK": replyOK(r), "hasSpeculative": hasSpec}, nil
		},
		func(t *testing.T, res bson.M) {
			if res["helloOK"] != true || res["hasSpeculative"] != false {
				t.Errorf("SPEC-03: unknown-user speculative should be swallowed, got %v", res)
			}
		}))

	// SPEC-07: continue the conversation started speculatively in hello and
	// complete authentication.
	harness.AuthPairTest(t, wireCase(t, "SPEC-07-speculative-then-continue", true,
		func(conn *wire.Conn, db, user, pwd string) (bson.M, error) {
			conv, first, err := scramClient(user, pwd)
			if err != nil {
				return nil, err
			}
			r, err := helloSpec(conn, "SCRAM-SHA-256", first, db)
			if err != nil {
				return nil, err
			}
			spec, ok := r["speculativeAuthenticate"].(bson.M)
			if !ok {
				return bson.M{"authOK": false}, nil
			}
			final, err := conv.Step(payloadStr(spec))
			if err != nil {
				return nil, err
			}
			cont, err := saslContinue(conn, spec["conversationId"], final, db)
			if err != nil {
				return nil, err
			}
			return bson.M{"authOK": replyOK(cont), "done": replyBool(cont, "done")}, nil
		},
		func(t *testing.T, res bson.M) {
			if res["authOK"] != true {
				t.Errorf("SPEC-07: speculative-then-continue should authenticate, got %v", res)
			}
		}))
}

// TestAuthScramReauthWire covers SCRAM-20: starting a new SASL conversation for
// a second user on a connection already authenticated as another user.
func TestAuthScramReauthWire(t *testing.T) {
	harness.AuthPairTest(t, authCase("SCRAM-20-reauth-second-user", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "wire_" + tgt.NS
		u1, u2, pw := "u1_"+tgt.NS, "u2_"+tgt.NS, "pw"
		defer func() {
			_ = harness.DropUser(ctx, tgt.Admin, db, u1)
			_ = harness.DropUser(ctx, tgt.Admin, db, u2)
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()
		for _, u := range []string{u1, u2} {
			if err := createUserMech(ctx, tgt.Admin, db, u, pw, rwRole(db), []string{"SCRAM-SHA-256"}); err != nil {
				return nil, err
			}
		}
		conn, err := wire.Dial(tgt.BaseURI)
		if err != nil {
			return nil, err
		}
		defer func() { _ = conn.Close() }()
		// Authenticate fully as u1.
		if _, err := fullHandshake(conn, db, u1, pw, true); err != nil {
			return nil, err
		}
		// Begin a fresh SASL conversation for u2 on the same connection.
		_, first, err := scramClient(u2, pw)
		if err != nil {
			return nil, err
		}
		start, err := saslStart(conn, "SCRAM-SHA-256", first, db, bson.D{{Key: "skipEmptyExchange", Value: true}})
		if err != nil {
			return nil, err
		}
		return bson.M{"secondStartOK": replyOK(start)}, nil
	}))
}
