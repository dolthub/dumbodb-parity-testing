package harness

import (
	"fmt"
	"io"
	"os"
)

// DocudoltSupport indicates how Docudolt handles the feature under test.
type DocudoltSupport int

const (
	// DocudoltFull: run both MongoDB and Docudolt, compare — divergence fails CI.
	DocudoltFull DocudoltSupport = iota
	// DocudoltMongoOnly: run MongoDB only, skip Docudolt — for deprecated/unsupported features.
	DocudoltMongoOnly
	// DocudoltXFail: run both, record Docudolt failure but do not fail CI.
	DocudoltXFail
)

// TestStatus is the outcome of a single test run.
type TestStatus int

const (
	StatusPass  TestStatus = iota
	StatusFail
	StatusSkip
	StatusXFail
)

func (s TestStatus) String() string {
	switch s {
	case StatusPass:
		return "PASS"
	case StatusFail:
		return "FAIL"
	case StatusSkip:
		return "SKIP"
	case StatusXFail:
		return "XFAIL"
	default:
		return "UNKNOWN"
	}
}

// TestResult records the outcome of a single PairTest call.
type TestResult struct {
	Name   string
	Status TestStatus
	Diff   string // non-empty on Diverge or Fail
}

// Summary aggregates results across all tests in a suite.
type Summary struct {
	Matching  int
	Diverging int
	MongoOnly int
	XFail     int
}

// Add records a test result into the summary.
func (s *Summary) Add(r TestResult) {
	switch r.Status {
	case StatusPass:
		s.Matching++
	case StatusFail:
		s.Diverging++
	case StatusSkip:
		s.MongoOnly++
	case StatusXFail:
		s.XFail++
	}
}

// HasUnexpectedFailures returns true if there are FULL-mode divergences.
func (s *Summary) HasUnexpectedFailures() bool {
	return s.Diverging > 0
}

// Print writes a human-readable summary to w.
func (s *Summary) Print(w io.Writer) {
	fmt.Fprintf(w, "\nParity Summary\n")
	fmt.Fprintf(w, "  Matching:   %d\n", s.Matching)
	fmt.Fprintf(w, "  Diverging:  %d\n", s.Diverging)
	fmt.Fprintf(w, "  Mongo-only: %d\n", s.MongoOnly)
	fmt.Fprintf(w, "  XFail:      %d\n", s.XFail)
	total := s.Matching + s.Diverging + s.MongoOnly + s.XFail
	fmt.Fprintf(w, "  Total:      %d\n", total)
}

// PrintSummary prints the summary to stdout.
func PrintSummary(s *Summary) {
	s.Print(os.Stdout)
}
