package causal

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// TestVarargTailRejectsForeignOwnerAtCausalBoundary keeps the hostile case at
// the Causal seam.  Authored Flow admits the row shape (its owner is a Body),
// but Causal must still prove that the open tail belongs to the Return's Body
// and is executable before treating it as a valid non-Call result producer.
func TestVarargTailRejectsForeignOwnerAtCausalBoundary(t *testing.T) {
	body := causalTerm(keyspace.FamilyBody, 1)
	functionBody := causalTerm(keyspace.FamilyBody, 2)
	function := causalTerm(keyspace.FamilyFunction, 1)
	outerReturn := causalTerm(keyspace.FamilyReturn, 1)
	outerValues := causalTerm(keyspace.FamilyValues, 1)
	returned := causalTerm(keyspace.FamilyReturn, 2)
	values := causalTerm(keyspace.FamilyValues, 2)
	vararg := causalTerm(keyspace.FamilyVararg, 1)
	cell := causalTerm(keyspace.FamilyCell, 1)
	counts := causalCounts(
		causalFamilyCount{keyspace.FamilyBody, 2},
		causalFamilyCount{keyspace.FamilyFunction, 1},
		causalFamilyCount{keyspace.FamilyReturn, 2},
		causalFamilyCount{keyspace.FamilyValues, 2},
		causalFamilyCount{keyspace.FamilyVararg, 1},
		causalFamilyCount{keyspace.FamilyCell, 1},
	)
	validFlow := authored.Input{
		Counts: counts,
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: body, Fixed: authored.Range{End: 1}},
				{Owner: functionBody, Fixed: authored.Range{Start: 1, End: 1}, Tail: vararg},
			},
			Terms: []keyspace.Term{function},
		},
		Storage: authored.StorageInput{
			Cells:   []authored.Cell{{Kind: authored.CellLocal, Body: functionBody}},
			Varargs: []authored.Vararg{{Owner: functionBody, Cell: cell}},
		},
		Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body, Body: functionBody, Vararg: cell}}},
		Control:   authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: outerValues}, {Owner: functionBody, Values: values}}},
	}
	valid := openCausalFixture(t, causalSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{outerReturn}, {returned}},
		flow:   validFlow,
	})

	malformedFlow := validFlow
	malformedFlow.Storage.Varargs = []authored.Vararg{{Owner: body, Cell: cell}}
	draft, err := authored.Build(malformedFlow)
	if err != nil {
		t.Fatalf("malformed authored fixture was rejected before Causal: %v", err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("malformed authored finalizer: %v", err)
	}
	view := finalizer.View()
	t.Cleanup(func() { _ = finalizer.Abort() })

	proof := &proofState{
		flow:   view,
		counts: counts,
		exec:   valid.executable,
		typedScratch: &typedScratch{
			valueParent: []keyspace.Term{0, outerReturn, returned},
		},
	}
	state := &boundaryState{
		structureState: &structureState{
			routeState: &routeState{
				evalState: &evalState{proofState: proof},
			},
		},
		callState: &callState{proofState: proof, tailPlans: make([]keyspace.Term, 1)},
	}
	if err := state.buildTailPlans(); err == nil || !strings.Contains(err.Error(), "malformed Vararg tail ownership") {
		t.Fatalf("foreign-owner Vararg tail error = %v, want ownership rejection", err)
	}
}
