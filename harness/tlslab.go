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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TLSOptions describes one server's TLS configuration, in terms that map onto
// the flags both servers take.
type TLSOptions struct {
	// CertificateKeyFile is the combined certificate and key. Empty means the
	// fixture's valid server material.
	CertificateKeyFile string

	// CAFile is the authority used to verify client certificates. Empty means
	// the fixture's own CA.
	CAFile string

	// AllowConnectionsWithoutCertificates maps to mongod's flag of the same
	// name: trust the CA but do not demand a client certificate.
	AllowConnectionsWithoutCertificates bool

	// CRLFile maps to --tlsCRLFile. Empty leaves revocation unconfigured,
	// which is the state in which a revoked certificate still authenticates.
	CRLFile string

	// DisabledProtocols maps to --tlsDisabledProtocols, as mongod spells them:
	// TLS1_0, TLS1_1, TLS1_2, TLS1_3.
	DisabledProtocols string

	// NoCAFile omits --tlsCAFile entirely, rather than substituting the
	// fixture's own. Needed to ask what each server does with revocation
	// configured and nothing to verify against.
	NoCAFile bool

	// ExtraArgs are appended verbatim, for flags only one server has.
	ExtraArgs []string

	// Mode maps to --tlsMode. Empty means requireTLS, which is what almost
	// every case here wants.
	Mode string

	// KeyPassword maps to --tlsCertificateKeyFilePassword.
	KeyPassword string

	// Auth enables access control. Both servers spell it differently, so the
	// lab renders it rather than the caller.
	Auth bool
}

// TLSServer is one server started with TLS, or one that refused to start.
//
// A server refusing to start is a result rather than an error. Half the
// behavior worth comparing here is which bad configurations each server
// rejects outright, so StartFailed carries that instead of failing the test.
type TLSServer struct {
	Addr          string
	StartFailed   bool
	FailureOutput string

	fixture *TLSFixture
	proc    *serverProc
	exited  chan struct{}

	bin  string
	name string
	dir  string
	args []string
}

// StartTLSDumboDB starts DumboDB with TLS on its only listening port.
//
// There is deliberately no plaintext port. DumboDB matches mongod here: TLS is
// a mode of the one address rather than a second address, so a test cannot
// accidentally reach the server over plaintext and conclude TLS was enforced.
func StartTLSDumboDB(t *testing.T, f *TLSFixture, opts TLSOptions) *TLSServer {
	t.Helper()
	bin := findDumboDBBinary()
	if bin == "" {
		t.Skip("dumbodb binary not found (set DUMBODB_BIN)")
	}
	port, dir := tlsPortAndDir(t, "dumbodb-tls-")
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	args := []string{
		"--addr", addr,
		"--data-dir", dir,
		"--tlsMode", f.orDefault(opts.Mode, "requireTLS"),
		"--tlsCertificateKeyFile", f.orDefault(opts.CertificateKeyFile, f.ServerPEMFile),
	}
	if !opts.NoCAFile {
		args = append(args, "--tlsCAFile", f.orDefault(opts.CAFile, f.CAFile))
	}
	if opts.Auth {
		args = append(args, "--auth")
	}
	args = append(args, optionalTLSArgs(opts)...)
	return startTLSServer(t, f, bin, "dumbodb-tls", addr, dir, args)
}

// StartTLSMongod starts mongod with the equivalent configuration, as the
// oracle every DumboDB behavior here is compared against.
func StartTLSMongod(t *testing.T, f *TLSFixture, opts TLSOptions) *TLSServer {
	t.Helper()
	bin := findMongodBin()
	if bin == "" {
		t.Skip("mongod binary not found (set MONGOD_BIN)")
	}
	port, dir := tlsPortAndDir(t, "mongod-tls-")
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	args := []string{
		"--port", fmt.Sprintf("%d", port),
		"--dbpath", dir,
		"--bind_ip", "127.0.0.1",
		"--nounixsocket",
		// Without this each mongod reserves roughly half of available RAM for
		// WiredTiger. Harmless one at a time, ruinous in parallel; this is the
		// cap that stopped the replication suite being flaky.
		"--wiredTigerCacheSizeGB", "0.25",
		"--tlsMode", f.orDefault(opts.Mode, "requireTLS"),
		"--tlsCertificateKeyFile", f.orDefault(opts.CertificateKeyFile, f.ServerPEMFile),
	}
	if !opts.NoCAFile {
		args = append(args, "--tlsCAFile", f.orDefault(opts.CAFile, f.CAFile))
	}
	if opts.Auth {
		args = append(args, "--auth")
	}
	args = append(args, optionalTLSArgs(opts)...)
	return startTLSServer(t, f, bin, "mongod-tls", addr, dir, args)
}

// optionalTLSArgs renders the options both servers spell identically, which
// is the point of comparing them: the same command line goes to each.
func optionalTLSArgs(opts TLSOptions) []string {
	var args []string
	if opts.AllowConnectionsWithoutCertificates {
		args = append(args, "--tlsAllowConnectionsWithoutCertificates")
	}
	if opts.CRLFile != "" {
		args = append(args, "--tlsCRLFile", opts.CRLFile)
	}
	if opts.DisabledProtocols != "" {
		args = append(args, "--tlsDisabledProtocols", opts.DisabledProtocols)
	}
	if opts.KeyPassword != "" {
		args = append(args, "--tlsCertificateKeyFilePassword", opts.KeyPassword)
	}
	return append(args, opts.ExtraArgs...)
}

func startTLSServer(t *testing.T, f *TLSFixture, bin, name, addr, dir string, args []string) *TLSServer {
	t.Helper()
	proc, err := startProc(exec.Command(bin, args...), name, dir)
	if err != nil {
		t.Fatalf("launching %s: %v", name, err)
	}
	s := &TLSServer{Addr: addr, fixture: f, proc: proc, bin: bin, name: name, dir: dir, args: args}

	// Reap the process here rather than leaving it to serverProc.stop.
	//
	// A server that rejects its configuration exits immediately, and an
	// unreaped exited process is a zombie whose pid still answers signal 0.
	// Polling liveness that way reported a dead server as running and spent
	// the full start timeout on every case meant to fail. Waiting is the only
	// way to see the difference, and only one waiter is allowed, so this owns
	// it and teardown does not.
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		_ = proc.cmd.Wait()
	}()
	s.exited = exited
	t.Cleanup(func() {
		if proc.cmd.Process != nil {
			_ = proc.cmd.Process.Kill()
		}
		<-exited
		if proc.logf != nil {
			_ = proc.logf.Close()
		}
	})

	// A server that rejects its configuration exits instead of listening, and
	// that is one of the outcomes under test rather than a harness failure.
	//
	// Watch for the process exiting as well as for the port opening, rather
	// than waiting out a timeout for an address that will never accept. A
	// rejected configuration is reported in under a second this way, where
	// waiting for the port alone spent twenty five seconds per server on every
	// case that was supposed to fail.
	if !waitListeningOrExit(exited, addr, 25*time.Second) {
		output := readServerLog(proc)
		// A server that could not take the address did not reject its
		// configuration; the harness handed it a port something else already
		// held. Several cases read StartFailed as "this configuration was
		// refused", so letting a collision through here would be a wrong
		// answer rather than an error. freePort stops this happening within
		// the process; this catches a collision with anything else.
		if addressInUse(output) {
			t.Fatalf("%s could not bind %s: the harness handed out a port something else holds.\n%s", name, addr, output)
		}
		s.StartFailed = true
		s.FailureOutput = output
		s.proc.stop()
		s.proc = nil
	}
	return s
}

// Exited reports whether the process has already terminated, which
// distinguishes a server that stopped accepting connections from one that
// died. Signal 0 cannot tell those apart, since it succeeds against a zombie.
func (s *TLSServer) Exited() bool {
	if s.exited == nil {
		return true
	}
	select {
	case <-s.exited:
		return true
	default:
		return false
	}
}

// Restart stops the server and starts it again on the same address, data
// directory and command line. Material named by that command line is read
// afresh, which is the whole point: rotation is a file being replaced and a
// process being restarted onto it.
func (s *TLSServer) Restart(t *testing.T) *TLSServer {
	t.Helper()
	if s.proc != nil && s.proc.cmd.Process != nil {
		_ = s.proc.cmd.Process.Kill()
	}
	if s.exited != nil {
		<-s.exited
	}
	return startTLSServer(t, s.fixture, s.bin, s.name, s.Addr, s.dir, s.args)
}

// ConnectX509 authenticates with MONGODB-X509, presenting certFile.
//
// The authentication source is $external and is not a parameter. The drivers
// specification fixes it for this mechanism and refuses any other value
// before contacting the server, so a lab that made it configurable would be
// testing something no client can do. Where a server STORES the user is its
// own business; this is the name on the wire.
func (s *TLSServer) ConnectX509(ctx context.Context, t *testing.T, certFile string) (*mongo.Client, error) {
	t.Helper()
	return s.connectAuth(ctx, t, certFile, options.Credential{AuthMechanism: "MONGODB-X509", AuthSource: "$external"})
}

// ConnectX509Claiming presents certFile while asking to be authenticated as
// claimedDN, which may be somebody else's name.
//
// MONGODB-X509 lets a client name itself in the request. The server is
// supposed to take the identity from the certificate regardless, so this is
// the call that separates a server checking the certificate from one taking
// the client's word.
func (s *TLSServer) ConnectX509Claiming(ctx context.Context, t *testing.T, certFile, claimedDN string) (*mongo.Client, error) {
	t.Helper()
	return s.connectAuth(ctx, t, certFile, options.Credential{
		AuthMechanism: "MONGODB-X509", AuthSource: "$external", Username: claimedDN,
	})
}

// ConnectX509Plaintext attempts MONGODB-X509 over a connection with no TLS,
// which only a server in allowTLS or preferTLS will accept at the transport
// layer. There is no certificate on such a connection, so there is nothing to
// identify the client with, and the attempt must fail.
func (s *TLSServer) ConnectX509Plaintext(ctx context.Context, t *testing.T) (*mongo.Client, error) {
	t.Helper()
	if s.StartFailed {
		t.Fatalf("cannot connect to %s: it never started", s.Addr)
	}
	client, err := mongo.Connect(ctx, options.Client().
		ApplyURI("mongodb://"+s.Addr+"/?directConnection=true").
		SetAuth(options.Credential{AuthMechanism: "MONGODB-X509", AuthSource: "$external"}).
		SetServerSelectionTimeout(6*time.Second))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	return client, nil
}

// ConnectAsUserWithout authenticates with SCRAM while presenting NO client
// certificate, which only a server allowing certificate-free connections will
// accept. It is how the matrix asks whether that flag leaves password
// authentication alone.
func (s *TLSServer) ConnectAsUserWithout(ctx context.Context, t *testing.T, user, password string) (*mongo.Client, error) {
	t.Helper()
	return s.connectAuth(ctx, t, "", options.Credential{
		AuthMechanism: "SCRAM-SHA-256", AuthSource: "admin", Username: user, Password: password,
	})
}

// ConnectAsUser authenticates with SCRAM, for bootstrapping the users the
// X.509 cases need.
func (s *TLSServer) ConnectAsUser(ctx context.Context, t *testing.T, user, password string) (*mongo.Client, error) {
	t.Helper()
	return s.connectAuth(ctx, t, s.fixture.ClientPEMFile, options.Credential{
		AuthMechanism: "SCRAM-SHA-256", AuthSource: "admin", Username: user, Password: password,
	})
}

func (s *TLSServer) connectAuth(ctx context.Context, t *testing.T, certFile string, cred options.Credential) (*mongo.Client, error) {
	t.Helper()
	if s.StartFailed {
		t.Fatalf("cannot connect to %s: it never started", s.Addr)
	}
	pool := x509.NewCertPool()
	caPEM, err := os.ReadFile(s.fixture.CAFile)
	if err != nil {
		t.Fatalf("reading CA: %v", err)
	}
	pool.AppendCertsFromPEM(caPEM)
	config := &tls.Config{RootCAs: pool, ServerName: "127.0.0.1"}
	if certFile != "" {
		cert, certErr := tls.LoadX509KeyPair(certFile, certFile)
		if certErr != nil {
			t.Fatalf("loading client certificate %s: %v", certFile, certErr)
		}
		config.Certificates = []tls.Certificate{cert}
	}
	client, err := mongo.Connect(ctx, options.Client().
		ApplyURI("mongodb://"+s.Addr+"/?directConnection=true").
		SetTLSConfig(config).
		SetAuth(cred).
		SetServerSelectionTimeout(6*time.Second))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	return client, nil
}

// ConnectAs dials presenting client material from a different fixture, so a
// test can ask what happens to a client whose certificate was signed by a CA
// the server no longer trusts. The server is still verified against the
// fixture it was started from.
func (s *TLSServer) ConnectAs(ctx context.Context, t *testing.T, client *TLSFixture) (*mongo.Client, error) {
	t.Helper()
	return s.connectWith(ctx, t, client.ClientCertFile, client.ClientKeyFile)
}

// Connect dials the server over TLS. withClientCertificate decides whether the
// client presents one, which is the difference the CA options govern.
//
// The returned error is the outcome under test in most cases here, so callers
// get it rather than the test being failed for them.
func (s *TLSServer) Connect(ctx context.Context, t *testing.T, withClientCertificate bool) (*mongo.Client, error) {
	t.Helper()
	if s.StartFailed {
		t.Fatalf("cannot connect to %s: it never started", s.Addr)
	}

	if !withClientCertificate {
		return s.connectWith(ctx, t, "", "")
	}
	return s.connectWith(ctx, t, s.fixture.ClientCertFile, s.fixture.ClientKeyFile)
}

func (s *TLSServer) connectWith(ctx context.Context, t *testing.T, certFile, keyFile string) (*mongo.Client, error) {
	t.Helper()
	if s.StartFailed {
		t.Fatalf("cannot connect to %s: it never started", s.Addr)
	}
	pool := x509.NewCertPool()
	caPEM, err := os.ReadFile(s.fixture.CAFile)
	if err != nil {
		t.Fatalf("reading CA: %v", err)
	}
	pool.AppendCertsFromPEM(caPEM)

	config := &tls.Config{RootCAs: pool, ServerName: "127.0.0.1"}
	if certFile != "" {
		cert, certErr := tls.LoadX509KeyPair(certFile, keyFile)
		if certErr != nil {
			t.Fatalf("loading client certificate: %v", certErr)
		}
		config.Certificates = []tls.Certificate{cert}
	}

	client, err := mongo.Connect(ctx, options.Client().
		ApplyURI("mongodb://"+s.Addr+"/?directConnection=true").
		SetTLSConfig(config).
		SetServerSelectionTimeout(6*time.Second))
	if err != nil {
		return nil, err
	}
	// Ping rather than trusting Connect. Under TLS 1.3 a client finishes its
	// side of the handshake before the server has validated the certificate it
	// was never sent, so a rejected connection looks established until
	// something is actually sent over it.
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	return client, nil
}

// ConnectWithVersion dials with the client pinned to exactly one TLS version,
// which is the only way to learn what a server actually negotiates. Reading
// the configuration back would just restate what was asked for.
//
// It returns the negotiated version so a caller can tell "refused" from
// "quietly negotiated something else".
func (s *TLSServer) ConnectWithVersion(t *testing.T, version uint16) (uint16, error) {
	t.Helper()
	return s.connect(t, version, version)
}

func (s *TLSServer) connect(t *testing.T, minVersion, maxVersion uint16) (uint16, error) {
	t.Helper()
	if s.StartFailed {
		t.Fatalf("cannot connect to %s: it never started", s.Addr)
	}
	pool := x509.NewCertPool()
	caPEM, err := os.ReadFile(s.fixture.CAFile)
	if err != nil {
		t.Fatalf("reading CA: %v", err)
	}
	pool.AppendCertsFromPEM(caPEM)
	cert, err := tls.LoadX509KeyPair(s.fixture.ClientCertFile, s.fixture.ClientKeyFile)
	if err != nil {
		t.Fatalf("loading client certificate: %v", err)
	}

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", s.Addr, &tls.Config{
		RootCAs:      pool,
		Certificates: []tls.Certificate{cert},
		ServerName:   "127.0.0.1",
		MinVersion:   minVersion,
		MaxVersion:   maxVersion,
	})
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()
	// Handshake completion is not acceptance under TLS 1.3, so read before
	// believing it. The server rejects after the client is already done.
	//
	// The window is short on purpose. A rejection is an alert the server has
	// already queued, so it arrives at once; silence means acceptance. Waiting
	// ten seconds to hear nothing cost ten seconds on every successful
	// connection, which is most of them.
	_ = conn.SetDeadline(time.Now().Add(1500 * time.Millisecond))
	state := conn.ConnectionState()
	if _, err := conn.Write([]byte{0}); err != nil {
		return state.Version, err
	}
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err != nil && !isBenignReadError(err) {
		return state.Version, err
	}
	return state.Version, nil
}

// MongodTLSFlags reports every TLS option mongod advertises, so a test can ask
// whether DumboDB accounts for all of them rather than for a list written down
// once and left to rot. A flag MongoDB adds later shows up here on its own.
func MongodTLSFlags(t *testing.T) []string {
	t.Helper()
	bin := findMongodBin()
	if bin == "" {
		t.Skip("mongod binary not found (set MONGOD_BIN)")
	}
	out, err := exec.Command(bin, "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("mongod --help: %v", err)
	}
	seen := map[string]bool{}
	var flags []string
	for _, match := range tlsFlagPattern.FindAllString(string(out), -1) {
		name := strings.TrimPrefix(match, "--")
		if !seen[name] {
			seen[name] = true
			flags = append(flags, name)
		}
	}
	if len(flags) == 0 {
		t.Fatal("mongod --help listed no TLS options, so this cannot tell a complete list from a broken parse")
	}
	sort.Strings(flags)
	return flags
}

var tlsFlagPattern = regexp.MustCompile(`--tls[A-Za-z0-9]*`)

// ConnectAnyVersion dials offering every version the library supports and
// reports which one the server chose. Pinning a client answers "will you
// accept this"; offering everything answers "what do you pick", and a server
// that picks a lower version than it allows has downgraded the connection.
func (s *TLSServer) ConnectAnyVersion(t *testing.T) (uint16, error) {
	t.Helper()
	return s.connect(t, tls.VersionTLS10, tls.VersionTLS13)
}

// isBenignReadError reports whether a read failure means the peer simply had
// nothing to say, rather than that it rejected us. A server that accepted the
// connection will not answer a junk byte with a protocol reply.
func isBenignReadError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return errors.Is(err, io.EOF)
}

// ConnectPlaintext dials without TLS at all, to prove the port does not serve
// it. A TLS server that still answers plaintext is protecting nothing.
func (s *TLSServer) ConnectPlaintext(ctx context.Context, t *testing.T) error {
	t.Helper()
	client, err := mongo.Connect(ctx, options.Client().
		ApplyURI("mongodb://"+s.Addr+"/?directConnection=true").
		SetServerSelectionTimeout(6*time.Second))
	if err != nil {
		return err
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	return client.Ping(ctx, nil)
}

func (f *TLSFixture) orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func tlsPortAndDir(t *testing.T, prefix string) (int, string) {
	t.Helper()
	port, err := freePort()
	if err != nil {
		t.Fatalf("allocating a port: %v", err)
	}
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		t.Fatalf("allocating a data directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return port, dir
}

func readServerLog(proc *serverProc) string {
	if proc == nil || proc.log == "" {
		return ""
	}
	body, err := os.ReadFile(proc.log)
	if err != nil {
		return ""
	}
	return string(body)
}

// waitListeningOrExit reports whether addr began accepting connections before
// the process gave up. It returns as soon as either happens.
// addressInUse reports whether a server died because its port was taken,
// which is a harness fault rather than a result.
func addressInUse(output string) bool {
	lowered := strings.ToLower(output)
	for _, marker := range []string{
		"address already in use",
		"address in use",
		"addrinuse",
		"failed to set up listener",
	} {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}

func waitListeningOrExit(exited <-chan struct{}, addr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if waitPort(addr, 200*time.Millisecond) {
			return true
		}
		select {
		case <-exited:
			// One last look: a server can bind and exit between polls, and
			// calling that a start failure would be wrong.
			return waitPort(addr, 200*time.Millisecond)
		default:
		}
	}
	return false
}
