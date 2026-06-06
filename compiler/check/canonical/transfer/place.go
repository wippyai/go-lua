package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/place"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

type Place = place.Place
type PlaceStep = place.Step
type PlaceStepKind = place.StepKind

const (
	PlaceStepStaticMember = place.StepStaticMember
	PlaceStepDynamicIndex = place.StepDynamicIndex
)

type placeValueUpdater func(product.AbstractValue) (product.AbstractValue, bool)

type finalPlaceWriter func(product.AbstractValue, PlaceStep, product.AbstractValue) (product.AbstractValue, bool)

type staticAccess struct {
	Root  *ast.IdentExpr
	Steps []PlaceStep
}

func staticAccessOfExpr(expr ast.Expr) (staticAccess, bool) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return staticAccess{Root: e}, true
	case *ast.AttrGetExpr:
		base, ok := staticAccessOfExpr(e.Object)
		if !ok {
			return staticAccess{}, false
		}
		member, ok := staticMemberKey(e)
		if !ok {
			return staticAccess{}, false
		}
		base.Steps = append(base.Steps, PlaceStep{Kind: PlaceStepStaticMember, Member: member})
		return base, true
	default:
		return staticAccess{}, false
	}
}

func (a staticAccess) place(sym cfg.SymbolID) (Place, bool) {
	if a.Root == nil || sym == 0 {
		return Place{}, false
	}
	steps := append([]PlaceStep(nil), a.Steps...)
	return Place{Root: sym, RootName: a.Root.Value, Steps: steps}, true
}

func (a staticAccess) segments() ([]constraint.Segment, bool) {
	if len(a.Steps) == 0 {
		return nil, true
	}
	segs := make([]constraint.Segment, 0, len(a.Steps))
	for _, step := range a.Steps {
		seg, ok := place.SegmentFromStep(step)
		if !ok {
			return nil, false
		}
		segs = append(segs, seg)
	}
	return segs, true
}

// placeOfExpr lowers an expression that denotes a storage location into the
// canonical Place IR. Dynamic index segments carry the already-evaluated abstract
// key value, so later read/write code consumes product operations without
// re-pattern-matching syntax.
func (t *Transfer) placeOfExpr(
	out *flow.PointState,
	expr ast.Expr,
	demand func(int, paramevidence.ParamContract),
) (Place, bool) {
	return t.placeOfExprWithConst(out, expr, demand, nil)
}

func (t *Transfer) placeOfExprAt(
	out *flow.PointState,
	p cfg.Point,
	expr ast.Expr,
	demand func(int, paramevidence.ParamContract),
) (Place, bool) {
	return t.placeOfExprWithConst(out, expr, demand, t.constResolverAt(p))
}

func (t *Transfer) placeOfExprWithConst(
	out *flow.PointState,
	expr ast.Expr,
	demand func(int, paramevidence.ParamContract),
	constResolver func(string) *flow.ConstValue,
) (Place, bool) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		sym := t.symbolOf(e)
		if sym == 0 {
			return Place{}, false
		}
		return Place{Root: sym, RootName: e.Value}, true
	case *ast.AttrGetExpr:
		base, ok := t.placeOfExprWithConst(out, e.Object, demand, constResolver)
		if !ok {
			return Place{}, false
		}
		if member, isStatic := staticMemberKeyWithConst(e, constResolver); isStatic {
			base.Steps = append(base.Steps, PlaceStep{Kind: PlaceStepStaticMember, Member: member})
			return base, true
		}
		key, ok := t.evalExpr(out, e.Key, demand)
		if !ok || key.IsZero() {
			return Place{}, false
		}
		base.Steps = append(base.Steps, PlaceStep{Kind: PlaceStepDynamicIndex, Key: key})
		return base, true
	default:
		return Place{}, false
	}
}

// staticPlaceOfExpr lowers only statically-addressable expressions into Place:
// identifiers and dot/static-index member chains. It is the common path/key
// projection source for non-mutating transfer facts such as numeric length,
// static members, key provenance, and function identity paths.
func (t *Transfer) staticPlaceOfExpr(expr ast.Expr) (Place, bool) {
	access, ok := staticAccessOfExpr(expr)
	if !ok {
		return Place{}, false
	}
	return access.place(t.symbolOf(access.Root))
}

func (t *Transfer) staticPathOfExpr(expr ast.Expr) (constraint.Path, bool) {
	place, ok := t.staticPlaceOfExpr(expr)
	if !ok {
		return constraint.Path{}, false
	}
	return place.StaticPath()
}

func staticPathOfExprWithRootSymbol(expr ast.Expr, sym cfg.SymbolID) (constraint.Path, bool) {
	access, ok := staticAccessOfExpr(expr)
	if !ok {
		return constraint.Path{}, false
	}
	place, ok := access.place(sym)
	if !ok {
		return constraint.Path{}, false
	}
	return place.StaticPath()
}

func symbolPathKey(p Place) (constraint.PathKey, bool) {
	path, ok := p.StaticPath()
	if !ok || path.Symbol == 0 {
		return "", false
	}
	return flow.SymbolPathKeyOf(path)
}

func (t *Transfer) staticRootSymbolOfExpr(expr ast.Expr) (cfg.SymbolID, bool) {
	access, ok := staticAccessOfExpr(expr)
	if !ok || access.Root == nil {
		return 0, false
	}
	sym := t.symbolOf(access.Root)
	return sym, sym != 0
}

func staticSegmentsOfExpr(expr ast.Expr) ([]constraint.Segment, bool) {
	access, ok := staticAccessOfExpr(expr)
	if !ok || len(access.Steps) == 0 {
		return nil, false
	}
	return access.segments()
}

// pathSymbol resolves an identifier or static field/index path to its base
// symbol and structural segments. It is the shared read-side path projection for
// branch narrowing, parameter effects, type casts, and value-origin demand.
func (t *Transfer) pathSymbol(expr ast.Expr) (cfg.SymbolID, []constraint.Segment, bool) {
	path, ok := t.staticPathOfExpr(expr)
	if !ok || path.Symbol == 0 {
		return 0, nil, false
	}
	return path.Symbol, append([]constraint.Segment(nil), path.Segments...), true
}

// pathSymbolInState resolves an expression to a static symbol path after first
// lowering through Place. Dynamic index steps whose key is a proven literal are
// normalized to static segments by Place.StaticPath, so all consumers share one
// path identity instead of pattern-matching constant-key syntax locally.
func (t *Transfer) pathSymbolInState(
	out *flow.PointState,
	expr ast.Expr,
	demand func(int, paramevidence.ParamContract),
) (cfg.SymbolID, []constraint.Segment, bool) {
	return t.pathSymbolInStateWithPoint(out, 0, expr, demand, false)
}

func (t *Transfer) pathSymbolInStateAt(
	out *flow.PointState,
	p cfg.Point,
	expr ast.Expr,
	demand func(int, paramevidence.ParamContract),
) (cfg.SymbolID, []constraint.Segment, bool) {
	return t.pathSymbolInStateWithPoint(out, p, expr, demand, true)
}

func (t *Transfer) pathSymbolInStateWithPoint(
	out *flow.PointState,
	p cfg.Point,
	expr ast.Expr,
	demand func(int, paramevidence.ParamContract),
	usePoint bool,
) (cfg.SymbolID, []constraint.Segment, bool) {
	if out != nil {
		var place Place
		var ok bool
		if usePoint {
			place, ok = t.placeOfExprAt(out, p, expr, demand)
		} else {
			place, ok = t.placeOfExpr(out, expr, demand)
		}
		if ok {
			if path, ok := place.StaticPath(); ok && path.Symbol != 0 {
				return path.Symbol, append([]constraint.Segment(nil), path.Segments...), true
			}
		}
	}
	return t.pathSymbol(expr)
}

// updatePlace applies update at p and returns the rebuilt root value. The caller
// writes the returned root through writeRootContainer so Env/Cells remain the
// only stores.
func (t *Transfer) updatePlace(
	out *flow.PointState,
	p Place,
	update placeValueUpdater,
) (product.AbstractValue, bool, bool) {
	return t.rewritePlace(out, p, func(root product.AbstractValue) (product.AbstractValue, bool) {
		return t.updateValueAtPlace(root, p.Steps, update)
	})
}

// assignPlaceValue writes val at p and returns the rebuilt root value. Static
// members are structural replacements. Dynamic indexes are replacements at the
// final step and element mutations at intermediate steps, which is the lvalue
// distinction `x[k] = v` vs `x[k].f = v`.
func (t *Transfer) assignPlaceValue(
	out *flow.PointState,
	p Place,
	val product.AbstractValue,
	finalDynamic finalPlaceWriter,
) (product.AbstractValue, bool, bool) {
	if val.IsZero() {
		return product.AbstractValue{}, false, false
	}
	return t.rewritePlace(out, p, func(root product.AbstractValue) (product.AbstractValue, bool) {
		return t.assignValueAtPlace(root, p.Steps, val, finalDynamic)
	})
}

func (t *Transfer) rewritePlace(
	out *flow.PointState,
	p Place,
	rewrite func(product.AbstractValue) (product.AbstractValue, bool),
) (product.AbstractValue, bool, bool) {
	if out == nil || p.Root == 0 || rewrite == nil {
		return product.AbstractValue{}, false, false
	}
	root, had, targetsCell := t.rootContainerValue(out, p.Root)
	if targetsCell && had && valueIsBottom(root) {
		root = product.FromType(typ.NewRecord().SetOpen(true).Build())
	}
	if !had || root.IsZero() {
		return product.AbstractValue{}, false, false
	}
	updated, ok := rewrite(root)
	if !ok || updated.IsZero() {
		return product.AbstractValue{}, false, false
	}
	return updated, targetsCell, true
}

func (t *Transfer) updateValueAtPlace(
	base product.AbstractValue,
	steps []PlaceStep,
	update placeValueUpdater,
) (product.AbstractValue, bool) {
	if len(steps) == 0 {
		return update(base)
	}
	step := steps[0]
	child, ok := t.readPlaceStep(base, step)
	if !ok || child.IsZero() {
		child = product.FromType(typ.NewRecord().Build())
	} else {
		child = product.NarrowPresent(child)
	}
	updatedChild, ok := t.updateValueAtPlace(child, steps[1:], update)
	if !ok || updatedChild.IsZero() {
		return product.AbstractValue{}, false
	}
	return t.writePlaceStep(base, step, updatedChild)
}

func (t *Transfer) assignValueAtPlace(
	base product.AbstractValue,
	steps []PlaceStep,
	val product.AbstractValue,
	finalDynamic finalPlaceWriter,
) (product.AbstractValue, bool) {
	if len(steps) == 0 {
		return val, true
	}
	step := steps[0]
	if len(steps) == 1 {
		return t.writeFinalPlaceStep(base, step, val, finalDynamic)
	}
	child, ok := t.readPlaceStep(base, step)
	if !ok || child.IsZero() {
		child = product.FromType(typ.NewRecord().Build())
	} else {
		child = product.NarrowPresent(child)
	}
	updatedChild, ok := t.assignValueAtPlace(child, steps[1:], val, finalDynamic)
	if !ok || updatedChild.IsZero() {
		return product.AbstractValue{}, false
	}
	return t.writePlaceStep(base, step, updatedChild)
}

func (t *Transfer) readPlaceStep(base product.AbstractValue, step PlaceStep) (product.AbstractValue, bool) {
	if base.IsZero() {
		return product.AbstractValue{}, false
	}
	switch step.Kind {
	case PlaceStepStaticMember:
		return product.MemberOf(base, step.Member)
	case PlaceStepDynamicIndex:
		if step.Key.IsZero() {
			return product.AbstractValue{}, false
		}
		return product.IndexOf(base, step.Key)
	default:
		return product.AbstractValue{}, false
	}
}

func (t *Transfer) writePlaceStep(
	base product.AbstractValue,
	step PlaceStep,
	child product.AbstractValue,
) (product.AbstractValue, bool) {
	if base.IsZero() || child.IsZero() {
		return product.AbstractValue{}, false
	}
	switch step.Kind {
	case PlaceStepStaticMember:
		return product.WithMember(base, step.Member, child), true
	case PlaceStepDynamicIndex:
		if step.Key.IsZero() {
			return product.AbstractValue{}, false
		}
		return product.MutateIndex(base, step.Key, child), true
	default:
		return product.AbstractValue{}, false
	}
}

func (t *Transfer) writeFinalPlaceStep(
	base product.AbstractValue,
	step PlaceStep,
	val product.AbstractValue,
	finalDynamic finalPlaceWriter,
) (product.AbstractValue, bool) {
	if base.IsZero() || val.IsZero() {
		return product.AbstractValue{}, false
	}
	switch step.Kind {
	case PlaceStepStaticMember:
		return product.WithMember(base, step.Member, val), true
	case PlaceStepDynamicIndex:
		if step.Key.IsZero() {
			return product.AbstractValue{}, false
		}
		if finalDynamic != nil {
			return finalDynamic(base, step, val)
		}
		return product.WriteIndexForeign(base, step.Key, val), true
	default:
		return product.AbstractValue{}, false
	}
}

func (t *Transfer) placeOfAssignTarget(
	out *flow.PointState,
	target cfg.AssignTarget,
	base product.AbstractValue,
	demand func(int, paramevidence.ParamContract),
) (Place, bool) {
	if target.Expr != nil {
		if p, ok := t.placeOfExpr(out, target.Expr, demand); ok && p.Root != 0 && len(p.Steps) > 0 {
			return p, true
		}
	}
	if target.Base != nil && target.Key != nil {
		attr := &ast.AttrGetExpr{Object: target.Base, Key: target.Key, KeySyntax: ast.AttrKeyIndex}
		if p, ok := t.placeOfExpr(out, attr, demand); ok && p.Root != 0 && len(p.Steps) > 0 {
			return p, true
		}
	}
	if target.BaseSymbol == 0 {
		return Place{}, false
	}
	p := Place{Root: target.BaseSymbol, RootName: target.BaseName}
	for _, name := range target.FieldPath {
		if name == "" {
			continue
		}
		p.Steps = append(p.Steps, PlaceStep{Kind: PlaceStepStaticMember, Member: value.MemberField(name)})
	}
	if target.Kind == cfg.TargetIndex {
		step, ok := t.placeStepForIndexTarget(out, target, base, demand)
		if !ok {
			return Place{}, false
		}
		p.Steps = append(p.Steps, step)
	}
	return p, len(p.Steps) > 0
}

// invalidationPlaceOfAssignTarget returns the largest static write footprint the
// target can affect. It is intentionally weaker than placeOfAssignTarget: when a
// dynamic index key cannot be resolved exactly, the static container prefix is
// still enough to kill stale product facts under that subtree.
func (t *Transfer) invalidationPlaceOfAssignTarget(target cfg.AssignTarget) (Place, bool) {
	if target.Expr != nil {
		if path, ok := t.containerExprPath(target.Expr); ok && path.Symbol != 0 {
			return place.FromStaticPath(path)
		}
		if attr, ok := target.Expr.(*ast.AttrGetExpr); ok {
			path, ok := t.containerExprPath(attr.Object)
			if !ok || path.Symbol == 0 {
				return Place{}, false
			}
			if member, isStatic := staticMemberKey(attr); isStatic {
				if seg, ok := place.SegmentFromMemberKey(member); ok {
					path.Segments = append(path.Segments, seg)
				}
			}
			return place.FromStaticPath(path)
		}
	}
	if target.Base != nil {
		path, ok := t.containerExprPath(target.Base)
		if !ok || path.Symbol == 0 {
			return Place{}, false
		}
		if target.Kind == cfg.TargetIndex {
			if seg, ok := staticIndexSegment(target.Key); ok {
				path.Segments = append(path.Segments, seg)
			}
		}
		return place.FromStaticPath(path)
	}
	if target.BaseSymbol == 0 {
		return Place{}, false
	}
	path := constraint.NewPath(target.BaseSymbol, target.BaseName)
	path.Segments = append(path.Segments, fieldSegments(target.FieldPath)...)
	if target.Kind == cfg.TargetIndex {
		if seg, ok := staticIndexSegment(target.Key); ok {
			path.Segments = append(path.Segments, seg)
		}
	}
	return place.FromStaticPath(path)
}

func (t *Transfer) placeStepForIndexTarget(
	out *flow.PointState,
	target cfg.AssignTarget,
	base product.AbstractValue,
	demand func(int, paramevidence.ParamContract),
) (PlaceStep, bool) {
	if member, isStatic := staticIndexMemberKey(target.Key); isStatic {
		return PlaceStep{Kind: PlaceStepStaticMember, Member: member}, true
	}
	key, ok := t.evalExpr(out, target.Key, demand)
	if !ok || key.IsZero() {
		key = t.dynamicWriteKey(out, target, base, demand)
	}
	if key.IsZero() {
		return PlaceStep{}, false
	}
	return PlaceStep{Kind: PlaceStepDynamicIndex, Key: key}, true
}

func (t *Transfer) staticPathOfAssignTarget(target cfg.AssignTarget) (constraint.Path, bool) {
	if target.Expr != nil {
		if path, ok := t.staticPathOfExpr(target.Expr); ok && !path.IsEmpty() {
			return path, true
		}
	}
	if target.Base != nil {
		path, ok := t.staticPathOfExpr(target.Base)
		if !ok || path.Symbol == 0 {
			return constraint.Path{}, false
		}
		if target.Kind == cfg.TargetIndex {
			if seg, ok := staticIndexSegment(target.Key); ok {
				path.Segments = append(path.Segments, seg)
			}
		}
		return path, true
	}
	switch target.Kind {
	case cfg.TargetIdent:
		if target.Symbol == 0 {
			return constraint.Path{}, false
		}
		return constraint.NewPath(target.Symbol, target.Name), true
	case cfg.TargetField:
		if target.BaseSymbol == 0 || len(target.FieldPath) == 0 {
			return constraint.Path{}, false
		}
		path := constraint.NewPath(target.BaseSymbol, target.BaseName)
		path.Segments = append(path.Segments, fieldSegments(target.FieldPath)...)
		return path, true
	case cfg.TargetIndex:
		if target.BaseSymbol == 0 {
			return constraint.Path{}, false
		}
		path := constraint.NewPath(target.BaseSymbol, target.BaseName)
		path.Segments = append(path.Segments, fieldSegments(target.FieldPath)...)
		if seg, ok := staticIndexSegment(target.Key); ok {
			path.Segments = append(path.Segments, seg)
		}
		return path, true
	default:
		return constraint.Path{}, false
	}
}

func (t *Transfer) staticContainerPathOfAssignTarget(target cfg.AssignTarget) (constraint.Path, bool) {
	if target.Base != nil {
		path, ok := t.staticPathOfExpr(target.Base)
		if !ok || path.Symbol == 0 {
			return constraint.Path{}, false
		}
		return path, true
	}
	if target.Expr != nil {
		if attr, ok := target.Expr.(*ast.AttrGetExpr); ok {
			path, ok := t.staticPathOfExpr(attr.Object)
			if ok && path.Symbol != 0 {
				return path, true
			}
		}
	}
	if target.BaseSymbol == 0 {
		return constraint.Path{}, false
	}
	path := constraint.NewPath(target.BaseSymbol, target.BaseName)
	path.Segments = append(path.Segments, fieldSegments(target.FieldPath)...)
	return path, true
}

func staticPlace(sym cfg.SymbolID, segments []constraint.Segment) (Place, bool) {
	return place.FromSymbolPath(sym, segments)
}
