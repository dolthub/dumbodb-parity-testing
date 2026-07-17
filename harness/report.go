package harness

import (
	"fmt"
	"io"
	"os"
)

// DumboDBSupport indicates how DumboDB handles the feature under test.
type DumboDBSupport int

const (
	// DumboDBFull: run both MongoDB and DumboDB, compare — divergence fails CI.
	DumboDBFull DumboDBSupport = iota
	// DumboDBMongoOnly: run MongoDB only, skip DumboDB — for deprecated/unsupported features.
	DumboDBMongoOnly
	// DumboDBXFail: run both, record DumboDB failure but do not fail CI.
	DumboDBXFail
	// DumboDBDeviates: DumboDB intentionally differs from MongoDB. Run both and
	// assert each server's own outcome via AuthCase.MongoExpect/DumboExpect.
	DumboDBDeviates
)

type TestStatus int

const (
	StatusPass TestStatus = iota
	StatusFail
	StatusSkip
	StatusXFail
	StatusDeviate
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
	case StatusDeviate:
		return "DEVIATE"
	default:
		return "UNKNOWN"
	}
}

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
	Deviating int
}

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
	case StatusDeviate:
		s.Deviating++
	}
}

// HasUnexpectedFailures returns true if there are FULL-mode divergences.
func (s *Summary) HasUnexpectedFailures() bool {
	return s.Diverging > 0
}

func (s *Summary) Print(w io.Writer) {
	fmt.Fprintf(w, "\nParity Summary\n")
	fmt.Fprintf(w, "  Matching:   %d\n", s.Matching)
	fmt.Fprintf(w, "  Diverging:  %d\n", s.Diverging)
	fmt.Fprintf(w, "  Mongo-only: %d\n", s.MongoOnly)
	fmt.Fprintf(w, "  XFail:      %d\n", s.XFail)
	fmt.Fprintf(w, "  Deviating:  %d\n", s.Deviating)
	total := s.Matching + s.Diverging + s.MongoOnly + s.XFail + s.Deviating
	fmt.Fprintf(w, "  Total:      %d\n", total)
}

func PrintSummary(s *Summary) {
	s.Print(os.Stdout)
}
