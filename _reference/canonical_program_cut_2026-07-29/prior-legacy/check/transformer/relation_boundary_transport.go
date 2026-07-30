package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/symbol"
)

type relationRootCarrier struct {
	shape Shape
	roots []relationStateRoot
}

type relationStateRoot struct {
	root Root
	slot key.Value
	path keyspace.Key
}

func sealRelationEnvironmentRoots(keys *keyspace.KeySpace, symbols []symbol.ID, mutable map[symbol.ID]struct{}) ([]relationEnvironmentRoot, error) {
	if keys == nil || !keys.Valid() {
		return nil, fmt.Errorf("transformer: ambient roots have no keyspace authority")
	}
	out := make([]relationEnvironmentRoot, 0, len(symbols))
	for _, id := range symbols {
		path := keys.FromPath(pathdom.NewPath(id, ""))
		if id == 0 || keys.FormatReadOnly(path) == "" {
			return nil, fmt.Errorf("transformer: ambient symbol %d has no structural keyspace spelling", id)
		}
		_, changes := mutable[id]
		out = append(out, relationEnvironmentRoot{symbol: id, slot: key.SymbolValue(id), path: path, mutable: changes})
	}
	return out, nil
}

func sealRelationRootCarrier(plan *operationplan.Plan, keys *keyspace.KeySpace, shape Shape) (relationRootCarrier, error) {
	return sealRelationRootCarrierWithAmbients(plan, keys, shape, nil)
}

func sealRelationRootCarrierWithAmbients(plan *operationplan.Plan, keys *keyspace.KeySpace, shape Shape, ambients []AmbientRoot) (relationRootCarrier, error) {
	if plan == nil || keys == nil || !keys.Valid() || !plan.BoundaryParamsValid() || !plan.BoundaryCapturesValid() || !plan.BoundaryGlobalsValid() {
		return relationRootCarrier{}, fmt.Errorf("transformer: relation root carrier has no sealed plan/keyspace authority")
	}
	if !validAmbientRoots(ambients) || len(ambients) != int(shape.Ambients) {
		return relationRootCarrier{}, fmt.Errorf("transformer: relation root carrier has a non-canonical ambient schema")
	}
	ambientSymbols := make([]symbol.ID, len(ambients))
	for index, root := range ambients {
		ambientSymbols[index] = root.Symbol
	}
	namespaces := []struct {
		kind    RootKind
		symbols []symbol.ID
	}{{RootParam, plan.BoundaryParams()}, {RootCapture, plan.BoundaryCaptures()}, {RootGlobal, plan.BoundaryGlobals()}, {RootAmbient, ambientSymbols}}
	if len(namespaces[0].symbols) != int(shape.Params) || len(namespaces[1].symbols) != int(shape.Captures) || len(namespaces[2].symbols) != int(shape.Globals) || len(namespaces[3].symbols) != int(shape.Ambients) {
		return relationRootCarrier{}, fmt.Errorf("transformer: relation root carrier differs from relation shape")
	}
	out := relationRootCarrier{shape: shape, roots: make([]relationStateRoot, 0, shape.InputCount())}
	for _, namespace := range namespaces {
		for index, sym := range namespace.symbols {
			path := keys.FromPath(pathdom.NewPath(sym, ""))
			if keys.FormatReadOnly(path) == "" {
				return relationRootCarrier{}, fmt.Errorf("transformer: relation root %d:%d has no structural keyspace spelling", namespace.kind, index)
			}
			out.roots = append(out.roots, relationStateRoot{root: Root{Kind: namespace.kind, Index: uint32(index)}, slot: key.SymbolValue(sym), path: path})
		}
	}
	return out, nil
}

func (c relationRootCarrier) valid(keys *keyspace.KeySpace) bool {
	if keys == nil || !keys.Valid() || len(c.roots) != c.shape.InputCount() {
		return false
	}
	for index, root := range c.roots {
		if !c.shape.validateInput(root.root) || c.shape.offset(root.root.Kind)+int(root.root.Index) != index || root.slot == 0 || keys.FormatReadOnly(root.path) == "" {
			return false
		}
	}
	return true
}

func (c relationRootCarrier) cursor(reg *axis.Registry, source state.State) (BindingCursor, error) {
	if reg == nil || len(c.roots) != c.shape.InputCount() {
		return BindingCursor{}, fmt.Errorf("transformer: relation root carrier cannot read entry world")
	}
	values := make([]product.Value, len(c.roots))
	paths := make([]pathdom.Path, len(c.roots))
	for index, root := range c.roots {
		values[index] = source.ReadValue(reg, root.slot)
		paths[index] = pathdom.NewPath(rootSymbol(root.slot), "")
	}
	return NewBindingCursor(c.shape, values, paths)
}

// pathCursor binds the immutable structural half of the root carrier without
// reading State values. Factor-native transactions resolve ValueTerms from
// guarded decision leaves and use this cursor only for PathTerms.
func (c relationRootCarrier) pathCursor() (BindingCursor, error) {
	if len(c.roots) != c.shape.InputCount() {
		return BindingCursor{}, fmt.Errorf("transformer: relation root carrier cannot bind structural paths")
	}
	values := make([]product.Value, len(c.roots))
	paths := make([]pathdom.Path, len(c.roots))
	for index, root := range c.roots {
		paths[index] = pathdom.NewPath(rootSymbol(root.slot), "")
	}
	return NewBindingCursor(c.shape, values, paths)
}

func (c relationRootCarrier) structuralPathRoot(id symbol.ID) (keyspace.Key, bool) {
	for _, root := range c.roots {
		if rootSymbol(root.slot) == id && root.path.Kind == keyspace.KindUnversionedSym && root.path.Segs == 0 {
			return root.path, true
		}
	}
	return keyspace.Key{}, false
}

func rootSymbol(slot key.Value) symbol.ID {
	symbol, _ := key.ParseSymbolValue(slot)
	return symbol
}
