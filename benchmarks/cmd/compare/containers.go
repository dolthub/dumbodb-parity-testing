package main

import (
	"context"
	"fmt"

	"github.com/dolthub/dumbodb-parity-testing/targets"
)

var (
	mongoContainer = targets.Container{
		Name:    "mongodb-bench",
		Image:   "mongo:8.0",
		HostURI: "mongodb://127.0.0.1:27017",
		RunArgs: []string{"-p", "127.0.0.1:27017:27017"},
	}
	// dumboContainer's Image and RunArgs are set by prepareDumboImage:
	// pulled-image mode uses dolthub/dumbodb (listens on 27017 inside),
	// source-build mode uses dumbodb-bench:local (listens on 27018 inside).
	dumboContainer = targets.Container{
		Name:    "dumbodb-bench",
		HostURI: "mongodb://127.0.0.1:27018",
	}
)

// prepareImages pulls the MongoDB image and builds or pulls the DumboDB image
// that each benchmark's fresh containers run.
func prepareImages(ctx context.Context) error {
	if err := targets.EnsureImage(ctx, mongoContainer.Image); err != nil {
		return fmt.Errorf("pull %s: %w", mongoContainer.Image, err)
	}
	return prepareDumboImage(ctx)
}

// prepareDumboImage builds dumbodb-bench:local from -dumbodb-src when set,
// otherwise pulls -dumbodb-image, and points dumboContainer at that image
// with its port mapping.
//
// Source-build mode additionally publishes port 6060 so the pprof endpoint
// baked into dumbodb-bench:local (see benchmarks/Dockerfile.dumbodb) is
// reachable from the host. Pulled-image mode does not, because the public
// image's CMD does not include -pprof-addr.
func prepareDumboImage(ctx context.Context) error {
	if *dumboSrc != "" {
		dumboContainer.Image = "dumbodb-bench:local"
		dumboContainer.RunArgs = []string{
			"-p", "127.0.0.1:27018:27018",
			"-p", "127.0.0.1:6060:6060",
		}
		if err := targets.BuildDumboDBImage(ctx, dumboContainer.Image, *dumboSrc); err != nil {
			return fmt.Errorf("build %s: %w", dumboContainer.Image, err)
		}
		return nil
	}
	dumboContainer.Image = *dumboImage
	dumboContainer.RunArgs = []string{"-p", "127.0.0.1:27018:27017"}
	if err := targets.EnsureImage(ctx, dumboContainer.Image); err != nil {
		return fmt.Errorf("pull %s: %w", dumboContainer.Image, err)
	}
	return nil
}
