// Command relparity is the external two-binary parity harness for the sealed
// relational engine cut (docs/architecture/relation-engine.md, Wave 4C).
//
// It builds the observation binary from a baseline revision and from a
// replacement revision, runs each as its own process over one shared fixture
// tree, and writes a machine-readable diff report naming the first divergent
// row.
//
//	go run ./cmd/relparity -baseline <rev> -replacement <rev> [-shard i -shards n] -out report.json
//
// Exit status: 0 the two sides agree, 1 they diverge, 2 the harness could not
// run the comparison.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/wippyai/go-lua/internal/relparity"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

const (
	statusAgree   = 0
	statusDiverge = 1
	statusRefused = 2
)

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("relparity", flag.ContinueOnError)
	flags.SetOutput(stderr)
	baseline := flags.String("baseline", "", "git revision the baseline observation binary is built from (required)")
	replacement := flags.String("replacement", "", "git revision the replacement observation binary is built from (required)")
	probePackage := flags.String("probe", relparity.DefaultProbe().Package, "go package of the observation command, built in each side's checkout")
	verbs := flags.String("verbs", strings.Join(relparity.DefaultProbe().Verbs, ","), "comma-separated dump verbs, in comparison order")
	timeout := flags.Duration("timeout", relparity.DefaultProbe().Timeout, "bound on one fixture-verb process on one side")
	shard := flags.Int("shard", 0, "index of the fixture shard to compare")
	shards := flags.Int("shards", 1, "number of fixture shards the corpus is partitioned into")
	list := flags.String("fixtures", "", "file naming the fixtures to compare, one per line; default is the whole corpus")
	scratch := flags.String("scratch", cutoverScratch(), "directory the shared clone and the built binaries live under")
	out := flags.String("out", "", "path the JSON report is written to; default is stdout")
	retained := flags.Int("retain", relparity.DefaultRetainedDivergences, "how many divergences the report carries in full")
	if err := flags.Parse(args); err != nil {
		return statusRefused
	}
	if *baseline == "" || *replacement == "" {
		fmt.Fprintln(stderr, "relparity: -baseline and -replacement are required")
		flags.Usage()
		return statusRefused
	}

	repository, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "relparity: %v\n", err)
		return statusRefused
	}

	probe := relparity.Probe{
		Package: *probePackage,
		Verbs:   splitVerbs(*verbs),
		Timeout: *timeout,
	}
	if len(probe.Verbs) == 0 {
		fmt.Fprintln(stderr, "relparity: -verbs named no verb")
		return statusRefused
	}

	checkout, err := relparity.OpenCheckout(*scratch, repository)
	if err != nil {
		fmt.Fprintf(stderr, "relparity: %v\n", err)
		return statusRefused
	}
	binaries := *scratch + string(os.PathSeparator) + "relparity-binaries"

	fmt.Fprintf(stdout, "building %s from %s\n", probe.Package, *baseline)
	baselineSide, err := relparity.BuildSide(checkout, probe, relparity.RoleBaseline, *baseline, binaries)
	if err != nil {
		fmt.Fprintf(stderr, "relparity: %v\n", err)
		return statusRefused
	}
	fmt.Fprintf(stdout, "building %s from %s\n", probe.Package, *replacement)
	replacementSide, err := relparity.BuildSide(checkout, probe, relparity.RoleReplacement, *replacement, binaries)
	if err != nil {
		fmt.Fprintf(stderr, "relparity: %v\n", err)
		return statusRefused
	}

	// The checkout now stands at the replacement revision, and that is the one
	// fixture tree both binaries read. Parity is a statement about two
	// runtimes over identical input, so the input is named once.
	fixtures, err := selectFixtures(*list, checkout.Path)
	if err != nil {
		fmt.Fprintf(stderr, "relparity: %v\n", err)
		return statusRefused
	}
	selected, err := relparity.Shard(fixtures, *shard, *shards)
	if err != nil {
		fmt.Fprintf(stderr, "relparity: %v\n", err)
		return statusRefused
	}

	report := relparity.Run(context.Background(), relparity.Plan{
		Probe:               probe,
		Baseline:            baselineSide,
		Replacement:         replacementSide,
		WorkingDirectory:    checkout.Path,
		Fixtures:            selected,
		Shard:               *shard,
		Shards:              *shards,
		RetainedDivergences: *retained,
	})

	text, err := report.JSON()
	if err != nil {
		fmt.Fprintf(stderr, "relparity: %v\n", err)
		return statusRefused
	}
	if *out == "" {
		stdout.Write(text)
	} else if err := os.WriteFile(*out, text, 0o644); err != nil {
		fmt.Fprintf(stderr, "relparity: write report %s: %v\n", *out, err)
		return statusRefused
	}
	fmt.Fprint(stdout, report.Summary())
	if report.Identical() {
		return statusAgree
	}
	return statusDiverge
}

func selectFixtures(listPath, checkout string) ([]string, error) {
	if listPath != "" {
		return relparity.ReadFixtureList(listPath)
	}
	return relparity.ListFixtures(checkout)
}

func splitVerbs(text string) []string {
	var verbs []string
	for _, verb := range strings.Split(text, ",") {
		if verb = strings.TrimSpace(verb); verb != "" {
			verbs = append(verbs, verb)
		}
	}
	return verbs
}

// cutoverScratch keeps the harness in the same scratch root the cutover
// ritual already uses, so one clone serves both.
func cutoverScratch() string {
	if directory := os.Getenv("CUTOVER_SCRATCH"); directory != "" {
		return directory
	}
	return os.TempDir()
}
