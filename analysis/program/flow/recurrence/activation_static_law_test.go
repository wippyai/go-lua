package recurrence

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// TestSealExcludesUnreachableDecisionThroughOwners protects the distinction
// between authored presence and executable reachability.  The Function is
// positioned after a terminal Return in the entry Body, so its activation and
// the Loop below it remain in the sealed SourceControl denominator but are
// not reachable from the entry node.  Recurrence must retain neither the
// unreachable Loop decision nor a Mu reset for it.
func TestSealExcludesUnreachableDecisionThroughOwners(t *testing.T) {
	counts := countsWith(
		familyCount(keyspace.FamilyBody, 3),
		familyCount(keyspace.FamilyValues, 2),
		familyCount(keyspace.FamilyFunction, 1),
		familyCount(keyspace.FamilyBind, 1),
		familyCount(keyspace.FamilyCell, 1),
		familyCount(keyspace.FamilyReturn, 1),
		familyCount(keyspace.FamilyNil, 1),
		familyCount(keyspace.FamilyLoop, 1),
	)
	entry := term(keyspace.FamilyBody, 1)
	functionBody := term(keyspace.FamilyBody, 2)
	loopBody := term(keyspace.FamilyBody, 3)
	function := term(keyspace.FamilyFunction, 1)
	bind := term(keyspace.FamilyBind, 1)
	cell := term(keyspace.FamilyCell, 1)
	returnTerm := term(keyspace.FamilyReturn, 1)
	returnValues := term(keyspace.FamilyValues, 1)
	functionValues := term(keyspace.FamilyValues, 2)
	loop := term(keyspace.FamilyLoop, 1)
	nilTerm := term(keyspace.FamilyNil, 1)

	fixture := openOwnerFixture(t, ownerSpec{
		counts: counts,
		// The bind carrying the Function occurs after Return.  It therefore
		// supplies a valid source position without making the Function
		// activation executable from the entry node.
		rows:      [][]keyspace.Term{{returnTerm, bind}, {loop}, nil},
		binds:     []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		nilOwners: []keyspace.Term{functionBody},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: entry}, {Owner: entry, Fixed: authored.Range{End: 1}}},
				Terms: []keyspace.Term{function},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: entry}},
				Binds: []authored.Bind{{Owner: entry, Values: functionValues}},
			},
			Functions: authored.FunctionsInput{
				Rows: []authored.Function{{Owner: entry, Body: functionBody}},
			},
			Control: authored.ControlInput{
				Returns: []authored.Return{{Owner: entry, Values: returnValues}},
				Loops:   []authored.Loop{{Owner: functionBody, Body: loopBody, Kind: kind.LoopWhile, Control: nilTerm}},
			},
		},
	})
	recurrence, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph,
		fixture.staticView.ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("recurrence.Seal: %v", err)
	}
	if count, ok := recurrence.DecisionCount(loop); ok || count != 0 {
		t.Fatalf("unreachable Loop decision stream = %d/%v, want 0/false", count, ok)
	}
	for index := 0; index < recurrence.ArcCount(); index++ {
		annotation, ok := recurrence.ArcAt(index)
		if !ok || annotation.Head != loop {
			continue
		}
		t.Fatalf("unreachable Loop acquired recurrence Arc %d: %#v", index, annotation)
	}
}

// TestSealKeepsFunctionActivationDecisionStreamsIndependentThroughOwners
// protects the activation boundary.  Both Function values are reachable from
// the entry Body, but each Function owns a different Loop and child Body.
// Their Mu streams and reset intervals must not merge merely because their
// source ordinals or traversal roots are adjacent.
func TestSealKeepsFunctionActivationDecisionStreamsIndependentThroughOwners(t *testing.T) {
	counts := countsWith(
		familyCount(keyspace.FamilyBody, 5),
		familyCount(keyspace.FamilyValues, 2),
		familyCount(keyspace.FamilyFunction, 2),
		familyCount(keyspace.FamilyBind, 2),
		familyCount(keyspace.FamilyCell, 2),
		familyCount(keyspace.FamilyNil, 2),
		familyCount(keyspace.FamilyLoop, 2),
	)
	entry := term(keyspace.FamilyBody, 1)
	firstFunctionBody, firstLoopBody := term(keyspace.FamilyBody, 2), term(keyspace.FamilyBody, 3)
	secondFunctionBody, secondLoopBody := term(keyspace.FamilyBody, 4), term(keyspace.FamilyBody, 5)
	firstFunction, secondFunction := term(keyspace.FamilyFunction, 1), term(keyspace.FamilyFunction, 2)
	firstBind, secondBind := term(keyspace.FamilyBind, 1), term(keyspace.FamilyBind, 2)
	firstCell, secondCell := term(keyspace.FamilyCell, 1), term(keyspace.FamilyCell, 2)
	firstValues, secondValues := term(keyspace.FamilyValues, 1), term(keyspace.FamilyValues, 2)
	firstLoop, secondLoop := term(keyspace.FamilyLoop, 1), term(keyspace.FamilyLoop, 2)
	firstNil, secondNil := term(keyspace.FamilyNil, 1), term(keyspace.FamilyNil, 2)

	fixture := openOwnerFixture(t, ownerSpec{
		counts:    counts,
		rows:      [][]keyspace.Term{{firstBind, secondBind}, {firstLoop}, nil, {secondLoop}, nil},
		binds:     []source.BindCells{{Bind: firstBind, Cells: []keyspace.Term{firstCell}}, {Bind: secondBind, Cells: []keyspace.Term{secondCell}}},
		nilOwners: []keyspace.Term{firstFunctionBody, secondFunctionBody},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: entry, Fixed: authored.Range{End: 1}}, {Owner: entry, Fixed: authored.Range{Start: 1, End: 2}}},
				Terms: []keyspace.Term{firstFunction, secondFunction},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{
					{Kind: authored.CellLocal, Body: entry},
					{Kind: authored.CellLocal, Body: entry},
				},
				Binds: []authored.Bind{
					{Owner: entry, Values: firstValues},
					{Owner: entry, Values: secondValues},
				},
			},
			Functions: authored.FunctionsInput{Rows: []authored.Function{
				{Owner: entry, Body: firstFunctionBody},
				{Owner: entry, Body: secondFunctionBody},
			}},
			Control: authored.ControlInput{Loops: []authored.Loop{
				{Owner: firstFunctionBody, Body: firstLoopBody, Kind: kind.LoopWhile, Control: firstNil},
				{Owner: secondFunctionBody, Body: secondLoopBody, Kind: kind.LoopWhile, Control: secondNil},
			}},
		},
	})
	recurrence, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph,
		fixture.staticView.ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("recurrence.Seal: %v", err)
	}
	for _, loop := range []keyspace.Term{firstLoop, secondLoop} {
		count, ok := recurrence.DecisionCount(loop)
		if !ok || count != 1 {
			t.Fatalf("Function Loop %v decision stream = %d/%v, want 1/true", loop, count, ok)
		}
		if got, ok := recurrence.DecisionAt(loop, 0); !ok || got != loop {
			t.Fatalf("Function Loop %v stream head = %v/%v, want %v/true", loop, got, ok, loop)
		}
	}
	for index := 0; index < recurrence.ArcCount(); index++ {
		annotation, ok := recurrence.ArcAt(index)
		if !ok {
			continue
		}
		if annotation.Head != firstLoop && annotation.Head != secondLoop {
			continue
		}
		if count, ok := recurrence.ResetCount(index); !ok || count != 1 {
			t.Fatalf("Function activation reset for %v = %d/%v, want 1/true", annotation.Head, count, ok)
		}
		if !recurrence.ResetContains(index, annotation.Head) {
			t.Fatalf("Function activation reset for %v omitted its own decision", annotation.Head)
		}
		other := firstLoop
		if other == annotation.Head {
			other = secondLoop
		}
		if recurrence.ResetContains(index, other) {
			t.Fatalf("Function activation reset for %v leaked decision %v", annotation.Head, other)
		}
	}
}
