package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
)

// The history CSV is the -csv format prefixed with columns identifying the
// run, so successive runs stack in one file or spreadsheet.
var historyHeader = []string{"date", "dumbodb_version", "mongodb_version", "host", "name", "dumbodb_ns_per_op", "mongodb_ns_per_op", "multiplier"}

type runInfo struct {
	Date           string
	DumboDBVersion string
	MongoDBVersion string
	Host           string
}

func historyRecords(info runInfo, rows []combined) [][]string {
	records := make([][]string, 0, len(rows))
	for _, r := range rows {
		records = append(records, []string{
			info.Date, info.DumboDBVersion, info.MongoDBVersion, info.Host,
			r.Name, fmtNs(r.DumboDBNs), fmtNs(r.MongoNs), fmtMultiplier(r.Multiplier),
		})
	}
	return records
}

func writeHistoryCSV(w io.Writer, records [][]string, header bool) error {
	cw := csv.NewWriter(w)
	if header {
		if err := cw.Write(historyHeader); err != nil {
			return err
		}
	}
	if err := cw.WriteAll(records); err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}

func historyCSVBytes(records [][]string) []byte {
	var b bytes.Buffer
	_ = writeHistoryCSV(&b, records, true)
	return b.Bytes()
}

// previousRun returns DumboDB ns/op by benchmark for the last run in the
// history file, and that run's date.
func previousRun(path string) (map[string]float64, string, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, "", fmt.Errorf("read history %s: %w", path, err)
	}
	if len(records) < 2 {
		return nil, "", nil
	}
	last := records[len(records)-1][0]
	prev := map[string]float64{}
	for _, rec := range records[1:] {
		if len(rec) != len(historyHeader) || rec[0] != last {
			continue
		}
		if ns, err := strconv.ParseFloat(rec[5], 64); err == nil {
			prev[rec[4]] = ns
		}
	}
	return prev, last, nil
}

func appendHistory(path string, records [][]string) error {
	_, statErr := os.Stat(path)
	newFile := errors.Is(statErr, os.ErrNotExist)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	werr := writeHistoryCSV(f, records, newFile)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return werr
}
