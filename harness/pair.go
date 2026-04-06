package harness

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

// TestCase defines a single parity test to run against both MongoDB and Docudolt.
type TestCase struct {
	// Name identifies the test in reports and generates the isolated DB name.
	Name string
	// Support controls how Docudolt is exercised (Full, MongoOnly, XFail).
	Support DocudoltSupport
	// Setup runs before the test operation; may be nil.
	// It is called with the same collection passed to Run.
	Setup func(ctx context.Context, col *mongo.Collection) error
	// Run performs the operation under test and returns the result to compare.
	Run func(ctx context.Context, col *mongo.Collection) (interface{}, error)
}

// PairTest runs tc against MongoDB and (depending on Support level) Docudolt,
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

	mongoCol, docudoltCol, cleanup, err := clients.TestDB(ctx, tc.Name)
	if err != nil {
		t.Fatalf("PairTest %s: could not allocate test DB: %v", tc.Name, err)
	}
	defer cleanup()

	switch tc.Support {
	case DocudoltMongoOnly:
		return runMongoOnly(t, ctx, tc, mongoCol)
	case DocudoltFull:
		return runFull(t, ctx, tc, mongoCol, docudoltCol)
	case DocudoltXFail:
		return runXFail(t, ctx, tc, mongoCol, docudoltCol)
	default:
		t.Fatalf("PairTest %s: unknown DocudoltSupport level %d", tc.Name, tc.Support)
		return TestResult{Name: tc.Name, Status: StatusFail}
	}
}

func runMongoOnly(t *testing.T, ctx context.Context, tc TestCase, mongoCol *mongo.Collection) TestResult {
	t.Helper()
	if tc.Setup != nil {
		if err := tc.Setup(ctx, mongoCol); err != nil {
			t.Logf("MONGO_ONLY %s: setup error: %v", tc.Name, err)
			return TestResult{Name: tc.Name, Status: StatusFail, Diff: fmt.Sprintf("mongo setup: %v", err)}
		}
	}
	_, mongoErr := tc.Run(ctx, mongoCol)
	if mongoErr != nil {
		t.Logf("MONGO_ONLY %s: mongo error: %v", tc.Name, mongoErr)
		return TestResult{Name: tc.Name, Status: StatusFail, Diff: fmt.Sprintf("mongo: %v", mongoErr)}
	}
	t.Logf("MONGO_ONLY %s: OK (Docudolt skipped)", tc.Name)
	return TestResult{Name: tc.Name, Status: StatusSkip}
}

func runFull(t *testing.T, ctx context.Context, tc TestCase, mongoCol, docudoltCol *mongo.Collection) TestResult {
	t.Helper()
	setup(t, ctx, tc, mongoCol, docudoltCol)
	mongoResult, mongoErr := tc.Run(ctx, mongoCol)
	docudoltResult, docudoltErr := tc.Run(ctx, docudoltCol)

	cmp := CompareResponses(mongoResult, mongoErr, docudoltResult, docudoltErr)
	if cmp.Result == Match {
		t.Logf("FULL %s: PASS", tc.Name)
		return TestResult{Name: tc.Name, Status: StatusPass}
	}
	t.Errorf("FULL %s: DIVERGE\n%s", tc.Name, cmp.Diff)
	return TestResult{Name: tc.Name, Status: StatusFail, Diff: cmp.Diff}
}

func runXFail(t *testing.T, ctx context.Context, tc TestCase, mongoCol, docudoltCol *mongo.Collection) TestResult {
	t.Helper()
	setup(t, ctx, tc, mongoCol, docudoltCol)
	mongoResult, mongoErr := tc.Run(ctx, mongoCol)
	docudoltResult, docudoltErr := tc.Run(ctx, docudoltCol)

	cmp := CompareResponses(mongoResult, mongoErr, docudoltResult, docudoltErr)
	if cmp.Result == Match {
		t.Logf("XFAIL %s: PASS (Docudolt matched)", tc.Name)
		return TestResult{Name: tc.Name, Status: StatusPass}
	}
	t.Logf("XFAIL %s: diverged as expected\n%s", tc.Name, cmp.Diff)
	return TestResult{Name: tc.Name, Status: StatusXFail, Diff: cmp.Diff}
}

// setup runs tc.Setup against both collections (if non-nil), logging errors but not failing.
func setup(t *testing.T, ctx context.Context, tc TestCase, mongoCol, docudoltCol *mongo.Collection) {
	t.Helper()
	if tc.Setup == nil {
		return
	}
	if err := tc.Setup(ctx, mongoCol); err != nil {
		t.Logf("%s: mongo setup error: %v", tc.Name, err)
	}
	if err := tc.Setup(ctx, docudoltCol); err != nil {
		t.Logf("%s: docudolt setup error: %v", tc.Name, err)
	}
}
