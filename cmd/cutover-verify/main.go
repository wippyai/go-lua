// Command cutover-verify mechanizes the cutover landing ritual: index
// cleanliness, a standalone build+vet in a cached shared clone, a
// protocol-zero grep, targeted tests, and an optional ladder-fixture
// regression diff against commit~1.
//
//	go run ./cmd/cutover-verify -commit <sha> -pkg <domain/pkg/path> [-ladders]
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wippyai/go-lua/internal/cutoververify"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("cutover-verify", flag.ContinueOnError)
	commit := fs.String("commit", "", "commit SHA to verify (required)")
	pkg := fs.String("pkg", "", "domain package import path to scope vet/grep/test to, e.g. domain/heap/allocation/empty (required)")
	ladders := fs.Bool("ladders", false, "also run the bench/fibonacci and basic/arithmetic ladder-fixture regression diff against commit~1")
	requireProtocolZero := fs.Bool("require-protocol-zero", false, "treat a nonzero protocol-zero grep as FAIL instead of WARN")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *commit == "" || *pkg == "" {
		fmt.Fprintln(stderr, "cutover-verify: -commit and -pkg are required")
		fs.SetOutput(stderr)
		fs.Usage()
		return 2
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "cutover-verify: %v\n", err)
		return 2
	}

	var results []cutoververify.Result

	fmt.Fprintln(stdout, "== INDEX ==")
	indexResult, err := cutoververify.CheckIndexClean(repoRoot)
	results = append(results, indexResult)
	if err != nil {
		fmt.Fprintf(stdout, "%s\n", err)
		printSummary(stdout, results, *requireProtocolZero)
		return 1
	}
	fmt.Fprintln(stdout, indexResult.Note)

	scratchRoot := cutoververify.ScratchRoot()
	fmt.Fprintf(stdout, "\n== CLONE (scratch=%s) ==\n", scratchRoot)
	clonePath, err := cutoververify.EnsureClone(scratchRoot, repoRoot)
	if err != nil {
		fmt.Fprintf(stderr, "cutover-verify: %v\n", err)
		return 2
	}
	if err := cutoververify.ResetClone(clonePath, *commit); err != nil {
		fmt.Fprintf(stderr, "cutover-verify: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "clone at %s reset to %s\n", clonePath, *commit)

	fmt.Fprintln(stdout, "\n== BUILD ==")
	buildResult := cutoververify.RunBuild(clonePath)
	results = append(results, buildResult)
	printCheckOutcome(stdout, buildResult)

	fmt.Fprintln(stdout, "\n== VET ==")
	vetResult := cutoververify.RunVet(clonePath, *pkg)
	results = append(results, vetResult)
	printCheckOutcome(stdout, vetResult)

	fmt.Fprintln(stdout, "\n== PROTOCOL-ZERO ==")
	pzResult, err := cutoververify.ProtocolZeroCheck(clonePath, *pkg, *requireProtocolZero)
	if err != nil {
		fmt.Fprintf(stderr, "cutover-verify: %v\n", err)
		return 2
	}
	results = append(results, pzResult)
	printCheckOutcome(stdout, pzResult)

	fmt.Fprintln(stdout, "\n== TESTS ==")
	testsResult := cutoververify.RunTargetedTests(clonePath, *pkg)
	results = append(results, testsResult)
	printCheckOutcome(stdout, testsResult)

	if *ladders {
		fmt.Fprintln(stdout, "\n== LADDERS ==")
		ladderResults, err := cutoververify.RunLadderSuite(clonePath, *commit, cutoververify.LadderFixtures)
		if err != nil {
			fmt.Fprintf(stderr, "cutover-verify: %v\n", err)
			return 2
		}
		for _, r := range ladderResults {
			results = append(results, r)
			printCheckOutcome(stdout, r)
		}
	}

	pass := printSummary(stdout, results, *requireProtocolZero)
	if pass {
		return 0
	}
	return 1
}

func printCheckOutcome(stdout *os.File, r cutoververify.Result) {
	fmt.Fprintf(stdout, "%s: %s\n", r.Status, r.Note)
	if r.Detail != "" {
		fmt.Fprintln(stdout, r.Detail)
	}
}

func printSummary(stdout *os.File, results []cutoververify.Result, requireProtocolZero bool) bool {
	fmt.Fprintln(stdout, "\n== SUMMARY ==")
	fmt.Fprint(stdout, cutoververify.FormatTable(results))
	pass, line := cutoververify.Overall(results, requireProtocolZero)
	fmt.Fprintln(stdout, line)
	return pass
}
