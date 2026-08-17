package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

func TestStaticCountRowsEnumeratesOwnGeneratedRelations(t *testing.T) {
	component := staticContentComponent(t, staticFixture(t))
	rows, err := CountRows(component.View())
	if err != nil {
		t.Fatalf("CountRows() error = %v", err)
	}
	if rows.Count() != 10 {
		t.Fatalf("Static CountRows count = %d, want 10 generated relations", rows.Count())
	}
	ids := denominator.GeneratedProgramStaticIDs()
	view := component.View()
	declarations, signatures, contracts, operators, operands := view.Declarations(), view.Signatures(), view.Contracts(), view.Operators(), view.Operands()
	primary := declarations.Aliases().Count() + declarations.Interfaces().Count() + declarations.TypeParams().Count() +
		view.Types().Primitives().Count() + view.Types().Literals().Count() + view.Types().Optionals().Count() +
		view.Types().Unions().Count() + view.Types().Intersections().Count() + view.Types().Generics().Count() +
		view.Types().Arrays().Count() + view.Types().Maps().Count() + view.Types().Records().Count() +
		view.References().Count() + signatures.TypeFunctions().Count() + signatures.Assertions().Count() +
		operators.TypeOfs().Count() + operators.KeyOfs().Count() + operators.IndexAccesses().Count() + operators.Conditionals().Count()
	callArguments := 0
	for index := 0; index < contracts.Calls().Count(); index++ {
		term, ok := contracts.Calls().At(index)
		if !ok {
			t.Fatalf("Calls.At(%d) failed", index)
		}
		count, ok := contracts.Calls().TypeArgumentCount(term)
		if !ok {
			t.Fatalf("TypeArgumentCount(%v) failed", term)
		}
		callArguments += count
	}
	want := []struct {
		name  string
		id    schema.EntryID
		value uint64
	}{
		{name: "program static", id: ids.ProgramStatic, value: uint64(primary)},
		{name: "function contract", id: ids.ProgramStaticFunctionContract, value: uint64(contracts.Functions().Count())},
		{name: "call arguments", id: ids.ProgramStaticCallTypeArguments, value: uint64(callArguments)},
		{name: "declared type", id: ids.ProgramStaticCellDeclaredType, value: uint64(declarations.DeclaredTypes().Count())},
		{name: "claim target", id: ids.ProgramStaticClaimTarget, value: uint64(operands.Claims().Count())},
		{name: "type value", id: ids.ProgramStaticTypeValueTarget, value: uint64(operands.TypeValues().Count())},
		{name: "typeof", id: ids.ProgramStaticTypeof, value: uint64(operators.TypeOfs().Count())},
		{name: "annotation", id: ids.ProgramStaticAnnotation, value: uint64(operands.Annotations().Count())},
		{name: "publication", id: ids.ProgramStaticPublication, value: uint64(view.Publications().Count())},
		{name: "type reference", id: ids.ProgramStaticTypeRef, value: uint64(view.References().Count())},
	}
	for _, id := range want {
		if !id.id.Available() {
			t.Fatalf("%s identity unavailable", id.name)
		}
		if got, ok := rows.Value(id.id); !ok || got != id.value {
			t.Fatalf("%s count = %d/%v, want %d", id.name, got, ok, id.value)
		}
	}
	if _, err := CountRows(View{}); err == nil {
		t.Fatal("CountRows accepted an unavailable View")
	}
}
