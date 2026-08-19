package source

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// repeatFixture adds one direct Loop occurrence owned by Body 1. Body 2 is
// its owner-local forest child; Body 3 is nested below Body 2 so the tests can
// distinguish a child-tail frontier from a non-child candidate. The exact
// Loop kind and Loop-to-child choice are intentionally not represented here:
// those are Flow position-seal obligations.
func repeatFixture() (Input, IndexInput) {
	input, index := sourceFixture(1)
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)

	input.Families[int(keyspace.FamilyBody)-1].Spans = append(
		input.Families[int(keyspace.FamilyBody)-1].Spans,
		Span{File: input.Name, StartLine: 3, StartCol: 1, EndLine: 3, EndCol: 1},
	)
	input.Families[int(keyspace.FamilyLoop)-1].Spans = []Span{{
		File: input.Name, StartLine: 4, StartCol: 1, EndLine: 4, EndCol: 1,
	}}

	// Body 3 is a direct authored child of Body 2. This supplies a valid
	// non-child candidate for the Repeat frontier checks below.
	input.Bodies[1].Terms = append(input.Bodies[1].Terms, body3)
	input.Bodies = append(input.Bodies, BodySource{Body: body3})
	body3Offset := uint32(len(input.Bodies[1].Terms) - 1)
	body3Cursor := uint32(len(input.Bodies[1].Terms) - 1)
	appendCanonicalFixturePosition(&index, Position{
		Term: body3, Root: body3, Body: body2,
		Offset: body3Offset, Cursor: body3Cursor,
		FrontierBody: body2, FrontierCursor: body3Cursor,
	})

	// Loop is the final direct source occurrence in Body 1. Its Repeat
	// frontier points at Body 2's complete root tail.
	input.Bodies[0].Terms = append(input.Bodies[0].Terms, loop)
	loopOffset := uint32(len(input.Bodies[0].Terms) - 1)
	loopCursor := uint32(len(input.Bodies[0].Terms) - 1)
	appendCanonicalFixturePosition(&index, Position{
		Term: loop, Root: loop, Body: body1,
		Offset: loopOffset, Cursor: loopCursor,
		FrontierBody: body2, FrontierCursor: uint32(len(input.Bodies[1].Terms)),
		Repeat: true,
	})
	return input, index
}

func TestSourceRepeatValidatesBodyChildTailGeometry(t *testing.T) {
	input, index := repeatFixture()
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	if body, offset, cursor, ok := component.View().Index().Position(loop); !ok ||
		body != body1 || offset != 1 || cursor != 1 {
		t.Fatalf("Repeat Position = %v/%d/%d/%v", body, offset, cursor, ok)
	}
	if root, ok := component.View().Index().Root(loop); !ok || root != loop {
		t.Fatalf("Repeat Root = %v/%v", root, ok)
	}
	if body, cursor, ok := component.View().Index().Frontier(loop); !ok || body != body2 || cursor != 2 {
		t.Fatalf("Repeat Frontier = %v/%d/%v, want Body2 tail 2", body, cursor, ok)
	}
}

func TestSourceRepeatDescendantInheritsRootFrontier(t *testing.T) {
	input, index := repeatFixture()
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	if !appendFixturePositionTerm(&index, cell, loop) {
		t.Fatal("repeat fixture lost Loop position root")
	}
	row := fixturePosition(&index, cell)
	row.Repeat = false
	replaceFixturePosition(&index, row)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := commitSource(draft, index); err != nil {
		t.Fatalf("Finalize inherited Repeat frontier: %v", err)
	}
}

func TestSourceRepeatDescendantRejectsOrdinaryFrontier(t *testing.T) {
	input, index := repeatFixture()
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	if !appendFixturePositionTerm(&index, cell, loop) {
		t.Fatal("repeat fixture lost Loop position root")
	}
	row := fixturePosition(&index, cell)
	row.Repeat = false
	row.FrontierBody = row.Body
	row.FrontierCursor = row.Cursor
	replaceFixturePosition(&index, row)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("Finalize accepted descendant with forged ordinary frontier")
	}
}

func TestSourceRepeatRejectsInvalidOwnerLocalGeometry(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*IndexInput)
	}{
		{
			name: "ordinary mismatch",
			mutate: func(index *IndexInput) {
				row := fixturePosition(index, keyspace.MakeTerm(keyspace.FamilyLoop, 1))
				row.Repeat = false
				replaceFixturePosition(index, row)
			},
		},
		{
			name: "non-loop direct root family",
			mutate: func(index *IndexInput) {
				cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
				if !appendFixturePositionTerm(index, cell, keyspace.MakeTerm(keyspace.FamilyBind, 1)) {
					panic("repeat fixture lost Bind root")
				}
				row := fixturePosition(index, cell)
				row.Repeat = true
				row.FrontierBody = keyspace.MakeTerm(keyspace.FamilyBody, 2)
				row.FrontierCursor = 2
				replaceFixturePosition(index, row)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input, index := repeatFixture()
			test.mutate(&index)
			draft, err := Build(input)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if _, err := commitSource(draft, index); err == nil {
				t.Fatal("Finalize accepted invalid Repeat geometry")
			}
		})
	}
}

func fixturePosition(index *IndexInput, term keyspace.Term) Position {
	if index == nil {
		return Position{}
	}
	for _, row := range index.Positions {
		if row.Term == term {
			return row
		}
	}
	return Position{}
}

func replaceFixturePosition(index *IndexInput, replacement Position) {
	if index == nil {
		return
	}
	for at := range index.Positions {
		if index.Positions[at].Term == replacement.Term {
			index.Positions[at] = replacement
			return
		}
	}
}
