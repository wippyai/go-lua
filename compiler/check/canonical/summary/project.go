package summary

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	domainpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/domain/predicate"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/pathkey"
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
		Returns:             returns,
		ReturnFunctionRefs:  projectReturnFunctionRefs(fs, g),
		ReturnClosureRefs:   projectReturnClosureRefs(fs, g),
		Params:              cloneContracts(fs.Contracts),
		Relations:           projectReturnRelations(fs, g, returns, opts.DeclaredReturns),
		CellEffects:         projectCellEffects(fs, g),
		ReceiverEffects:     projectReceiverEffects(fs, g),
		BoundaryFacts:       projectBoundaryFacts(fs, g),
		CaptureExports:      projectCaptureExports(fs, g),
		CaptureFunctionRefs: projectCaptureFunctionRefs(fs, g),
		CaptureClosureRefs:  projectCaptureClosureRefs(fs, g),
		PrototypeSelf:       projectPrototypeSelf(fs, g),
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

// projectCaptureExports folds the captured-cell store visible at normal function
// boundaries. It includes both explicit captured cells this function carries and
// ordinary Env symbols this function declares: a nested closure captures lexical
// locations, and a parent publishes those locations as store entries for the
// child entry-state seed. Unlike CellEffects, this is a store snapshot.
func projectCaptureExports(fs state.FunctionState, g *cfg.Graph) flow.CaptureCells {
	if g == nil {
		return flow.CaptureCellsDomain.Top()
	}
	envExports := captureExportSymbols(g)
	out := flow.CaptureCellsDomain.Bottom()
	g.EachReturn(func(p cfg.Point, _ *cfg.ReturnInfo) {
		ps, ok := fs.Points[p]
		if !ok {
			return
		}
		point := flow.CaptureCellsDomain.Join(ps.Cells, flow.PointFactsOf(ps).EnvCaptureCells(envExports))
		point = captureCellsWithStaticMembers(point, ps.StaticMembers)
		out = flow.CaptureCellsDomain.Join(out, point)
	})
	return out
}

func projectCaptureFunctionRefs(fs state.FunctionState, g *cfg.Graph) flow.FunctionRefs {
	if g == nil {
		return flow.FunctionRefsDomain.Top()
	}
	out := flow.FunctionRefsDomain.Bottom()
	g.EachReturn(func(p cfg.Point, _ *cfg.ReturnInfo) {
		ps, ok := fs.Points[p]
		if !ok {
			return
		}
		out = flow.FunctionRefsDomain.Join(out, ps.FunctionRefs)
	})
	return out
}

func projectCaptureClosureRefs(fs state.FunctionState, g *cfg.Graph) flow.ClosureRefs {
	if g == nil {
		return flow.ClosureRefsDomain.Top()
	}
	out := flow.ClosureRefsDomain.Bottom()
	g.EachReturn(func(p cfg.Point, _ *cfg.ReturnInfo) {
		ps, ok := fs.Points[p]
		if !ok {
			return
		}
		out = flow.ClosureRefsDomain.Join(out, ps.ClosureRefs)
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

func captureCellsWithStaticMembers(cells flow.CaptureCells, facts flow.StaticMemberFacts) flow.CaptureCells {
	if cells.IsTop() || facts.IsBottom() || len(facts.Entries()) == 0 {
		return cells
	}
	out := cells
	for _, entry := range cells.Entries() {
		next := captureCellWithStaticMembers(entry.Symbol, entry.Value, facts)
		if next.IsZero() || product.Domain.Equal(next, entry.Value) {
			continue
		}
		out = out.With(entry.Symbol, next)
	}
	return out
}

func captureCellWithStaticMembers(sym cfg.SymbolID, av product.AbstractValue, facts flow.StaticMemberFacts) product.AbstractValue {
	if sym == 0 || av.IsZero() {
		return av
	}
	root, ok := flow.StableAddressOfSymbol(sym, nil)
	if !ok {
		return av
	}
	out := av
	for _, fact := range facts.AddressEntriesUnder(root) {
		segments := fact.Address.Segments()
		if len(segments) == 0 || fact.Value.IsZero() {
			continue
		}
		next := writeStaticMemberProduct(out, segments, fact.Value)
		if !next.IsZero() {
			out = next
		}
	}
	return out
}

func writeStaticMemberProduct(root product.AbstractValue, segments []constraint.Segment, val product.AbstractValue) product.AbstractValue {
	if len(segments) == 0 {
		return val
	}
	member, ok := value.MemberFromSegment(segments[0])
	if !ok {
		return root
	}
	if len(segments) == 1 {
		return product.WithMember(root, member, val)
	}
	child, ok := product.MemberOf(root, member)
	if !ok || child.IsZero() {
		child = product.FromType(typ.NewRecord().Build())
	}
	updated := writeStaticMemberProduct(child, segments[1:], val)
	if updated.IsZero() {
		return root
	}
	return product.WithMember(root, member, updated)
}

func captureExportSymbols(g *cfg.Graph) map[cfg.SymbolID]bool {
	if g == nil || g.Bindings() == nil {
		return nil
	}
	out := make(map[cfg.SymbolID]bool)
	for _, nested := range g.NestedFunctions() {
		if nested.Func == nil {
			continue
		}
		for _, sym := range g.Bindings().CapturedSymbols(nested.Func) {
			if g.OwnsSymbol(sym) {
				out[sym] = true
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
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

func projectReturnFunctionRefs(fs state.FunctionState, g *cfg.Graph) []flow.FunctionRefs {
	if g == nil {
		return nil
	}
	acc := returnFunctionRefsTupleLattice{}
	var tuple []flow.FunctionRefs
	g.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil || len(info.Exprs) == 0 {
			return
		}
		ps, ok := fs.Points[p]
		if !ok {
			return
		}
		stmt := returnFunctionRefsTupleAt(ps, info)
		tuple = acc.Join(tuple, stmt)
	})
	return tuple
}

func projectReturnClosureRefs(fs state.FunctionState, g *cfg.Graph) []flow.ClosureRefs {
	if g == nil {
		return nil
	}
	acc := returnClosureRefsTupleLattice{}
	var tuple []flow.ClosureRefs
	g.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil || len(info.Exprs) == 0 {
			return
		}
		ps, ok := fs.Points[p]
		if !ok {
			return
		}
		stmt := returnClosureRefsTupleAt(ps, info)
		tuple = acc.Join(tuple, stmt)
	})
	return tuple
}

func returnClosureRefsTupleAt(ps flow.PointState, info *cfg.ReturnInfo) []flow.ClosureRefs {
	out := make([]flow.ClosureRefs, len(info.Exprs))
	for i := range info.Exprs {
		out[i] = flow.ProjectClosureRefsByPath(ps.ClosureRefs, constraint.NewPlaceholder(i))
	}
	return out
}

func returnFunctionRefsTupleAt(ps flow.PointState, info *cfg.ReturnInfo) []flow.FunctionRefs {
	out := make([]flow.FunctionRefs, len(info.Exprs))
	for i := range info.Exprs {
		out[i] = flow.ProjectFunctionRefsByPath(ps.FunctionRefs, constraint.NewPlaceholder(i))
	}
	return out
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
	return returnSlotValueFromPointState(ps, info, i, opts)
}

func returnSlotValueFromPointState(ps flow.PointState, info *cfg.ReturnInfo, i int, opts projectOptions) product.AbstractValue {
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
	if set, ok := flow.FunctionRefAtPath(ps.FunctionRefs, path); ok && len(set.Refs()) > 0 {
		return true
	}
	if set, ok := flow.ClosureRefAtPath(ps.ClosureRefs, path); ok && len(set.Refs()) > 0 {
		return true
	}
	return false
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
			flow.MergeReturnRelationProofs(
				projectForwardedReturnRelations(fs, g),
				projectGuardedReturnRelations(fs, g),
			),
			flow.MergeReturnRelationProofs(
				projectPointLengthParamRelations(fs, g),
				projectPointReturnKeyParamRelations(fs, g),
			),
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

func projectPointLengthParamRelations(fs state.FunctionState, g *cfg.Graph) flow.ReturnRelations {
	if g == nil {
		return flow.ReturnRelationsDomain.Top()
	}
	resolver := pathkey.NewResolver(g)
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
		var proven []flow.ReturnLengthParamRelation
		for i := range info.Exprs {
			key, ok := returnExprPathKey(g, resolver, p, info, i)
			if !ok {
				continue
			}
			for _, rel := range ps.Rel.LengthParamsForTarget(key) {
				proven = append(proven, flow.ReturnLengthParamRelation{
					ReturnIndex: i,
					ParamIndex:  rel.ParamIndex,
				})
			}
		}
		if len(proven) > 0 {
			rels = flow.ReturnRelationsOfLengthParams(proven)
		}
		out = flow.ReturnRelationsDomain.Join(out, rels)
		sawReturn = true
	})
	if !sawReturn || flow.ReturnRelationsDomain.Equal(out, flow.ReturnRelationsDomain.Bottom()) {
		return flow.ReturnRelationsDomain.Top()
	}
	return out
}

func projectPointReturnKeyParamRelations(fs state.FunctionState, g *cfg.Graph) flow.ReturnRelations {
	if g == nil {
		return flow.ReturnRelationsDomain.Top()
	}
	paramBySymbol := returnRelationParamSymbolMap(g)
	if len(paramBySymbol) == 0 {
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
		var proven []flow.ReturnKeyParamRelation
		for i := range info.Exprs {
			key, ok := returnExprKeyPresenceKey(g, p, info, i)
			if !ok {
				continue
			}
			for _, entry := range ps.KeyPresence.Entries() {
				if entry.Key != key {
					continue
				}
				param, segments, ok := keyPresenceTableParamPath(entry.Table, paramBySymbol)
				if !ok {
					continue
				}
				proven = append(proven, flow.ReturnKeyParamRelation{
					ReturnIndex:   i,
					ParamIndex:    param,
					ParamSegments: segments,
				})
			}
		}
		if len(proven) > 0 {
			rels = flow.ReturnRelationsOfKeyParams(proven)
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
		mapper := boundaryPathMapper{
			paramBySymbol:  paramBySymbol,
			returnBySymbol: returnBoundarySymbolMap(g, p, info),
		}
		paramFacts, buckets := projectPointBoundaryFacts(ps, mapper).PartitionByReturnIndices()
		point := boundaryPointFacts{
			mappedReturns: mapper.mappedReturnIndices(),
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

type boundaryPathMapper struct {
	paramBySymbol  map[cfg.SymbolID]int
	returnBySymbol map[cfg.SymbolID][]flow.BoundaryPath
}

func projectPointBoundaryFacts(ps flow.PointState, mapper boundaryPathMapper) flow.BoundaryFacts {
	var keyPresence []flow.BoundaryKeyPresenceFact
	for _, fact := range ps.KeyPresence.Entries() {
		tables := mapper.pathsFromKey(fact.Table)
		if len(tables) == 0 {
			continue
		}
		keys := mapper.pathsFromKey(fact.Key)
		if len(keys) == 0 {
			continue
		}
		for _, table := range tables {
			for _, key := range keys {
				keyPresence = append(keyPresence, flow.BoundaryKeyPresenceFact{Table: table, Key: key})
			}
		}
	}
	var keyArrays []flow.BoundaryKeyArrayFact
	for _, fact := range ps.KeyPresence.KeyArrayEntries() {
		arrays := mapper.pathsFromKey(fact.Array)
		if len(arrays) == 0 {
			continue
		}
		tables := mapper.pathsFromKey(fact.Table)
		if len(tables) == 0 {
			continue
		}
		for _, array := range arrays {
			for _, table := range tables {
				keyArrays = append(keyArrays, flow.BoundaryKeyArrayFact{Array: array, Table: table})
			}
		}
	}
	var keyArrayValues []flow.BoundaryKeyArrayValueFact
	for _, fact := range ps.KeyPresence.KeyArrayValueEntries() {
		arrays := mapper.pathsFromKey(fact.Array)
		if len(arrays) == 0 {
			continue
		}
		tables := mapper.pathsFromKey(fact.Table)
		if len(tables) == 0 {
			continue
		}
		for _, array := range arrays {
			for _, table := range tables {
				keyArrayValues = append(keyArrayValues, flow.BoundaryKeyArrayValueFact{
					Array: array,
					Table: table,
					Value: fact.Value,
				})
			}
		}
	}
	var appendKeys []flow.BoundaryAppendKeyFact
	for _, fact := range ps.KeyPresence.AppendedKeyEntries() {
		arrays := mapper.pathsFromKey(fact.Array)
		if len(arrays) == 0 {
			continue
		}
		keys := mapper.pathsFromKey(fact.Key)
		if len(keys) == 0 {
			continue
		}
		for _, array := range arrays {
			for _, key := range keys {
				appendKeys = append(appendKeys, flow.BoundaryAppendKeyFact{
					Array: array,
					Key:   key,
				})
			}
		}
	}
	var appendOrigins []flow.BoundaryAppendElementFieldOriginFact
	for _, fact := range ps.KeyPresence.AppendElementFieldOriginEntries() {
		arrays := mapper.pathsFromKey(fact.Array)
		if len(arrays) == 0 {
			continue
		}
		field, ok := flow.AppendElementFieldSegments(fact.Field)
		if !ok {
			continue
		}
		sources := mapper.pathsFromKey(fact.Source)
		if len(sources) == 0 {
			continue
		}
		sourceField, _ := flow.AppendElementFieldSegments(fact.SourceField)
		for _, array := range arrays {
			for _, source := range sources {
				appendOrigins = append(appendOrigins, flow.BoundaryAppendElementFieldOriginFact{
					Array:       array,
					Field:       field,
					Source:      source,
					SourceField: sourceField,
				})
			}
		}
	}
	for _, fact := range ps.KeyPresence.PendingKeyArrayEntries() {
		arrays := mapper.pathsFromKey(fact.Array)
		if len(arrays) == 0 {
			continue
		}
		keys := mapper.pathsFromKey(fact.Key)
		if len(keys) == 0 {
			continue
		}
		var tables []flow.BoundaryPath
		if fact.Table != "" {
			tables = mapper.pathsFromKey(fact.Table)
			if len(tables) == 0 {
				continue
			}
		}
		for _, array := range arrays {
			for _, key := range keys {
				if len(tables) == 0 {
					appendKeys = append(appendKeys, flow.BoundaryAppendKeyFact{
						Array: array,
						Key:   key,
					})
					continue
				}
				for _, table := range tables {
					appendKeys = append(appendKeys, flow.BoundaryAppendKeyFact{
						Array:    array,
						Key:      key,
						Table:    table,
						HasTable: true,
					})
				}
			}
		}
	}
	var lengths []flow.BoundaryLengthLowerBound
	if ps.Num != nil {
		ps.Num.ForEachLenBound(func(key constraint.PathKey, lower, _ int64) bool {
			targets := mapper.pathsFromKey(key)
			if lower > 0 {
				for _, target := range targets {
					lengths = append(lengths, flow.BoundaryLengthLowerBound{Target: target, Lower: lower})
				}
			}
			return true
		})
	}
	var indexWrites []flow.BoundaryIndexWriteFact
	for _, fact := range ps.IndexWrites.Entries() {
		if fact.Target == "" || fact.KeyPath == "" || fact.Value.IsZero() {
			continue
		}
		tables := mapper.pathsFromKey(fact.Target)
		if len(tables) == 0 {
			continue
		}
		keys := mapper.pathsFromKey(fact.KeyPath)
		if len(keys) == 0 {
			continue
		}
		for _, table := range tables {
			for _, key := range keys {
				indexWrites = append(indexWrites, flow.BoundaryIndexWriteFact{
					Table: table,
					Key:   key,
					Value: fact.Value,
				})
			}
		}
	}
	return flow.BoundaryFactsOf(keyPresence, keyArrays, keyArrayValues, appendKeys, lengths, indexWrites).
		WithAppendElementFieldOrigins(appendOrigins)
}

func (m boundaryPathMapper) pathsFromKey(key constraint.PathKey) []flow.BoundaryPath {
	path, ok := flow.StablePathFromKey(key)
	if !ok || path.Symbol == 0 {
		return nil
	}
	var out []flow.BoundaryPath
	if idx, ok := m.paramBySymbol[path.Symbol]; ok {
		out = append(out, flow.BoundaryPath{
			Kind:     flow.BoundaryPathParam,
			Index:    idx,
			Segments: append([]constraint.Segment(nil), path.Segments...),
		})
	}
	for _, root := range m.returnBySymbol[path.Symbol] {
		nextSegments := path.Segments
		if len(root.Segments) > 0 {
			trimmed, ok := trimBoundarySegmentPrefix(nextSegments, root.Segments)
			if !ok {
				continue
			}
			nextSegments = trimmed
		}
		root.Segments = append([]constraint.Segment(nil), nextSegments...)
		out = append(out, root)
	}
	return out
}

func (m boundaryPathMapper) mappedReturnIndices() map[int]bool {
	if len(m.returnBySymbol) == 0 {
		return nil
	}
	out := make(map[int]bool)
	for _, roots := range m.returnBySymbol {
		for _, root := range roots {
			if root.Kind == flow.BoundaryPathReturn && root.Index >= 0 {
				out[root.Index] = true
			}
		}
	}
	return out
}

func trimBoundarySegmentPrefix(segments, prefix []constraint.Segment) ([]constraint.Segment, bool) {
	if len(prefix) > len(segments) {
		return nil, false
	}
	for i := range prefix {
		if segments[i] != prefix[i] {
			return nil, false
		}
	}
	return segments[len(prefix):], true
}

func returnBoundarySymbolMap(g *cfg.Graph, p cfg.Point, info *cfg.ReturnInfo) map[cfg.SymbolID][]flow.BoundaryPath {
	if g == nil || info == nil {
		return nil
	}
	out := make(map[cfg.SymbolID][]flow.BoundaryPath)
	for i := range info.Exprs {
		path := returnExprBoundaryPath(g, p, info, i)
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

func returnExprBoundaryPath(g *cfg.Graph, p cfg.Point, info *cfg.ReturnInfo, i int) constraint.Path {
	return returnSourceBoundaryPath(g, p, info, i)
}

func returnExprPathKey(g *cfg.Graph, resolver *pathkey.Resolver, p cfg.Point, info *cfg.ReturnInfo, i int) (constraint.PathKey, bool) {
	if g == nil || resolver == nil || info == nil || i < 0 || i >= len(info.Exprs) {
		return "", false
	}
	path := returnSourceBoundaryPath(g, p, info, i)
	if path.Symbol == 0 {
		return "", false
	}
	path.Version = 0
	key := resolver.KeyAt(p, path)
	return key, key != ""
}

func returnExprKeyPresenceKey(g *cfg.Graph, p cfg.Point, info *cfg.ReturnInfo, i int) (constraint.PathKey, bool) {
	if g == nil || info == nil || i < 0 || i >= len(info.Exprs) {
		return "", false
	}
	path := returnSourceBoundaryPath(g, p, info, i)
	if path.Symbol == 0 {
		return "", false
	}
	key := flow.KeyPresencePathKey(path)
	return key, key != ""
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

func keyPresenceTableParamPath(key constraint.PathKey, paramBySymbol map[cfg.SymbolID]int) (int, []constraint.Segment, bool) {
	path, ok := flow.StablePathFromKey(key)
	if !ok || path.Symbol == 0 {
		return 0, nil, false
	}
	idx, ok := paramBySymbol[path.Symbol]
	if !ok {
		return 0, nil, false
	}
	return idx, append([]constraint.Segment(nil), path.Segments...), true
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
