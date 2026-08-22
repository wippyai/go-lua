package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/domain/effect/capability"
)

// TestDiagnosticRegisterPartitionsEveryNamedCode is the vocabulary law: the
// sealed table and the declared-not-composed register partition the analyzer's
// published code space. A code that is composed and also registered would make
// the register a stale note; a registered code that is composed and
// collectable would let a landed producer go on being reported as owed.
func TestDiagnosticRegisterPartitionsEveryNamedCode(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("sealed composition unavailable")
	}
	table, tableOK := Diagnostics(compilation)
	if !tableOK {
		t.Fatal("sealed diagnostic declaration table unavailable")
	}
	registered := make(map[diagnostic.Code]DiagnosticDeclaredCode, len(diagnosticDeclaredCodes))
	for _, row := range diagnosticDeclaredCodes {
		if row.Code == "" {
			t.Error("declared-not-composed register holds a row with no code")
			continue
		}
		if row.Owner == "" || row.Reason == "" {
			t.Errorf("declared-not-composed code %q has no owner or no reason; an unowned judgment is the silence the register exists to end", row.Code)
		}
		if _, duplicate := registered[row.Code]; duplicate {
			t.Errorf("declared-not-composed code %q is registered twice", row.Code)
		}
		registered[row.Code] = row
	}
	for position := 0; position < table.Count(); position++ {
		entry, entryOK := table.At(position)
		if !entryOK {
			t.Fatalf("sealed diagnostic row %d is unavailable", position)
		}
		row, isRegistered := registered[entry.Code()]
		if !isRegistered {
			continue
		}
		if entry.Collectable() {
			t.Errorf("code %q composes a collectable producer and is still registered as owed by %s; remove its register row", entry.Code(), row.Owner)
		}
	}
}

// TestTypestateJudgmentsAgreeWithTheCapabilityRegister binds the two registers
// that describe the same gap from opposite ends.
//
// The Target seal admits a typestate declaration, translates it, and publishes a
// protocol table of states, acquisitions, transitions, and escapes. Nothing
// lowers that table into facts today, which the effect capability register
// states directly: the lifecycle capabilities are manifest-validated, with the
// rationale that no lowering consumes them. The typestate diagnostic codes are
// the judgments that would read the lowered facts, so while the capability
// register says the facts are not produced, those codes cannot be composed -
// composing one would decide a program from a table nothing fills.
//
// The law therefore fails from both sides: composing a typestate judgment
// before the lowering lands, and leaving these codes registered after the
// lowering makes the capabilities operational. Either way the two registers are
// re-read together rather than one drifting behind the other.
func TestTypestateJudgmentsAgreeWithTheCapabilityRegister(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("sealed composition unavailable")
	}
	lowered := false
	for _, descriptor := range capability.All() {
		if descriptor.Family != "lifecycle" {
			continue
		}
		if descriptor.Status != capability.StatusManifestValidated {
			lowered = true
		}
	}
	codes := []diagnostic.Code{
		DiagnosticCodeTypestateInvalidRequirement,
		DiagnosticCodeTypestateInvalidTransition,
		DiagnosticCodeTypestateUnprovenRequirement,
	}
	for _, code := range codes {
		status, row := DiagnosticCodeAnswer(compilation, code)
		if lowered {
			if status != DiagnosticCodeComposed {
				t.Errorf("a lifecycle capability is lowered, so typestate judgment %q must be composed rather than left owed by %s", code, row.Owner)
			}
			continue
		}
		if status != DiagnosticCodeDeclared {
			t.Errorf("typestate judgment %q answers %d while no lifecycle capability is lowered; it would decide a program from a protocol table nothing fills", code, status)
		}
		if row.Owner != diagnosticOwnerTypestate {
			t.Errorf("typestate judgment %q is owed by %q, want %q", code, row.Owner, diagnosticOwnerTypestate)
		}
	}
}

// TestDiagnosticCodeAnswerReadsBothHalves proves the single reading a consumer
// takes: a composed code answers composed, a registered one answers declared
// with its owner, and a name in neither half answers unknown rather than
// passing as an absent-but-fine selection.
func TestDiagnosticCodeAnswerReadsBothHalves(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("sealed composition unavailable")
	}
	status, _ := DiagnosticCodeAnswer(compilation, DiagnosticCodeUnresolvedValueReference)
	if status != DiagnosticCodeComposed {
		t.Errorf("composed code answered %d, want composed", status)
	}
	status, row := DiagnosticCodeAnswer(compilation, DiagnosticCodeMemberMissing)
	if status != DiagnosticCodeDeclared || row.Owner == "" {
		t.Errorf("registered code answered %d owner=%q, want declared with an owner", status, row.Owner)
	}
	// A declared-lane row is composed as a declaration and still uncollectable,
	// so the register is what keeps it from reading as a producer.
	status, row = DiagnosticCodeAnswer(compilation, DiagnosticCodeUnusedLocal)
	if status != DiagnosticCodeDeclared || row.Owner == "" {
		t.Errorf("declared-lane code answered %d owner=%q, want declared with an owner", status, row.Owner)
	}
	if status, _ := DiagnosticCodeAnswer(compilation, diagnostic.Code("no.such.code")); status != DiagnosticCodeUnknown {
		t.Errorf("unnamed code answered %d, want unknown", status)
	}
}
