package sourcecontrol

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

func TestResumeLabelsSelectNextRootAndBodyTail(t *testing.T) {
	counts := countsWith(
		familyCount{keyspace.FamilyBody, 1},
		familyCount{keyspace.FamilyLabel, 4},
		familyCount{keyspace.FamilyBind, 1},
		familyCount{keyspace.FamilyValues, 1},
		familyCount{keyspace.FamilyCell, 1},
	)
	body := term(keyspace.FamilyBody, 1)
	labels := []keyspace.Term{
		term(keyspace.FamilyLabel, 1), term(keyspace.FamilyLabel, 2),
		term(keyspace.FamilyLabel, 3), term(keyspace.FamilyLabel, 4),
	}
	bind, values, cell := term(keyspace.FamilyBind, 1), term(keyspace.FamilyValues, 1), term(keyspace.FamilyCell, 1)
	spec := semanticSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{labels[0], labels[1], bind, labels[2], labels[3]}},
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}}, Terms: nil},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
				Binds: []authored.Bind{{Owner: body, Values: values}},
			},
			Control: authored.ControlInput{Labels: []authored.Label{
				{Owner: body}, {Owner: body}, {Owner: body}, {Owner: body},
			}},
		},
	}
	fixture := openSemanticFixture(t, spec)
	for _, label := range labels[:2] {
		if got, ok := fixture.result.Resume(label); !ok || got != bind {
			t.Fatalf("Resume(%v) = %v/%v, want next dynamic root %v", label, got, ok, bind)
		}
	}
	for _, label := range labels[2:] {
		if got, ok := fixture.result.Resume(label); !ok || got != body {
			t.Fatalf("Resume(%v) = %v/%v, want owning Body tail %v", label, got, ok, body)
		}
	}
}

func TestResumeLoopsSelectNextRootAndBodyTail(t *testing.T) {
	counts := countsWith(
		familyCount{keyspace.FamilyBody, 3},
		familyCount{keyspace.FamilyLoop, 2},
		familyCount{keyspace.FamilyNil, 2},
	)
	parent, child1, child2 := term(keyspace.FamilyBody, 1), term(keyspace.FamilyBody, 2), term(keyspace.FamilyBody, 3)
	loop1, loop2 := term(keyspace.FamilyLoop, 1), term(keyspace.FamilyLoop, 2)
	nil1, nil2 := term(keyspace.FamilyNil, 1), term(keyspace.FamilyNil, 2)
	spec := semanticSpec{
		counts:    counts,
		rows:      [][]keyspace.Term{{loop1, loop2}, nil, nil},
		nilOwners: []keyspace.Term{parent, parent},
		flow: authored.Input{Control: authored.ControlInput{
			Loops: []authored.Loop{
				{Owner: parent, Body: child1, Kind: kind.LoopWhile, Control: nil1},
				{Owner: parent, Body: child2, Kind: kind.LoopWhile, Control: nil2},
			},
		}},
	}
	fixture := openSemanticFixture(t, spec)
	if got, ok := fixture.result.Resume(loop1); !ok || got != loop2 {
		t.Fatalf("Resume(%v) = %v/%v, want next root %v", loop1, got, ok, loop2)
	}
	if got, ok := fixture.result.Resume(loop2); !ok || got != parent {
		t.Fatalf("Resume(%v) = %v/%v, want owning Body tail %v", loop2, got, ok, parent)
	}
}

func TestResumeRejectsInvalidTermsAndMalformedAnchors(t *testing.T) {
	counts := countsWith(
		familyCount{keyspace.FamilyBody, 1},
		familyCount{keyspace.FamilyLabel, 1},
	)
	body, label := term(keyspace.FamilyBody, 1), term(keyspace.FamilyLabel, 1)
	fixture := openSemanticFixture(t, semanticSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{label}},
		flow:   authored.Input{Control: authored.ControlInput{Labels: []authored.Label{{Owner: body}}}},
	})
	var nilResult *Result
	invalid := []keyspace.Term{0, body, term(keyspace.FamilyBind, 1), keyspace.MakeTerm(keyspace.FamilyLabel, 2), keyspace.MakeTerm(keyspace.FamilyLoop, 1)}
	for _, term := range invalid {
		if got, ok := fixture.result.Resume(term); ok || got != 0 {
			t.Fatalf("Resume(%v) accepted invalid term: %v/%v", term, got, ok)
		}
		if got, ok := nilResult.Resume(term); ok || got != 0 {
			t.Fatalf("nil Result.Resume(%v) = %v/%v", term, got, ok)
		}
	}
	badOwner := term(keyspace.FamilyBody, 2)
	if _, err := validateResumeSourcePosition(fixture.sourceView, fixture.bodies, fixture.forest,
		&geometry{coordinates: fixture.result.coordinates, labelNodes: []uint32{noNode, noNode}},
		label, badOwner, []uint32{noNode, noNode}); err == nil {
		t.Fatal("resume accepted an authored Label with a foreign owner")
	}
	if _, err := validateResumeSourcePosition(fixture.sourceView, fixture.bodies, fixture.forest,
		&geometry{coordinates: fixture.result.coordinates, labelNodes: []uint32{noNode, noNode}},
		label, body, []uint32{noNode, noNode}); err == nil {
		t.Fatal("resume accepted a Label with a malformed source-root coordinate")
	}
}

func TestResumeQueriesAllocateNothingAndAreDeterministic(t *testing.T) {
	counts := countsWith(
		familyCount{keyspace.FamilyBody, 1},
		familyCount{keyspace.FamilyLabel, 1},
		familyCount{keyspace.FamilyBind, 1},
		familyCount{keyspace.FamilyValues, 1},
		familyCount{keyspace.FamilyCell, 1},
	)
	body, label := term(keyspace.FamilyBody, 1), term(keyspace.FamilyLabel, 1)
	bind, values, cell := term(keyspace.FamilyBind, 1), term(keyspace.FamilyValues, 1), term(keyspace.FamilyCell, 1)
	spec := semanticSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{label, bind}},
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}}, Terms: nil},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
				Binds: []authored.Bind{{Owner: body, Values: values}},
			},
			Control: authored.ControlInput{Labels: []authored.Label{{Owner: body}}},
		},
	}
	first, second := openSemanticFixture(t, spec), openSemanticFixture(t, spec)
	want, wantOK := first.result.Resume(label)
	got, gotOK := second.result.Resume(label)
	if got != want || gotOK != wantOK {
		t.Fatalf("deterministic Resume(%v) changed: %v/%v vs %v/%v", label, want, wantOK, got, gotOK)
	}
	allocs := testing.AllocsPerRun(100, func() {
		if _, ok := first.result.Resume(label); !ok {
			t.Fatal("Resume unexpectedly failed during allocation law")
		}
		if _, ok := first.result.Resume(keyspace.MakeTerm(keyspace.FamilyLoop, 1)); ok {
			t.Fatal("Resume accepted an absent Loop")
		}
	})
	if allocs != 0 {
		t.Fatalf("Resume allocated %v times per query batch", allocs)
	}
}
