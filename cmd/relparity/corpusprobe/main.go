// Command corpusprobe answers one corpus fixture on both engines.
//
// It is the observation process of the full-corpus differential driver: it
// compiles one fixture from the frozen corpus exactly once, solves that one
// compiled artifact on the old engine and on the relation engine, and writes
// one sealed envelope carrying both answers.
//
//	corpusprobe <fixture>
//
// Compiling once and solving twice is what makes the comparison a statement
// about two solvers rather than about two compiles: any difference the report
// names is a difference the engines made, not one the front end made.
//
// Exit status: 0 an envelope was written, 1 the fixture could not be brought
// as far as an envelope, 2 the command was called wrongly.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/internal/relparity/corpus"
	"github.com/wippyai/go-lua/internal/testfixture"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: corpusprobe <fixture>")
		os.Exit(2)
	}
	envelope, err := observe(context.Background(), os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := envelope.Write(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// observe compiles one fixture once and answers it on both engines.
//
// A failure to reach a compiled artifact refuses the whole process: neither
// engine was asked, so there is no answer to compare and the driver records a
// probe failure rather than a false parity.
func observe(ctx context.Context, fixture string) (corpus.Envelope, error) {
	root, err := testfixture.RepositoryRoot(".")
	if err != nil {
		return corpus.Envelope{}, fmt.Errorf("corpusprobe: locate repository: %w", err)
	}
	loaded, err := testfixture.LoadCorpus(root)
	if err != nil {
		return corpus.Envelope{}, fmt.Errorf("corpusprobe: load corpus: %w", err)
	}
	project, err := loaded.Project(fixture)
	if err != nil {
		return corpus.Envelope{}, fmt.Errorf("corpusprobe: %w", err)
	}
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		return corpus.Envelope{}, fmt.Errorf("corpusprobe: standard target: %w", err)
	}
	linked, err := testfixture.SealCorpusProject(target, project)
	if err != nil {
		return corpus.Envelope{}, fmt.Errorf("corpusprobe: seal %s: %w", fixture, err)
	}

	workspace := analysis.NewWorkspace()
	defer workspace.Close()
	plan, compileStatus := workspace.Compile(linked)
	if plan != nil {
		defer plan.Close()
	}
	if plan == nil || compileStatus != analysis.CompileComplete {
		// Neither engine can be asked about a fixture that did not compile.
		// Both sides say so identically and the driver counts the fixture as
		// unreached: the engines did not disagree, but neither answered, so
		// it is not evidence of parity either.
		refusal := "compile: " + compileStatusSpelling(compileStatus)
		return corpus.Seal(fixture, []corpus.Answer{
			{Side: corpus.SideOld, Status: corpus.StatusUncompiled, Detail: refusal},
			{Side: corpus.SideNew, Status: corpus.StatusUncompiled, Detail: refusal},
		})
	}
	// This marker is the explicit compile/solve phase boundary. The external
	// corpus driver starts its five-second analysis budget only after it reads
	// this line; the process watchdog still covers the compiler above it.
	if _, err := fmt.Fprintln(os.Stdout, corpus.SolveReady); err != nil {
		return corpus.Envelope{}, fmt.Errorf("corpusprobe: announce solve phase: %w", err)
	}

	return corpus.Seal(fixture, []corpus.Answer{
		oldAnswer(ctx, plan),
		newAnswer(plan, project),
	})
}
