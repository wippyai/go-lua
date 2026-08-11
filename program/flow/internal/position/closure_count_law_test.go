package position

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

func TestAnchorClosureCountsSharedDirectAndRootlessPaths(t *testing.T) {
	counts := positionCounts(1, 1, 1, 1, 0, 0, 0, 0, 0, 0)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	integer := keyspace.MakeTerm(keyspace.FamilyInteger, 1)
	fixture := openPositionFixture(t, positionSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{returned}},
		ints:   []source.IntegerLiteral{{Owner: body, Value: 7}},
		flow:   authoredInputForPositionCountLaw(body, values, integer),
	})
	staticID := fixture.staticFinalize.View().ContentID()
	moduleID := fixture.moduleFinalize.View().ContentID()
	validatedCounts, total, err := validateInputs(fixture.preimage, fixture.flow, fixture.bodies, fixture.forest, fixture.outcomes, fixture.entry, staticID, moduleID)
	if err != nil {
		t.Fatalf("validateInputs: %v", err)
	}
	direct, err := directSources(fixture.preimage, fixture.bodies, fixture.forest, validatedCounts)
	if err != nil {
		t.Fatalf("directSources: %v", err)
	}
	if err := sealLoops(fixture.flow, fixture.bodies, fixture.forest, validatedCounts, direct); err != nil {
		t.Fatalf("sealLoops: %v", err)
	}
	wantDirect := direct[keyspace.FamilyReturn][0]
	gotCount, err := anchorClosure(fixture.forest, validatedCounts, total, direct)
	if err != nil {
		t.Fatalf("anchorClosure: %v", err)
	}
	if gotCount != 3 {
		t.Fatalf("position count = %d, want shared Return/Values/Integer count 3", gotCount)
	}
	if direct[keyspace.FamilyReturn][0] != wantDirect {
		t.Fatalf("direct Return anchor was overwritten: before=%#v after=%#v", wantDirect, direct[keyspace.FamilyReturn][0])
	}
	if direct[keyspace.FamilyBody][0].root != 0 {
		t.Fatalf("rootless Body acquired anchor: %#v", direct[keyspace.FamilyBody][0])
	}
	for _, family := range []keyspace.Family{keyspace.FamilyReturn, keyspace.FamilyValues, keyspace.FamilyInteger} {
		anchor := direct[family][0]
		if anchor.root != returned || anchor.body != body || anchor.frontierBody != body {
			t.Fatalf("family %v anchor = %#v, want shared Return anchor", family, anchor)
		}
	}
	positions, err := emitPositions(validatedCounts, direct, gotCount)
	if err != nil {
		t.Fatalf("emitPositions: %v", err)
	}
	if len(positions) != gotCount {
		t.Fatalf("emitted positions = %d, want %d", len(positions), gotCount)
	}
}

func TestEmitPositionsRequiresExactCountAndPreservesRepeat(t *testing.T) {
	counts := positionCounts(1, 1, 0, 0, 0, 0, 0, 0, 1, 0)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	direct := makeDirectTableForCountLaw(counts)
	direct[keyspace.FamilyReturn][0] = positionAnchor{root: returned, body: body, frontierBody: body}
	direct[keyspace.FamilyLoop][0] = positionAnchor{root: loop, body: body, frontierBody: body, repeat: true}

	positions, err := emitPositions(counts, direct, 2)
	if err != nil {
		t.Fatalf("emitPositions exact count: %v", err)
	}
	if len(positions) != 2 || positions[0].Term != returned || positions[1].Term != loop || !positions[1].Repeat {
		t.Fatalf("positions = %#v, want Return then Repeat Loop", positions)
	}
	if _, err := emitPositions(counts, direct, 1); err == nil {
		t.Fatal("emitPositions accepted a short count")
	}
	if _, err := emitPositions(counts, direct, 3); err == nil {
		t.Fatal("emitPositions accepted a long count")
	}
}

func TestPositionClosureRejectsTableDenominatorMismatch(t *testing.T) {
	counts := positionCounts(1, 1, 0, 0, 0, 0, 0, 0, 0, 0)
	var direct directTable
	if got, err := anchorClosure(nil, counts, 2, direct); err == nil || got != 0 {
		t.Fatalf("anchorClosure mismatch = %d/%v, want zero/error", got, err)
	}
	if positions, err := emitPositions(counts, direct, 0); err == nil || positions != nil {
		t.Fatalf("emitPositions mismatch = %#v/%v, want nil/error", positions, err)
	}
	tooWide := makeDirectTableForCountLaw(counts)
	tooWide[keyspace.FamilyBody] = make([]positionAnchor, counts[keyspace.FamilyBody]+1)
	if positions, err := emitPositions(counts, tooWide, 0); err == nil || positions != nil {
		t.Fatalf("emitPositions oversized table = %#v/%v, want nil/error", positions, err)
	}
	if got, err := anchorClosure(nil, counts, -1, makeDirectTableForCountLaw(counts)); err == nil || got != 0 {
		t.Fatalf("anchorClosure negative denominator = %d/%v, want zero/error", got, err)
	}
}

func makeDirectTableForCountLaw(counts [keyspace.FamilyCount]uint32) (direct directTable) {
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		direct[family] = make([]positionAnchor, counts[family])
	}
	return direct
}

func authoredInputForPositionCountLaw(body, values, integer keyspace.Term) authored.Input {
	return authored.Input{
		Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{integer}},
		Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values}}},
	}
}
