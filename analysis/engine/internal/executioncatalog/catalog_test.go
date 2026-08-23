package executioncatalog

import "testing"

func TestSealPacksDenseFlatHandleColumns(t *testing.T) {
	catalog, ok := Seal([]Draft{
		{Family: 2, Local: 3, Rule: 5, Member: 7, Candidate: 11, InputCount: 2, OutputCount: 1},
		{Family: 2, Local: 4, Rule: 5, Member: 8, Candidate: 12, InputCount: 0, OutputCount: 1},
	})
	if !ok || catalog == nil || catalog.Count() != 2 {
		t.Fatal("seal catalog")
	}
	first, ok := catalog.At(0)
	if !ok || first.FamilyOrdinal() != 2 || first.LocalOrdinal() != 3 || first.RuleOrdinal() != 5 || first.MemberOrdinal() != 7 || first.CandidateOrdinal() != 11 {
		t.Fatalf("first row = %+v/%t", first, ok)
	}
	inputs, ok := catalog.Inputs(first)
	if !ok || len(inputs) != 2 || inputs[0] != 0 || inputs[1] != 1 {
		t.Fatalf("first inputs = %v/%t", inputs, ok)
	}
	outputs, ok := catalog.Outputs(first)
	if !ok || len(outputs) != 1 || outputs[0] != 0 {
		t.Fatalf("first outputs = %v/%t", outputs, ok)
	}
	second, ok := catalog.At(1)
	if !ok {
		t.Fatal("second row")
	}
	inputs, ok = catalog.Inputs(second)
	if !ok || len(inputs) != 0 {
		t.Fatalf("second inputs = %v/%t", inputs, ok)
	}
	if _, ok := catalog.At(2); ok {
		t.Fatal("out of range row accepted")
	}
	if len(catalog.inputs) != 2 || len(catalog.outputs) != 2 {
		t.Fatalf("flat columns = inputs:%d outputs:%d", len(catalog.inputs), len(catalog.outputs))
	}
}

func TestSealRejectsNoRowsOnlyByAbsentReference(t *testing.T) {
	catalog, ok := Seal(nil)
	if !ok || catalog == nil || catalog.Count() != 0 {
		t.Fatal("empty sealed catalog")
	}
	if _, ok := catalog.At(0); ok {
		t.Fatal("empty catalog fabricated row")
	}
}

func TestSealRejectsDuplicateMemberAddress(t *testing.T) {
	if catalog, ok := Seal([]Draft{
		{Family: 0, Local: 0, Member: 4, OutputCount: 1},
		{Family: 0, Local: 1, Member: 4, OutputCount: 1},
	}); ok || catalog != nil {
		t.Fatal("duplicate member address accepted")
	}
}
