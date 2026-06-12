package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeaccess"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/discriminant"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

func lowerSymbolTypes(bindings *bind.Result, graph cfg.Graph, result *semantics.Result, resolver *typeResolver) map[symbol.ID]typ.Type {
	if bindings == nil || graph == nil || result == nil {
		return nil
	}
	if resolver == nil {
		resolver = newTypeResolver(bindings)
	}
	out := make(map[symbol.ID]typ.Type)
	add := func(id symbol.ID, expr ast.TypeExpr) {
		if id == 0 || expr == nil {
			return
		}
		t, ok := resolver.Type(expr)
		if !ok {
			return
		}
		out[id] = t
	}
	if fn := result.Function(); fn != nil {
		for _, slot := range bindings.ParamSlots(fn) {
			add(slot.Symbol, slot.Type)
		}
	}
	for _, point := range graph.RPO() {
		fact, ok := result.LocalAssignment(point)
		if !ok || !fact.HasSymbol {
			continue
		}
		add(fact.Symbol, fact.Type)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (l *lowerer) typedPresenceRefinement(target path.Path, value presence.Value) factflow.ValueRefinement {
	refinement := l.presenceRefinement(value)
	if !presence.Equal(value, presence.Present()) {
		return refinement
	}
	staticValue, ok := l.pathStaticValue(target)
	if !ok {
		return refinement
	}
	return refinement.WithConstraint(l.registry, staticValue)
}

func (l *lowerer) pathStaticValue(target path.Path) (product.Value, bool) {
	if l == nil || l.registry == nil || target.Symbol == 0 {
		return product.Value{}, false
	}
	t, ok := l.symbolTypes[target.Symbol]
	if !ok {
		return product.Value{}, false
	}
	for _, seg := range target.Segments {
		t, ok = projectSegmentType(t, seg)
		if !ok {
			return product.Value{}, false
		}
	}
	if !unwrap.IsOptionalLike(t) {
		return product.Value{}, false
	}
	present := unwrap.Optional(t)
	if present == nil {
		return product.Value{}, false
	}
	return typevalue.FromType(l.registry, present), true
}

func projectSegmentType(t typ.Type, seg segment.Segment) (typ.Type, bool) {
	switch seg.Kind {
	case segment.SegmentField:
		return typeaccess.Field(t, seg.Name)
	case segment.SegmentIndexString:
		return typeaccess.RuntimeIndex(t, typ.LiteralString(seg.Name))
	case segment.SegmentIndexInt:
		return typeaccess.RuntimeIndex(t, typ.LiteralInt(int64(seg.Index)))
	default:
		return nil, false
	}
}

func (l *lowerer) rootPresenceRefinement(target path.Path, cond bool) (factflow.BranchRefinement, bool) {
	if target.Symbol == 0 || len(target.Segments) == 0 {
		return factflow.BranchRefinement{}, false
	}
	rootType, ok := l.symbolTypes[target.Symbol]
	if !ok {
		return factflow.BranchRefinement{}, false
	}
	narrowed, ok := narrowTypeByPathPresence(rootType, target.Segments, 0)
	if !ok || typ.SameNodeOrAcyclicEqual(rootType, narrowed) {
		return factflow.BranchRefinement{}, false
	}
	root := target
	root.Segments = nil
	value := factflow.NewValueConstraint(typevalue.FromType(l.registry, narrowed))
	if cond {
		return factflow.NewBranchRefinement(root, value, true, factflow.ValueRefinement{}, false), true
	}
	return factflow.NewBranchRefinement(root, factflow.ValueRefinement{}, false, value, true), true
}

func (l *lowerer) rootLiteralRefinement(target path.Path, lit typ.Type, cond bool) (factflow.BranchRefinement, bool) {
	if target.Symbol == 0 || len(target.Segments) == 0 {
		return factflow.BranchRefinement{}, false
	}
	rootType, ok := l.symbolTypes[target.Symbol]
	if !ok {
		return factflow.BranchRefinement{}, false
	}
	narrowed, ok := discriminant.NarrowByPathLiteral(rootType, target.Segments, lit)
	if !ok || typ.SameNodeOrAcyclicEqual(rootType, narrowed) {
		return factflow.BranchRefinement{}, false
	}
	root := target
	root.Segments = nil
	value := factflow.NewValueConstraint(typevalue.FromType(l.registry, narrowed))
	if cond {
		return factflow.NewBranchRefinement(root, value, true, factflow.ValueRefinement{}, false), true
	}
	return factflow.NewBranchRefinement(root, factflow.ValueRefinement{}, false, value, true), true
}

func (l *lowerer) literalBranchRefinement(target path.Path, kind branchcond.CheckKind, literal string) (factflow.BranchRefinement, bool) {
	if target.Symbol == 0 || len(target.Segments) == 0 {
		return factflow.BranchRefinement{}, false
	}
	rootType, ok := l.symbolTypes[target.Symbol]
	if !ok {
		return factflow.BranchRefinement{}, false
	}
	lit := typ.LiteralString(literal)
	matched, hasMatched := discriminant.NarrowByPathLiteral(rootType, target.Segments, lit)
	unmatched, hasUnmatched := discriminant.NarrowByPathLiteralNot(rootType, target.Segments, lit)
	if !hasMatched && !hasUnmatched {
		return factflow.BranchRefinement{}, false
	}
	root := target
	root.Segments = nil
	var trueValue factflow.ValueRefinement
	var hasTrue bool
	var falseValue factflow.ValueRefinement
	var hasFalse bool
	if kind == branchcond.CheckLiteralNot {
		matched, unmatched = unmatched, matched
		hasMatched, hasUnmatched = hasUnmatched, hasMatched
	}
	if hasMatched {
		trueValue = factflow.NewValueConstraint(typevalue.FromType(l.registry, matched))
		hasTrue = true
	}
	if hasUnmatched {
		falseValue = factflow.NewValueConstraint(typevalue.FromType(l.registry, unmatched))
		hasFalse = true
	}
	return factflow.NewBranchRefinement(root, trueValue, hasTrue, falseValue, hasFalse), true
}

func narrowTypeByPathPresence(t typ.Type, suffix []segment.Segment, depth int) (typ.Type, bool) {
	if t == nil || len(suffix) == 0 || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	switch v := unwrap.Annotated(typ.NormalizeNilType(t)).(type) {
	case *typ.Alias:
		return narrowTypeByPathPresence(v.UnaliasedTarget(), suffix, depth+1)
	case *typ.Optional:
		return narrowTypeByPathPresence(v.Inner, suffix, depth+1)
	case *typ.Union:
		out := make([]typ.Type, 0, len(v.Members))
		changed := false
		for _, member := range v.Members {
			if pathCanBePresent(member, suffix, depth+1) {
				out = append(out, member)
				continue
			}
			changed = true
		}
		if !changed {
			return t, false
		}
		if len(out) == 0 {
			return typ.Never, true
		}
		return normalize.UnionForEvidence(out...), true
	default:
		if pathCanBePresent(t, suffix, depth+1) {
			return t, false
		}
		return typ.Never, true
	}
}

func pathCanBePresent(t typ.Type, suffix []segment.Segment, depth int) bool {
	projected := t
	var ok bool
	for _, seg := range suffix {
		projected, ok = projectSegmentType(projected, seg)
		if !ok {
			return false
		}
	}
	return typeCanBePresent(projected, depth+1)
}

func typeCanBePresent(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	switch v := unwrap.Annotated(typ.NormalizeNilType(t)).(type) {
	case *typ.Alias:
		return typeCanBePresent(v.UnaliasedTarget(), depth+1)
	case *typ.Optional:
		return typeCanBePresent(v.Inner, depth+1)
	case *typ.Union:
		for _, member := range v.Members {
			if typeCanBePresent(member, depth+1) {
				return true
			}
		}
		return false
	default:
		return v.Kind() != kind.Nil && !typ.IsNever(v)
	}
}
