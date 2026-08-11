package static

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

func effectRowsFixture() Input {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyFunction] = 3
	return Input{
		Counts:    counts,
		Contracts: ContractsInput{Function: []FunctionContract{{}, {}, {}}},
		EffectRows: EffectRowsInput{Rows: []EffectRow{
			{Function: keyspace.MakeTerm(keyspace.FamilyFunction, 3), Row: RowSpec{RowFormals: 3, Tail: RowVariable, Var: 2}},
			{Function: keyspace.MakeTerm(keyspace.FamilyFunction, 1), Row: RowSpec{Tail: RowClosed}},
		}},
	}
}

func TestEffectRowsPreserveSparsePresenceAndCanonicalOrder(t *testing.T) {
	component := staticContentComponent(t, effectRowsFixture())
	rows := component.View().EffectRows()
	if rows.Count() != 2 {
		t.Fatalf("sparse row count = %d, want 2", rows.Count())
	}
	functionOne := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	functionThree := keyspace.MakeTerm(keyspace.FamilyFunction, 3)
	if got, ok := rows.At(0); !ok || got != functionOne {
		t.Fatalf("first owner = %v/%v, want %v", got, ok, functionOne)
	}
	if got, ok := rows.At(1); !ok || got != functionThree {
		t.Fatalf("second owner = %v/%v, want %v", got, ok, functionThree)
	}
	if got, ok := rows.Get(functionOne); !ok || got.Tail != RowClosed || got.Var != 0 || len(got.Occurrences) != 0 {
		t.Fatalf("explicit closed row = %#v/%v", got, ok)
	}
	if got, ok := rows.Get(functionThree); !ok || got.RowFormals != 3 || got.Tail != RowVariable || got.Var != 2 {
		t.Fatalf("variable row = %#v/%v", got, ok)
	}
	if _, ok := rows.Get(keyspace.MakeTerm(keyspace.FamilyFunction, 2)); ok {
		t.Fatal("omitted Function row reported as present")
	}
	if count, ok := rows.OccurrenceCount(functionOne); !ok || count != 0 {
		t.Fatalf("closed row occurrence count = %d/%v", count, ok)
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_, _ = rows.Get(functionOne)
		_, _ = rows.Tail(functionThree)
		_, _ = rows.Variable(functionThree)
	}); allocations != 0 {
		t.Fatalf("effect row queries allocate %f", allocations)
	}
	closedFormals := effectRowsFixture()
	closedFormals.EffectRows.Rows[1].Row.RowFormals = 7
	closed := staticContentComponent(t, closedFormals).View().EffectRows()
	if got, ok := closed.Get(functionOne); !ok || got.RowFormals != 7 || got.Tail != RowClosed || got.Var != 0 {
		t.Fatalf("closed row lost declared formals = %#v/%v", got, ok)
	}
}

func TestEffectRowsRejectForeignDuplicateAndUnadmittedRows(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Input)
	}{
		{
			name: "foreign owner family",
			edit: func(input *Input) {
				input.EffectRows.Rows[0].Function = keyspace.MakeTerm(keyspace.FamilyCall, 1)
			},
		},
		{
			name: "duplicate owner",
			edit: func(input *Input) {
				input.EffectRows.Rows = append(input.EffectRows.Rows, EffectRow{Function: keyspace.MakeTerm(keyspace.FamilyFunction, 1), Row: RowSpec{Tail: RowClosed}})
			},
		},
		{
			name: "missing tail",
			edit: func(input *Input) { input.EffectRows.Rows[0].Row = RowSpec{} },
		},
		{
			name: "closed variable",
			edit: func(input *Input) { input.EffectRows.Rows[0].Row = RowSpec{Tail: RowClosed, Var: 1} },
		},
		{
			name: "variable zero denominator",
			edit: func(input *Input) {
				input.EffectRows.Rows[0].Row = RowSpec{Tail: RowVariable, Var: 0}
			},
		},
		{
			name: "variable out of range",
			edit: func(input *Input) {
				input.EffectRows.Rows[0].Row = RowSpec{RowFormals: 3, Tail: RowVariable, Var: 3}
			},
		},
		{
			name: "occurrence labels unavailable",
			edit: func(input *Input) {
				input.EffectRows.Rows[0].Row = RowSpec{Occurrences: []EffectOccurrence{{}}, Tail: RowClosed}
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := effectRowsFixture()
			test.edit(&input)
			if _, err := Build(input); err == nil {
				t.Fatal("Build accepted invalid EffectRows relation")
			}
		})
	}
}

func TestEffectRowsContentAndArtifactReplay(t *testing.T) {
	firstInput := effectRowsFixture()
	first := staticContentComponent(t, firstInput)
	firstID := first.Cold().ContentID()

	mutated := effectRowsFixture()
	mutated.EffectRows.Rows[0].Row.RowFormals = 4
	mutatedComponent := staticContentComponent(t, mutated)
	if second := mutatedComponent.Cold().ContentID(); second == firstID {
		t.Fatal("effect row mutation left ContentID unchanged")
	}

	encoded := encodeStaticArtifactComponent(t, first, false)
	mutatedEncoded := encodeStaticArtifactComponent(t, mutatedComponent, false)
	if bytes.Equal(mutatedEncoded, encoded) {
		t.Fatal("effect row formals mutation left artifact unchanged")
	}
	reader := newStaticArtifactReader(t, encoded)
	if _, err := reader.Record(); err != nil {
		t.Fatal(err)
	}
	decoded, err := ReadArtifactSection(reader)
	if err != nil {
		t.Fatalf("ReadArtifactSection: %v", err)
	}
	decoded.Counts = firstInput.Counts
	if len(decoded.EffectRows.Rows) != 2 || decoded.EffectRows.Rows[1].Row.RowFormals != 3 {
		t.Fatalf("decoded effect row formals = %#v", decoded.EffectRows.Rows)
	}
	rebuilt, err := Build(decoded)
	if err != nil {
		t.Fatalf("Build(decoded): %v", err)
	}
	rebuiltComponent, err := commitStaticDraft(t, rebuilt)
	if err != nil {
		t.Fatalf("commit replay: %v", err)
	}
	if rebuiltComponent.Cold().ContentID() != firstID {
		t.Fatal("effect row artifact replay changed ContentID")
	}
}
