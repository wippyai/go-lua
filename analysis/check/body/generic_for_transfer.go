package body

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factquery"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/projection"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func genericForNodeTransfer(
	base transfer.NodeTransfer,
	genericFors map[cfg.Point]GenericForFact,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	symbolTypes map[symbol.ID]typ.Type,
	signatures signaturelookup.Source,
	signatureID *signatureIdentityResolver,
	typeResolver *typeresolve.Resolver,
	typeValues *typevalue.Cache,
	callOutcome callpayload.CallOutcomeProvider,
	ks *keyspace.KeySpace,
	resolver *visibility.Resolver,
	sourcePath func(ast.Expr) (pathdom.Path, bool),
) transfer.NodeTransfer {
	expressionRefinements := sourcevalue.NewExpressionRefinements(facts.ExpressionRefinements())
	var refinedSourceRegistry *axis.Registry
	var refinedSources sourcevalue.SourceValues
	return func(ctx transfer.NodeContext, in state.State) state.State {
		out := in
		if base != nil {
			out = base(ctx, in)
		}
		if sources == nil || signatureID == nil {
			return out
		}
		fact, ok := genericFors[ctx.Point]
		if !ok || fact.Role != GenericForRoleVariable || !fact.HasSymbols ||
			fact.VariableIndex < 0 || fact.VariableIndex >= len(fact.Symbols) {
			return out
		}
		target := fact.Symbols[fact.VariableIndex]
		if target == 0 {
			return out
		}
		targetPath := pathdom.Path{Symbol: target}
		if resolver != nil {
			if targetKey, ok := visibility.AddressAt(resolver, ctx.Point, targetPath).VisibleStateKey(); ok {
				out = out.ClearKeyMembershipsForPath(targetKey)
			}
		}
		if refinedSources == nil || refinedSourceRegistry != ctx.Registry {
			refinedSources = expressionRefinements.Bind(ctx.Registry, sources)
			refinedSourceRegistry = ctx.Registry
		}
		boundSources := refinedSources
		if value, ok := genericForVariableValue(ctx, typeValues, fact, facts, boundSources, symbolTypes, signatures, signatureID, typeResolver, callOutcome, ks, resolver, sourcePath, in); ok {
			out = out.WriteValue(ctx.Registry, key.SymbolValue(target), value)
		}
		return genericForKeyMembershipTransfer(ctx, typeValues, fact, facts, signatures, signatureID, resolver, out, targetPath)
	}
}

func genericForVariableValue(
	ctx transfer.NodeContext,
	typeValues *typevalue.Cache,
	generic GenericForFact,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	symbolTypes map[symbol.ID]typ.Type,
	signatures signaturelookup.Source,
	signatureID *signatureIdentityResolver,
	typeResolver *typeresolve.Resolver,
	callOutcome callpayload.CallOutcomeProvider,
	ks *keyspace.KeySpace,
	resolver *visibility.Resolver,
	sourcePath func(ast.Expr) (pathdom.Path, bool),
	in state.State,
) (product.Value, bool) {
	if len(generic.Sources) == 0 {
		return product.Value{}, false
	}
	source := generic.Sources[0]
	if source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint {
		if value, ok := genericForSourceIteratorFunctionVariableValue(ctx, typeValues, generic, source, sourcePath, in); ok {
			return value, true
		}
		return product.Value{}, false
	}
	site, ok := facts.CallSiteView(source.CallPoint)
	if !ok {
		return product.Value{}, false
	}
	name, ok := signatureID.nameForIndexedIteratorCallSiteView(site)
	if !ok {
		return genericForFunctionIteratorVariableValue(ctx, typeValues, generic, source, site, callOutcome, in)
	}
	iter, ok := genericForIterator(name, signatures)
	if !ok {
		return genericForFunctionIteratorVariableValue(ctx, typeValues, generic, source, site, callOutcome, in)
	}
	sourceIndex, ok := effect.ResolveParamIndex(iter.Source, site.ArgumentSourceCount())
	if !ok {
		return product.Value{}, false
	}
	argSource, ok := site.ArgumentSourceAt(sourceIndex)
	if !ok {
		return product.Value{}, false
	}
	assertedSourceType, hasAssertedSourceType := genericForAssertedIteratorSourceType(generic, sourceIndex, typeResolver)
	preferIteratorSourceContract := hasAssertedSourceType
	if !hasAssertedSourceType {
		if recovered, ok := genericForDeclaredPathIteratorSourceType(argSource, facts, resolver, symbolTypes); ok {
			if genericForIteratorSourceTypeProjects(iter, generic.VariableIndex, recovered) {
				assertedSourceType = recovered
				hasAssertedSourceType = true
				preferIteratorSourceContract = true
			}
		}
	}
	if !hasAssertedSourceType {
		if recovered, ok := genericForSolvedPathIteratorSourceType(ctx, typeValues, argSource, facts, resolver, in); ok {
			if genericForIteratorSourceTypeProjects(iter, generic.VariableIndex, recovered) {
				assertedSourceType = recovered
				hasAssertedSourceType = true
			}
		}
	}
	if !hasAssertedSourceType {
		if recovered, ok := genericForDominatingPathIteratorSourceType(ctx, typeValues, argSource, facts, resolver, sources); ok {
			assertedSourceType = recovered
			hasAssertedSourceType = true
		}
	}
	sourceValue, ok := sources.ValueOfSource(ctx.Point, argSource, in, ctx.Read)
	if !ok && !hasAssertedSourceType {
		return product.Value{}, false
	}
	if preferIteratorSourceContract {
		if value, ok := luasourcevalue.IteratorVariableValue(ctx.Registry, typeValues, iter, generic.VariableIndex, sourceValue, assertedSourceType, true); ok {
			return value, true
		}
	}
	if ok {
		if value, ok := genericForLiteralContainerVariableValue(ctx, typeValues, iter, generic.VariableIndex, facts, sources, sourceValue, argSource, ks, in); ok {
			return value, true
		}
	} else {
		sourceValue = product.Top()
	}
	if value, ok := luasourcevalue.IteratorVariableValue(ctx.Registry, typeValues, iter, generic.VariableIndex, sourceValue, assertedSourceType, hasAssertedSourceType); ok {
		if genericForValueIsTopLike(ctx.Registry, typeValues, value) {
			if precise, ok := genericForPathStaticMemberContainerVariableValue(ctx, resolver, typeValues, iter, generic.VariableIndex, facts, argSource, in); ok {
				return precise, true
			}
			if precise, ok := genericForDynamicIndexContainerVariableValue(ctx, resolver, typeValues, iter, generic.VariableIndex, facts, argSource, in); ok {
				return precise, true
			}
		}
		return value, true
	}
	if value, ok := genericForPathStaticMemberContainerVariableValue(ctx, resolver, typeValues, iter, generic.VariableIndex, facts, argSource, in); ok {
		return value, true
	}
	if value, ok := genericForDynamicIndexContainerVariableValue(ctx, resolver, typeValues, iter, generic.VariableIndex, facts, argSource, in); ok {
		return value, true
	}
	return product.Value{}, false
}

func genericForSourceIteratorFunctionVariableValue(
	ctx transfer.NodeContext,
	typeValues *typevalue.Cache,
	generic GenericForFact,
	source sourceprovenance.ASTSource,
	sourcePath func(ast.Expr) (pathdom.Path, bool),
	in state.State,
) (product.Value, bool) {
	if source.Kind != sourceprovenance.SourceExpression || source.Expr == nil || sourcePath == nil {
		return product.Value{}, false
	}
	p, ok := sourcePath(source.Expr)
	if !ok || p.Symbol == 0 || len(p.Segments) != 0 {
		return product.Value{}, false
	}
	sourceValue := in.ReadValue(ctx.Registry, key.SymbolValue(p.Symbol))
	if product.Equal(ctx.Registry, sourceValue, product.Bottom(ctx.Registry)) {
		return product.Value{}, false
	}
	return genericForIteratorFunctionResultValue(ctx, typeValues, generic.VariableIndex, sourceValue)
}

func genericForPathStaticMemberContainerVariableValue(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	typeValues *typevalue.Cache,
	iter iteration.Iterator,
	variableIndex int,
	facts factflow.Facts,
	source factflow.ValueSource,
	in state.State,
) (product.Value, bool) {
	if ctx.Registry == nil || resolver == nil || typeValues == nil {
		return product.Value{}, false
	}
	if iter.Kind != iteration.IterateIndexed && iter.Kind != iteration.IterateKeyed {
		return product.Value{}, false
	}
	if variableIndex < 0 || variableIndex > 1 {
		return product.Value{}, false
	}
	sourcePath, ok := valueSourcePath(facts, resolver, source)
	if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return product.Value{}, false
	}
	sourceKey, ok := valueSourcePathKeyspaceKey(resolver, ctx.Point, sourcePath)
	if !ok {
		return product.Value{}, false
	}
	domain := product.Domain(ctx.Registry)
	joined := domain.Bottom()
	found := false
	in.ForEachPathStaticMember(func(memberKey keyspace.Key, value product.Value) bool {
		segments, ok := resolver.KeySpace().ExactRemainderAfterPrefix(memberKey, sourceKey)
		if !ok || len(segments) != 1 || !genericForStaticMemberSegmentMatchesIterator(iter, segments[0]) {
			return true
		}
		if presence.Equal(product.PresenceOf(value), presence.Absent()) {
			return true
		}
		if variableIndex == 0 {
			keyValue, ok := genericForStaticMemberKeyValue(ctx, typeValues, segments[0])
			if !ok {
				return true
			}
			value = keyValue
		}
		if domain.Equal(value, domain.Bottom()) {
			return true
		}
		if !found {
			joined = value
			found = true
			return true
		}
		joined = domain.Join(joined, value)
		return true
	})
	return joined, found
}

func genericForStaticMemberSegmentMatchesIterator(iter iteration.Iterator, seg segment.Segment) bool {
	switch iter.Kind {
	case iteration.IterateIndexed:
		return genericForDirectContainerSegment(iter, seg)
	case iteration.IterateKeyed:
		switch seg.Kind {
		case segment.SegmentField, segment.SegmentIndexString, segment.SegmentIndexInt:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func genericForStaticMemberKeyValue(ctx transfer.NodeContext, typeValues *typevalue.Cache, seg segment.Segment) (product.Value, bool) {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		if seg.Name == "" {
			return product.Value{}, false
		}
		return product.WithPresence(ctx.Registry, typeValues.FromTypeWithWitness(ctx.Registry, typ.LiteralString(seg.Name)), presence.Present()), true
	case segment.SegmentIndexInt:
		return product.WithPresence(ctx.Registry, typeValues.FromTypeWithWitness(ctx.Registry, typ.LiteralInt(int64(seg.Index))), presence.Present()), true
	default:
		return product.Value{}, false
	}
}

func genericForValueIsTopLike(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) bool {
	if reg == nil || typeValues == nil {
		return false
	}
	t, ok := typeValues.TypeOf(reg, value)
	return !ok || typ.IsAny(t) || typ.IsUnknown(t)
}

func genericForIteratorSourceTypeProjects(iter iteration.Iterator, variableIndex int, sourceType typ.Type) bool {
	if sourceType == nil {
		return false
	}
	switch variableIndex {
	case 0:
		_, ok := projection.KeyOf(sourceType)
		return ok
	case 1:
		_, ok := projection.ElementOf(sourceType)
		return ok
	default:
		return false
	}
}

func genericForKeyMembershipTransfer(
	ctx transfer.NodeContext,
	typeValues *typevalue.Cache,
	generic GenericForFact,
	facts factflow.Facts,
	signatures signaturelookup.Source,
	signatureID *signatureIdentityResolver,
	resolver *visibility.Resolver,
	out state.State,
	targetPath pathdom.Path,
) state.State {
	if resolver == nil || signatureID == nil || len(generic.Sources) == 0 || targetPath.Symbol == 0 {
		return out
	}
	source := generic.Sources[0]
	if source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint {
		return out
	}
	site, ok := facts.CallSiteView(source.CallPoint)
	if !ok {
		return out
	}
	iter, ok := genericForKnownIterator(ctx, site, signatures, signatureID)
	if !ok {
		return out
	}
	sourceIndex, ok := effect.ResolveParamIndex(iter.Source, site.ArgumentSourceCount())
	if !ok {
		return out
	}
	argSource, ok := site.ArgumentSourceAt(sourceIndex)
	if !ok {
		return out
	}
	sourcePath, ok := valueSourcePath(facts, resolver, argSource)
	if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return out
	}
	targetKey, ok := visibility.AddressAt(resolver, ctx.Point, targetPath).VisibleStateKey()
	if !ok {
		return out
	}
	switch iter.Kind {
	case iteration.IterateKeyed:
		tableKey, ok := valueSourcePathStateKey(resolver, ctx.Point, sourcePath)
		if !ok {
			return out
		}
		if generic.VariableIndex == 0 {
			return out.AddPathKeyMembership(targetKey, tableKey)
		}
		if generic.VariableIndex != 1 || len(generic.Symbols) == 0 || generic.Symbols[0] == 0 {
			return out
		}
		containerKey, ok := valueSourcePathKeyspaceKey(resolver, ctx.Point, sourcePath)
		if !ok {
			return out
		}
		keyStateKey, ok := visibility.AddressAt(resolver, ctx.Point, pathdom.Path{Symbol: generic.Symbols[0]}).VisibleStateKey()
		if !ok {
			return out
		}
		return out.AddDynamicIndexReadOrigin(targetKey, containerKey, keyStateKey)
	case iteration.IterateIndexed:
		if generic.VariableIndex != 1 {
			return out
		}
		containerKey, ok := valueSourcePathKeyspaceKey(resolver, ctx.Point, sourcePath)
		if !ok {
			return out
		}
		out = genericForCopyIndexedStaticMemberDescendants(ctx, resolver, iter, out, containerKey, targetPath)
		for _, site := range indexedContainerDynamicValueOriginSites(ctx.Registry, typeValues, out, containerKey) {
			out = out.AddDynamicIndexValueOrigin(targetKey, containerKey, site)
		}
		for _, table := range indexedContainerCommonKeyMembershipTables(ctx.Registry, typeValues, out, containerKey) {
			out = out.AddPathKeyMembership(targetKey, table)
		}
		return out
	default:
		return out
	}
}

type genericForStaticDescendantCopy struct {
	value product.Value
	seen  map[segment.Segment]struct{}
}

func genericForCopyIndexedStaticMemberDescendants(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	iter iteration.Iterator,
	out state.State,
	containerKey keyspace.Key,
	targetPath pathdom.Path,
) state.State {
	if ctx.Registry == nil || resolver == nil || targetPath.Symbol == 0 || iter.Kind != iteration.IterateIndexed {
		return out
	}
	ks := resolver.KeySpace()
	elements := make(map[segment.Segment]struct{})
	copies := make(map[keyspace.Key]genericForStaticDescendantCopy)
	out.ForEachPathStaticMember(func(memberKey keyspace.Key, value product.Value) bool {
		remainder, ok := ks.ExactRemainderAfterPrefix(memberKey, containerKey)
		if !ok || len(remainder) == 0 || !genericForDirectContainerSegment(iter, remainder[0]) {
			return true
		}
		element := remainder[0]
		elements[element] = struct{}{}
		if len(remainder) == 1 || presence.Equal(product.PresenceOf(value), presence.Absent()) {
			return true
		}
		suffixKey, ok := ks.FromRootlessSuffix(remainder[1:])
		if !ok {
			return true
		}
		copy := copies[suffixKey]
		if copy.seen == nil {
			copy.seen = make(map[segment.Segment]struct{}, 1)
			copy.value = value
		} else if _, seen := copy.seen[element]; !seen {
			copy.value = product.Join(ctx.Registry, copy.value, value)
		} else if refined := product.Meet(ctx.Registry, copy.value, value); !product.Equal(ctx.Registry, refined, product.Bottom(ctx.Registry)) {
			copy.value = refined
		}
		copy.seen[element] = struct{}{}
		copies[suffixKey] = copy
		return true
	})
	if len(elements) == 0 || len(copies) == 0 {
		return out
	}
	for suffixKey, copy := range copies {
		if len(copy.seen) != len(elements) {
			continue
		}
		suffix, ok := ks.SuffixSegmentsView(suffixKey)
		if !ok {
			continue
		}
		targetMember := targetPath.AppendSegments(suffix)
		targetKey, ok := visibility.AddressAt(resolver, ctx.Point, targetMember).VisibleLocalKeyspaceKey()
		if !ok {
			continue
		}
		out = out.WriteLocalPathStaticMember(targetKey, copy.value)
	}
	return out
}

func genericForKnownIterator(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	signatures signaturelookup.Source,
	signatureID *signatureIdentityResolver,
) (iteration.Iterator, bool) {
	if signatureID == nil {
		return iteration.Iterator{}, false
	}
	if name, ok := signatureID.nameForCallSiteView(ctx, site); ok {
		if iter, ok := genericForIterator(name, signatures); ok {
			return iter, true
		}
	}
	if name, ok := signatureID.nameForIndexedIteratorCallSiteView(site); ok {
		if iter, ok := genericForIterator(name, signatures); ok {
			return iter, true
		}
	}
	return iteration.Iterator{}, false
}

func indexedContainerDynamicValueOriginSites(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	out state.State,
	containerKey keyspace.Key,
) []dynamicindex.Site {
	seen := make(map[dynamicindex.Site]struct{})
	var sites []dynamicindex.Site
	if out.ForEachDynamicIndexFact(func(dynamicKey dynamicindex.Key, fact dynamicindex.Fact) bool {
		if dynamicKey.Table != containerKey || !genericForIndexedDynamicIndexFact(reg, typeValues, fact) {
			return true
		}
		if presence.Equal(product.PresenceOf(fact.Value), presence.Absent()) {
			return true
		}
		if len(out.DynamicIndexValueKeyMembershipTables(containerKey, dynamicKey.Site)) == 0 {
			return true
		}
		if _, ok := seen[dynamicKey.Site]; ok {
			return true
		}
		seen[dynamicKey.Site] = struct{}{}
		sites = append(sites, dynamicKey.Site)
		return true
	}) {
		return nil
	}
	return sites
}

func indexedContainerCommonKeyMembershipTables(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	out state.State,
	containerKey keyspace.Key,
) []pathaddr.StateKey {
	if tables := out.DynamicIndexAllValuesKeyMembershipTables(containerKey); len(tables) != 0 {
		return tables
	}
	common := map[pathaddr.StateKey]struct{}{}
	foundValueSource := false
	aborted := false
	if out.ForEachDynamicIndexFact(func(dynamicKey dynamicindex.Key, fact dynamicindex.Fact) bool {
		if dynamicKey.Table != containerKey || !genericForIndexedDynamicIndexFact(reg, typeValues, fact) {
			return true
		}
		if presence.Equal(product.PresenceOf(fact.Value), presence.Absent()) {
			return true
		}
		tables := out.DynamicIndexValueKeyMembershipTables(containerKey, dynamicKey.Site)
		if len(tables) == 0 {
			aborted = true
			return false
		}
		if !foundValueSource {
			for _, table := range tables {
				common[table] = struct{}{}
			}
			foundValueSource = true
			return true
		}
		next := make(map[pathaddr.StateKey]struct{}, len(common))
		for _, table := range tables {
			if _, ok := common[table]; ok {
				next[table] = struct{}{}
			}
		}
		common = next
		if len(common) == 0 {
			aborted = true
			return false
		}
		return true
	}) {
		return nil
	}
	if aborted {
		return nil
	}
	if !foundValueSource {
		return nil
	}
	outTables := make([]pathaddr.StateKey, 0, len(common))
	for table := range common {
		outTables = append(outTables, table)
	}
	return outTables
}

// genericForFunctionIteratorVariableValue types a generic-for loop variable when
// the iterator source is a call returning a stateless iterator function. The Lua
// protocol calls that function each iteration and binds the loop variables to its
// results; the loop continues while the first result is non-nil. The variable at
// generic.VariableIndex therefore takes the iterator function's matching result
// type, with the first result narrowed to its non-nil form for the in-body value.
func genericForFunctionIteratorVariableValue(
	ctx transfer.NodeContext,
	typeValues *typevalue.Cache,
	generic GenericForFact,
	source sourceprovenance.ASTSource,
	site factflow.CallSiteView,
	callOutcome callpayload.CallOutcomeProvider,
	in state.State,
) (product.Value, bool) {
	if !source.HasCallPoint || source.CallPoint == 0 || callOutcome == nil || ctx.Read == nil {
		return product.Value{}, false
	}
	callCtx := transfer.NodeContext{
		Graph:    ctx.Graph,
		Registry: ctx.Registry,
		Point:    source.CallPoint,
		Read:     ctx.Read,
	}
	if ctx.Graph != nil {
		callCtx.Node = ctx.Graph.Node(source.CallPoint)
	}
	outcome := callOutcome(callCtx, site, ctx.Read(source.CallPoint), ctx.Read)
	var callResult product.Value
	found := false
	for _, result := range outcome.Results {
		if result.Index == 0 {
			callResult = result.Value
			found = true
			break
		}
	}
	if !found {
		return product.Value{}, false
	}
	return genericForIteratorFunctionResultValue(ctx, typeValues, generic.VariableIndex, callResult)
}

func genericForIteratorFunctionResultValue(
	ctx transfer.NodeContext,
	typeValues *typevalue.Cache,
	variableIndex int,
	iterValue product.Value,
) (product.Value, bool) {
	iterType, ok := typevalue.WitnessOf(ctx.Registry, iterValue)
	if !ok {
		return product.Value{}, false
	}
	iterFunc, ok := iterType.(*typ.Function)
	if !ok || typ.IsAny(iterType) || typ.IsUnknown(iterType) {
		return product.Value{}, false
	}
	if variableIndex < 0 || variableIndex >= len(iterFunc.Returns) {
		return product.Value{}, false
	}
	resultType := iterFunc.Returns[variableIndex]
	if resultType == nil || typ.IsAny(resultType) || typ.IsUnknown(resultType) {
		return product.Value{}, false
	}
	if variableIndex == 0 {
		if optional, ok := resultType.(*typ.Optional); ok && optional.Inner != nil {
			resultType = optional.Inner
		}
	}
	return typeValues.FromTypeWithWitness(ctx.Registry, resultType), true
}

func genericForIterator(name string, signatures signaturelookup.Source) (iteration.Iterator, bool) {
	if sig, ok := signatures.Lookup(name); ok {
		return iteration.ActiveIterator(sig.Effect.Labels)
	}
	switch name {
	case "pairs":
		return iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateKeyed}, true
	case "ipairs":
		return iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateIndexed}, true
	default:
		return iteration.Iterator{}, false
	}
}

func genericForLiteralContainerVariableValue(
	ctx transfer.NodeContext,
	typeValues *typevalue.Cache,
	iter iteration.Iterator,
	variableIndex int,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	sourceValue product.Value,
	source factflow.ValueSource,
	ks *keyspace.KeySpace,
	in state.State,
) (product.Value, bool) {
	if variableIndex != 1 || iter.Kind != iteration.IterateIndexed || !source.HasExpr {
		if value, ok := genericForHeapContainerVariableValue(ctx, typeValues, iter, variableIndex, ks, sourceValue, in); ok {
			return value, true
		}
		return product.Value{}, false
	}
	if value, ok := genericForHeapContainerVariableValue(ctx, typeValues, iter, variableIndex, ks, sourceValue, in); ok {
		return value, true
	}
	literal, ok := facts.ObjectLiteral(source.ExprRef)
	if !ok {
		return product.Value{}, false
	}
	var out product.Value
	seen := false
	for _, entry := range literal.Entries() {
		if !genericForDirectContainerElement(iter, entry) {
			continue
		}
		value, ok := sources.ValueOfSource(ctx.Point, entry.Source(), in, ctx.Read)
		if !ok {
			continue
		}
		if !seen {
			out = value
			seen = true
			continue
		}
		out = product.Join(ctx.Registry, out, value)
	}
	if source, ok := literal.ListElementSource(); ok {
		value, ok := sources.ValueOfSource(ctx.Point, source, in, ctx.Read)
		if ok {
			if !seen {
				out = value
				seen = true
			} else {
				out = product.Join(ctx.Registry, out, value)
			}
		}
	}
	return out, seen
}

func genericForHeapContainerVariableValue(
	ctx transfer.NodeContext,
	typeValues *typevalue.Cache,
	iter iteration.Iterator,
	variableIndex int,
	ks *keyspace.KeySpace,
	sourceValue product.Value,
	in state.State,
) (product.Value, bool) {
	id, ok := product.Get(ctx.Registry, sourceValue, identity.Key).ID()
	if !ok {
		return product.Value{}, false
	}
	object := in.ReadHeapTableObject(ctx.Registry, id)
	root := object.Root()
	rootID, ok := product.Get(ctx.Registry, root, identity.Key).ID()
	if !ok || rootID != id || product.Equal(ctx.Registry, product.Meet(ctx.Registry, root, sourceValue), product.Bottom(ctx.Registry)) {
		return product.Value{}, false
	}
	if variableIndex == 1 && (iter.Kind == iteration.IterateIndexed || iter.Kind == iteration.IterateKeyed) {
		if value, ok := genericForHeapStaticMemberVariableValue(ctx, iter, ks, object); ok {
			return value, true
		}
	}
	return genericForDynamicIndexFactsVariableValue(ctx, typeValues, iter, variableIndex, object.DynamicIndexFacts())
}

func genericForHeapStaticMemberVariableValue(ctx transfer.NodeContext, iter iteration.Iterator, ks *keyspace.KeySpace, object heapidentity.TableObject) (product.Value, bool) {
	var out product.Value
	seen := false
	for key, value := range object.StaticMembers() {
		segs, ok := ks.SuffixSegmentsView(key)
		if !ok || len(segs) != 1 || !genericForDirectContainerSegment(iter, segs[0]) {
			continue
		}
		if presence.Equal(product.PresenceOf(value), presence.Absent()) {
			continue
		}
		if !seen {
			out = value
			seen = true
			continue
		}
		out = product.Join(ctx.Registry, out, value)
	}
	return out, seen
}

func genericForDynamicIndexFactsVariableValue(
	ctx transfer.NodeContext,
	typeValues *typevalue.Cache,
	iter iteration.Iterator,
	variableIndex int,
	facts map[dynamicindex.Key]dynamicindex.Fact,
) (product.Value, bool) {
	if ctx.Registry == nil || typeValues == nil || len(facts) == 0 {
		return product.Value{}, false
	}
	domain := product.Domain(ctx.Registry)
	joined := domain.Bottom()
	found := false
	for _, fact := range facts {
		if !genericForDynamicIndexFactMatchesIterator(ctx.Registry, typeValues, iter, fact) {
			continue
		}
		if presence.Equal(product.PresenceOf(fact.Value), presence.Absent()) {
			continue
		}
		value := fact.Value
		if variableIndex == 0 {
			value = fact.KeyValue
		}
		if domain.Equal(value, domain.Bottom()) {
			continue
		}
		if !found {
			joined = value
			found = true
			continue
		}
		joined = domain.Join(joined, value)
	}
	return joined, found
}

func genericForDynamicIndexContainerVariableValue(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	typeValues *typevalue.Cache,
	iter iteration.Iterator,
	variableIndex int,
	facts factflow.Facts,
	source factflow.ValueSource,
	in state.State,
) (product.Value, bool) {
	if ctx.Registry == nil || resolver == nil || typeValues == nil {
		return product.Value{}, false
	}
	if iter.Kind != iteration.IterateIndexed && iter.Kind != iteration.IterateKeyed {
		return product.Value{}, false
	}
	if variableIndex < 0 || variableIndex > 1 {
		return product.Value{}, false
	}
	sourcePath, ok := valueSourcePath(facts, resolver, source)
	if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return product.Value{}, false
	}
	tableKey, ok := valueSourcePathKeyspaceKey(resolver, ctx.Point, sourcePath)
	if !ok {
		return product.Value{}, false
	}
	if in.ForEachDynamicIndexFact(func(_ dynamicindex.Key, _ dynamicindex.Fact) bool {
		return false
	}) {
		return product.Value{}, false
	}
	domain := product.Domain(ctx.Registry)
	joined := domain.Bottom()
	found := false
	if in.ForEachDynamicIndexFact(func(key dynamicindex.Key, fact dynamicindex.Fact) bool {
		if key.Table != tableKey || !genericForDynamicIndexFactMatchesIterator(ctx.Registry, typeValues, iter, fact) {
			return true
		}
		if presence.Equal(product.PresenceOf(fact.Value), presence.Absent()) {
			return true
		}
		value := fact.Value
		if variableIndex == 0 {
			value = fact.KeyValue
		}
		if domain.Equal(value, domain.Bottom()) {
			return true
		}
		if !found {
			joined = value
			found = true
			return true
		}
		joined = domain.Join(joined, value)
		return true
	}) {
		return product.Value{}, false
	}
	if !found {
		return product.Value{}, false
	}
	return joined, true
}

func genericForDynamicIndexFactMatchesIterator(reg *axis.Registry, typeValues *typevalue.Cache, iter iteration.Iterator, fact dynamicindex.Fact) bool {
	if fact.Admission == dynamicindex.AdmissionRejected {
		return false
	}
	switch iter.Kind {
	case iteration.IterateIndexed:
		return genericForIndexedDynamicIndexFact(reg, typeValues, fact)
	case iteration.IterateKeyed:
		return true
	default:
		return false
	}
}

func genericForIndexedDynamicIndexFact(reg *axis.Registry, typeValues *typevalue.Cache, fact dynamicindex.Fact) bool {
	if fact.Admission == dynamicindex.AdmissionRejected {
		return false
	}
	keyType, ok := typeValues.TypeOf(reg, fact.KeyValue)
	return ok && typ.IsIntegerIndexType(keyType)
}

func genericForDirectContainerElement(iter iteration.Iterator, entry factflow.ObjectEntry) bool {
	segs := entry.Suffix().Segments
	if len(segs) != 1 {
		return false
	}
	return genericForDirectContainerSegment(iter, segs[0])
}

func genericForDirectContainerSegment(iter iteration.Iterator, seg segment.Segment) bool {
	switch iter.Kind {
	case iteration.IterateIndexed:
		return seg.Kind == segment.SegmentIndexInt
	default:
		return false
	}
}

func genericForAssertedIteratorSourceType(generic GenericForFact, sourceIndex int, resolver *typeresolve.Resolver) (typ.Type, bool) {
	if sourceIndex < 0 || resolver == nil {
		return nil, false
	}
	arg := genericForCallArgument(generic, sourceIndex)
	if arg == nil {
		return nil, false
	}
	return assertedIteratorSourceType(arg, resolver)
}

func genericForCallArgument(generic GenericForFact, sourceIndex int) ast.Expr {
	for _, expr := range generic.Exprs {
		call, ok := expr.(*ast.FuncCallExpr)
		if !ok || call == nil || sourceIndex >= len(call.Args) {
			continue
		}
		return call.Args[sourceIndex]
	}
	return nil
}

func genericForDominatingPathIteratorSourceType(
	ctx transfer.NodeContext,
	typeValues *typevalue.Cache,
	argSource factflow.ValueSource,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	sources sourcevalue.SourceValues,
) (typ.Type, bool) {
	if ctx.Graph == nil || ctx.Read == nil || sources == nil {
		return nil, false
	}
	p, ok := valueSourcePath(facts, resolver, argSource)
	if !ok || p.Symbol == 0 || len(p.Segments) == 0 {
		return nil, false
	}
	declaration, ok := factquery.DominatingPathRootDeclarationSource(ctx.Point, p, facts, ctx.Graph)
	if !ok {
		return nil, false
	}
	declState := ctx.Read(declaration.Point)
	rootValue, ok := sources.ValueOfSource(declaration.Point, declaration.Source, declState, ctx.Read)
	if !ok {
		return nil, false
	}
	rootType, ok := luasourcevalue.ObjectLiteralEntryType(ctx.Registry, typeValues, rootValue)
	if !ok {
		return nil, false
	}
	return luatypeprojection.ApplySegments(rootType, p.Segments)
}

func genericForDeclaredPathIteratorSourceType(argSource factflow.ValueSource, facts factflow.Facts, resolver *visibility.Resolver, symbolTypes map[symbol.ID]typ.Type) (typ.Type, bool) {
	p, ok := valueSourcePath(facts, resolver, argSource)
	if !ok || p.Symbol == 0 {
		return nil, false
	}
	rootType, ok := symbolTypes[p.Symbol]
	if !ok || rootType == nil {
		return nil, false
	}
	if len(p.Segments) == 0 {
		return rootType, true
	}
	return luatypeprojection.ApplySegments(rootType, p.Segments)
}

func genericForSolvedPathIteratorSourceType(
	ctx transfer.NodeContext,
	typeValues *typevalue.Cache,
	argSource factflow.ValueSource,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	in state.State,
) (typ.Type, bool) {
	if ctx.Registry == nil || typeValues == nil || resolver == nil {
		return nil, false
	}
	p, ok := valueSourcePath(facts, resolver, argSource)
	if !ok || p.Symbol == 0 {
		return nil, false
	}
	rootValue := in.ReadValue(ctx.Registry, key.SymbolValue(p.Symbol))
	if product.Equal(ctx.Registry, rootValue, product.Bottom(ctx.Registry)) {
		return nil, false
	}
	rootType, ok := typeValues.TypeOf(ctx.Registry, rootValue)
	if !ok || rootType == nil || typ.IsAny(rootType) || typ.IsUnknown(rootType) {
		return nil, false
	}
	if len(p.Segments) == 0 {
		return rootType, true
	}
	return luatypeprojection.ApplySegments(rootType, p.Segments)
}

func assertedIteratorSourceType(expr ast.Expr, resolver *typeresolve.Resolver) (typ.Type, bool) {
	switch expr := expr.(type) {
	case *ast.CastExpr:
		t, ok := resolver.Type(expr.Type)
		if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
			return nil, false
		}
		return t, true
	case *ast.NonNilAssertExpr:
		return assertedIteratorSourceType(expr.Expr, resolver)
	default:
		return nil, false
	}
}
