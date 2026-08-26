// Command corpusdrive is the full-corpus differential driver of the relational
// engine cut.
//
// It walks the frozen fixture corpus, has one observation process answer each
// fixture on both engines, and writes one JSON catalogue naming, per fixture
// per family per query site, what the old engine answered and what the new one
// answered.
//
//	go run ./cmd/relparity/corpusdrive -out report.json [-shard i -shards n]
//
// The driver links no engine. It builds the observation command and reads its
// stdout, which is what keeps the measurement a statement about two engines
// rather than about one process that holds both.
//
// Exit status: 0 the two engines agree over the walked corpus, 1 they diverge,
// 2 the driver could not run the walk.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/wippyai/go-lua/internal/relparity"
	"github.com/wippyai/go-lua/internal/relparity/corpus"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

const (
	statusAgree   = 0
	statusDiverge = 1
	statusRefused = 2
)

// DefaultProbePackage is the observation command the driver builds.
const DefaultProbePackage = "./cmd/relparity/corpusprobe"

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("corpusdrive", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "checkout whose fixture corpus is walked")
	probePackage := flags.String("probe-package", DefaultProbePackage, "go package of the observation command")
	probeBinary := flags.String("probe", "", "prebuilt observation binary; default is to build -probe-package")
	binaryDirectory := flags.String("binaries", "", "directory the observation binary is built into; default is a temporary directory")
	list := flags.String("fixtures", "", "file naming the fixtures to walk, one per line; default is the whole corpus")
	shard := flags.Int("shard", 0, "index of the corpus shard to walk")
	shards := flags.Int("shards", 1, "number of shards the corpus is partitioned into")
	workers := flags.Int("workers", corpus.MaximumWorkers, "concurrent observation processes, capped at the standing ceiling")
	timeout := flags.Duration("timeout", corpus.DefaultFixtureTimeout, "solve/publication bound after the probe's compile phase")
	processTimeout := flags.Duration("process-timeout", corpus.DefaultProcessTimeout, "compile-inclusive watchdog for one observation process")
	retained := flags.Int("retain", corpus.DefaultRetainedDivergences, "how many divergences the catalogue carries in full")
	out := flags.String("out", "", "path the JSON catalogue is written to; default is stdout")
	if err := flags.Parse(args); err != nil {
		return statusRefused
	}

	checkout, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintf(stderr, "corpusdrive: %v\n", err)
		return statusRefused
	}

	binary := *probeBinary
	if binary == "" {
		directory := *binaryDirectory
		if directory == "" {
			directory, err = os.MkdirTemp("", "corpusdrive-")
			if err != nil {
				fmt.Fprintf(stderr, "corpusdrive: %v\n", err)
				return statusRefused
			}
			defer os.RemoveAll(directory)
		}
		fmt.Fprintf(stdout, "building %s\n", *probePackage)
		binary, err = buildProbe(checkout, *probePackage, directory)
		if err != nil {
			fmt.Fprintf(stderr, "corpusdrive: %v\n", err)
			return statusRefused
		}
	}

	fixtures, err := selectFixtures(*list, checkout)
	if err != nil {
		fmt.Fprintf(stderr, "corpusdrive: %v\n", err)
		return statusRefused
	}
	selected, err := corpus.Select(fixtures, *shard, *shards)
	if err != nil {
		fmt.Fprintf(stderr, "corpusdrive: %v\n", err)
		return statusRefused
	}

	report, err := corpus.Run(context.Background(), corpus.Plan{
		Probe: corpus.Probe{
			Binary:           binary,
			WorkingDirectory: checkout,
			Timeout:          *timeout,
			ProcessTimeout:   *processTimeout,
		},
		Fixtures:            selected,
		Shard:               *shard,
		Shards:              *shards,
		Workers:             *workers,
		RetainedDivergences: *retained,
	})
	if err != nil {
		fmt.Fprintf(stderr, "corpusdrive: %v\n", err)
		return statusRefused
	}

	text, err := report.JSON()
	if err != nil {
		fmt.Fprintf(stderr, "corpusdrive: %v\n", err)
		return statusRefused
	}
	if *out == "" {
		stdout.Write(text)
	} else if err := os.WriteFile(*out, text, 0o644); err != nil {
		fmt.Fprintf(stderr, "corpusdrive: write catalogue %s: %v\n", *out, err)
		return statusRefused
	}
	fmt.Fprint(stdout, report.Summary())
	if report.Identical() {
		return statusAgree
	}
	return statusDiverge
}

// buildProbe builds the observation command out of the checkout. The driver
// shells out rather than importing anything the command imports, which is the
// fence: a driver that cannot link an engine cannot answer for one.
func buildProbe(checkout, packagePath, directory string) (string, error) {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", fmt.Errorf("create binary directory %s: %w", directory, err)
	}
	binary := filepath.Join(directory, "corpusprobe")
	command := exec.Command("go", "build", "-buildvcs=false", "-o", binary, packagePath)
	command.Dir = checkout
	if output, err := command.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build %s: %w\n%s", packagePath, err, output)
	}
	return binary, nil
}

func selectFixtures(listPath, checkout string) ([]string, error) {
	if listPath != "" {
		fixtures, err := relparity.ReadFixtureList(listPath)
		if err != nil {
			return nil, err
		}
		if len(fixtures) == 0 {
			return nil, fmt.Errorf("%w: %s", corpus.ErrEmptyCorpus, listPath)
		}
		return fixtures, nil
	}
	return corpus.Enumerate(checkout)
}
