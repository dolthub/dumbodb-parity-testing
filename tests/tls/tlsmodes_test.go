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

// tlsMode, workspace-09n.4.
//
// allowTLS and preferTLS serve plaintext and TLS on one port, which is what
// workspace-lkd was filed as a P1 for doing under requireTLS. The mode that
// must NOT do it is therefore tested here beside the modes that must, so the
// two can never drift apart unnoticed.

package tls

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func TestTLSModes_PlaintextAcceptedOnlyWhereTheModeSaysSo(t *testing.T) {
	t.Parallel()
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)

	for _, mode := range []string{"requireTLS", "allowTLS", "preferTLS"} {
		t.Run(mode, func(t *testing.T) {
			opts := harness.TLSOptions{Mode: mode}
			mongod := harness.StartTLSMongod(t, f, opts)
			if mongod.StartFailed {
				t.Fatalf("premise failed: mongod would not start with --tlsMode %s:\n%s", mode, mongod.FailureOutput)
			}
			dumbodb := harness.StartTLSDumboDB(t, f, opts)
			if dumbodb.StartFailed {
				t.Fatalf("dumbodb would not start with --tlsMode %s, which mongod accepts: %s",
					mode, firstLine(dumbodb.FailureOutput))
			}

			mongodPlaintext := mongod.ConnectPlaintext(ctx, t) == nil
			dumbodbPlaintext := dumbodb.ConnectPlaintext(ctx, t) == nil
			t.Logf("--tlsMode %s: plaintext accepted by mongod=%v, dumbodb=%v", mode, mongodPlaintext, dumbodbPlaintext)

			if mode == "requireTLS" && dumbodbPlaintext {
				t.Error("dumbodb served a plaintext client under requireTLS; this is workspace-lkd returning")
			}
			if mongodPlaintext != dumbodbPlaintext {
				t.Errorf("--tlsMode %s: mongod accepted plaintext=%v but dumbodb accepted=%v",
					mode, mongodPlaintext, dumbodbPlaintext)
			}

			// Whatever the mode does about plaintext, TLS has to keep working.
			for _, s := range []struct {
				name   string
				server *harness.TLSServer
			}{{"mongod", mongod}, {"dumbodb", dumbodb}} {
				client, err := s.server.Connect(ctx, t, true)
				if err != nil {
					t.Errorf("%s refused a TLS client under --tlsMode %s: %v", s.name, mode, err)
					continue
				}
				_ = client.Disconnect(context.Background())
			}
		})
	}
}

// A plaintext client and a TLS client against the same running server, so the
// routing is shown to work per connection rather than being fixed at the
// first one.
func TestTLSModes_AllowTLSServesBothOnOneListener(t *testing.T) {
	t.Parallel()
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	opts := harness.TLSOptions{Mode: "allowTLS"}

	for _, s := range []struct {
		name   string
		server *harness.TLSServer
	}{
		{"mongod", harness.StartTLSMongod(t, f, opts)},
		{"dumbodb", harness.StartTLSDumboDB(t, f, opts)},
	} {
		t.Run(s.name, func(t *testing.T) {
			if s.server.StartFailed {
				t.Fatalf("%s would not start with --tlsMode allowTLS: %s", s.name, firstLine(s.server.FailureOutput))
			}
			if err := s.server.ConnectPlaintext(ctx, t); err != nil {
				t.Errorf("%s refused a plaintext client under allowTLS: %v", s.name, err)
			}
			client, err := s.server.Connect(ctx, t, true)
			if err != nil {
				t.Errorf("%s refused a TLS client under allowTLS: %v", s.name, err)
				return
			}
			_ = client.Disconnect(context.Background())
			// Again, after the TLS connection, to show one does not poison
			// the listener for the other.
			if err := s.server.ConnectPlaintext(ctx, t); err != nil {
				t.Errorf("%s refused a plaintext client after serving a TLS one: %v", s.name, err)
			}
		})
	}
}

// A plaintext message whose length makes its header look like a TLS record.
//
// Under allowTLS the listener peeks at the first bytes to decide between
// plaintext and TLS. A MongoDB message begins with its length as a
// little-endian int32, so a message of exactly 66326 bytes begins 16 03 01,
// which is also how a TLS record begins. workspace-09n.18 was that collision:
// such a message was handed to the TLS parser and the connection died.
//
// The lengths below are the ones whose low three bytes are 16 03 01 through
// 16 03 03. requestID matters too: an earlier fix checked bytes 3 to 5, which
// in a MongoDB header are the length's high byte and the first two bytes of
// requestID, so ids from 260 to 511 still collided.
func TestTLSModes_PlaintextSurvivesTLSLookingLengths(t *testing.T) {
	t.Parallel()
	ctx := tlsContext(t)
	f := harness.NewTLSFixture(t)
	server := harness.StartTLSDumboDB(t, f, harness.TLSOptions{Mode: "allowTLS"})
	if server.StartFailed {
		t.Fatalf("dumbodb would not start with --tlsMode allowTLS: %s", firstLine(server.FailureOutput))
	}
	// A TLS client on the same port, so a server that simply stopped
	// detecting TLS cannot pass this test.
	cli, err := server.Connect(ctx, t, true)
	if err != nil {
		t.Fatalf("the port no longer serves TLS at all: %v", err)
	}
	_ = cli.Disconnect(context.Background())

	for _, c := range []struct {
		length    int
		requestID int32
	}{
		{66326, 1}, {66326, 260}, {66326, 300}, {66326, 511},
		{131862, 400}, {197398, 260},
	} {
		t.Run(fmt.Sprintf("len%d_id%d", c.length, c.requestID), func(t *testing.T) {
			msg := opMsgOfExactLength(t, c.length, c.requestID)
			if msg[0] != 0x16 || msg[1] != 0x03 {
				t.Fatalf("premise failed: a %d byte message does not begin 16 03, it begins %02x %02x",
					c.length, msg[0], msg[1])
			}
			reply, err := sendRaw(t, server.Addr, msg)
			if err != nil {
				t.Fatalf("length %d, requestID %d: %v; the message was handed to the TLS parser",
					c.length, c.requestID, err)
			}
			if len(reply) > 0 && reply[0] == 0x15 {
				t.Fatalf("length %d, requestID %d: the server answered with a TLS alert record, so a plaintext message was parsed as TLS",
					c.length, c.requestID)
			}
		})
	}
}

// opMsgOfExactLength builds a valid OP_MSG ping of exactly totalLen bytes by
// padding one field, so the message really is the length its header claims.
func opMsgOfExactLength(t *testing.T, totalLen int, requestID int32) []byte {
	t.Helper()
	const headerAndFlags = 16 + 4 + 1
	target := totalLen - headerAndFlags

	pad := target // first guess, corrected below
	var body []byte
	for i := 0; i < 8; i++ {
		var err error
		body, err = bson.Marshal(bson.D{
			{Key: "ping", Value: int32(1)},
			{Key: "$db", Value: "admin"},
			{Key: "pad", Value: strings.Repeat("x", pad)},
		})
		if err != nil {
			t.Fatalf("marshalling the padded command: %v", err)
		}
		if len(body) == target {
			break
		}
		pad += target - len(body)
	}
	if len(body) != target {
		t.Fatalf("could not build a body of exactly %d bytes, got %d", target, len(body))
	}

	msg := make([]byte, 0, totalLen)
	msg = binary.LittleEndian.AppendUint32(msg, uint32(totalLen))
	msg = binary.LittleEndian.AppendUint32(msg, uint32(requestID))
	msg = binary.LittleEndian.AppendUint32(msg, 0) // responseTo
	msg = binary.LittleEndian.AppendUint32(msg, 2013)
	msg = binary.LittleEndian.AppendUint32(msg, 0) // flagBits
	msg = append(msg, 0)                           // section kind 0
	msg = append(msg, body...)
	if len(msg) != totalLen {
		t.Fatalf("built %d bytes, wanted %d", len(msg), totalLen)
	}
	return msg
}

// sendRaw writes a complete message over plaintext TCP and reads the reply.
func sendRaw(t *testing.T, addr string, msg []byte) ([]byte, error) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	if _, err := conn.Write(msg); err != nil {
		return nil, err
	}
	reply := make([]byte, 64)
	n, err := conn.Read(reply)
	if err != nil {
		return nil, err
	}
	return reply[:n], nil
}
