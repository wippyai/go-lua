// Package callresult adapts fixpoint summaries into factflow call outcomes.
package callresult

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/symbol"
	typenormalize "github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// KeyFunc maps one call site in context to an exact summary key.
type KeyFunc func(ctx transfer.NodeContext, site factflow.CallSite) (summary.SummaryKey, bool)

// CalleeValueFunc resolves the current callee expression value at a call site.
type CalleeValueFunc func(ctx transfer.NodeContext, site factflow.CallSite, in state.State, read func(cfg.Point) state.State) (product.Value, bool)

// ProviderConfig configures summary-backed call outcomes.
type ProviderConfig struct {
	Summaries              summary.Reader
	KeyFor                 KeyFunc
	CalleeValue            CalleeValueFunc
	Facts                  factflow.Facts
	FunctionKeys           map[symbol.ID]summary.SummaryKey
	FunctionExpressionKeys map[factflow.ExprRef]summary.SummaryKey
	FunctionIDs            map[identity.ID]summary.SummaryKey
	PathKeys               map[pathdom.PathKey]summary.SummaryKey
	PathMultiKeys          map[pathdom.PathKey][]summary.SummaryKey
	FunctionTypes          map[summary.SummaryKey]*typ.Function
	Sources                sourcevalue.SourceValues
}

// OutcomeProvider returns a generic call-boundary outcome provider backed by
// exact summary reads.
func OutcomeProvider(config ProviderConfig) factapply.CallOutcomeProvider {
	summaries := config.Summaries
	keyFor := config.KeyFor
	calleeValue := config.CalleeValue
	facts := config.Facts
	functionKeys := cloneFunctionKeys(config.FunctionKeys)
	functionExpressionKeys := cloneFunctionExpressionKeys(config.FunctionExpressionKeys)
	functionIDs := cloneFunctionIdentityKeys(config.FunctionIDs)
	pathKeys := clonePathKeys(config.PathKeys)
	pathMultiKeys := clonePathMultiKeys(config.PathMultiKeys)
	functionTypes := cloneFunctionTypes(config.FunctionTypes)
	sources := config.Sources
	return func(ctx transfer.NodeContext, site factflow.CallSite, in state.State, read func(cfg.Point) state.State) factapply.CallOutcome {
		if summaries == nil {
			return factapply.CallOutcome{}
		}
		key, ok := summaryKeyForCall(ctx, site, in, read, keyFor, calleeValue, functionIDs, pathKeys)
		var got summary.Summary
		var fn *typ.Function
		if ok {
			var readOK bool
			got, readOK = summaries.Read(key)
			if !readOK {
				return factapply.CallOutcome{}
			}
			fn = functionTypes[key]
			got = applyDeclaredSummaryReturns(ctx.Registry, got, fn)
			got = specializeGenericSummary(ctx, site, got, fn, summaries, facts, functionKeys, functionExpressionKeys, functionTypes, sources, in, read)
		} else if joined, joinedOK := joinedSummaryForDefinitionPath(ctx, site, in, read, calleeValue, summaries, pathMultiKeys, functionTypes, facts, functionKeys, functionExpressionKeys, sources); joinedOK {
			got = joined
		} else {
			if out, ok := unresolvedFunctionCallOutcome(ctx, site, in, read, calleeValue); ok {
				return out
			}
			return factapply.CallOutcome{}
		}
		out := outcomeFromSummary(got, func(index int) bool {
			if index < 0 || index >= len(got.ParamObligations) {
				return false
			}
			return summary.UsefulParamObligation(ctx.Registry, got.ParamObligations[index])
		}, func(index int) bool {
			if index < 0 || index >= len(got.NormalReturnParams) {
				return false
			}
			return summary.UsefulNormalReturnParam(ctx.Registry, got.NormalReturnParams[index])
		})
		out.ParamObligations = append(out.ParamObligations, functionTypeParamObligations(ctx.Registry, site, fn)...)
		out.ParamObligations = append(out.ParamObligations, memberCallParamObligations(ctx, site, got, sources, in, read)...)
		return out
	}
}

func joinedSummaryForDefinitionPath(
	ctx transfer.NodeContext,
	site factflow.CallSite,
	in state.State,
	read func(cfg.Point) state.State,
	calleeValue CalleeValueFunc,
	summaries summary.Reader,
	pathMultiKeys map[pathdom.PathKey][]summary.SummaryKey,
	functionTypes map[summary.SummaryKey]*typ.Function,
	facts factflow.Facts,
	functionKeys map[symbol.ID]summary.SummaryKey,
	functionExpressionKeys map[factflow.ExprRef]summary.SummaryKey,
	sources sourcevalue.SourceValues,
) (summary.Summary, bool) {
	if ctx.Registry == nil || summaries == nil || len(pathMultiKeys) == 0 {
		return summary.Summary{}, false
	}
	calleePath := site.CalleePath()
	if calleePath.IsEmpty() {
		return summary.Summary{}, false
	}
	keys := pathMultiKeys[calleePath.Key()]
	if len(keys) < 2 {
		return summary.Summary{}, false
	}
	if calleeValue != nil {
		if value, ok := calleeValue(ctx, site, in, read); ok {
			if _, hasID := product.Get(ctx.Registry, value, identity.Key).ID(); hasID {
				return summary.Summary{}, false
			}
		}
	}
	var out summary.Summary
	seen := false
	for _, key := range keys {
		got, ok := summaries.Read(key)
		if !ok {
			continue
		}
		fn := functionTypes[key]
		got = applyDeclaredSummaryReturns(ctx.Registry, got, fn)
		got = specializeGenericSummary(ctx, site, got, fn, summaries, facts, functionKeys, functionExpressionKeys, functionTypes, sources, in, read)
		if !seen {
			out = got
			seen = true
			continue
		}
		out = joinPossibleCallSummaries(ctx.Registry, out, got)
	}
	return out, seen
}

func joinPossibleCallSummaries(reg *axis.Registry, left, right summary.Summary) summary.Summary {
	left = materializeReturnRootTypesFromFacts(reg, left)
	right = materializeReturnRootTypesFromFacts(reg, right)
	out := summary.Join(reg, left, right)
	for i := range out.Returns {
		leftValue, leftOK := summaryReturnValueAt(reg, left, i)
		rightValue, rightOK := summaryReturnValueAt(reg, right, i)
		if !leftOK || !rightOK {
			continue
		}
		leftType, leftTypeOK := typevalue.TypeOf(reg, leftValue)
		rightType, rightTypeOK := typevalue.TypeOf(reg, rightValue)
		if !leftTypeOK || !rightTypeOK {
			continue
		}
		t := typenormalize.UnionForEvidence(leftType, rightType)
		value := typevalue.FromType(reg, t)
		out.Returns[i] = typevalue.WithWitness(reg, value, t)
	}
	return dropDescendantFactsBelowMaybeAbsentReturns(out)
}

func materializeReturnRootTypesFromFacts(reg *axis.Registry, sum summary.Summary) summary.Summary {
	if reg == nil || len(sum.NormalReturnFacts.PathRefinements) == 0 && len(sum.NormalReturnFacts.PathStaticMembers) == 0 {
		return sum
	}
	maxIndex := len(sum.Returns) - 1
	for _, fact := range sum.NormalReturnFacts.PathRefinements {
		if index := fact.Path.PlaceholderIndex(); index > maxIndex {
			maxIndex = index
		}
	}
	for _, fact := range sum.NormalReturnFacts.PathStaticMembers {
		if index := fact.Path.PlaceholderIndex(); index > maxIndex {
			maxIndex = index
		}
	}
	if maxIndex < 0 {
		return sum
	}
	if len(sum.Returns) <= maxIndex {
		expanded := make([]product.Value, maxIndex+1)
		copy(expanded, sum.Returns)
		sum.Returns = expanded
	}
	for index := 0; index <= maxIndex; index++ {
		if !returnSlotNeedsFactType(reg, sum.Returns[index]) {
			continue
		}
		t, ok := returnRecordTypeFromFacts(reg, sum.NormalReturnFacts, index)
		if !ok {
			continue
		}
		value := typevalue.FromType(reg, t)
		sum.Returns[index] = typevalue.WithWitness(reg, value, t)
	}
	return sum
}

func returnSlotNeedsFactType(reg *axis.Registry, value product.Value) bool {
	if product.Equal(reg, value, product.Bottom(reg)) {
		return true
	}
	_, ok := typevalue.TypeOf(reg, value)
	return !ok
}

func returnRecordTypeFromFacts(reg *axis.Registry, facts callboundary.NormalReturnFacts, index int) (typ.Type, bool) {
	var parts typ.RecordParts
	seen := false
	add := func(path pathdom.Path, value product.Value) {
		if path.PlaceholderIndex() != index || len(path.Segments) != 1 {
			return
		}
		t, ok := typevalue.TypeOf(reg, value)
		if !ok || t == nil {
			return
		}
		if name, ok := path.DirectFieldName(); ok {
			parts.Fields = append(parts.Fields, typ.Field{Name: name, Type: t})
			seen = true
			return
		}
		if index, ok := path.DirectIntIndex(); ok {
			parts.StaticMembers = append(parts.StaticMembers, typ.StaticMember{Kind: typ.StaticMemberIntIndex, Index: int64(index), Type: t})
			seen = true
		}
	}
	for _, fact := range facts.PathRefinements {
		add(fact.Path, fact.Value)
	}
	for _, fact := range facts.PathStaticMembers {
		add(fact.Path, fact.Value)
	}
	if !seen {
		return nil, false
	}
	return typ.RebuildRecord(parts), true
}

func summaryReturnValueAt(reg *axis.Registry, sum summary.Summary, index int) (product.Value, bool) {
	if index < 0 || index >= len(sum.Returns) {
		return product.Value{}, false
	}
	value := sum.Returns[index]
	if product.Equal(reg, value, product.Bottom(reg)) {
		return product.Value{}, false
	}
	return value, true
}

func dropDescendantFactsBelowMaybeAbsentReturns(sum summary.Summary) summary.Summary {
	if len(sum.Returns) == 0 {
		return sum
	}
	maybeAbsent := make(map[int]struct{})
	for i, value := range sum.Returns {
		if !product.DefinitelyPresent(value) {
			maybeAbsent[i] = struct{}{}
		}
	}
	if len(maybeAbsent) == 0 {
		return sum
	}
	facts := sum.NormalReturnFacts
	facts.PathRefinements = filterPathValueFactsBelowReturns(facts.PathRefinements, maybeAbsent)
	facts.PathStaticMembers = filterPathStaticMemberFactsBelowReturns(facts.PathStaticMembers, maybeAbsent)
	facts.DynamicIndexFacts = filterDynamicIndexFactsBelowReturns(facts.DynamicIndexFacts, maybeAbsent)
	facts.BranchProofs = filterBranchProofsBelowReturns(facts.BranchProofs, maybeAbsent)
	facts.ChannelSelects = filterChannelSelectsBelowReturns(facts.ChannelSelects, maybeAbsent)
	facts.EffectDeltas = filterEffectDeltasBelowReturns(facts.EffectDeltas, maybeAbsent)
	sum.NormalReturnFacts = facts
	return sum
}

func filterPathValueFactsBelowReturns(in []callboundary.PathValueFact, roots map[int]struct{}) []callboundary.PathValueFact {
	out := in[:0]
	for _, fact := range in {
		if strictPlaceholderDescendant(fact.Path, roots) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func filterPathStaticMemberFactsBelowReturns(in []callboundary.PathStaticMemberFact, roots map[int]struct{}) []callboundary.PathStaticMemberFact {
	out := in[:0]
	for _, fact := range in {
		if strictPlaceholderDescendant(fact.Path, roots) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func filterDynamicIndexFactsBelowReturns(in []callboundary.DynamicIndexFact, roots map[int]struct{}) []callboundary.DynamicIndexFact {
	out := in[:0]
	for _, fact := range in {
		if strictPlaceholderDescendant(fact.Table, roots) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func filterBranchProofsBelowReturns(in []callboundary.BranchProof, roots map[int]struct{}) []callboundary.BranchProof {
	out := in[:0]
	for _, fact := range in {
		if strictPlaceholderDescendant(fact.Path, roots) || strictPlaceholderDescendant(fact.Other, roots) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func filterChannelSelectsBelowReturns(in []callboundary.ChannelSelectFact, roots map[int]struct{}) []callboundary.ChannelSelectFact {
	out := in[:0]
	for _, fact := range in {
		if strictPlaceholderDescendant(fact.Result, roots) || strictPlaceholderDescendant(fact.Case, roots) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func filterEffectDeltasBelowReturns(in []callboundary.EffectDelta, roots map[int]struct{}) []callboundary.EffectDelta {
	out := in[:0]
	for _, fact := range in {
		if strictPlaceholderDescendant(fact.Target, roots) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func strictPlaceholderDescendant(p pathdom.Path, roots map[int]struct{}) bool {
	if len(p.Segments) == 0 {
		return false
	}
	index := p.PlaceholderIndex()
	if index < 0 {
		return false
	}
	_, ok := roots[index]
	return ok
}

func unresolvedFunctionCallOutcome(
	ctx transfer.NodeContext,
	site factflow.CallSite,
	in state.State,
	read func(cfg.Point) state.State,
	calleeValue CalleeValueFunc,
) (factapply.CallOutcome, bool) {
	if ctx.Registry == nil || calleeValue == nil {
		return factapply.CallOutcome{}, false
	}
	value, ok := calleeValue(ctx, site, in, read)
	if !ok {
		return factapply.CallOutcome{}, false
	}
	if functionWitnessHasUsableReturns(ctx.Registry, value) {
		return factapply.CallOutcome{}, false
	}
	if !product.Get(ctx.Registry, value, runtimekind.Key).Contains(runtimekind.Function) {
		return factapply.CallOutcome{}, false
	}
	results := unknownResultSlots(ctx.Registry, site)
	if len(results) == 0 {
		return factapply.CallOutcome{}, false
	}
	return factapply.CallOutcome{Results: results}, true
}

func functionWitnessHasUsableReturns(reg *axis.Registry, value product.Value) bool {
	if reg == nil {
		return false
	}
	t, ok := product.Get(reg, value, typewitness.Key).Type()
	if !ok {
		return false
	}
	fn, ok := typecall.Callable(t)
	return ok && fn != nil && len(fn.Returns) != 0
}

func unknownResultSlots(reg *axis.Registry, site factflow.CallSite) []factapply.CallResult {
	if reg == nil {
		return nil
	}
	unknown := product.Set(reg, product.Top(), evidence.Key, evidence.GradualTop())
	seen := make(map[int]struct{})
	var out []factapply.CallResult
	for _, target := range site.ResultTargets() {
		index := target.ResultIndex()
		if index < 0 {
			continue
		}
		if _, ok := seen[index]; ok {
			continue
		}
		seen[index] = struct{}{}
		out = append(out, factapply.CallResult{Index: index, Value: unknown})
	}
	return out
}

func summaryKeyForCall(
	ctx transfer.NodeContext,
	site factflow.CallSite,
	in state.State,
	read func(cfg.Point) state.State,
	keyFor KeyFunc,
	calleeValue CalleeValueFunc,
	functionIDs map[identity.ID]summary.SummaryKey,
	pathKeys map[pathdom.PathKey]summary.SummaryKey,
) (summary.SummaryKey, bool) {
	if keyFor != nil {
		if key, ok := keyFor(ctx, site); ok {
			return key, true
		}
	}
	if key, ok := summaryKeyForCurrentCalleeIdentity(ctx, site, in, read, calleeValue, functionIDs); ok {
		return key, true
	}
	return summaryKeyForDefinitionPath(ctx, site, in, read, calleeValue, pathKeys)
}

// summaryKeyForDefinitionPath resolves a member-call callee to its summary by the
// callee's syntactic definition path. It is the sound fallback for a callee value
// rehydrated across a closure boundary, where the function value carries no
// runtime identity: the path resolves only when the current callee value holds no
// conflicting identity, so a later reassignment (whose value carries the new
// identity) is resolved by the current-identity route instead.
func summaryKeyForDefinitionPath(
	ctx transfer.NodeContext,
	site factflow.CallSite,
	in state.State,
	read func(cfg.Point) state.State,
	calleeValue CalleeValueFunc,
	pathKeys map[pathdom.PathKey]summary.SummaryKey,
) (summary.SummaryKey, bool) {
	if len(pathKeys) == 0 {
		return summary.SummaryKey{}, false
	}
	calleePath := site.CalleePath()
	if calleePath.IsEmpty() {
		return summary.SummaryKey{}, false
	}
	key, ok := pathKeys[calleePath.Key()]
	if !ok {
		return summary.SummaryKey{}, false
	}
	if calleeValue != nil {
		if value, ok := calleeValue(ctx, site, in, read); ok {
			if _, hasID := product.Get(ctx.Registry, value, identity.Key).ID(); hasID {
				return summary.SummaryKey{}, false
			}
		}
	}
	return key, true
}

func summaryKeyForCurrentCalleeIdentity(
	ctx transfer.NodeContext,
	site factflow.CallSite,
	in state.State,
	read func(cfg.Point) state.State,
	calleeValue CalleeValueFunc,
	functionIDs map[identity.ID]summary.SummaryKey,
) (summary.SummaryKey, bool) {
	if calleeValue == nil || len(functionIDs) == 0 {
		return summary.SummaryKey{}, false
	}
	value, ok := calleeValue(ctx, site, in, read)
	if !ok {
		return summary.SummaryKey{}, false
	}
	id, ok := product.Get(ctx.Registry, value, identity.Key).ID()
	if !ok {
		return summary.SummaryKey{}, false
	}
	key, ok := functionIDs[id]
	return key, ok
}

// SummaryArgumentTypeProvider resolves call argument types using summary-backed
// function expression identities, then falls back to generic source-value
// projection. It is used by public signature lowering when imported generic
// calls receive local callbacks whose return types are known only after the
// program fixed point.
func SummaryArgumentTypeProvider(config ProviderConfig) func(transfer.NodeContext, factflow.ValueSource, state.State, func(cfg.Point) state.State) (typ.Type, bool) {
	summaries := config.Summaries
	facts := config.Facts
	functionKeys := cloneFunctionKeys(config.FunctionKeys)
	functionExpressionKeys := cloneFunctionExpressionKeys(config.FunctionExpressionKeys)
	functionTypes := cloneFunctionTypes(config.FunctionTypes)
	sources := config.Sources
	return func(ctx transfer.NodeContext, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (typ.Type, bool) {
		return callArgumentType(ctx, source, summaries, facts, functionKeys, functionExpressionKeys, functionTypes, sources, in, read)
	}
}

func specializeGenericSummary(
	ctx transfer.NodeContext,
	site factflow.CallSite,
	got summary.Summary,
	fn *typ.Function,
	summaries summary.Reader,
	facts factflow.Facts,
	functionKeys map[symbol.ID]summary.SummaryKey,
	functionExpressionKeys map[factflow.ExprRef]summary.SummaryKey,
	functionTypes map[summary.SummaryKey]*typ.Function,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) summary.Summary {
	if ctx.Registry == nil || fn == nil || len(fn.TypeParams) == 0 || sources == nil {
		return got
	}
	args := callArgumentTypes(ctx, site, summaries, facts, functionKeys, functionExpressionKeys, functionTypes, sources, in, read)
	if len(args) == 0 {
		return got
	}
	instantiated, violations := typecall.InstantiateGenericCall(fn, args)
	if len(violations) != 0 || instantiated == nil || instantiated == fn {
		return got
	}
	return specializeSummaryReturns(ctx.Registry, got, fn.Returns, instantiated.Returns)
}

func applyDeclaredSummaryReturns(reg *axis.Registry, got summary.Summary, fn *typ.Function) summary.Summary {
	if reg == nil || fn == nil || len(fn.TypeParams) != 0 || len(fn.Returns) == 0 {
		return got
	}
	return specializeSummaryReturns(reg, got, fn.Returns, fn.Returns)
}

func callArgumentTypes(
	ctx transfer.NodeContext,
	site factflow.CallSite,
	summaries summary.Reader,
	facts factflow.Facts,
	functionKeys map[symbol.ID]summary.SummaryKey,
	functionExpressionKeys map[factflow.ExprRef]summary.SummaryKey,
	functionTypes map[summary.SummaryKey]*typ.Function,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) []typ.Type {
	if sources == nil {
		return nil
	}
	argSources := site.ArgumentSources()
	if len(argSources) == 0 {
		return nil
	}
	args := make([]typ.Type, len(argSources))
	seen := false
	for i, source := range argSources {
		t, ok := callArgumentType(ctx, source, summaries, facts, functionKeys, functionExpressionKeys, functionTypes, sources, in, read)
		if !ok {
			continue
		}
		args[i] = t
		seen = true
	}
	if !seen {
		return nil
	}
	return args
}

func sourceValueAtArgument(
	reg *axis.Registry,
	point cfg.Point,
	source factflow.ValueSource,
	facts factflow.Facts,
	graph cfg.Graph,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	value, ok := valueOfSource(point, source, sources, in, read)
	if t, okType := typeFromValue(reg, value); okType && usableRecoveredType(reg, value, t) {
		return value, true
	}
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return value, ok
	}
	if value, ok := valueFromRootDeclarationSource(reg, point, source.ExprRef, facts, graph, sources, in, read); ok {
		return value, true
	}
	return value, ok
}

func valueFromRootDeclarationSource(
	reg *axis.Registry,
	point cfg.Point,
	expr factflow.ExprRef,
	facts factflow.Facts,
	graph cfg.Graph,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	if reg == nil {
		return product.Value{}, false
	}
	exprPath, ok := facts.ExpressionPath(expr)
	if !ok || exprPath.Symbol == 0 || len(exprPath.Segments) != 0 {
		return product.Value{}, false
	}
	decl, ok := recoverRootDeclarationSource(point, exprPath.Symbol, facts, graph)
	if !ok {
		return product.Value{}, false
	}
	declState := in
	if read != nil {
		declState = read(decl.point)
	}
	if decl.symbol != 0 {
		v := declState.ReadSymbolValue(reg, decl.symbol)
		if !product.Equal(reg, v, product.Bottom(reg)) {
			return v, true
		}
	}
	return valueOfSource(decl.point, decl.source, sources, declState, read)
}

func valueOfSource(
	point cfg.Point,
	source factflow.ValueSource,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	if sources == nil {
		return product.Value{}, false
	}
	value, ok := sources.ValueOfSource(point, source, in, read)
	if !ok {
		return product.Value{}, false
	}
	return value, true
}

type recoveredRootDeclarationSource struct {
	point  cfg.Point
	source factflow.ValueSource
	symbol symbol.ID
}

func recoverRootDeclarationSource(
	point cfg.Point,
	target symbol.ID,
	facts factflow.Facts,
	graph cfg.Graph,
) (recoveredRootDeclarationSource, bool) {
	if point == 0 || target == 0 || graph == nil {
		return recoveredRootDeclarationSource{}, false
	}
	dominators := dominance.ComputeImmediateDominatorInfo(graph)
	if dominators == nil {
		return recoveredRootDeclarationSource{}, false
	}
	idom := dominators.Map()
	visited := make(map[cfg.Point]struct{}, graph.Size())
	var recovered recoveredRootDeclarationSource
	for cursor := point; ; {
		if _, ok := visited[cursor]; ok {
			return recoveredRootDeclarationSource{}, false
		}
		visited[cursor] = struct{}{}
		assignment, ok := facts.RootAssignment(cursor)
		if ok && assignment.TargetSymbol() == target && len(assignment.TargetPath().Segments) == 0 {
			switch assignment.Kind() {
			case factflow.RootAssignmentLocalDeclaration:
				recovered = recoveredRootDeclarationSource{
					point:  cursor,
					source: assignment.Source(),
					symbol: target,
				}
				return recovered, true
			case factflow.RootAssignmentOrdinaryRootWrite:
				return recoveredRootDeclarationSource{}, false
			default:
				return recoveredRootDeclarationSource{}, false
			}
		}
		parent, ok := idom[cursor]
		if !ok || parent == cursor {
			return recoveredRootDeclarationSource{}, false
		}
		cursor = parent
	}
}

func callArgumentType(
	ctx transfer.NodeContext,
	source factflow.ValueSource,
	summaries summary.Reader,
	facts factflow.Facts,
	functionKeys map[symbol.ID]summary.SummaryKey,
	functionExpressionKeys map[factflow.ExprRef]summary.SummaryKey,
	functionTypes map[summary.SummaryKey]*typ.Function,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) (typ.Type, bool) {
	if t, ok := functionExpressionTypeFromSummary(ctx, source, summaries, facts, functionKeys, functionExpressionKeys, functionTypes); ok {
		return t, true
	}
	if sources == nil {
		return nil, false
	}
	value, ok := sourceValueAtArgument(ctx.Registry, ctx.Point, source, facts, ctx.Graph, sources, in, read)
	if !ok {
		return nil, false
	}
	t, ok := typeFromValue(ctx.Registry, value)
	if !ok || !usableRecoveredType(ctx.Registry, value, t) {
		return nil, false
	}
	return t, true
}

func usableRecoveredType(reg *axis.Registry, value product.Value, t typ.Type) bool {
	if reg == nil || t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) || refinement.ContainsFreeTypeParam(t) {
		return false
	}
	ev := product.Get(reg, value, evidence.Key)
	return !ev.IsExplicitTop() && !ev.IsGradualTop()
}

func functionExpressionTypeFromSummary(
	ctx transfer.NodeContext,
	source factflow.ValueSource,
	summaries summary.Reader,
	facts factflow.Facts,
	functionKeys map[symbol.ID]summary.SummaryKey,
	functionExpressionKeys map[factflow.ExprRef]summary.SummaryKey,
	functionTypes map[summary.SummaryKey]*typ.Function,
) (typ.Type, bool) {
	if !source.HasExpr || summaries == nil {
		return nil, false
	}
	key, ok := functionExpressionKeys[source.ExprRef]
	if !ok {
		functionSymbol, ok := facts.ExpressionFunction(source.ExprRef)
		if !ok || functionSymbol == 0 {
			return nil, false
		}
		key, ok = functionKeys[functionSymbol]
		if !ok {
			return nil, false
		}
	}
	fn := functionTypes[key]
	if fn == nil {
		return nil, false
	}
	sum, ok := summaries.Read(key)
	if !ok || len(sum.Returns) == 0 {
		return fn, true
	}
	out := functionTypeWithSummaryReturns(ctx.Registry, fn, sum.Returns)
	return out, true
}

func functionTypeWithSummaryReturns(reg *axis.Registry, fn *typ.Function, returns []product.Value) *typ.Function {
	if reg == nil || fn == nil || len(returns) == 0 {
		return fn
	}
	outReturns := make([]typ.Type, 0, len(returns))
	for _, ret := range returns {
		t, ok := typeFromValue(reg, ret)
		if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) || refinement.ContainsFreeTypeParam(t) {
			return fn
		}
		outReturns = append(outReturns, t)
	}
	if len(outReturns) == 0 {
		return fn
	}
	return typ.RebuildFunction(typ.FunctionParts{
		TypeParams: fn.TypeParams,
		Params:     fn.Params,
		Variadic:   fn.Variadic,
		Returns:    outReturns,
	})
}

func typeFromValue(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	return typevalue.TypeOf(reg, value)
}

func specializeSummaryReturns(reg *axis.Registry, got summary.Summary, originalReturns []typ.Type, returns []typ.Type) summary.Summary {
	if reg == nil || len(got.Returns) == 0 || len(returns) == 0 {
		return got
	}
	originalReturns = callResultReturnTypes(got, originalReturns)
	returns = callResultReturnTypes(got, returns)
	nextReturns := make([]product.Value, len(got.Returns))
	copy(nextReturns, got.Returns)
	changed := false
	for i := range nextReturns {
		if i >= len(returns) {
			break
		}
		ret := returns[i]
		if ret == nil || typ.IsAny(ret) || typ.IsUnknown(ret) || refinement.ContainsFreeTypeParam(ret) {
			continue
		}
		declared := typevalue.WithWitness(reg, typevalue.FromType(reg, ret), ret)
		next := joinInstantiatedReturnValue(reg, nextReturns[i], declared, originalReturnTypeAt(originalReturns, i))
		if product.Equal(reg, nextReturns[i], next) {
			continue
		}
		nextReturns[i] = next
		changed = true
	}
	if !changed {
		return got
	}
	out := got.Clone()
	out.Returns = nextReturns
	return summary.Normalize(reg, out)
}

func originalReturnTypeAt(returns []typ.Type, index int) typ.Type {
	if index < 0 || index >= len(returns) {
		return nil
	}
	return returns[index]
}

func callResultReturnTypes(got summary.Summary, returns []typ.Type) []typ.Type {
	if len(returns) == 1 && len(got.Returns) > 1 {
		if tuple, ok := returns[0].(*typ.Tuple); ok {
			return append([]typ.Type(nil), tuple.Elements...)
		}
	}
	return returns
}

func joinInstantiatedReturnValue(reg *axis.Registry, value product.Value, declared product.Value, original typ.Type) product.Value {
	if product.Equal(reg, value, product.Top()) {
		return declared
	}
	if refinement.ContainsFreeTypeParam(original) || valueContainsFreeTypeParam(reg, value) {
		return declared
	}
	joined := product.Join(reg, value, declared)
	declaredWitness := product.Get(reg, declared, typewitness.Key)
	if !declaredWitness.IsBottom() && !declaredWitness.IsTop() {
		joinedWitness := product.Get(reg, joined, typewitness.Key)
		if joinedWitness.IsTop() {
			joined = product.Set(reg, joined, typewitness.Key, declaredWitness)
		}
	}
	declaredOrigin := product.Get(reg, declared, variantorigin.Key)
	if declaredOrigin.IsBottom() || declaredOrigin.IsTop() {
		return joined
	}
	joinedOrigin := product.Get(reg, joined, variantorigin.Key)
	if !joinedOrigin.IsTop() && !originContainsFreeTypeParam(joinedOrigin) {
		return joined
	}
	return product.Set(reg, joined, variantorigin.Key, declaredOrigin)
}

func valueContainsFreeTypeParam(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	return ok && refinement.ContainsFreeTypeParam(t)
}

func originContainsFreeTypeParam(origin variantorigin.Value) bool {
	if origin.IsBottom() || origin.IsTop() {
		return false
	}
	t, ok := variant.TypeFromOrigin(origin.Family(), origin.Cases())
	return ok && refinement.ContainsFreeTypeParam(t)
}

func cloneFunctionTypes(in map[summary.SummaryKey]*typ.Function) map[summary.SummaryKey]*typ.Function {
	if len(in) == 0 {
		return nil
	}
	out := make(map[summary.SummaryKey]*typ.Function, len(in))
	for key, fn := range in {
		out[key] = fn
	}
	return out
}

func cloneFunctionKeys(in map[symbol.ID]summary.SummaryKey) map[symbol.ID]summary.SummaryKey {
	if len(in) == 0 {
		return nil
	}
	out := make(map[symbol.ID]summary.SummaryKey, len(in))
	for id, key := range in {
		out[id] = key
	}
	return out
}

func cloneFunctionExpressionKeys(in map[factflow.ExprRef]summary.SummaryKey) map[factflow.ExprRef]summary.SummaryKey {
	if len(in) == 0 {
		return nil
	}
	out := make(map[factflow.ExprRef]summary.SummaryKey, len(in))
	for expr, key := range in {
		out[expr] = key
	}
	return out
}

func cloneFunctionIdentityKeys(in map[identity.ID]summary.SummaryKey) map[identity.ID]summary.SummaryKey {
	if len(in) == 0 {
		return nil
	}
	out := make(map[identity.ID]summary.SummaryKey, len(in))
	for id, key := range in {
		out[id] = key
	}
	return out
}

func clonePathKeys(in map[pathdom.PathKey]summary.SummaryKey) map[pathdom.PathKey]summary.SummaryKey {
	if len(in) == 0 {
		return nil
	}
	out := make(map[pathdom.PathKey]summary.SummaryKey, len(in))
	for pathKey, key := range in {
		out[pathKey] = key
	}
	return out
}

func clonePathMultiKeys(in map[pathdom.PathKey][]summary.SummaryKey) map[pathdom.PathKey][]summary.SummaryKey {
	if len(in) == 0 {
		return nil
	}
	out := make(map[pathdom.PathKey][]summary.SummaryKey, len(in))
	for pathKey, keys := range in {
		if len(keys) == 0 {
			continue
		}
		out[pathKey] = append([]summary.SummaryKey(nil), keys...)
	}
	return out
}

func outcomeFromSummary(
	got summary.Summary,
	usefulParamObligation func(int) bool,
	usefulNormalReturnParam func(int) bool,
) factapply.CallOutcome {
	out := factapply.CallOutcome{}
	if len(got.Returns) != 0 {
		out.Results = make([]factapply.CallResult, len(got.Returns))
		for i, value := range got.Returns {
			out.Results[i] = factapply.CallResult{Index: i, Value: value}
		}
	}
	for i, value := range got.ParamObligations {
		if usefulParamObligation == nil || !usefulParamObligation(i) {
			continue
		}
		out.ParamObligations = append(out.ParamObligations, factapply.CallParamObligation{
			ParamIndex: i,
			Value:      value,
		})
	}
	for i, value := range got.NormalReturnParams {
		if usefulNormalReturnParam == nil || !usefulNormalReturnParam(i) {
			continue
		}
		out.ParamPathRefinements = append(out.ParamPathRefinements, factapply.CallParamPathRefinement{
			Path:  pathdom.NewPlaceholder(i),
			Value: value,
		})
	}
	for i, condition := range got.NormalReturnParamConditions {
		value, ok := paramConditionValue(condition)
		if !ok {
			continue
		}
		out.ParamConditions = append(out.ParamConditions, factapply.CallParamCondition{
			ParamIndex: i,
			Value:      value,
		})
	}
	for _, equality := range got.NormalReturnParamEqualities {
		if equality.Left < 0 || equality.Right < 0 || equality.Left == equality.Right {
			continue
		}
		out.ParamPathRelations = append(out.ParamPathRelations, factapply.CallParamPathRelation{
			Kind:  factapply.CallPathRelationEqual,
			Left:  pathdom.NewPlaceholder(equality.Left),
			Right: pathdom.NewPlaceholder(equality.Right),
		})
	}
	out.NormalReturnFacts = cloneNormalReturnFacts(got.NormalReturnFacts)
	if len(got.ReturnConditionParamRefinements) != 0 {
		out.ReturnConditionRefinements = make([]factapply.CallReturnConditionRefinement, len(got.ReturnConditionParamRefinements))
		for i, refinement := range got.ReturnConditionParamRefinements {
			out.ReturnConditionRefinements[i] = factapply.CallReturnConditionRefinement{
				ReturnIndex: refinement.ReturnIndex,
				ReturnValue: refinement.ReturnValue,
				Target:      copyPath(refinement.Target),
				Value:       refinement.Value,
			}
		}
	}
	if len(got.ReturnPresenceRelations) != 0 {
		out.ReturnPresenceRelations = make([]factapply.CallReturnPresenceRelation, len(got.ReturnPresenceRelations))
		for i, relation := range got.ReturnPresenceRelations {
			out.ReturnPresenceRelations[i] = factapply.CallReturnPresenceRelation{
				TriggerIndex:    relation.TriggerIndex,
				TriggerPresence: relation.TriggerPresence,
				TargetIndex:     relation.TargetIndex,
				TargetPresence:  relation.TargetPresence,
			}
		}
	}
	return out
}

func memberCallParamObligations(
	ctx transfer.NodeContext,
	site factflow.CallSite,
	got summary.Summary,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) []factapply.CallParamObligation {
	if ctx.Registry == nil || sources == nil || len(got.ParamMemberCallObligations) == 0 {
		return nil
	}
	args := site.ArgumentSources()
	if len(args) == 0 {
		return nil
	}
	var out []factapply.CallParamObligation
	for _, obligation := range got.ParamMemberCallObligations {
		if obligation.ReceiverParam < 0 || obligation.ReceiverParam >= len(args) ||
			obligation.ArgParam < 0 || obligation.ArgParam >= len(args) ||
			obligation.MemberParamIndex < 0 || obligation.Member == "" {
			continue
		}
		receiverValue, ok := sources.ValueOfSource(ctx.Point, args[obligation.ReceiverParam], in, read)
		if !ok {
			continue
		}
		receiverType, ok := typeFromValue(ctx.Registry, receiverValue)
		if !ok {
			continue
		}
		memberType, status := typecall.MemberCall(receiverType, obligation.Member)
		if status != typecall.MemberCallOK {
			continue
		}
		callable, ok := typecall.Callable(memberType)
		if !ok || callable == nil || obligation.MemberParamIndex >= len(callable.Params) {
			continue
		}
		want := callable.Params[obligation.MemberParamIndex].Type
		if want == nil || typ.IsAny(want) || typ.IsUnknown(want) || refinement.ContainsFreeTypeParam(want) {
			continue
		}
		value := typevalue.WithWitness(ctx.Registry, typevalue.FromType(ctx.Registry, want), want)
		if !summary.UsefulParamObligation(ctx.Registry, value) {
			continue
		}
		out = append(out, factapply.CallParamObligation{
			ParamIndex: obligation.ArgParam,
			Value:      value,
		})
	}
	return out
}

func functionTypeParamObligations(reg *axis.Registry, site factflow.CallSite, fn *typ.Function) []factapply.CallParamObligation {
	if reg == nil || fn == nil || len(fn.Params) == 0 {
		return nil
	}
	args := site.ArgumentSources()
	limit := len(args)
	if limit > len(fn.Params) {
		limit = len(fn.Params)
	}
	var out []factapply.CallParamObligation
	for i := 0; i < limit; i++ {
		want := fn.Params[i].Type
		if want == nil || typ.IsAny(want) || typ.IsUnknown(want) || refinement.ContainsFreeTypeParam(want) {
			continue
		}
		value := typevalue.WithWitness(reg, typevalue.FromType(reg, want), want)
		if !summary.UsefulParamObligation(reg, value) {
			continue
		}
		out = append(out, factapply.CallParamObligation{
			ParamIndex: i,
			Value:      value,
		})
	}
	return out
}

func cloneNormalReturnFacts(in callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	out := callboundary.NormalReturnFacts{}
	if len(in.PathRefinements) != 0 {
		out.PathRefinements = make([]callboundary.PathValueFact, len(in.PathRefinements))
		for i, fact := range in.PathRefinements {
			fact.Path = copyPath(fact.Path)
			out.PathRefinements[i] = fact
		}
	}
	if len(in.PathStaticMembers) != 0 {
		out.PathStaticMembers = make([]callboundary.PathStaticMemberFact, len(in.PathStaticMembers))
		for i, fact := range in.PathStaticMembers {
			fact.Path = copyPath(fact.Path)
			out.PathStaticMembers[i] = fact
		}
	}
	if len(in.DynamicIndexFacts) != 0 {
		out.DynamicIndexFacts = make([]callboundary.DynamicIndexFact, len(in.DynamicIndexFacts))
		for i, fact := range in.DynamicIndexFacts {
			fact.Table = copyPath(fact.Table)
			out.DynamicIndexFacts[i] = fact
		}
	}
	if len(in.BranchProofs) != 0 {
		out.BranchProofs = make([]callboundary.BranchProof, len(in.BranchProofs))
		for i, proof := range in.BranchProofs {
			proof.Path = copyPath(proof.Path)
			proof.Other = copyPath(proof.Other)
			out.BranchProofs[i] = proof
		}
	}
	if len(in.ChannelSelects) != 0 {
		out.ChannelSelects = make([]callboundary.ChannelSelectFact, len(in.ChannelSelects))
		for i, fact := range in.ChannelSelects {
			fact.Result = copyPath(fact.Result)
			fact.Case = copyPath(fact.Case)
			out.ChannelSelects[i] = fact
		}
	}
	if len(in.EffectDeltas) != 0 {
		out.EffectDeltas = make([]callboundary.EffectDelta, len(in.EffectDeltas))
		for i, delta := range in.EffectDeltas {
			delta.Target = copyPath(delta.Target)
			out.EffectDeltas[i] = delta
		}
	}
	if len(in.EscapeEvents) != 0 {
		out.EscapeEvents = make([]callboundary.EscapeEventFact, len(in.EscapeEvents))
		for i, event := range in.EscapeEvents {
			event.Target = copyPath(event.Target)
			out.EscapeEvents[i] = event
		}
	}
	return out
}

func paramConditionValue(condition summary.ParamCondition) (bool, bool) {
	switch condition {
	case summary.ParamConditionTruthy:
		return true, true
	case summary.ParamConditionFalsy:
		return false, true
	default:
		return false, false
	}
}

func copyPath(p pathdom.Path) pathdom.Path {
	if len(p.Segments) == 0 {
		return p
	}
	out := p
	out.Segments = append(p.Segments[:0:0], p.Segments...)
	return out
}

// ByCalleeIdentity maps direct callee symbols to summary keys. Mutable callee
// paths are intentionally not resolved here; path calls must go through current
// value identity so reassignments and non-dominating writes stay sound.
func ByCalleeIdentity(symbolKeys map[symbol.ID]summary.SummaryKey) KeyFunc {
	clonedSymbols := make(map[symbol.ID]summary.SummaryKey, len(symbolKeys))
	for id, key := range symbolKeys {
		clonedSymbols[id] = key
	}
	return func(_ transfer.NodeContext, site factflow.CallSite) (summary.SummaryKey, bool) {
		if key, ok := clonedSymbols[site.CalleeSymbol()]; ok {
			return key, true
		}
		return summary.SummaryKey{}, false
	}
}
