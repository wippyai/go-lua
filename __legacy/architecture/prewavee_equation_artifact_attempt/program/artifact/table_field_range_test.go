package artifact_test

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/artifact"
)

// TestTableFieldRangeLaw proves that a sealed table exposes precisely its
// authored field relation. The fields exercise all constructor key forms, so
// the range—not a global-family scan—is the authority for source order.
func TestTableFieldRangeLaw(t *testing.T) {
	p := mustLower(t, "table-range.lua", `
local key = 4
local empty = {}
local value = { 1, name = 2, [3] = 4, [key] = 5 }
`)
	var table program.Term
	for index := 0; index < p.TableCount(); index++ {
		candidate, ok := p.TableAt(index)
		if !ok {
			t.Fatalf("TableAt(%d) is absent", index)
		}
		length, ok := p.TableFieldLen(candidate)
		if !ok {
			t.Fatalf("TableFieldLen(Table[%d]) is absent", index)
		}
		if length == 0 {
			if field, ok := p.TableFieldAtTable(candidate, 0); ok || field != 0 {
				t.Fatalf("empty TableFieldAtTable = %v/%v, want 0/false", field, ok)
			}
		}
		if length == 4 {
			table = candidate
		}
	}
	if table == 0 {
		t.Fatal("mixed-field table constructor is absent")
	}

	length, ok := p.TableFieldLen(table)
	if !ok || length != 4 {
		t.Fatalf("TableFieldLen = %d/%v, want 4/true", length, ok)
	}
	wantKinds := []program.FieldKind{
		program.FieldList,
		program.FieldName,
		program.FieldExact,
		program.FieldKey,
	}
	for index, wantKind := range wantKinds {
		field, ok := p.TableFieldAtTable(table, index)
		if !ok {
			t.Fatalf("TableFieldAtTable(%d) is absent", index)
		}
		parent, _, _, kind, _, ok := p.TableField(field)
		if !ok || parent != table || kind != wantKind {
			t.Fatalf("TableFieldAtTable(%d) = parent %v kind %v ok %v, want parent %v kind %v", index, parent, kind, ok, table, wantKind)
		}
	}

	entry, _ := p.Entry()
	for _, invalid := range []program.Term{0, entry} {
		if length, ok := p.TableFieldLen(invalid); ok || length != 0 {
			t.Fatalf("TableFieldLen(%v) = %d/%v, want 0/false", invalid, length, ok)
		}
		if field, ok := p.TableFieldAtTable(invalid, 0); ok || field != 0 {
			t.Fatalf("TableFieldAtTable(%v, 0) = %v/%v, want 0/false", invalid, field, ok)
		}
	}
	for _, index := range []int{-1, length} {
		if field, ok := p.TableFieldAtTable(table, index); ok || field != 0 {
			t.Fatalf("TableFieldAtTable(%d) = %v/%v, want 0/false", index, field, ok)
		}
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		for index := 0; index < length; index++ {
			_, _ = p.TableFieldAtTable(table, index)
		}
		_, _ = p.TableFieldLen(table)
	}); allocations != 0 {
		t.Fatalf("table field range access allocated %v times", allocations)
	}
}

// TestTableFieldRangeArtifactReplayLaw proves the sealed range is reconstructed
// from authored persistence, rather than serialized as a parallel projection.
func TestTableFieldRangeArtifactReplayLaw(t *testing.T) {
	contract := mustProfile(t)
	p := mustLower(t, "table-range-replay.lua", `
local key = 4
return { 1, name = 2, [3] = 4, [key] = 5 }
`)
	encoded, err := artifact.Encode(p, contract, artifact.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	replayed, metadata, err := artifact.Decode(encoded, contract)
	if err != nil {
		t.Fatal(err)
	}
	reencoded, err := artifact.Encode(replayed, contract, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, reencoded) {
		t.Fatal("artifact replay changed canonical bytes")
	}

	for index := 0; index < p.TableCount(); index++ {
		beforeTable, _ := p.TableAt(index)
		afterTable, _ := replayed.TableAt(index)
		beforeLength, beforeOK := p.TableFieldLen(beforeTable)
		afterLength, afterOK := replayed.TableFieldLen(afterTable)
		if beforeOK != afterOK || afterLength != beforeLength {
			t.Fatalf("TableFieldLen(Table[%d]) = %d/%v, want %d/%v", index, afterLength, afterOK, beforeLength, beforeOK)
		}
		for fieldIndex := 0; fieldIndex < beforeLength; fieldIndex++ {
			before, beforeOK := p.TableFieldAtTable(beforeTable, fieldIndex)
			after, afterOK := replayed.TableFieldAtTable(afterTable, fieldIndex)
			if beforeOK != afterOK || after != before {
				t.Fatalf("TableFieldAtTable(Table[%d], %d) = %v/%v, want %v/%v", index, fieldIndex, after, afterOK, before, beforeOK)
			}
		}
	}
}
