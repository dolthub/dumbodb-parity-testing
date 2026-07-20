package harness

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

// Topology is the Mongo deployment shape a test expects to compare against.
// The default zero value is TopologyStandalone. Tests that require multi-
// document transactions or replica-set-only features (e.g. $changeStream)
// set Topology to TopologyReplicaSet.
type Topology int

const (
	TopologyStandalone Topology = iota
	TopologyReplicaSet
)

type serverURIKey struct{}

// ServerURI returns the URI of the server that col is connected to, as injected
// by the harness before calling TestCase.Run. Tests that need a second client
// to the same server use this to avoid guessing the URI.
func ServerURI(ctx context.Context) string {
	if v, ok := ctx.Value(serverURIKey{}).(string); ok {
		return v
	}
	return ""
}

func withServerURI(ctx context.Context, uri string) context.Context {
	return context.WithValue(ctx, serverURIKey{}, uri)
}

// TestCase defines a single parity test to run against both MongoDB and DumboDB.
type TestCase struct {
	// Name identifies the test in reports and generates the isolated DB name.
	Name string
	// Support controls how DumboDB is exercised (Full, MongoOnly, XFail).
	Support DumboDBSupport
	// Topology selects which Mongo deployment shape the test compares
	// against. Zero value is TopologyStandalone (the default). Tests
	// that require multi-document transactions or replica-set features
	// set TopologyReplicaSet and skip when MONGO_RS_URI is not set.
	Topology Topology
	// Setup runs before the test operation; may be nil.
	// It is called with the same collection passed to Run.
	Setup func(ctx context.Context, col *mongo.Collection) error
	// Run performs the operation under test and returns the result to compare.
	Run func(ctx context.Context, col *mongo.Collection) (interface{}, error)
}

// PairTest runs tc against MongoDB and (depending on Support level) DumboDB,
// compares the responses, logs the outcome, and returns a TestResult.
// It calls t.Errorf on unexpected FULL-mode divergences.
func PairTest(t *testing.T, tc TestCase) TestResult {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	clients, err := GetClients(ctx)
	if err != nil {
		t.Fatalf("PairTest %s: could not get clients: %v", tc.Name, err)
	}

	mongoCol, dumboDBCol, cleanup, err := clients.TestDBForTopology(ctx, tc.Name, tc.Topology)
	if err != nil {
		if errors.Is(err, ErrTopologyUnavailable) {
			t.Skipf("PairTest %s: %v", tc.Name, err)
			return TestResult{Name: tc.Name, Status: StatusSkip}
		}
		t.Fatalf("PairTest %s: could not allocate test DB: %v", tc.Name, err)
	}
	defer cleanup()

	mongoURIForTest := mongoURIForTopology(tc.Topology)

	switch tc.Support {
	case DumboDBMongoOnly:
		return runMongoOnly(t, ctx, tc, mongoCol)
	case DumboDBFull:
		return runFull(t, ctx, tc, mongoCol, dumboDBCol, mongoURIForTest)
	case DumboDBXFail:
		return runXFail(t, ctx, tc, mongoCol, dumboDBCol, mongoURIForTest)
	default:
		t.Fatalf("PairTest %s: unknown DumboDBSupport level %d", tc.Name, tc.Support)
		return TestResult{Name: tc.Name, Status: StatusFail}
	}
}

func runMongoOnly(t *testing.T, ctx context.Context, tc TestCase, mongoCol *mongo.Collection) TestResult {
	t.Helper()
	if tc.Setup != nil {
		if err := tc.Setup(ctx, mongoCol); err != nil {
			t.Errorf("MONGO_ONLY %s: mongo setup failed: %v", tc.Name, err)
			return TestResult{Name: tc.Name, Status: StatusFail, Diff: fmt.Sprintf("mongo setup: %v", err)}
		}
	}
	_, mongoErr := tc.Run(ctx, mongoCol)
	if mongoErr != nil {
		t.Logf("MONGO_ONLY %s: mongo error: %v", tc.Name, mongoErr)
		return TestResult{Name: tc.Name, Status: StatusFail, Diff: fmt.Sprintf("mongo: %v", mongoErr)}
	}
	t.Logf("MONGO_ONLY %s: OK (DumboDB skipped)", tc.Name)
	return TestResult{Name: tc.Name, Status: StatusSkip}
}

func runFull(t *testing.T, ctx context.Context, tc TestCase, mongoCol, dumboDBCol *mongo.Collection, mongoURIForTest string) TestResult {
	t.Helper()
	if !setup(t, ctx, tc, mongoCol, dumboDBCol) {
		return TestResult{Name: tc.Name, Status: StatusFail, Diff: "setup failed"}
	}
	mongoResult, mongoErr := tc.Run(withServerURI(ctx, mongoURIForTest), mongoCol)
	dumboDBResult, dumboDBErr := tc.Run(withServerURI(ctx, dumboDBURI()), dumboDBCol)

	cmp := CompareResponses(mongoResult, mongoErr, dumboDBResult, dumboDBErr)
	if cmp.Result == Match {
		t.Logf("FULL %s: PASS", tc.Name)
		return TestResult{Name: tc.Name, Status: StatusPass}
	}
	t.Errorf("FULL %s: DIVERGE\n%s", tc.Name, cmp.Diff)
	return TestResult{Name: tc.Name, Status: StatusFail, Diff: cmp.Diff}
}

func runXFail(t *testing.T, ctx context.Context, tc TestCase, mongoCol, dumboDBCol *mongo.Collection, mongoURIForTest string) TestResult {
	t.Helper()
	if !setup(t, ctx, tc, mongoCol, dumboDBCol) {
		return TestResult{Name: tc.Name, Status: StatusFail, Diff: "setup failed"}
	}
	mongoResult, mongoErr := tc.Run(withServerURI(ctx, mongoURIForTest), mongoCol)
	dumboDBResult, dumboDBErr := tc.Run(withServerURI(ctx, dumboDBURI()), dumboDBCol)

	cmp := CompareResponses(mongoResult, mongoErr, dumboDBResult, dumboDBErr)
	if cmp.Result == Match {
		t.Logf("XFAIL %s: PASS (DumboDB matched)", tc.Name)
		return TestResult{Name: tc.Name, Status: StatusPass}
	}
	t.Logf("XFAIL %s: diverged as expected\n%s", tc.Name, cmp.Diff)
	return TestResult{Name: tc.Name, Status: StatusXFail, Diff: cmp.Diff}
}

// setup runs tc.Setup against both collections (if non-nil). A setup failure
// fails the test loudly and returns false: a missing precondition must not flow
// into the response comparison, where an identical setup failure on both servers
// would "match" and pass having exercised nothing.
func setup(t *testing.T, ctx context.Context, tc TestCase, mongoCol, dumboDBCol *mongo.Collection) bool {
	t.Helper()
	if tc.Setup == nil {
		return true
	}
	if err := tc.Setup(ctx, mongoCol); err != nil {
		t.Errorf("%s: mongo setup failed: %v", tc.Name, err)
		return false
	}
	if err := tc.Setup(ctx, dumboDBCol); err != nil {
		t.Errorf("%s: dumbodb setup failed: %v", tc.Name, err)
		return false
	}
	return true
}
