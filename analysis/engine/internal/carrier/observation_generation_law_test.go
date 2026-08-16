package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/identity"
)

// TestObservationGenerationsAdvanceOnceAndFenceStaleHandles proves the scratch
// fence of one observation namespace. Its stamp is a Generation of that
// namespace alone: BeginObservation advances it by exactly one, an unset or
// superseded stamp is refused by every issue/resolve entry, and a row that
// escaped its callback stops naming a support page once the stamp moves.
func TestObservationGenerationsAdvanceOnceAndFenceStaleHandles(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	operation := &carryOnlyOperation{guards: manager}
	if _, ok := attachTestComposition(t, []FactorOperation{operation}); !ok {
		t.Fatal("composition")
	}
	issuer := operation.issuer
	work, ok := issuer.NewObservationWork()
	if !ok {
		t.Fatal("observation work")
	}
	var unset identity.Generation
	if _, issued := issuer.IssueObservation(work, unset, 1); issued {
		t.Fatal("an unset stamp issued an observation")
	}
	first, opened := issuer.BeginObservation(work)
	if !opened || first != unset.Next() {
		t.Fatalf("first observation generation = %d", first)
	}
	handle, issued := issuer.IssueObservation(work, first, 7)
	row, rowOK := work.Row(handle, whole)
	if !issued || !rowOK || !row.Region().Valid() {
		t.Fatal("observation row")
	}
	if _, resolved := issuer.ResolveObservation(work, first.Next(), handle); resolved {
		t.Fatal("a future stamp resolved a live observation")
	}
	if _, resolved := issuer.ResolveObservation(work, unset, handle); resolved {
		t.Fatal("an unset stamp resolved a live observation")
	}
	if id, resolved := issuer.ResolveObservation(work, first, handle); !resolved || id != 7 {
		t.Fatal("live observation did not resolve under its own stamp")
	}
	issuer.EndObservation(work, first)

	second, reopened := issuer.BeginObservation(work)
	if !reopened || second != first.Next() || !first.Precedes(second) {
		t.Fatalf("reopened observation generation = %d after %d", second, first)
	}
	if _, resolved := issuer.ResolveObservation(work, first, handle); resolved {
		t.Fatal("a superseded stamp resolved an escaped observation")
	}
	if _, resolved := issuer.ResolveObservation(work, second, handle); resolved {
		t.Fatal("an escaped handle resolved under the live stamp")
	}
	if row.Region().Valid() {
		t.Fatal("an escaped row still named a support page after its generation closed")
	}
	if _, rowOK := work.Row(handle, whole); rowOK {
		t.Fatal("a superseded handle appended a row to the live generation")
	}
	issuer.EndObservation(work, first)
	if _, resolved := issuer.ResolveObservation(work, second, handle); resolved {
		t.Fatal("a stale close ended the live generation")
	}
	issuer.EndObservation(work, second)
	if _, reissued := issuer.IssueObservation(work, second, 7); reissued {
		t.Fatal("a closed generation issued an observation")
	}
}
