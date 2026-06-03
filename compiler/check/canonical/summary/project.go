package summary

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/predicate"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	domainpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/types/constraint"
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
	return ProjectWithDeclaredReturns(fs, g, nil)
}

// ProjectOptions carries analysis metadata that projection needs but does not
// own. It stays intentionally narrow: summary projection can classify whether a
// returned call is a finite local dependency without importing driver/program
// target-resolution code.
type ProjectOptions struct {
	DeclaredReturns           []typ.Type
	ReturnCallHasFiniteTarget func(*cfg.CallInfo) bool
}

// ProjectWithDeclaredReturns is Project plus annotation context from the
// function signature. The declared return tuple is not a second analysis: it is a
// caller-visible contract used only where the summary projection needs to know
// that a success-only `(value, nil)` body is allowed to prove an error-return
// relation for an optional value slot.
func ProjectWithDeclaredReturns(fs state.FunctionState, g *cfg.Graph, declaredReturns []typ.Type) Summary {
	return ProjectWithOptions(fs, g, ProjectOptions{DeclaredReturns: declaredReturns})
}

// ProjectWithOptions is Project with the complete projection metadata bundle.
func ProjectWithOptions(fs state.FunctionState, g *cfg.Graph, opts ProjectOptions) Summary {
	returns := projectReturns(fs, g, opts)
	return Summary{
		Returns:             returns,
		ReturnFunctionRefs:  projectReturnFunctionRefs(fs, g),
		ReturnClosureRefs:   projectReturnClosureRefs(fs, g),
		Params:              cloneContracts(fs.Contracts),
		Relations:           projectReturnRelations(fs, g, returns, opts.DeclaredReturns),
		CellEffects:         projectCellEffects(fs, g),
		ReceiverEffects:     projectReceiverEffects(fs, g),
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
		point := flow.CaptureCellsDomain.Join(ps.Cells, captureCellsFromEnv(ps.Env, envExports))
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

func captureCellsFromEnv(env map[flow.ValueKey]product.AbstractValue, allowed map[cfg.SymbolID]bool) flow.CaptureCells {
	if len(env) == 0 || len(allowed) == 0 {
		return flow.CaptureCellsDomain.Bottom()
	}
	entries := make([]flow.CaptureCell, 0, len(env))
	for key, av := range env {
		sym, ok := symbolFromEnvKey(key)
		if !ok || !allowed[sym] || av.IsZero() {
			continue
		}
		entries = append(entries, flow.CaptureCell{Symbol: sym, Value: av})
	}
	return flow.CaptureCellsOf(entries)
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

func symbolFromEnvKey(key flow.ValueKey) (cfg.SymbolID, bool) {
	return flow.ParseSymbolValueKey(key)
}

// projectReturns folds the per-return-point Env into a single return tuple. For
// each return node, slot i takes the value of the i-th return expression; the
// tuple is the slotwise Join over all return points (a function with two return
// statements returns the least upper bound of both). Slots beyond a given
// statement's arity contribute Bottom for that statement.
func projectReturns(fs state.FunctionState, g *cfg.Graph, opts ProjectOptions) []product.AbstractValue {
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
func returnTupleAt(ps flow.PointState, info *cfg.ReturnInfo, opts ProjectOptions) []product.AbstractValue {
	arity := len(info.Exprs)
	if len(info.Exprs) == 1 && info.SourceCallAt(0) != nil {
		if stored := returnSlotStoredArity(ps); stored > arity {
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

func returnSlotStoredArity(ps flow.PointState) int {
	maxSlot := -1
	for key, av := range ps.Env {
		if av.IsZero() {
			continue
		}
		idx, ok := flow.ParseReturnSlotValueKey(key)
		if !ok || idx < 0 {
			continue
		}
		if idx > maxSlot {
			maxSlot = idx
		}
	}
	return maxSlot + 1
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

func returnSlotValue(ps flow.PointState, info *cfg.ReturnInfo, i int, opts ProjectOptions) product.AbstractValue {
	return returnSlotValueFromPointState(ps, info, i, opts)
}

func returnSlotValueFromPointState(ps flow.PointState, info *cfg.ReturnInfo, i int, opts ProjectOptions) product.AbstractValue {
	if i < len(info.Symbols) && info.Symbols[i] != 0 {
		if av, ok := flow.PointFactsOf(ps).SymbolValue(info.Symbols[i]); ok && !av.IsZero() {
			return av
		}
	}
	// A non-identifier return (a literal, an arithmetic result, a call) carries
	// its value in the return-slot Env key written by applyReturn. This is a
	// summary-owned read of transfer assembly facts through the PointFacts
	// typed-key boundary.
	if av, ok := flow.PointFactsOf(ps).ValueKeyValue(flow.ReturnSlotValueKey(i)); ok && !av.IsZero() {
		return av
	}
	if returnSlotHasFiniteCallTarget(ps, info, i, opts) {
		return product.Bottom()
	}
	return product.Domain.Top()
}

func returnSlotHasFiniteCallTarget(ps flow.PointState, info *cfg.ReturnInfo, i int, opts ProjectOptions) bool {
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
	key := path.Key()
	if set, ok := flow.FunctionRefAt(ps.FunctionRefs, key); ok && len(set.Refs()) > 0 {
		return true
	}
	if set, ok := flow.ClosureRefAt(ps.ClosureRefs, key); ok && len(set.Refs()) > 0 {
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
// solved return-point states. This is the summary-level counterpart of legacy
// ErrorReturn inference, but it lives inside Summary projection so recursive
// callees and callers see it through the same interprocedural fixed point.
func projectReturnRelations(fs state.FunctionState, g *cfg.Graph, returns []product.AbstractValue, declaredReturns []typ.Type) flow.ReturnRelations {
	return flow.MergeReturnRelationProofs(
		projectErrorReturnRelations(fs, g, returns, declaredReturns),
		flow.MergeReturnRelationProofs(
			projectForwardedReturnRelations(fs, g),
			projectPointLengthParamRelations(fs, g),
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

func returnExprPathKey(g *cfg.Graph, resolver *pathkey.Resolver, p cfg.Point, info *cfg.ReturnInfo, i int) (constraint.PathKey, bool) {
	if g == nil || resolver == nil || info == nil || i < 0 || i >= len(info.Exprs) {
		return "", false
	}
	constResolver := predicate.BuildConstResolver(nil, p)
	path := domainpath.FromExprWithBindingsAt(info.Exprs[i], constResolver, g.Bindings(), g, p)
	if path.Symbol == 0 && i < len(info.Symbols) && info.Symbols[i] != 0 {
		path = constraint.Path{Symbol: info.Symbols[i]}
	}
	if path.Symbol == 0 {
		return "", false
	}
	path.Version = 0
	key := resolver.KeyAt(p, path)
	return key, key != ""
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
	return classifyReturnValue(returnSlotValue(ps, info, idx, ProjectOptions{}))
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
