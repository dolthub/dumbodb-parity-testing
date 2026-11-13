package main

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/dolthub/dumbodb-parity-testing/targets"
)

// runLocalTargets runs each benchmark against a freshly started server of
// each target, so no benchmark sees state left by another.
func runLocalTargets(workDir string, suite *benchSuite) (dumbo, mongo []result, err error) {
	localTargets := []struct {
		label   string
		dataDir string
		command targets.ServerCommand
		results *[]result
	}{
		{"dumbodb", filepath.Join(workDir, "dumbodb-data"), targets.DumboDBCommand(*dumboBin), &dumbo},
		{"mongodb", filepath.Join(workDir, "mongodb-data"), targets.MongodCommand(*mongodBin), &mongo},
	}
	var errs []error
	for _, name := range suite.names {
		for _, t := range localTargets {
			res, err := runLocalBenchmark(suite, name, t.label, t.dataDir, t.command)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s %s: %w", t.label, name, err))
			}
			*t.results = append(*t.results, res...)
		}
	}
	return dumbo, mongo, errors.Join(errs...)
}

func runLocalBenchmark(suite *benchSuite, name, label, dataDir string, command targets.ServerCommand) ([]result, error) {
	srv, err := targets.StartLocal(label, dataDir, command, *healthTimeout)
	if err != nil {
		return nil, err
	}
	defer srv.Stop()
	results, err := suite.run(name, label, srv.URI())
	if !srv.Alive() {
		err = errors.Join(err, fmt.Errorf("%s exited during the run:\n%s", label, targets.LogTail(srv.LogPath, 20)))
	}
	return results, err
}
