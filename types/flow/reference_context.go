package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/typ"
)

// ReferenceContext is the callee-entry view of caller-owned reference state:
// captured cell values plus function and closure identity paths. These axes are
// projected, overlaid, and keyed together because closure entry and call entry
// semantics depend on their correlation.
type ReferenceContext struct {
	cells        CaptureCells
	functionRefs FunctionRefs
	closureRefs  ClosureRefs
}

// ReferenceContextKey is the comparable cache key for a normalized reference
// context. It preserves axis correlation at cache boundaries without exposing
// independent key fields to callers.
type ReferenceContextKey struct {
	cells        CaptureCellsKey
	functionRefs FunctionRefsKey
	closureRefs  ClosureRefsKey
}

// ReferenceContextOf constructs a canonical reference context from independent
// axes. The map-shaped axes are cloned so callers cannot mutate a stored context.
func ReferenceContextOf(cells CaptureCells, functionRefs FunctionRefs, closureRefs ClosureRefs) ReferenceContext {
	return ReferenceContext{
		cells:        CaptureCellsDomain.Join(cells, CaptureCellsDomain.Bottom()),
		functionRefs: FunctionRefsDomain.Join(functionRefs, FunctionRefsDomain.Bottom()),
		closureRefs:  ClosureRefsDomain.Join(closureRefs, ClosureRefsDomain.Bottom()),
	}
}

// ReferenceContextBottom returns the empty finite reference context.
func ReferenceContextBottom() ReferenceContext {
	return ReferenceContextOf(CaptureCellsDomain.Bottom(), FunctionRefsDomain.Bottom(), ClosureRefsDomain.Bottom())
}

// ReferenceContextTop returns the greatest reference context.
func ReferenceContextTop() ReferenceContext {
	return ReferenceContextOf(CaptureCellsDomain.Top(), FunctionRefsDomain.Top(), ClosureRefsDomain.Top())
}

// ReferenceContextDomain is the product lattice for correlated lexical/reference
// context. Callers use it when a boundary owns all reference axes together.
var ReferenceContextDomain = lattice.Lattice[ReferenceContext]{
	Bottom: ReferenceContextBottom,
	Top:    ReferenceContextTop,
	Equal: func(a, b ReferenceContext) bool {
		return CaptureCellsDomain.Equal(a.CaptureCells(), b.CaptureCells()) &&
			FunctionRefsDomain.Equal(a.FunctionRefs(), b.FunctionRefs()) &&
			ClosureRefsDomain.Equal(a.ClosureRefs(), b.ClosureRefs())
	},
	LessOrEq: func(a, b ReferenceContext) bool {
		return CaptureCellsDomain.LessOrEq(a.CaptureCells(), b.CaptureCells()) &&
			FunctionRefsDomain.LessOrEq(a.FunctionRefs(), b.FunctionRefs()) &&
			ClosureRefsDomain.LessOrEq(a.ClosureRefs(), b.ClosureRefs())
	},
	Join: func(a, b ReferenceContext) ReferenceContext {
		return ReferenceContextOf(
			CaptureCellsDomain.Join(a.CaptureCells(), b.CaptureCells()),
			FunctionRefsDomain.Join(a.FunctionRefs(), b.FunctionRefs()),
			ClosureRefsDomain.Join(a.ClosureRefs(), b.ClosureRefs()),
		)
	},
	Meet: nil,
	Widen: func(prev, next ReferenceContext) ReferenceContext {
		return ReferenceContextOf(
			CaptureCellsDomain.Widen(prev.CaptureCells(), next.CaptureCells()),
			FunctionRefsDomain.Widen(prev.FunctionRefs(), next.FunctionRefs()),
			ClosureRefsDomain.Widen(prev.ClosureRefs(), next.ClosureRefs()),
		)
	},
}

// ReferenceContextKeyOf constructs the comparable key for a reference context.
func ReferenceContextKeyOf(c ReferenceContext) ReferenceContextKey {
	return ReferenceContextKey{
		cells:        c.CaptureCells().Key(),
		functionRefs: FunctionRefsKeyOf(c.FunctionRefs()),
		closureRefs:  ClosureRefsKeyOf(c.ClosureRefs()),
	}
}

// ReferenceContextFromPoint extracts the live reference context from a point
// state. Nil points represent the empty caller context.
func ReferenceContextFromPoint(point *PointState) ReferenceContext {
	if point == nil {
		return ReferenceContextOf(CaptureCellsDomain.Bottom(), FunctionRefsDomain.Bottom(), ClosureRefsDomain.Bottom())
	}
	return ReferenceContextOf(point.Cells, point.FunctionRefs, point.ClosureRefs)
}

// ReferenceContextWithStaticMembersFromPoint extracts the live reference store
// and folds point-local static-member facts into captured root products. Use
// this when a reference context crosses a function/closure boundary; the
// StaticMembers axis is point-local, but captured cell snapshots must carry the
// proven child facts they expose to the callee.
func ReferenceContextWithStaticMembersFromPoint(point *PointState) ReferenceContext {
	if point == nil {
		return ReferenceContextOf(CaptureCellsDomain.Bottom(), FunctionRefsDomain.Bottom(), ClosureRefsDomain.Bottom())
	}
	return ReferenceContextOf(point.Cells.WithStaticMembers(point.StaticMembers), point.FunctionRefs, point.ClosureRefs)
}

// MergeReferenceContextWithFixed combines caller-provided fixed reference
// context with a fallback context discovered from the callee/call site. Fixed
// entries dominate equal roots; missing roots inherit fallback evidence. Capture
// cell values keep the more precise product value when the fallback refines the
// fixed seed.
func MergeReferenceContextWithFixed(fixed, fallback ReferenceContext) ReferenceContext {
	return ReferenceContextOf(
		mergeCaptureCellsWithFixed(fixed.CaptureCells(), fallback.CaptureCells()),
		mergeFunctionRefsWithFixed(fixed.FunctionRefs(), fallback.FunctionRefs()),
		mergeClosureRefsWithFixed(fixed.ClosureRefs(), fallback.ClosureRefs()),
	)
}

// CaptureCells returns the captured-cell axis.
func (c ReferenceContext) CaptureCells() CaptureCells {
	return CaptureCellsDomain.Join(c.cells, CaptureCellsDomain.Bottom())
}

// FunctionRefs returns the function-identity axis.
func (c ReferenceContext) FunctionRefs() FunctionRefs {
	return FunctionRefsDomain.Join(c.functionRefs, FunctionRefsDomain.Bottom())
}

// ClosureRefs returns the closure-identity axis.
func (c ReferenceContext) ClosureRefs() ClosureRefs {
	return ClosureRefsDomain.Join(c.closureRefs, ClosureRefsDomain.Bottom())
}

// CaptureCells returns the captured-cell key axis.
func (k ReferenceContextKey) CaptureCells() CaptureCells {
	return k.cells.Cells()
}

// FunctionRefs returns the function-identity key axis.
func (k ReferenceContextKey) FunctionRefs() FunctionRefs {
	return k.functionRefs.Refs()
}

// ClosureRefs returns the closure-identity key axis.
func (k ReferenceContextKey) ClosureRefs() ClosureRefs {
	return k.closureRefs.Refs()
}

// Context reconstructs the normalized reference context represented by this key.
func (k ReferenceContextKey) Context() ReferenceContext {
	return ReferenceContextOf(k.CaptureCells(), k.FunctionRefs(), k.ClosureRefs())
}

// Join adds independent reference evidence from other into c. This is the
// ordinary lattice join for call-entry evidence; use OverlayReferenceContext for
// mutable closure-entry snapshots where live locations override older captures.
func (c ReferenceContext) Join(other ReferenceContext) ReferenceContext {
	return ReferenceContextOf(
		CaptureCellsDomain.Join(c.cells, other.cells),
		FunctionRefsDomain.Join(c.functionRefs, other.functionRefs),
		ClosureRefsDomain.Join(c.closureRefs, other.closureRefs),
	)
}

// HasCallablePath reports whether path has finite function or closure identity
// evidence. It is the identity-only existence query; signature projection and
// value refinement live in PointFacts because they also need product values.
func (c ReferenceContext) HasCallablePath(path constraint.Path) bool {
	if path.IsEmpty() {
		return false
	}
	if set, ok := FunctionRefAtPath(c.FunctionRefs(), path); ok && len(set.Refs()) > 0 {
		return true
	}
	if set, ok := ClosureRefAtPath(c.ClosureRefs(), path); ok && len(set.Refs()) > 0 {
		return true
	}
	return false
}

func mergeCaptureCellsWithFixed(fixed, fallback CaptureCells) CaptureCells {
	if fixed.IsTop() || fallback.IsTop() {
		if fixed.IsTop() {
			return fixed
		}
		return fallback
	}
	if !fixed.HasFiniteEntries() {
		return CaptureCellsDomain.Join(fallback, CaptureCellsDomain.Bottom())
	}
	out := fixed
	for _, entry := range fallback.entries {
		if current, ok := out.Value(entry.Symbol); ok {
			out = out.With(entry.Symbol, mergeCaptureCellValue(current, entry.Value))
			continue
		}
		out = out.With(entry.Symbol, entry.Value)
	}
	return CaptureCellsDomain.Join(out, CaptureCellsDomain.Bottom())
}

func mergeCaptureCellValue(fixed, fallback product.AbstractValue) product.AbstractValue {
	if fixed.IsZero() {
		return fallback
	}
	if fallback.IsZero() || product.Domain.Equal(fixed, fallback) {
		return fixed
	}
	fixedType := product.ProjectValueOrUnknown(fixed)
	fallbackType := product.ProjectValueOrUnknown(fallback)
	if typ.MorePrecise(fallbackType, fixedType) {
		return fallback
	}
	if typ.MorePrecise(fixedType, fallbackType) {
		return fixed
	}
	if fallback.Covers(fixed) {
		return fixed
	}
	if fixed.Covers(fallback) {
		return fallback
	}
	return fixed
}

func mergeFunctionRefsWithFixed(fixed, fallback FunctionRefs) FunctionRefs {
	if FunctionRefsDomain.Equal(fixed, FunctionRefsDomain.Top()) ||
		FunctionRefsDomain.Equal(fallback, FunctionRefsDomain.Top()) {
		if FunctionRefsDomain.Equal(fixed, FunctionRefsDomain.Top()) {
			return fixed
		}
		return fallback
	}
	if len(fixed) == 0 {
		return FunctionRefsDomain.Join(fallback, FunctionRefsDomain.Bottom())
	}
	out := FunctionRefsDomain.Join(fixed, FunctionRefsDomain.Bottom())
	for path, set := range fallback {
		if set.IsBottom() {
			continue
		}
		addr, ok := StableAddressFromCanonicalKey(path)
		if !ok {
			continue
		}
		if _, ok := FunctionRefAtAddress(out, addr); ok {
			continue
		}
		out = WithFunctionRefAddress(out, addr, set)
	}
	return FunctionRefsDomain.Join(out, FunctionRefsDomain.Bottom())
}

func mergeClosureRefsWithFixed(fixed, fallback ClosureRefs) ClosureRefs {
	if ClosureRefsDomain.Equal(fixed, ClosureRefsDomain.Top()) ||
		ClosureRefsDomain.Equal(fallback, ClosureRefsDomain.Top()) {
		if ClosureRefsDomain.Equal(fixed, ClosureRefsDomain.Top()) {
			return fixed
		}
		return fallback
	}
	if len(fixed) == 0 {
		return ClosureRefsDomain.Join(fallback, ClosureRefsDomain.Bottom())
	}
	out := ClosureRefsDomain.Join(fixed, ClosureRefsDomain.Bottom())
	for path, set := range fallback {
		if set.IsBottom() {
			continue
		}
		addr, ok := StableAddressFromCanonicalKey(path)
		if !ok {
			continue
		}
		if _, ok := ClosureRefAtAddress(out, addr); ok {
			continue
		}
		out = WithClosureRefAddress(out, addr, set)
	}
	return ClosureRefsDomain.Join(out, ClosureRefsDomain.Bottom())
}

// CallableIdentity returns only function and closure identity facts. Captured
// lexical cells have distinct value seeding policy at closure entry and should
// not be copied by identity-only projections.
func (c ReferenceContext) CallableIdentity() ReferenceContext {
	return ReferenceContextOf(CaptureCellsDomain.Bottom(), c.functionRefs, c.closureRefs)
}

// RootSymbols returns the finite symbol roots referenced by any axis.
func (c ReferenceContext) RootSymbols() []cfg.SymbolID {
	var symbols []cfg.SymbolID
	for _, entry := range c.CaptureCells().Entries() {
		if entry.Symbol != 0 {
			symbols = append(symbols, entry.Symbol)
		}
	}
	symbols = append(symbols, FunctionRefRootSymbols(c.FunctionRefs())...)
	symbols = append(symbols, ClosureRefRootSymbols(c.ClosureRefs())...)
	return compactSortedSymbols(symbols)
}

// RebaseCallablePaths moves callable identity facts under source to target.
// Captured cells are lexical storage, not callable identity paths, so this
// operation intentionally returns an empty cell axis.
func (c ReferenceContext) RebaseCallablePaths(source, target constraint.Path) ReferenceContext {
	if source.IsEmpty() || target.IsEmpty() {
		return ReferenceContextBottom()
	}
	return ReferenceContextOf(
		CaptureCellsDomain.Bottom(),
		RebaseFunctionRefsPath(c.functionRefs, source, target),
		RebaseClosureRefsPath(c.closureRefs, source, target),
	)
}

// JoinFunctionRefAddress additively publishes function identity at addr.
func (c ReferenceContext) JoinFunctionRefAddress(addr StableAddress, set FunctionRefSet) ReferenceContext {
	if set.IsBottom() {
		return c
	}
	refs := c.functionRefs
	if prev, ok := FunctionRefAtAddress(refs, addr); ok {
		set = FunctionRefSetDomain.Join(prev, set)
	}
	return ReferenceContextOf(c.cells, WithFunctionRefAddress(refs, addr, set), c.closureRefs)
}

// JoinFunctionRefPath additively publishes function identity at path.
func (c ReferenceContext) JoinFunctionRefPath(path constraint.Path, set FunctionRefSet) ReferenceContext {
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return c
	}
	return c.JoinFunctionRefAddress(addr, set)
}

// JoinClosureRefAddress additively publishes closure identity at addr.
func (c ReferenceContext) JoinClosureRefAddress(addr StableAddress, set ClosureRefSet) ReferenceContext {
	if set.IsBottom() {
		return c
	}
	refs := c.closureRefs
	if prev, ok := ClosureRefAtAddress(refs, addr); ok {
		set = ClosureRefSetDomain.Join(prev, set)
	}
	return ReferenceContextOf(c.cells, c.functionRefs, WithClosureRefAddress(refs, addr, set))
}

// JoinClosureRefPath additively publishes closure identity at path.
func (c ReferenceContext) JoinClosureRefPath(path constraint.Path, set ClosureRefSet) ReferenceContext {
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return c
	}
	return c.JoinClosureRefAddress(addr, set)
}

// ProjectPaths keeps only paths visible through projection on every reference
// axis.
func (c ReferenceContext) ProjectPaths(projection ReferencePathProjection) ReferenceContext {
	return ReferenceContextOf(
		c.cells.ProjectPaths(projection),
		ProjectFunctionRefsByReferencePaths(c.functionRefs, projection),
		ProjectClosureRefsByReferencePaths(c.closureRefs, projection),
	)
}

// ProjectSymbols keeps only captured symbols on every reference axis.
func (c ReferenceContext) ProjectSymbols(symbols []cfg.SymbolID) ReferenceContext {
	return ReferenceContextOf(
		c.cells.Project(symbols),
		ProjectFunctionRefsBySymbols(c.functionRefs, symbols),
		ProjectClosureRefsBySymbols(c.closureRefs, symbols),
	)
}

// OverlayReferenceContext returns a closure-entry reference context where live
// captured locations override allocation-time snapshots.
func OverlayReferenceContext(base, live ReferenceContext) ReferenceContext {
	return ReferenceContextOf(
		OverlayCaptureCells(base.cells, live.cells),
		OverlayFunctionRefs(base.functionRefs, live.functionRefs),
		OverlayClosureRefs(base.closureRefs, live.closureRefs),
	)
}
