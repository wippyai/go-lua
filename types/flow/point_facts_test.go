package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPointFactsSymbolValuePrefersCellsOverEnv(t *testing.T) {
	const sym = cfg.SymbolID(7)
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): product.FromType(typ.String),
		},
		Cells: CaptureCellsDomain.Bottom().With(sym, product.FromType(typ.Number)),
	}

	got, ok := PointFactsOf(state).SymbolType(sym)
	if !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("SymbolType(cell-backed) = %v/%v, want number/true", got, ok)
	}
}

func TestPointFactsValueKeyValueUsesSymbolCellPrecedence(t *testing.T) {
	const sym = cfg.SymbolID(8)
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): product.FromType(typ.String),
		},
		Cells: CaptureCellsDomain.Bottom().With(sym, product.FromType(typ.Number)),
	}

	got, ok := PointFactsOf(state).ValueKeyValue(SymbolValueKey(sym))
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.Number) {
		t.Fatalf("ValueKeyValue(symbol) = %v/%v, want number/true", got.ProjectValue(), ok)
	}
}

func TestPointFactsPrimitiveEnvAndCellReadsStaySeparate(t *testing.T) {
	const sym = cfg.SymbolID(10)
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): product.FromType(typ.String),
		},
		Cells: CaptureCellsDomain.Bottom().With(sym, product.FromType(typ.Number)),
	}

	env, ok := PointFactsOf(state).EnvValue(SymbolValueKey(sym))
	if !ok || !typ.TypeEquals(env.ProjectValue(), typ.String) {
		t.Fatalf("EnvValue(symbol) = %v/%v, want string/true", env.ProjectValue(), ok)
	}
	cell, ok := PointFactsOf(state).CellValue(sym)
	if !ok || !typ.TypeEquals(cell.ProjectValue(), typ.Number) {
		t.Fatalf("CellValue(symbol) = %v/%v, want number/true", cell.ProjectValue(), ok)
	}
}

func TestPointFactsValueKeyValueReadsReturnSlots(t *testing.T) {
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			ReturnSlotValueKey(2): product.FromType(typ.Boolean),
		},
	}

	got, ok := PointFactsOf(state).ValueKeyValue(ReturnSlotValueKey(2))
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.Boolean) {
		t.Fatalf("ValueKeyValue(return slot) = %v/%v, want boolean/true", got.ProjectValue(), ok)
	}
}

func TestPointFactsPathValueUsesStaticMemberFactsBeforeRootTraversal(t *testing.T) {
	const sym = cfg.SymbolID(11)
	path := constraint.NewPath(sym, "entry").Field("meta").Field("id")
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): product.FromType(typ.NewRecord().
				Field("meta", typ.NewRecord().Field("id", typ.String).Build()).
				Build()),
		},
		StaticMembers: StaticMemberFactsDomain.Top().
			WithAddress(testStableAddressKey(t, SymbolPathKey(sym, path.Segments)), product.FromType(typ.Number)),
	}

	got, ok := PointFactsOf(state).PathType(path)
	if !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("PathType(static fact override) = %v/%v, want number/true", got, ok)
	}
}

func TestPointFactsChildPathFactsEnumeratesDirectMaterializedChildren(t *testing.T) {
	const sym = cfg.SymbolID(12)
	root := constraint.NewPath(sym, "entry")
	meta := root.Field("meta")
	id := meta.Field("id")
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): product.FromType(typ.NewRecord().
				Field("meta", typ.NewRecord().Field("id", typ.String).Build()).
				Field("name", typ.String).
				Build()),
		},
		StaticMembers: StaticMemberFactsDomain.Top().
			WithAddress(testStableAddressKey(t, SymbolPathKey(sym, id.Segments)), product.FromType(typ.Number)),
	}

	rootChildren := PointFactsOf(state).ChildPathFacts(root)
	if len(rootChildren) != 1 || !rootChildren[0].Path.Equal(meta) {
		t.Fatalf("root child facts = %#v, want only meta", rootChildren)
	}
	if rec, ok := rootChildren[0].Type.(*typ.Record); !ok || rec.GetField("id") == nil {
		t.Fatalf("root child type = %v, want record with id", rootChildren[0].Type)
	}

	metaChildren := PointFactsOf(state).ChildPathFacts(meta)
	if len(metaChildren) != 1 || !metaChildren[0].Path.Equal(id) || !typ.TypeEquals(metaChildren[0].Type, typ.Number) {
		t.Fatalf("meta child facts = %#v, want id:number", metaChildren)
	}
}

func TestPointFactsLengthLowerBoundUsesSymbolPathKey(t *testing.T) {
	const sym = cfg.SymbolID(19)
	path := constraint.NewPath(sym, "rows").Field("items")
	num := numeric.NewState()
	num.ApplyLenGeConst(SymbolPathKey(sym, path.Segments), 3)

	got, ok := PointFactsOf(PointState{Num: num}).LengthLowerBound(path)
	if !ok || got != 3 {
		t.Fatalf("LengthLowerBound(%s) = %d/%v, want 3/true", path, got, ok)
	}
}
