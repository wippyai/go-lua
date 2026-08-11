package verify

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
)

func TestVerifyImportDeltaIsExactAndAcyclic(t *testing.T) {
	before := snapshot("a.go", "example/a", "package a\nimport old \"example/b\"")
	after := snapshot("a.go", "example/a", "package a\nimport next \"example/c\"")
	request := Request{
		Before:         before,
		After:          after,
		Imports:        []cutplan.Import{{Consumer: "a.go", From: &cutplan.ImportRef{Path: "example/b", Name: "old", Alias: "old"}, To: &cutplan.ImportRef{Path: "example/c", Name: "next", Alias: "next"}, Symbols: []cutplan.SymbolRef{{Object: "example/c#package:Value"}}}},
		RequestedGates: []cutplan.Gate{cutplan.GateImportDAG},
		Dispositions:   []GateDisposition{{Gate: cutplan.GateImportDAG}},
	}
	report, err := Verify(request)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if got, want := report.Executed, []cutplan.Gate{cutplan.GateImportDAG}; !sameGates(got, want) {
		t.Fatalf("executed = %v, want %v", got, want)
	}
	if got, want := report.ImportDeltas, []ImportDelta{{Consumer: "a.go", Removed: []ImportSpec{{Consumer: "a.go", Path: "example/b", Alias: "old"}}, Added: []ImportSpec{{Consumer: "a.go", Path: "example/c", Alias: "next"}}}}; !sameImportDeltas(got, want) {
		t.Fatalf("import deltas = %#v, want %#v", got, want)
	}
	if report.Digest == "" {
		t.Fatal("successful structural report needs a deterministic digest")
	}
}

func TestVerifyImportDeltaPreservesImplicitAliasSpelling(t *testing.T) {
	request := Request{
		Before: snapshot("a.go", "example/a", "package a\nimport \"example/b\""),
		After:  snapshot("a.go", "example/a", "package a\nimport \"example/c\""),
		Imports: []cutplan.Import{{
			Consumer: "a.go",
			From:     &cutplan.ImportRef{Path: "example/b", Name: "b", Alias: ""},
			To:       &cutplan.ImportRef{Path: "example/c", Name: "c", Alias: ""},
			Symbols:  []cutplan.SymbolRef{{Object: "example/c#package:Value"}},
		}},
	}
	report, err := Verify(request)
	if err != nil {
		t.Fatalf("implicit import delta rejected: %v", err)
	}
	want := []ImportDelta{{Consumer: "a.go", Removed: []ImportSpec{{Consumer: "a.go", Path: "example/b", Alias: ""}}, Added: []ImportSpec{{Consumer: "a.go", Path: "example/c", Alias: ""}}}}
	if !sameImportDeltas(report.ImportDeltas, want) {
		t.Fatalf("implicit delta = %#v, want %#v", report.ImportDeltas, want)
	}
}

func TestVerifyRejectsUndeclaredImportGraphDelta(t *testing.T) {
	request := Request{
		Before:         snapshot("a.go", "a", "package a\nimport \"b\""),
		After:          snapshot("a.go", "a", "package a\nimport \"c\""),
		RequestedGates: []cutplan.Gate{cutplan.GateImportDAG},
		Dispositions:   []GateDisposition{{Gate: cutplan.GateImportDAG}},
	}
	wantFailure(t, request, "exact import-spec delta")
}

func TestVerifyRejectsPostCutImportCycle(t *testing.T) {
	request := Request{
		Before: snapshot("a.go", "a", "package a\nimport \"b\""),
		After: Snapshot{Sources: SourceMap{
			"a.go": {Path: "a.go", Package: "a", Source: []byte("package a\nimport \"b\"")},
			"b.go": {Path: "b.go", Package: "b", Source: []byte("package b\nimport \"a\"")},
		}},
		Imports:        []cutplan.Import{{Consumer: "b.go", To: &cutplan.ImportRef{Path: "a", Name: "a", Alias: ""}, Symbols: []cutplan.SymbolRef{{Object: "a#package:Value"}}}},
		RequestedGates: []cutplan.Gate{cutplan.GateImportDAG},
		Dispositions:   []GateDisposition{{Gate: cutplan.GateImportDAG}},
	}
	wantFailure(t, request, "cycle")
}

func TestVerifyImportDeltaIsPerConsumerAndAliasExact(t *testing.T) {
	before := Snapshot{Sources: SourceMap{
		"one.go": {Path: "one.go", Package: "p", Source: []byte("package p\nimport old \"example/x\"")},
		"two.go": {Path: "two.go", Package: "p", Source: []byte("package p\nimport old \"example/x\"")},
	}}
	after := Snapshot{Sources: SourceMap{
		"one.go": {Path: "one.go", Package: "p", Source: []byte("package p\nimport next \"example/y\"")},
		"two.go": {Path: "two.go", Package: "p", Source: []byte("package p\nimport old \"example/x\"")},
	}}
	request := Request{Before: before, After: after, Imports: []cutplan.Import{{
		Consumer: "one.go",
		From:     &cutplan.ImportRef{Path: "example/x", Name: "old", Alias: "old"},
		To:       &cutplan.ImportRef{Path: "example/y", Name: "next", Alias: "next"},
		Symbols:  []cutplan.SymbolRef{{Object: "example/y#package:Value"}},
	}}}
	if _, err := Verify(request); err != nil {
		t.Fatalf("exact consumer-local route should pass: %v", err)
	}
	request.Imports[0].Consumer = "two.go"
	wantFailure(t, request, "for one.go")
	request.Imports[0].Consumer = "one.go"
	request.Imports[0].To.Alias = "wrong"
	wantFailure(t, request, "for one.go")
}

func TestVerifyRejectsDuplicateOrUnroutedImportSpec(t *testing.T) {
	request := Request{
		Before: snapshot("a.go", "a", "package a"),
		After:  snapshot("a.go", "a", "package a\nimport \"b\"\nimport \"b\""),
	}
	wantFailure(t, request, "duplicates import")
}

func TestVerifyReportDigestIsStable(t *testing.T) {
	request := Request{
		Before:         snapshot("a.go", "a", "package a"),
		After:          snapshot("a.go", "a", "package a"),
		RequestedGates: []cutplan.Gate{cutplan.GateDiagnostics},
		Dispositions: []GateDisposition{{Gate: cutplan.GateDiagnostics, External: &ExternalEvidence{
			Kind: cutplan.GateDiagnostics, Passed: true, Digest: "semantic",
		}}},
	}
	first, err := Verify(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Verify(request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest {
		t.Fatalf("digest = %q then %q", first.Digest, second.Digest)
	}
	if first.PostDigest == "" || len(first.Evidence) != 1 || first.Evidence[0] != (GateEvidence{Gate: cutplan.GateDiagnostics, Digest: "semantic"}) {
		t.Fatalf("evidence = %#v, post digest = %q", first.Evidence, first.PostDigest)
	}
}

func TestVerifyRunsOnlyRequestedGate(t *testing.T) {
	request := Request{
		Before:         snapshot("a.go", "a", "package a"),
		After:          snapshot("a.go", "a", "package a"),
		RequestedGates: []cutplan.Gate{cutplan.GateDiagnostics},
		Dispositions: []GateDisposition{{
			Gate:     cutplan.GateDiagnostics,
			External: &ExternalEvidence{Kind: cutplan.GateDiagnostics, Passed: true, Digest: "semantic-output-v1"},
		}},
	}
	report, err := Verify(request)
	if err != nil {
		t.Fatalf("unrequested structural gate must not run: %v", err)
	}
	if !sameGates(report.Executed, []cutplan.Gate{cutplan.GateDiagnostics}) {
		t.Fatalf("executed = %v", report.Executed)
	}
	request.Dispositions = append(request.Dispositions, GateDisposition{Gate: cutplan.GateImportDAG})
	wantFailure(t, request, "unrequested")
}

func TestVerifyRequiresOneValidDispositionPerRequestedGate(t *testing.T) {
	request := Request{
		Before:         snapshot("a.go", "a", "package a"),
		After:          snapshot("a.go", "a", "package a"),
		RequestedGates: []cutplan.Gate{cutplan.GateDiagnostics, cutplan.GateResidue},
		Dispositions: []GateDisposition{{
			Gate:     cutplan.GateDiagnostics,
			External: &ExternalEvidence{Kind: cutplan.GateDiagnostics, Passed: true, Digest: "ok"},
		}},
	}
	wantFailure(t, request, "has no disposition")
	request.Dispositions = append(request.Dispositions, GateDisposition{
		Gate:     cutplan.GateResidue,
		External: &ExternalEvidence{Kind: cutplan.GateResidue, Passed: false, Digest: "not-ok"},
	})
	wantFailure(t, request, "requires successful")
}

func TestVerifyRejectsUnparseableSourceRegardlessOfGates(t *testing.T) {
	request := Request{
		Before: snapshot("a.go", "a", "package a"),
		After:  snapshot("a.go", "a", "package a func"),
	}
	wantFailure(t, request, "does not parse")
}

func snapshot(path, pkg, source string) Snapshot {
	return Snapshot{Sources: SourceMap{path: {Path: path, Package: pkg, Source: []byte(source)}}}
}

func wantFailure(t *testing.T, request Request, text string) {
	t.Helper()
	_, err := Verify(request)
	if err == nil || !strings.Contains(err.Error(), text) {
		t.Fatalf("Verify() error = %v, want containing %q", err, text)
	}
}

func sameGates(left, right []cutplan.Gate) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameImportDeltas(left, right []ImportDelta) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Consumer != right[index].Consumer || !sameImportSpecs(left[index].Removed, right[index].Removed) || !sameImportSpecs(left[index].Added, right[index].Added) {
			return false
		}
	}
	return true
}

func sameImportSpecs(left, right []ImportSpec) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
