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
	envSym, ok := PointFactsOf(state).EnvSymbolValue(sym)
	if !ok || !typ.TypeEquals(envSym.ProjectValue(), typ.String) {
		t.Fatalf("EnvSymbolValue = %v/%v, want string/true", envSym.ProjectValue(), ok)
	}
	cell, ok := PointFactsOf(state).CellValue(sym)
	if !ok || !typ.TypeEquals(cell.ProjectValue(), typ.Number) {
		t.Fatalf("CellValue(symbol) = %v/%v, want number/true", cell.ProjectValue(), ok)
	}
}

func TestPointFactsEnvCaptureCellsProjectsAllowedEnvSymbolsOnly(t *testing.T) {
	const allowedSym = cfg.SymbolID(10)
	const blockedSym = cfg.SymbolID(11)
	const cellSym = cfg.SymbolID(12)
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(allowedSym): product.FromType(typ.String),
			SymbolValueKey(blockedSym): product.FromType(typ.Number),
			ReturnSlotValueKey(0):      product.FromType(typ.Boolean),
		},
		Cells: CaptureCellsDomain.Bottom().With(cellSym, product.FromType(typ.Boolean)),
	}

	var allowed cfgSymbolSet
	allowed.Add(allowedSym)
	allowed.Add(cellSym)
	cells := PointFactsOf(state).EnvCaptureCells(allowed)
	entries := cells.Entries()
	if len(entries) != 1 {
		t.Fatalf("EnvCaptureCells entries = %#v, want one allowed Env symbol", entries)
	}
	if entries[0].Symbol != allowedSym || !typ.TypeEquals(entries[0].Value.ProjectValue(), typ.String) {
		t.Fatalf("EnvCaptureCells entry = %#v, want allowed string symbol", entries[0])
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

func TestPointFactsReturnSlotValueAndStoredArity(t *testing.T) {
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			ReturnSlotValueKey(0): product.FromType(typ.String),
			ReturnSlotValueKey(2): product.FromType(typ.Number),
			SymbolValueKey(9):     product.FromType(typ.Boolean),
		},
	}

	facts := PointFactsOf(state)
	got, ok := facts.ReturnSlotValue(2)
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.Number) {
		t.Fatalf("ReturnSlotValue(2) = %v/%v, want number/true", got.ProjectValue(), ok)
	}
	if _, ok := facts.ReturnSlotValue(-1); ok {
		t.Fatalf("ReturnSlotValue(-1) reported present")
	}
	if got := facts.ReturnSlotStoredArity(); got != 3 {
		t.Fatalf("ReturnSlotStoredArity = %d, want 3", got)
	}
}

func TestSingleChangedValueKeyReportsEnvKey(t *testing.T) {
	key := ReturnSlotValueKey(1)
	before := PointState{Env: map[ValueKey]product.AbstractValue{key: product.FromType(typ.String)}}
	after := PointState{Env: map[ValueKey]product.AbstractValue{key: product.FromType(typ.Number)}}

	got, ok := SingleChangedValueKey(before, after)
	if !ok || got != key {
		t.Fatalf("SingleChangedValueKey(env) = %s/%v, want %s/true", got, ok, key)
	}
	slot, ok := SingleChangedValueSlot(before, after)
	if !ok {
		t.Fatalf("SingleChangedValueSlot(env) reported absent")
	}
	if got, ok := slot.Key(); !ok || got != key {
		t.Fatalf("SingleChangedValueSlot(env).Key = %s/%v, want %s/true", got, ok, key)
	}
}

func TestSingleChangedValueKeyReportsCellAsSymbolKey(t *testing.T) {
	const sym = cfg.SymbolID(13)
	before := PointState{Cells: CaptureCellsDomain.Bottom().With(sym, product.FromType(typ.String))}
	after := PointState{Cells: CaptureCellsDomain.Bottom().With(sym, product.FromType(typ.Number))}

	got, ok := SingleChangedValueKey(before, after)
	if !ok || got != SymbolValueKey(sym) {
		t.Fatalf("SingleChangedValueKey(cell) = %s/%v, want %s/true", got, ok, SymbolValueKey(sym))
	}
	slot, ok := SingleChangedValueSlot(before, after)
	if !ok {
		t.Fatalf("SingleChangedValueSlot(cell) reported absent")
	}
	if got, ok := slot.Symbol(); !ok || got != sym {
		t.Fatalf("SingleChangedValueSlot(cell).Symbol = %d/%v, want %d/true", got, ok, sym)
	}
}

func TestSingleChangedValueKeyRejectsMultipleChanges(t *testing.T) {
	before := PointState{}
	after := PointState{
		Env: map[ValueKey]product.AbstractValue{
			ReturnSlotValueKey(1): product.FromType(typ.String),
			ReturnSlotValueKey(2): product.FromType(typ.Number),
		},
	}

	if got, ok := SingleChangedValueKey(before, after); ok || got != "" {
		t.Fatalf("SingleChangedValueKey(multiple) = %s/%v, want empty/false", got, ok)
	}
}

func TestPointFactsPathValueUsesStaticMemberFactsForMissingRootMember(t *testing.T) {
	const sym = cfg.SymbolID(11)
	path := constraint.NewPath(sym, "entry").Field("meta").Field("id")
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): product.FromType(typ.NewRecord().Build()),
		},
		StaticMembers: StaticMemberFactsDomain.Top().
			WithAddress(testStableAddressKey(t, SymbolPathKey(sym, path.Segments)), product.FromType(typ.Number)),
	}

	got, ok := PointFactsOf(state).PathType(path)
	if !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("PathType(static fact fill) = %v/%v, want number/true", got, ok)
	}
}

func TestPointFactsPathValuePreservesRootProductEvidenceWithStaticMember(t *testing.T) {
	const sym = cfg.SymbolID(12)
	path := constraint.NewPath(sym, "graph").Field("edges")
	live := typ.NewMap(typ.String, typ.NewRecord().Field("targets", typ.NewFreshArray()).Build())
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): product.FromType(typ.NewRecord().Field("edges", live).Build()),
		},
		StaticMembers: StaticMemberFactsDomain.Top().
			WithAddress(testStableAddressKey(t, SymbolPathKey(sym, path.Segments)), product.FromType(typ.NewRecord().Build())),
	}

	got, ok := PointFactsOf(state).PathValue(path)
	if !ok {
		t.Fatal("PathValue(graph.edges) did not resolve")
	}
	if field, ok := product.IndexOf(got, product.FromType(typ.String)); !ok || typ.IsAbsentOrUnknown(field.ProjectValue()) {
		t.Fatalf("PathValue(graph.edges) = %v, want map evidence preserved", got.ProjectValue())
	}
	if gotType, ok := PointFactsOf(state).PathType(path); !ok || !typ.TypeEquals(gotType, live) {
		t.Fatalf("PathType(graph.edges) = %v/%v, want %v", gotType, ok, live)
	}
}

func TestPointFactsReadPathTypeUsesRootProductTraversal(t *testing.T) {
	const sym = cfg.SymbolID(11)
	path := constraint.NewPath(sym, "graph").Field("static_data_sources")
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): product.FromType(typ.NewRecord().
				Field("static_data_sources", typ.NewArray(typ.NewRecord().
					Field("routes", typ.NewFreshArray()).
					Build())).
				Build()),
		},
	}

	got := PointFactsOf(state).ReadPathType(path, PointReadPolicy{})
	if got.State != StateResolved || !typ.TypeEquals(got.Type, typ.NewArray(typ.NewRecord().
		Field("routes", typ.NewFreshArray()).
		Build())) {
		t.Fatalf("ReadPathType(root product member) = %#v, want structured array", got)
	}
}

func TestPointFactsAddressValueUsesPathReadLaw(t *testing.T) {
	const sym = cfg.SymbolID(13)
	path := constraint.NewPath(sym, "entry").Field("id")
	want := product.FromType(typ.String)
	addr := testStableAddressKey(t, SymbolPathKey(sym, path.Segments))
	state := PointState{
		StaticMembers: StaticMemberFactsDomain.Top().WithAddress(addr, want),
	}

	got, ok := PointFactsOf(state).AddressValue(addr)
	if !ok || !product.Domain.Equal(got, want) {
		t.Fatalf("AddressValue(static fact) = %v/%v, want string/true", got, ok)
	}
}

func TestPointFactsPathValueStaticFactRefinesMapFallback(t *testing.T) {
	const sym = cfg.SymbolID(14)
	message := typ.NewRecord().Field("_topic", typ.String).Build()
	path := constraint.NewPath(sym, "messages").IndexStr("root")
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): product.FromType(typ.NewMap(typ.String, message)),
		},
		StaticMembers: StaticMemberFactsDomain.Top().
			WithAddress(testStableAddressKey(t, SymbolPathKey(sym, path.Segments)), product.FromType(message)),
	}

	got, ok := PointFactsOf(state).PathValue(path)
	if !ok || !got.DefinitelyPresent() || !typ.TypeEquals(got.ProjectValue(), message) {
		t.Fatalf("PathValue(static fact over map fallback) = %v/%v present=%v, want %v present", got.ProjectValue(), ok, got.DefinitelyPresent(), message)
	}
}

func TestPointFactsPathValuePresenceFactRefinesMapFallback(t *testing.T) {
	const sym = cfg.SymbolID(15)
	message := typ.NewRecord().Field("_topic", typ.String).Build()
	path := constraint.NewPath(sym, "messages").IndexStr("root")
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): product.FromType(typ.NewMap(typ.String, message)),
		},
		StaticMembers: StaticMemberFactsDomain.Top().
			WithAddress(testStableAddressKey(t, SymbolPathKey(sym, path.Segments)), product.PresentDynamic()),
	}

	got, ok := PointFactsOf(state).PathValue(path)
	if !ok || !got.DefinitelyPresent() || !typ.TypeEquals(got.ProjectValue(), message) {
		t.Fatalf("PathValue(presence fact over map fallback) = %v/%v present=%v, want %v present", got.ProjectValue(), ok, got.DefinitelyPresent(), message)
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
	if len(metaChildren) != 1 || !metaChildren[0].Path.Equal(id) || !typ.TypeEquals(metaChildren[0].Type, typ.String) {
		t.Fatalf("meta child facts = %#v, want id:string from root product", metaChildren)
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

func TestPointFactsReadPathValueProjectsUnannotatedGradualRoot(t *testing.T) {
	const sym = cfg.SymbolID(21)
	path := constraint.NewPath(sym, "payload").Field("id")

	got := PointFactsOf(PointState{}).ReadPathValue(path, PointReadPolicy{
		UnannotatedSymbol: func(candidate cfg.SymbolID) bool {
			return candidate == sym
		},
	})

	if got.State != StateResolved || got.Value.IsZero() || !got.Value.IsGradualTop() {
		t.Fatalf("ReadPathValue(unannotated member) = %#v, want resolved gradual-top value", got)
	}
	if !typ.IsAny(got.Value.ProjectValue()) {
		t.Fatalf("ReadPathValue(unannotated member) type = %v, want any", got.Value.ProjectValue())
	}
}

func TestPointFactsReadPathTypeRefinesStaticIndexWithLengthProof(t *testing.T) {
	const sym = cfg.SymbolID(22)
	root := constraint.NewPath(sym, "items")
	indexed := root.IndexInt(1)
	num := numeric.NewState()
	num.ApplyLenGeConst(SymbolPathKey(sym, nil), 1)
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): product.FromType(typ.NewArray(typ.String)),
		},
		Num: num,
	}

	got := PointFactsOf(state).ReadPathType(indexed, PointReadPolicy{})

	if got.State != StateResolved || !typ.TypeEquals(got.Type, typ.String) {
		t.Fatalf("ReadPathType(static in-bounds index) = %#v, want resolved string", got)
	}
}
