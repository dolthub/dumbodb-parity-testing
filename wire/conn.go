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

// Package wire is a minimal MongoDB wire-protocol client for
// parity tests that need server behavior the Go driver hides. The MongoDB
// Driver Specification forbids drivers from accepting a caller-supplied lsid,
// so tests that share an lsid across connections must speak OP_MSG directly.
package wire

import (
	"bytes"
	"compress/zlib"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	opCompressed   int32 = 2012
	opMsg          int32 = 2013
	compressorZlib byte  = 2
)

// Conn is not safe for concurrent use.
type Conn struct {
	c     net.Conn
	reqID int32
}

// Dial accepts "host:port" or "mongodb://host:port".
func Dial(addr string) (*Conn, error) {
	hp, err := hostPort(addr)
	if err != nil {
		return nil, err
	}
	c, err := net.Dial("tcp", hp)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", hp, err)
	}
	return &Conn{c: c}, nil
}

func (c *Conn) Close() error { return c.c.Close() }

func (c *Conn) SetDeadline(t time.Time) error { return c.c.SetDeadline(t) }

// OP_MSG flag bits.
const (
	// FlagMoreToCome is set by a server on each response of an exhaust
	// stream except the last, and by a client on a fire-and-forget request.
	FlagMoreToCome uint32 = 1 << 1
	// FlagExhaustAllowed is set by a client to tell the server it may reply
	// with an exhaust stream.
	FlagExhaustAllowed uint32 = 1 << 16
)

// RunCommand sends cmd as the body section of an OP_MSG.
// cmd must include "$db".
func (c *Conn) RunCommand(cmd interface{}) (bson.M, error) {
	reply, _, err := c.RunCommandFlags(cmd, 0)
	return reply, err
}

// RunCommandFlags is RunCommand with control over the request's OP_MSG flag
// bits, returning the response's flag bits alongside its body.
//
// The flags are the point of several member-protocol assertions: an exhaust
// hello is a request carrying FlagExhaustAllowed and a reply carrying
// FlagMoreToCome, and neither is visible through the Go driver or through
// RunCommand, which sends zero and discards what comes back.
func (c *Conn) RunCommandFlags(cmd interface{}, flags uint32) (bson.M, uint32, error) {
	body, err := bson.Marshal(cmd)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal cmd: %w", err)
	}

	c.reqID++
	const headerLen = 16
	const flagBitsLen = 4
	const sectionKindLen = 1
	msgLen := int32(headerLen + flagBitsLen + sectionKindLen + len(body))

	out := make([]byte, 0, msgLen)
	out = appendI32(out, msgLen)
	out = appendI32(out, c.reqID)
	out = appendI32(out, 0) // responseTo
	out = appendI32(out, opMsg)
	out = appendI32(out, int32(flags))
	out = append(out, 0) // section kind 0 (body)
	out = append(out, body...)

	if _, err := c.c.Write(out); err != nil {
		return nil, 0, fmt.Errorf("write: %w", err)
	}
	return c.readCommandReply()
}

// RunZlibCompressedCommand sends an OP_COMPRESSED command without negotiating
// compression first.
func (c *Conn) RunZlibCompressedCommand(cmd interface{}) (bson.M, error) {
	document, err := bson.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("marshal cmd: %w", err)
	}
	uncompressed := make([]byte, 0, 5+len(document))
	uncompressed = appendI32(uncompressed, 0)
	uncompressed = append(uncompressed, 0)
	uncompressed = append(uncompressed, document...)

	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	if _, err := writer.Write(uncompressed); err != nil {
		return nil, fmt.Errorf("compress cmd: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("finish compressed cmd: %w", err)
	}

	c.reqID++
	const headerLen = 16
	const compressedHeaderLen = 9
	msgLen := int32(headerLen + compressedHeaderLen + compressed.Len())
	out := make([]byte, 0, msgLen)
	out = appendI32(out, msgLen)
	out = appendI32(out, c.reqID)
	out = appendI32(out, 0)
	out = appendI32(out, opCompressed)
	out = appendI32(out, opMsg)
	out = appendI32(out, int32(len(uncompressed)))
	out = append(out, compressorZlib)
	out = append(out, compressed.Bytes()...)
	if _, err := c.c.Write(out); err != nil {
		return nil, fmt.Errorf("write compressed command: %w", err)
	}
	reply, _, err := c.readCommandReply()
	return reply, err
}

func (c *Conn) readCommandReply() (bson.M, uint32, error) {
	const headerLen = 16
	const flagBitsLen = 4
	const sectionKindLen = 1
	var hdr [headerLen]byte
	if _, err := io.ReadFull(c.c, hdr[:]); err != nil {
		return nil, 0, fmt.Errorf("read header: %w", err)
	}
	replyLen := int32(binary.LittleEndian.Uint32(hdr[0:4]))
	if replyLen < headerLen+flagBitsLen+sectionKindLen {
		return nil, 0, fmt.Errorf("short OP_MSG reply: %d bytes", replyLen)
	}
	replyOp := int32(binary.LittleEndian.Uint32(hdr[12:16]))
	if replyOp != opMsg {
		return nil, 0, fmt.Errorf("unexpected reply opcode %d (want OP_MSG)", replyOp)
	}

	rest := make([]byte, replyLen-headerLen)
	if _, err := io.ReadFull(c.c, rest); err != nil {
		return nil, 0, fmt.Errorf("read body: %w", err)
	}

	// rest layout: [4 flagBits][1 sectionKind][body...]
	if len(rest) < 5 {
		return nil, 0, errors.New("malformed OP_MSG reply: truncated section header")
	}
	replyFlags := binary.LittleEndian.Uint32(rest[0:4])
	sectionKind := rest[4]
	if sectionKind != 0 {
		return nil, 0, fmt.Errorf("unexpected section kind %d (only kind 0 supported)", sectionKind)
	}

	var reply bson.M
	if err := bson.Unmarshal(rest[5:], &reply); err != nil {
		return nil, 0, fmt.Errorf("decode reply: %w", err)
	}
	return reply, replyFlags, nil
}

// NewLsid returns a random UUID v4 as a BSON subtype-4 binary, suitable as
// the inner field of `lsid: { id: ... }`.
func NewLsid() primitive.Binary {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0F) | 0x40 // version 4
	b[8] = (b[8] & 0x3F) | 0x80 // RFC 4122 variant
	return primitive.Binary{Subtype: 0x04, Data: append([]byte(nil), b[:]...)}
}

func appendI32(buf []byte, v int32) []byte {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(v))
	return append(buf, b[:]...)
}

func hostPort(addr string) (string, error) {
	if !strings.Contains(addr, "://") {
		return addr, nil
	}
	u, err := url.Parse(addr)
	if err != nil {
		return "", fmt.Errorf("parse uri: %w", err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("no host in URI %q", addr)
	}
	return u.Host, nil
}
