package relbind_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/relbind"
)

// root is the module root the axis packages live under. The law resolves it
// from its own source position, so it holds wherever the suite is run from.
func root(t testing.TB) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve the law's own position")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..")
}

// TestTheCheckedInArtifactsAreTheEmission is the freshness law. A generated
// artifact that is edited, or that survives the declaration it was emitted
// from, is a second source of truth; here it is a refusal instead.
func TestTheCheckedInArtifactsAreTheEmission(t *testing.T) {
	drifts, err := relbind.Check(root(t))
	if err != nil {
		t.Fatalf("check the emission: %v", err)
	}
	for _, drift := range drifts {
		t.Errorf("%s", drift)
	}
	if len(drifts) != 0 {
		t.Log("rerun: go run ./analysis/schema/rule/relbindgen/relbind/cmd/relbind -root .")
	}
}

// choreography is the vocabulary section 6.1 keeps out of generated code. Each
// entry is written as the call it would appear as, because the law is about
// what an artifact does and not about which letters its identifiers contain: a
// family named for a route is fine, a generated file that routes is not.
var choreography = []string{
	".Join(", ".Route(", ".Routes(", ".Schedule(", ".Ticket(", ".Settle(",
	".Publish(", ".Select(", ".Complete(", ".Group(", ".Merge(", ".Read(",
	".Seal(", ".Commit(", ".Widen(", ".LessOrEq(",
}

// admitted is every import a generated artifact may carry beyond the owner
// types its own columns are spelled with.
var admitted = map[string]bool{
	"github.com/wippyai/go-lua/analysis/identity":                    true,
	"github.com/wippyai/go-lua/analysis/relation/schema/model":       true,
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding":   true,
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome":   true,
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature": true,
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen":      true,
}

// TestNoEmittedArtifactChoreographs greps the generator's own output. Generated
// code may decode, encode and admit; it may not read a relation, join, route,
// schedule, ticket, settle an outcome, select a form or choreograph a
// publication, and the proof is the emission itself rather than a review.
func TestNoEmittedArtifactChoreographs(t *testing.T) {
	artifacts, err := relbind.Emit(relbind.Declared())
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	if len(artifacts) == 0 {
		t.Fatal("the corpus emitted nothing")
	}
	for _, artifact := range artifacts {
		source := string(artifact.Bytes)
		for _, token := range choreography {
			if strings.Contains(source, token) {
				t.Errorf("%s performs %s", artifact.Path(), strings.Trim(token, ".("))
			}
		}
	}
}

// TestEveryEmittedBindingMakesExactlyOneAdmission states the shape of the
// generated half: one family, one substrate call. A file with none binds
// nothing, and a file with two carries a choice about which binding answers,
// which is a form selection.
func TestEveryEmittedBindingMakesExactlyOneAdmission(t *testing.T) {
	artifacts, err := relbind.Emit(relbind.Declared())
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	bindings := 0
	for _, artifact := range artifacts {
		if artifact.Name == relbind.PayloadFile {
			if count := strings.Count(string(artifact.Bytes), "relbindgen.Bind("); count != 0 {
				t.Errorf("%s admits %d bindings; an owner-column publisher admits none", artifact.Path(), count)
			}
			continue
		}
		bindings++
		if count := strings.Count(string(artifact.Bytes), "relbindgen.Bind("); count != 1 {
			t.Errorf("%s makes %d admissions, and a family makes one", artifact.Path(), count)
		}
	}
	if bindings == 0 {
		t.Fatal("the corpus emitted no binding")
	}
}

// TestEveryEmittedArtifactImportsOnlyTheSubstrateAndItsOwnTypes is the
// stronger form of the same fence: an artifact that cannot name an engine
// package cannot reach one, whatever it is written to do.
func TestEveryEmittedArtifactImportsOnlyTheSubstrateAndItsOwnTypes(t *testing.T) {
	artifacts, err := relbind.Emit(relbind.Declared())
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	set := token.NewFileSet()
	for _, artifact := range artifacts {
		file, parseErr := parser.ParseFile(set, artifact.Name, artifact.Bytes, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", artifact.Path(), parseErr)
		}
		for _, declared := range file.Imports {
			imported, quoteErr := strconv.Unquote(declared.Path.Value)
			if quoteErr != nil {
				t.Fatalf("unquote %s: %v", artifact.Path(), quoteErr)
			}
			if admitted[imported] || strings.HasPrefix(imported, "github.com/wippyai/go-lua/domain/") {
				continue
			}
			t.Errorf("%s imports %s, which a generated artifact may not name", artifact.Path(), imported)
		}
	}
}

// TestEveryEmittedArtifactIsFormatted states the emission is canonical, so a
// freshness refusal is always a real declaration change and never a spacing
// difference between two writers.
func TestEveryEmittedArtifactIsFormatted(t *testing.T) {
	artifacts, err := relbind.Emit(relbind.Declared())
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	for _, artifact := range artifacts {
		if !strings.HasPrefix(string(artifact.Bytes), "// Code generated by ") {
			t.Errorf("%s does not declare itself generated", artifact.Path())
		}
	}
}

// TestTheEmissionIsDeterministic states that two runs of the same declaration
// are the same bytes, which is what makes the freshness check a proof.
func TestTheEmissionIsDeterministic(t *testing.T) {
	first, err := relbind.Emit(relbind.Declared())
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	second, err := relbind.Emit(relbind.Declared())
	if err != nil {
		t.Fatalf("emit again: %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("two emissions produced %d and %d artifacts", len(first), len(second))
	}
	for index := range first {
		if first[index].Path() != second[index].Path() || string(first[index].Bytes) != string(second[index].Bytes) {
			t.Fatalf("%s is not stable across emissions", first[index].Path())
		}
	}
}
