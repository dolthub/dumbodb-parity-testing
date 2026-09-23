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
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"testing"
	"time"
)

// The fixture is only worth anything if the material completes a handshake.
// Files that parse but do not verify would let a TLS test pass while proving
// nothing, so this stands a real server up and connects to it.
func TestTLSFixture_CompletesAHandshake(t *testing.T) {
	f := NewTLSFixture(t)

	pool := x509.NewCertPool()
	caPEM, err := os.ReadFile(f.CAFile)
	if err != nil {
		t.Fatalf("reading CA: %v", err)
	}
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("the generated CA is not a usable PEM certificate")
	}

	serverCert, err := tls.LoadX509KeyPair(f.ServerCertFile, f.ServerKeyFile)
	if err != nil {
		t.Fatalf("loading the server certificate from its separate files: %v", err)
	}
	// mongod takes the combined form, so it has to load too.
	if _, err := tls.LoadX509KeyPair(f.ServerPEMFile, f.ServerPEMFile); err != nil {
		t.Fatalf("loading the server certificate from the combined pem: %v", err)
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
	})
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	defer func() { _ = listener.Close() }()
	// Accept in a loop: the subtests below dial once each, and a single
	// Accept would leave the second one hanging until the test timed out.
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go func() {
				_ = conn.(*tls.Conn).Handshake()
				_ = conn.Close()
			}()
		}
	}()

	clientCert, err := tls.LoadX509KeyPair(f.ClientCertFile, f.ClientKeyFile)
	if err != nil {
		t.Fatalf("loading the client certificate: %v", err)
	}

	// Dialed by IP, because a replica set member configured as 127.0.0.1 is
	// verified against that address and not against a hostname.
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("splitting listener address: %v", err)
	}
	for _, host := range []string{"127.0.0.1", "localhost"} {
		t.Run(host, func(t *testing.T) {
			conn, dialErr := tls.Dial("tcp", net.JoinHostPort(host, port), &tls.Config{
				RootCAs:      pool,
				Certificates: []tls.Certificate{clientCert},
				ServerName:   host,
			})
			if dialErr != nil {
				t.Fatalf("handshake against %s failed, so the server certificate does not cover it: %v", host, dialErr)
			}
			_ = conn.Close()
		})
	}
}

// A server that demands client certificates must actually reject a client
// without one. If it does not, a mutual-TLS test proves nothing.
func TestTLSFixture_RejectsAClientWithoutACertificate(t *testing.T) {
	f := NewTLSFixture(t)

	pool := x509.NewCertPool()
	caPEM, err := os.ReadFile(f.CAFile)
	if err != nil {
		t.Fatalf("reading CA: %v", err)
	}
	pool.AppendCertsFromPEM(caPEM)
	serverCert, err := tls.LoadX509KeyPair(f.ServerCertFile, f.ServerKeyFile)
	if err != nil {
		t.Fatalf("loading the server certificate: %v", err)
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
	})
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	defer func() { _ = listener.Close() }()
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			_ = conn.(*tls.Conn).Handshake()
			_ = conn.Close()
		}
	}()

	conn, err := tls.Dial("tcp", listener.Addr().String(), &tls.Config{
		RootCAs:    pool,
		ServerName: "127.0.0.1",
	})
	if err != nil {
		return // rejected during the handshake, which is the outcome under test
	}
	defer func() { _ = conn.Close() }()

	// Dialing without error is not acceptance. Under TLS 1.3 the client
	// finishes its side of the handshake before the server has validated the
	// certificate it was never sent, so the rejection arrives on the first
	// read. A test that stopped at the dial would report a server enforcing
	// nothing as a server enforcing everything.
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("a client with no certificate was served by a server requiring one")
	}
}
