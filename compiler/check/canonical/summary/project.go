package summary

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/domain/functionsymbols"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	domainpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/domain/predicate"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Project reduces a solved intraprocedural FunctionState to the interprocedural
// Summary a caller consumes: the return tuple projected from the function's
// return points, paired with the parameter Contracts the body imposed.
//
// The Params half is the FunctionState.Contracts unchanged — it is already the
// caller-facing parameter obligation. The Returns half is assembled by joining,
// across every return node, the value of each returned expression read from that
// point's converged Env. A returned identifier contributes its Env value; a
// returned form whose value the intraprocedural transfer does not pin (a call
// result the transfer defers, a complex expression) contributes the value-domain
// Top in that slot, the sound over-approximation. Return arity is the widest
// return statement's expression count.
func Project(fs state.FunctionState, g *cfg.Graph) Summary {
	return projectWithDeclaredReturns(fs, g, nil)
}

// projectOptions carries analysis metadata that projection needs but does not
// own. It stays intentionally narrow: summary projection can classify whether a
// returned call is a finite local dependency without importing driver/program
// target-resolution code.
type projectOptions struct {
	DeclaredReturns           []typ.Type
	ReturnCallHasFiniteTarget func(*cfg.CallInfo) bool
}

// projectWithDeclaredReturns is Project plus annotation context from the
// function signature. The declared return tuple is not a second analysis: it is a
// caller-visible contract used only where the summary projection needs to know
// that a success-only `(value, nil)` body is allowed to prove an error-return
// relation for an optional value slot.
func projectWithDeclaredReturns(fs state.FunctionState, g *cfg.Graph, declaredReturns []typ.Type) Summary {
	return projectWithOptions(fs, g, projectOptions{DeclaredReturns: declaredReturns})
}

// projectWithOptions is Project with the complete projection metadata bundle.
func projectWithOptions(fs state.FunctionState, g *cfg.Graph, opts projectOptions) Summary {
	returns := projectReturns(fs, g, opts)
	return Summary{
		Returns:           returns,
		ReturnRefs:        projectReturnRefs(fs, g),
		Params:            cloneContracts(fs.Contracts),
		Relations:         projectReturnRelations(fs, g, returns, opts.DeclaredReturns),
		CellEffects:       projectCellEffects(fs, g),
		ReceiverEffects:   projectReceiverEffects(fs, g),
		BoundaryFacts:     projectBoundaryFacts(fs, g),
		CaptureReferences: projectCaptureReferences(fs, g),
		PrototypeSelf:     projectPrototypeSelf(fs, g),
	}
}

// projectCellEffects folds the caller-visible capture-cell effects at every
// normal function boundary. Return nodes include implicit fallthrough returns
// with zero expressions, so this intentionally does not filter by arity.
func projectCellEffects(fs state.FunctionState, g *cfg.Graph) flow.CaptureEffects {
	if g == nil {
		return flow.CaptureEffectsDomain.Top()
	}
	out := flow.CaptureEffectsDomain.Bottom()
	g.EachReturn(func(p cfg.Point, _ *cfg.ReturnInfo) {
		ps, ok := fs.Points[p]
		if !ok {
			return
		}
		out = flow.CaptureEffectsDomain.Join(out, ps.CellEffects)
	})
	return out
}

// projectReceiverEffects folds caller-visible runtime-argument effects at every
// normal function boundary. The effect carrier distinguishes identity from
// conditional writes, so unchanged entry arguments are not mistaken for writes.
func projectReceiverEffects(fs state.FunctionState, g *cfg.Graph) flow.ReceiverEffects {
	if g == nil {
		return flow.ReceiverEffectsDomain.Top()
	}
	out := flow.ReceiverEffectsDomain.Bottom()
	g.EachReturn(func(p cfg.Point, _ *cfg.ReturnInfo) {
		ps, ok := fs.Points[p]
		if !ok {
			return
		}
		out = flow.ReceiverEffectsDomain.Join(out, ps.ReceiverEffects)
	})
	return out
}

// projectCaptureReferences folds the lexical/reference store visible at normal
// function boundaries. It includes explicit captured cells, ordinary Env symbols
// this function declares for direct nested closures, and callable identity paths
// that live alongside those values. Unlike CellEffects, this is a store snapshot.
func projectCaptureReferences(fs state.FunctionState, g *cfg.Graph) flow.ReferenceContext {
	if g == nil {
		return flow.ReferenceContextDomain.Top()
	}
	envExports := functionsymbols.OwnedCapturedByNested(g)
	out := flow.ReferenceContextDomain.Bottom()
	g.EachReturn(func(p cfg.Point, _ *cfg.ReturnInfo) {
		ps, ok := fs.Points[p]
		if !ok {
			return
		}
		cells := flow.CaptureCellsDomain.Join(ps.Cells, flow.PointFactsOf(ps).EnvCaptureCells(envExports))
		cells = cells.WithStaticMembers(ps.StaticMembers)
		out = flow.ReferenceContextDomain.Join(out, flow.ReferenceContextOf(cells, ps.FunctionRefs, ps.ClosureRefs))
	})
	return out
}

// projectPrototypeSelf folds the solved receiver-self product relation at normal
// function boundaries. Source semantics that create or update the relation live
// in transfer; projection only carries the product-state component into Summary.
func projectPrototypeSelf(fs state.FunctionState, g *cfg.Graph) flow.PrototypeSelf {
	if g == nil {
		return flow.PrototypeSelfDomain.Top()
	}
	out := flow.PrototypeSelfDomain.Bottom()
	g.EachReturn(func(p cfg.Point, _ *cfg.ReturnInfo) {
		ps, ok := fs.Points[p]
		if !ok {
			return
		}
		out = flow.PrototypeSelfDomain.Join(out, ps.PrototypeSelf)
	})
	return out
}

// projectReturns folds the per-return-point Env into a single return tuple. For
// each return node, slot i takes the value of the i-th return expression; the
// tuple is the slotwise Join over all return points (a function with two return
// statements returns the least upper bound of both). Slots beyond a given
// statement's arity contribute Bottom for that statement.
func projectReturns(fs state.FunctionState, g *cfg.Graph, opts projectOptions) []product.AbstractValue {
	if g == nil {
		return nil
	}
	acc := returnTupleLattice{}
	var rows []returnTupleRow
	maxArity := 0
	g.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil || len(info.Exprs) == 0 {
			return
		}
		ps, ok := fs.Points[p]
		if !ok {
			// An unreached return point (no converged state) contributes nothing;
			// its slots stay Bottom until the point becomes reachable.
			return
		}
		stmt := returnTupleAt(ps, info, opts)
		rows = append(rows, returnTupleRow{info: info, tuple: stmt})
		if len(stmt) > maxArity {
			maxArity = len(stmt)
		}
	})
	var tuple []product.AbstractValue
	for _, row := range rows {
		tuple = acc.Join(tuple, materializeImplicitNilReturnSlots(row.info, row.tuple, maxArity))
	}
	return tuple
}

type returnTupleRow struct {
	info  *cfg.ReturnInfo
	tuple []product.AbstractValue
}

func projectReturnRefs(fs state.FunctionState, g *cfg.Graph) flow.ReturnRefs {
	if g == nil {
		return flow.ReturnRefsDomain.Bottom()
	}
	var tuple flow.ReturnRefs
	g.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil || len(info.Exprs) == 0 {
			return
		}
		ps, ok := fs.Points[p]
		if !ok {
			return
		}
		tuple = flow.ReturnRefsDomain.Join(tuple, returnRefsTupleAt(ps, info))
	})
	return tuple
}

func returnRefsTupleAt(ps flow.PointState, info *cfg.ReturnInfo) flow.ReturnRefs {
	out := make([]flow.ReturnRefSlot, len(info.Exprs))
	references := flow.ReferenceContextFromPoint(&ps).CallableIdentity()
	for i := range info.Exprs {
		placeholder := constraint.NewPlaceholder(i)
		out[i] = flow.ReturnRefSlotOfReferenceContext(references.ProjectPaths(flow.ReferencePathProjection{
			Subtrees: []constraint.Path{placeholder},
		}))
	}
	return flow.ReturnRefsOfSlots(out)
}

// returnTupleAt reads the values of one return statement's expressions from the
// converged point state ps. A returned identifier (ReturnInfo.Symbols[i] != 0)
// projects its Env value; otherwise or when identifier fallback is unavailable,
// the function falls back to the transfer-owned return-slot Env key. When both
// lookups miss, it projects value-domain Top, and later transfer-fidelity passes
// (call-return typing) can still refine.
func returnTupleAt(ps flow.PointState, info *cfg.ReturnInfo, opts projectOptions) []product.AbstractValue {
	arity := len(info.Exprs)
	if len(info.Exprs) == 1 && info.SourceCallAt(0) != nil {
		if stored := flow.PointFactsOf(ps).ReturnSlotStoredArity(); stored > arity {
			arity = stored
		}
	}
	out := make([]product.AbstractValue, arity)
	for i := range info.Exprs {
		out[i] = returnSlotValue(ps, info, i, opts)
	}
	for i := len(info.Exprs); i < arity; i++ {
		out[i] = returnSlotValue(ps, info, i, opts)
	}
	return out
}

func materializeImplicitNilReturnSlots(info *cfg.ReturnInfo, tuple []product.AbstractValue, arity int) []product.AbstractValue {
	if info == nil || arity <= len(tuple) {
		return tuple
	}
	out := make([]product.AbstractValue, arity)
	copy(out, tuple)
	for i := len(tuple); i < arity; i++ {
		if implicitReturnSlotIsNil(info.Exprs, i) {
			out[i] = product.FromType(typ.Nil)
		}
	}
	return out
}

func returnSlotValue(ps flow.PointState, info *cfg.ReturnInfo, i int, opts projectOptions) product.AbstractValue {
	if i < len(info.Symbols) && info.Symbols[i] != 0 {
		if av, ok := flow.PointFactsOf(ps).SymbolValue(info.Symbols[i]); ok && !av.IsZero() {
			return av
		}
	}
	// A non-identifier return (a literal, an arithmetic result, a call) carries
	// its value in the return-slot Env key written by applyReturn. This is a
	// summary-owned read of transfer assembly facts through the PointFacts
	// typed-key boundary.
	if av, ok := flow.PointFactsOf(ps).ReturnSlotValue(i); ok && !av.IsZero() {
		return av
	}
	if returnSlotHasFiniteCallTarget(ps, info, i, opts) {
		return product.Bottom()
	}
	return product.Domain.Top()
}

func returnSlotHasFiniteCallTarget(ps flow.PointState, info *cfg.ReturnInfo, i int, opts projectOptions) bool {
	call := info.SourceCallAt(i)
	if call == nil {
		return false
	}
	if opts.ReturnCallHasFiniteTarget != nil && opts.ReturnCallHasFiniteTarget(call) {
		return true
	}
	path, ok := returnCallTargetPath(call)
	if !ok {
		return false
	}
	return flow.ReferenceContextFromPoint(&ps).HasCallablePath(path)
}

func returnCallTargetPath(call *cfg.CallInfo) (constraint.Path, bool) {
	if call == nil || call.CalleePath.IsEmpty() || call.CalleePath.Symbol == 0 {
		return constraint.Path{}, false
	}
	path := call.CalleePath
	if call.Method != "" {
		path = path.Field(call.Method)
	}
	return path, true
}

// projectReturnRelations proves caller-visible return-slot relations from the
// solved return-point states. It lives inside Summary projection so recursive
// callees and callers see it through the same interprocedural fixed point.
func projectReturnRelations(fs state.FunctionState, g *cfg.Graph, returns []product.AbstractValue, declaredReturns []typ.Type) flow.ReturnRelations {
	return flow.MergeReturnRelationProofs(
		projectErrorReturnRelations(fs, g, returns, declaredReturns),
		flow.MergeReturnRelationProofs(
			projectForwardedReturnRelations(fs, g),
			projectGuardedReturnRelations(fs, g),
		),
	)
}

func projectErrorReturnRelations(fs state.FunctionState, g *cfg.Graph, returns []product.AbstractValue, declaredReturns []typ.Type) flow.ReturnRelations {
	if g == nil {
		return flow.ReturnRelationsDomain.Top()
	}
	const valueIdx, errorIdx = 0, 1
	proof := returnRelationProof{consistent: true}
	g.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if !proof.consistent || info == nil {
			return
		}
		ps, ok := fs.Points[p]
		if !ok {
			return
		}
		if len(info.Exprs) == 0 && info.Stmt == nil {
			return
		}
		if len(info.Exprs) == 1 && info.SourceCallAt(0) != nil {
			if ps.ReturnRel.HasErrorReturn(flow.ReturnCorrelation{ValueIndex: valueIdx, ErrorIndex: errorIdx}) {
				proof.sawSuccess = true
				proof.sawFailure = true
				return
			}
			proof.consistent = false
			return
		}
		valState, okVal := classifyReturnSlot(ps, info, valueIdx)
		errState, okErr := classifyReturnSlot(ps, info, errorIdx)
		if !okVal || !okErr {
			proof.consistent = false
			return
		}
		switch {
		case valState == returnNonNil && errState == returnNil:
			proof.sawSuccess = true
		case valState == returnNil && errState == returnNonNil:
			proof.sawFailure = true
		default:
			proof.consistent = false
		}
	})
	if !proof.consistent || !proof.sawSuccess {
		return flow.ReturnRelationsDomain.Top()
	}
	if proof.sawFailure || returnValueSlotOptional(returns, valueIdx) || declaredReturnSlotOptional(declaredReturns, valueIdx) {
		return flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{{ValueIndex: valueIdx, ErrorIndex: errorIdx}})
	}
	return flow.ReturnRelationsDomain.Top()
}

func projectGuardedReturnRelations(fs state.FunctionState, g *cfg.Graph) flow.ReturnRelations {
	if g == nil {
		return flow.ReturnRelationsDomain.Top()
	}
	const guardIdx, targetIdx = 0, 1
	var target typ.Type
	sawGuard := false
	consistent := true
	g.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if !consistent || info == nil {
			return
		}
		ps, ok := fs.Points[p]
		if !ok {
			return
		}
		if len(info.Exprs) == 0 && info.Stmt == nil {
			return
		}
		guardTruthy, okGuard := classifyReturnTruthiness(ps, info, guardIdx)
		if !okGuard {
			consistent = false
			return
		}
		if !guardTruthy {
			return
		}
		sawGuard = true
		targetValue := returnSlotValue(ps, info, targetIdx, projectOptions{})
		if targetValue.IsZero() {
			return
		}
		t := product.ProjectValueOrUnknown(targetValue)
		if typ.IsAbsentOrUnknown(t) || typ.IsAny(t) {
			return
		}
		if target == nil {
			target = t
		} else {
			target = typ.NewUnion(target, t)
		}
	})
	if !consistent || !sawGuard || target == nil {
		return flow.ReturnRelationsDomain.Top()
	}
	return flow.ReturnRelationsOfGuardedTypes([]flow.ReturnGuardRelation{{
		GuardIndex:    guardIdx,
		TargetIndex:   targetIdx,
		GuardOnTruthy: true,
		TargetType:    target,
	}})
}

func classifyReturnTruthiness(ps flow.PointState, info *cfg.ReturnInfo, idx int) (bool, bool) {
	av := returnSlotValue(ps, info, idx, projectOptions{})
	if av.IsZero() {
		return false, false
	}
	t := unwrap.Alias(product.ProjectValueOrUnknown(av))
	if t == nil || typ.IsAbsentOrUnknown(t) || typ.IsAny(t) {
		return false, false
	}
	switch v := t.(type) {
	case *typ.Literal:
		if b, ok := v.Value.(bool); ok {
			return b, true
		}
		return true, true
	}
	if t.Kind() == kind.Nil {
		return false, true
	}
	return true, true
}

func projectForwardedReturnRelations(fs state.FunctionState, g *cfg.Graph) flow.ReturnRelations {
	if g == nil {
		return flow.ReturnRelationsDomain.Top()
	}
	out := flow.ReturnRelationsDomain.Bottom()
	sawReturn := false
	g.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil {
			return
		}
		ps, ok := fs.Points[p]
		if !ok {
			return
		}
		rels := flow.ReturnRelationsDomain.Top()
		if len(info.Exprs) == 1 && info.SourceCallAt(0) != nil {
			rels = ps.ReturnRel
		}
		out = flow.ReturnRelationsDomain.Join(out, rels)
		sawReturn = true
	})
	if !sawReturn || flow.ReturnRelationsDomain.Equal(out, flow.ReturnRelationsDomain.Bottom()) {
		return flow.ReturnRelationsDomain.Top()
	}
	return out
}

func projectBoundaryFacts(fs state.FunctionState, g *cfg.Graph) flow.BoundaryFacts {
	if g == nil {
		return flow.BoundaryFactsDomain.Top()
	}
	paramBySymbol := returnRelationParamSymbolMap(g)
	var points []boundaryPointFacts
	bucketSpecs := make(map[string][]int)
	sawReturn := false
	g.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil {
			return
		}
		ps, ok := fs.Points[p]
		if !ok {
			return
		}
		mapper := flow.NewBoundaryPathProjection(paramBySymbol, returnBoundarySymbolMap(g, p, info))
		facts := flow.MergeBoundaryFactProofs(
			projectPointBoundaryFacts(ps, mapper),
			projectReturnLengthBoundaryFacts(ps, g, p, info, mapper),
		)
		paramFacts, buckets := facts.PartitionByReturnIndices()
		point := boundaryPointFacts{
			mappedReturns: mapper.MappedReturnIndices(),
			paramFacts:    paramFacts,
		}
		if len(buckets) > 0 {
			point.returnFacts = make(map[string]flow.BoundaryFacts, len(buckets))
		}
		for _, bucket := range buckets {
			indices := bucket.Indices()
			key := boundaryReturnIndicesKey(indices)
			point.returnFacts[key] = bucket.Facts()
			if _, ok := bucketSpecs[key]; !ok {
				bucketSpecs[key] = indices
			}
		}
		points = append(points, point)
		sawReturn = true
	})
	if !sawReturn {
		return flow.BoundaryFactsDomain.Top()
	}
	out := flow.BoundaryFactsDomain.Bottom()
	for _, point := range points {
		out = flow.BoundaryFactsDomain.Join(out, point.paramFacts)
	}
	for key, indices := range bucketSpecs {
		acc := flow.BoundaryFactsDomain.Bottom()
		sawEligible := false
		for _, point := range points {
			if !point.mapsAllReturnIndices(indices) {
				continue
			}
			facts, ok := point.returnFacts[key]
			if !ok {
				facts = flow.BoundaryFactsDomain.Top()
			}
			acc = flow.BoundaryFactsDomain.Join(acc, facts)
			sawEligible = true
		}
		if sawEligible {
			out = flow.MergeBoundaryFactProofs(out, acc)
		}
	}
	return out
}

type boundaryPointFacts struct {
	mappedReturns map[int]bool
	paramFacts    flow.BoundaryFacts
	returnFacts   map[string]flow.BoundaryFacts
}

func (p boundaryPointFacts) mapsAllReturnIndices(indices []int) bool {
	for _, idx := range indices {
		if !p.mappedReturns[idx] {
			return false
		}
	}
	return true
}

func boundaryReturnIndicesKey(indices []int) string {
	var out []byte
	for i, idx := range indices {
		if i > 0 {
			out = append(out, ',')
		}
		out = appendBoundaryIndexKey(out, idx)
	}
	return string(out)
}

func appendBoundaryIndexKey(out []byte, idx int) []byte {
	if idx == 0 {
		return append(out, '0')
	}
	var buf [20]byte
	i := len(buf)
	for idx > 0 {
		i--
		buf[i] = byte('0' + idx%10)
		idx /= 10
	}
	return append(out, buf[i:]...)
}

func projectPointBoundaryFacts(ps flow.PointState, mapper flow.BoundaryPathProjection) flow.BoundaryFacts {
	return flow.ProjectBoundaryFacts(
		flow.BoundaryFactProjectionInput{
			KeyPresence: ps.KeyPresence,
			Num:         ps.Num,
			IndexWrites: ps.IndexWrites,
		},
		mapper,
		flow.BoundaryFactProjectionPolicy{
			KeyPresence: flow.KeyPresenceBoundaryProjection{IncludePendingKeyArrays: true},
		},
	)
}

func projectReturnLengthBoundaryFacts(ps flow.PointState, g *cfg.Graph, p cfg.Point, info *cfg.ReturnInfo, mapper flow.BoundaryPathProjection) flow.BoundaryFacts {
	var facts []flow.BoundaryLengthRelationFact
	for i := range info.Exprs {
		target, ok := returnExprLocalAddress(g, p, info, i)
		if !ok {
			continue
		}
		targetAddr, ok := target.Stable()
		if !ok {
			continue
		}
		targets := mapper.PathsFromAddress(targetAddr)
		if len(targets) == 0 {
			continue
		}
		for _, rel := range ps.Rel.LengthParamsForTargetLocal(target) {
			source := flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: rel.ParamIndex}
			for _, boundaryTarget := range targets {
				facts = append(facts, flow.BoundaryLengthRelationFact{
					Target: boundaryTarget,
					Source: source,
				})
			}
		}
	}
	return flow.BoundaryFactsDomain.Top().WithLengthRelations(facts)
}

func returnBoundarySymbolMap(g *cfg.Graph, p cfg.Point, info *cfg.ReturnInfo) map[cfg.SymbolID][]flow.BoundaryPath {
	if g == nil || info == nil {
		return nil
	}
	out := make(map[cfg.SymbolID][]flow.BoundaryPath)
	for i := range info.Exprs {
		path := returnSourceBoundaryPath(g, p, info, i)
		if path.Symbol == 0 {
			continue
		}
		out[path.Symbol] = append(out[path.Symbol], flow.BoundaryPath{
			Kind:     flow.BoundaryPathReturn,
			Index:    i,
			Segments: append([]constraint.Segment(nil), path.Segments...),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func returnExprLocalAddress(g *cfg.Graph, p cfg.Point, info *cfg.ReturnInfo, i int) (flow.LocalAddress, bool) {
	if g == nil || info == nil || i < 0 || i >= len(info.Exprs) {
		return flow.LocalAddress{}, false
	}
	path := returnSourceBoundaryPath(g, p, info, i)
	if path.Symbol == 0 {
		return flow.LocalAddress{}, false
	}
	path = domainpath.WithVersion(path, g, p)
	return flow.LocalAddressOfPath(path)
}

func returnSourceBoundaryPath(g *cfg.Graph, p cfg.Point, info *cfg.ReturnInfo, i int) constraint.Path {
	if g == nil || info == nil || i < 0 || i >= len(info.Exprs) {
		return constraint.Path{}
	}
	if ident, ok := info.Exprs[i].(*ast.IdentExpr); ok && g.Bindings() != nil {
		if sym, ok := g.Bindings().SymbolOf(ident); ok && sym != 0 {
			return constraint.NewPath(sym, ident.Value)
		}
	}
	constResolver := predicate.BuildConstResolver(nil, p)
	path := domainpath.FromExprWithBindingsAt(info.Exprs[i], constResolver, g.Bindings(), g, p)
	if path.Symbol == 0 && i < len(info.Symbols) && info.Symbols[i] != 0 {
		path = constraint.Path{Symbol: info.Symbols[i]}
	}
	return path
}

func returnRelationParamSymbolMap(g *cfg.Graph) map[cfg.SymbolID]int {
	params := g.ParamSymbols()
	if len(params) == 0 {
		return nil
	}
	out := make(map[cfg.SymbolID]int, len(params))
	for idx, sym := range params {
		if sym != 0 {
			out[sym] = idx
		}
	}
	return out
}

type returnRelationProof struct {
	sawSuccess bool
	sawFailure bool
	consistent bool
}

type returnNilState uint8

const (
	returnNil returnNilState = iota + 1
	returnNonNil
)

func classifyReturnSlot(ps flow.PointState, info *cfg.ReturnInfo, idx int) (returnNilState, bool) {
	if idx < 0 {
		return 0, false
	}
	if idx >= len(info.Exprs) {
		if implicitReturnSlotIsNil(info.Exprs, idx) {
			return returnNil, true
		}
		return 0, false
	}
	return classifyReturnValue(returnSlotValue(ps, info, idx, projectOptions{}))
}

func classifyReturnValue(av product.AbstractValue) (returnNilState, bool) {
	if av.IsZero() {
		return 0, false
	}
	if av.DefinitelyPresent() {
		return returnNonNil, true
	}
	t := av.ProjectValue()
	if t == nil || typ.IsAbsentOrUnknown(t) {
		return 0, false
	}
	u := unwrap.Alias(t)
	if u == nil || u.Kind() == kind.Never {
		return 0, false
	}
	if u.Kind() == kind.Nil {
		return returnNil, true
	}
	if _, optional := typ.SplitNilableFieldType(t); optional {
		return 0, false
	}
	return returnNonNil, true
}

func implicitReturnSlotIsNil(exprs []ast.Expr, idx int) bool {
	if idx < 0 || idx < len(exprs) || len(exprs) == 0 {
		return false
	}
	last := exprs[len(exprs)-1]
	switch last.(type) {
	case *ast.FuncCallExpr, *ast.Comma3Expr:
		return false
	default:
		return true
	}
}

func returnValueSlotOptional(returns []product.AbstractValue, idx int) bool {
	if idx < 0 || idx >= len(returns) || returns[idx].IsZero() {
		return false
	}
	_, optional := typ.SplitNilableFieldType(returns[idx].ProjectValue())
	return optional
}

func declaredReturnSlotOptional(returns []typ.Type, idx int) bool {
	if idx < 0 || idx >= len(returns) || returns[idx] == nil {
		return false
	}
	return unwrap.IsOptionalLike(returns[idx])
}

func itoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// cloneContracts copies the Contracts map so the Summary does not alias the
// FunctionState's mutable map.
func cloneContracts(c paramevidence.Contracts) paramevidence.Contracts {
	if len(c) == 0 {
		return nil
	}
	out := make(paramevidence.Contracts, len(c))
	for k, v := range c {
		out[k] = v
	}
	return out
}
