package indexread

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

type fakeFlow struct {
	bounds     map[cfg.SymbolID][2]int64
	lenBounds  map[string][2]int64
	lenRefs    map[cfg.SymbolID]lenRef
	keyOf      bool
	readback   typ.Type
	readbackOK bool
	readTarget constraint.Path
	readKey    constraint.Path
}

type lenRef struct {
	path   constraint.Path
	offset int64
}

func (f fakeFlow) NumericBoundsAt(_ cfg.Point, sym cfg.SymbolID) (int64, int64, bool) {
	if f.bounds == nil {
		return 0, 0, false
	}
	b, ok := f.bounds[sym]
	return b[0], b[1], ok
}

func (f fakeFlow) ArrayLenRefPathAt(_ cfg.Point, sym cfg.SymbolID) (constraint.Path, int64, bool) {
	if f.lenRefs == nil {
		return constraint.Path{}, 0, false
	}
	ref, ok := f.lenRefs[sym]
	return ref.path, ref.offset, ok
}

func (f fakeFlow) LengthBoundsAt(_ cfg.Point, path constraint.Path) (int64, int64, bool) {
	if f.lenBounds == nil {
		return 0, 0, false
	}
	b, ok := f.lenBounds[string(path.Key())]
	return b[0], b[1], ok
}

func (f fakeFlow) HasKeyOf(_ cfg.Point, tablePath, keyPath constraint.Path) bool {
	return f.keyOf && !tablePath.IsEmpty() && !keyPath.IsEmpty()
}

func (f fakeFlow) IndexReadPointFacts(cfg.Point, flow.PathReadView) flow.PointFacts {
	if !f.readbackOK || typ.IsAbsentOrUnknown(f.readback) || f.readTarget.IsEmpty() || f.readKey.IsEmpty() {
		return flow.PointFactsOf(flow.PointState{})
	}
	target, targetOK := flow.StableAddressOfPath(f.readTarget)
	key, keyOK := flow.StableAddressOfPath(f.readKey)
	if !targetOK || !keyOK {
		return flow.PointFactsOf(flow.PointState{})
	}
	return flow.PointFactsOf(flow.PointState{
		IndexWrites: flow.IndexWriteAdmissionFacts{}.WithAddress(flow.IndexWriteAdmissionAddressFact{
			Target:     target,
			KeyPath:    key,
			HasKeyPath: true,
			Key:        product.FromType(typ.String),
			Value:      product.FromType(f.readback),
		}),
	})
}

func TestContextUsesLiteralStringKeyType(t *testing.T) {
	index, ok := Context(ContextQuery{
		Object:  &ast.IdentExpr{Value: "t"},
		Key:     &ast.StringExpr{Value: "foo"},
		KeyType: typ.String,
		PathOf: func(expr ast.Expr) constraint.Path {
			if _, ok := expr.(*ast.IdentExpr); ok {
				return constraint.Path{Root: "t", Symbol: 10}
			}
			return constraint.Path{}
		},
	})
	if !ok {
		t.Fatal("Context did not recognize literal string index")
	}
	if !typ.TypeEquals(index.KeyType, typ.LiteralString("foo")) {
		t.Fatalf("Context KeyType = %v, want literal foo", index.KeyType)
	}
}

func TestRefine_TupleIndexBoundedByNumericForRemovesNil(t *testing.T) {
	obj := &ast.IdentExpr{Value: "values"}
	key := &ast.IdentExpr{Value: "i"}
	keyPath := constraint.Path{Root: "i", Symbol: 11}
	result := typ.NewOptional(typ.NewUnion(typ.LiteralInt(10), typ.LiteralInt(20), typ.LiteralInt(30)))

	refined, ok := Refine(Query{
		Point:     7,
		Container: typ.NewTuple(typ.LiteralInt(10), typ.LiteralInt(20), typ.LiteralInt(30)),
		Result:    result,
		Object:    obj,
		Key:       key,
		Flow:      fakeFlow{bounds: map[cfg.SymbolID][2]int64{11: {1, 3}}},
		PathOf: func(expr ast.Expr) constraint.Path {
			if expr == key {
				return keyPath
			}
			return constraint.Path{}
		},
	})

	want := typ.NewUnion(typ.LiteralInt(10), typ.LiteralInt(20), typ.LiteralInt(30))
	if !ok || !typ.TypeEquals(refined, want) {
		t.Fatalf("Refine(tuple[i]) = %v, %v; want %v, true", refined, ok, want)
	}
}

func TestRefine_TupleIndexOutOfRangeKeepsNil(t *testing.T) {
	result := typ.NewOptional(typ.Number)
	key := &ast.IdentExpr{Value: "i"}
	keyPath := constraint.Path{Root: "i", Symbol: 12}

	refined, ok := Refine(Query{
		Point:     7,
		Container: typ.NewTuple(typ.Number, typ.Number, typ.Number),
		Result:    result,
		Key:       key,
		Flow:      fakeFlow{bounds: map[cfg.SymbolID][2]int64{12: {1, 4}}},
		PathOf: func(expr ast.Expr) constraint.Path {
			if expr == key {
				return keyPath
			}
			return constraint.Path{}
		},
	})

	if ok || refined != nil {
		t.Fatalf("Refine(tuple[i]) = %v, %v; want no refinement", refined, ok)
	}
}

func TestRefine_NumericBoundsRequireSymbolPath(t *testing.T) {
	result := typ.NewOptional(typ.Number)

	refined, ok := Refine(Query{
		Point:     7,
		Container: typ.NewTuple(typ.Number),
		Result:    result,
		Key:       &ast.IdentExpr{Value: "i"},
		Flow:      fakeFlow{bounds: map[cfg.SymbolID][2]int64{12: {1, 1}}},
	})

	if ok || refined != nil {
		t.Fatalf("Refine(tuple[i] without symbol path) = %v, %v; want no refinement", refined, ok)
	}
}

func TestRefine_LengthRelationUsesSymbolPath(t *testing.T) {
	obj := &ast.IdentExpr{Value: "rows"}
	key := &ast.IdentExpr{Value: "i"}
	objPath := constraint.Path{Root: "rows", Symbol: 31}
	keyPath := constraint.Path{Root: "i", Symbol: 32}

	refined, ok := Refine(Query{
		Point:     9,
		Container: typ.NewArray(typ.String),
		Result:    typ.NewOptional(typ.String),
		Object:    obj,
		Key:       key,
		Flow: fakeFlow{
			bounds:  map[cfg.SymbolID][2]int64{32: {1, 3}},
			lenRefs: map[cfg.SymbolID]lenRef{32: {path: objPath}},
		},
		PathOf: func(expr ast.Expr) constraint.Path {
			switch expr {
			case obj:
				return objPath
			case key:
				return keyPath
			default:
				return constraint.Path{}
			}
		},
	})

	if !ok || !typ.TypeEquals(refined, typ.String) {
		t.Fatalf("Refine(rows[i] with length relation) = %v, %v; want string,true", refined, ok)
	}
}

func TestRefine_LengthBoundedLiteralIndexRemovesNil(t *testing.T) {
	obj := &ast.IdentExpr{Value: "rows"}
	objPath := constraint.Path{Root: "rows", Symbol: 41}
	result := typ.NewOptional(typ.String)

	refined, ok := Refine(Query{
		Point:     9,
		Container: typ.NewArray(typ.String),
		Result:    result,
		Object:    obj,
		Key:       &ast.NumberExpr{Value: "1"},
		Flow:      fakeFlow{lenBounds: map[string][2]int64{string(objPath.Key()): {1, 1}}},
		PathOf: func(expr ast.Expr) constraint.Path {
			if expr == obj {
				return objPath
			}
			return constraint.Path{}
		},
	})

	if !ok || !typ.TypeEquals(refined, typ.String) {
		t.Fatalf("Refine(rows[1]) = %v, %v; want string, true", refined, ok)
	}
}

func TestRefine_KeyPresenceRemovesNil(t *testing.T) {
	obj := &ast.IdentExpr{Value: "map"}
	key := &ast.IdentExpr{Value: "name"}
	objPath := constraint.Path{Root: "map", Symbol: 51}
	keyPath := constraint.Path{Root: "name", Symbol: 52}

	refined, ok := Refine(Query{
		Point:     5,
		Container: typ.NewMap(typ.String, typ.Number),
		Result:    typ.NewOptional(typ.Number),
		Object:    obj,
		Key:       key,
		Flow:      fakeFlow{keyOf: true},
		PathOf: func(expr ast.Expr) constraint.Path {
			switch expr {
			case obj:
				return objPath
			case key:
				return keyPath
			default:
				return constraint.Path{}
			}
		},
	})

	if !ok || !typ.TypeEquals(refined, typ.Number) {
		t.Fatalf("Refine(map[name]) = %v, %v; want number, true", refined, ok)
	}
}

func TestRefine_IndexReadbackComposesKeyPresence(t *testing.T) {
	obj := &ast.IdentExpr{Value: "map"}
	key := &ast.IdentExpr{Value: "name"}
	objPath := constraint.Path{Root: "map", Symbol: 61}
	keyPath := constraint.Path{Root: "name", Symbol: 62}

	refined, ok := Refine(Query{
		Point:     6,
		Container: typ.NewMap(typ.String, typ.Number),
		Result:    typ.NewOptional(typ.Number),
		Object:    obj,
		Key:       key,
		KeyType:   typ.String,
		Flow: fakeFlow{
			keyOf:      true,
			readback:   typ.NewOptional(typ.Number),
			readbackOK: true,
			readTarget: objPath,
			readKey:    keyPath,
		},
		PathOf: func(expr ast.Expr) constraint.Path {
			switch expr {
			case obj:
				return objPath
			case key:
				return keyPath
			default:
				return constraint.Path{}
			}
		},
	})

	if !ok || !typ.TypeEquals(refined, typ.Number) {
		t.Fatalf("Refine(map[name]) = %v, %v; want present number readback", refined, ok)
	}
}
