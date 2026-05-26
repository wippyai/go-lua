package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/cond"
	"github.com/wippyai/go-lua/compiler/check/abstract/constprop"
	"github.com/wippyai/go-lua/compiler/check/abstract/decl"
	"github.com/wippyai/go-lua/compiler/check/abstract/predicate"
	"github.com/wippyai/go-lua/compiler/check/abstract/tblutil"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/calleffect"
	"github.com/wippyai/go-lua/compiler/check/domain/guard"
	"github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/domain/resolve"
	checkscope "github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

type assignmentPointEmitter struct {
	state         *assignmentExtractionState
	p             cfg.Point
	info          *cfg.AssignInfo
	sc            *checkscope.State
	keysCollector KeysCollectorFunc
	constResolver func(string) *flow.ConstValue
	values        []typ.Type
	valuesReady   bool
}

func newAssignmentPointEmitter(
	state *assignmentExtractionState,
	p cfg.Point,
	info *cfg.AssignInfo,
	sc *checkscope.State,
	keysCollector KeysCollectorFunc,
) *assignmentPointEmitter {
	return &assignmentPointEmitter{
		state:         state,
		p:             p,
		info:          info,
		sc:            sc,
		keysCollector: keysCollector,
		constResolver: predicate.BuildConstResolver(state.inputs, p),
	}
}

func (e *assignmentPointEmitter) emit() {
	if e == nil || e.state == nil || e.info == nil {
		return
	}
	if e.state.emitNumericForAssignment(e.p, e.info) {
		return
	}
	if e.state.emitGenericForAssignments(e.p, e.info, e.sc) {
		return
	}
	e.emitTargets()
	e.emitExpandingSourceSiblings()
}

func (e *assignmentPointEmitter) emitTargets() {
	for i, target := range e.info.Targets {
		source := e.info.SourceAt(i)
		switch target.Kind {
		case cfg.TargetIdent:
			e.emitIdentTarget(i, target, source)
		case cfg.TargetField:
			e.emitFieldTarget(i, target, source)
		case cfg.TargetIndex:
			e.emitIndexTarget(i, target, source)
		}
	}
}

func (e *assignmentPointEmitter) ensureValues() {
	if e.valuesReady {
		return
	}
	rhsOverlay := rhsSpecTypesAtAssignPoint(e.state.fc.Graph, e.info, e.p, e.state.overlayTypesAt(e.p), e.state.resolverWithSpec)
	rhsOverlay = enrichStructuredOverlayAtPoint(
		e.state.fc.Graph,
		e.state.idom,
		e.state.structuredWrites,
		e.p,
		rhsOverlay,
		assignmentSourceSymbols(e.info, e.state.bindings),
		e.state.resolverWithSpec,
		e.state.wrappedSynth,
	)
	e.values = expandedAssignValues(e.state.fc.API, e.info, e.p, rhsOverlay)
	e.valuesReady = true
}

func (e *assignmentPointEmitter) emitIdentTarget(i int, target cfg.AssignTarget, source ast.Expr) {
	if target.Name == "" {
		return
	}
	sym := target.Symbol
	assignedType := e.assignedIdentType(i, target, source)
	if assignedType == nil {
		assignedType = typ.Unknown
	}

	sourceInfo := e.assignmentSourceForIdent(i, source)
	sourceInfo = e.withAssignmentSourceProjection(i, sourceInfo)
	if link := e.predicateLinkForIdent(i); link != nil {
		e.state.inputs.PredicateLinks[predicate.LinkKey(target.Name, e.p)] = *link
	}
	e.emitKeysProvenance(i, sym)
	if src := e.containerElementSource(i); !src.IsZero() {
		sourceInfo = src
	}

	targetPath := constraint.Path{Root: resolve.RootName(e.state.fc.Graph, sym, target.Name), Symbol: sym}
	e.state.inputs.Assignments = append(e.state.inputs.Assignments, flow.UnifiedAssignment{
		Point:      e.p,
		TargetPath: targetPath,
		Type:       resolve.Ref(assignedType, e.sc),
		Source:     sourceInfo,
	})
	e.emitTableLiteralFields(sym, target.Name, source)
	e.emitVariantFieldOrigins(i, targetPath)
}

func (e *assignmentPointEmitter) assignedIdentType(i int, target cfg.AssignTarget, source ast.Expr) typ.Type {
	sym := target.Symbol
	assignedType := typ.Unknown
	if _, isFunctionLiteral := source.(*ast.FunctionExpr); isFunctionLiteral {
		if inputType := functionSignatureInputType(e.state.inputs, sym); inputType != nil {
			assignedType = inputType
		}
	}
	if typ.IsAbsentOrUnknown(assignedType) && e.info.IsLocal {
		switch {
		case e.state.inputs.AnnotatedVars != nil && e.state.inputs.AnnotatedVars[sym]:
			if dt, ok := e.state.inputs.DeclaredTypes[sym]; ok && dt != nil {
				assignedType = dt
			}
		case len(e.info.Sources) == 0:
			if t, ok := e.state.resolverWithSpec(e.p, sym); ok && t != nil && !isTopLikeResolvedAssignType(t) {
				assignedType = t
			}
		}
	}
	if typ.IsAbsentOrUnknown(assignedType) {
		assignedType = e.valueOrSynthAt(i, source)
		if value := assignValueAt(e.values, i); value != nil {
			assignedType = preferPreciseDirectSourceType(assignedType, source, e.p, e.sc, e.state.wrappedSynth, len(e.info.Targets) == 1)
		}
	}
	if refined := e.specReceiverCallValue(i, sym); refined != nil {
		assignedType = refined
	}
	if source != nil && e.info.IsLocal && !e.isAnnotated(sym) {
		if inferred := e.state.inferredTypes[sym]; typ.MorePrecise(inferred, assignedType) {
			assignedType = inferred
		}
	}
	if narrowed, ok := e.state.specNarrowed[sym]; ok {
		assignedType = narrowed
	}
	return assignedType
}

func (e *assignmentPointEmitter) valueOrSynthAt(i int, source ast.Expr) typ.Type {
	e.ensureValues()
	if value := assignValueAt(e.values, i); value != nil {
		return value
	}
	if e.state.wrappedSynth != nil && source != nil {
		return e.state.wrappedSynth(source, e.p)
	}
	return typ.Unknown
}

func (e *assignmentPointEmitter) specReceiverCallValue(i int, sym cfg.SymbolID) typ.Type {
	call, _ := e.info.CallForTarget(i)
	if call == nil || call.Receiver == nil || call.ReceiverSymbol == 0 {
		return nil
	}
	if _, narrowed := e.state.specNarrowed[call.ReceiverSymbol]; !narrowed {
		return nil
	}
	e.ensureValues()
	value := assignValueAt(e.values, i)
	if typ.IsAbsentOrUnknown(value) {
		return nil
	}
	e.state.specNarrowed[sym] = value
	return value
}

func (e *assignmentPointEmitter) isAnnotated(sym cfg.SymbolID) bool {
	return e.state.inputs.AnnotatedVars != nil && e.state.inputs.AnnotatedVars[sym]
}

func (e *assignmentPointEmitter) assignmentSourceForIdent(i int, source ast.Expr) flow.AssignmentSource {
	sourceInfo := flow.AssignmentSource{}
	if source != nil {
		sourceInfo = e.sourceInfoFromExpr(source)
	}
	if call, retIndex := e.info.CallForTarget(i); call != nil {
		if src := callReturnAssignmentSource(call, retIndex, e.constResolver, e.state.bindings); !src.IsZero() {
			sourceInfo = src
		}
	}
	return sourceInfo
}

func (e *assignmentPointEmitter) withAssignmentSourceProjection(i int, source flow.AssignmentSource) flow.AssignmentSource {
	projectionKind := assignmentSourceProjectionKind(source)
	if projectionKind == flow.AssignmentSourceProjectionNone {
		return source
	}
	projected := e.projectedAssignmentSourceType(i)
	if typ.IsAbsentOrUnknown(projected) {
		return source
	}
	if projectionKind == flow.AssignmentSourceProjectionCallable && !isCallableProjection(projected) {
		return source
	}
	source.ProjectionKind = projectionKind
	source.ProjectedType = resolve.Ref(projected, e.sc)
	return source
}

func assignmentSourceProjectionKind(source flow.AssignmentSource) flow.AssignmentSourceProjectionKind {
	switch source.Kind {
	case flow.AssignmentSourcePath:
		return flow.AssignmentSourceProjectionCallable
	case flow.AssignmentSourceCallReturn:
		return flow.AssignmentSourceProjectionCallReturn
	default:
		return flow.AssignmentSourceProjectionNone
	}
}

func (e *assignmentPointEmitter) projectedAssignmentSourceType(i int) typ.Type {
	e.ensureValues()
	if value := assignValueAt(e.values, i); value != nil {
		return value
	}
	return nil
}

func isCallableProjection(t typ.Type) bool {
	_, ok := unwrap.Optional(unwrap.Alias(t)).(*typ.Function)
	return ok
}

func (e *assignmentPointEmitter) sourceInfoFromExpr(source ast.Expr) flow.AssignmentSource {
	if sp := path.FromExprWithBindings(source, e.constResolver, e.state.bindings); !sp.IsEmpty() {
		return flow.AssignmentSource{
			Kind: flow.AssignmentSourcePath,
			Path: constraint.Path{
				Root:     resolve.RootNameFromBindings(e.state.bindings, sp.Symbol, sp.Root),
				Symbol:   sp.Symbol,
				Segments: sp.Segments,
			},
		}
	}
	attr, ok := source.(*ast.AttrGetExpr)
	if !ok {
		return flow.AssignmentSource{}
	}
	if src, ok := lengthIndexSourceFromAttr(attr, e.constResolver, e.state.bindings); ok {
		return src
	}
	if _, isStatic := staticSegmentForAttrKey(attr.Key, e.constResolver); isStatic {
		return flow.AssignmentSource{}
	}
	mp := path.FromExprWithBindings(attr.Object, e.constResolver, e.state.bindings)
	if mp.IsEmpty() || mp.Symbol == 0 {
		return flow.AssignmentSource{}
	}
	var keySym cfg.SymbolID
	var keyVar string
	if keyIdent, ok := attr.Key.(*ast.IdentExpr); ok && e.state.bindings != nil {
		keySym, _ = e.state.bindings.SymbolOf(keyIdent)
		keyVar = resolve.RootNameFromBindings(e.state.bindings, keySym, keyIdent.Value)
	}
	return flow.AssignmentSource{
		Kind: flow.AssignmentSourceMapElement,
		MapPath: constraint.Path{
			Root:     resolve.RootNameFromBindings(e.state.bindings, mp.Symbol, mp.Root),
			Symbol:   mp.Symbol,
			Segments: mp.Segments,
		},
		KeySymbol: keySym,
		KeyVar:    keyVar,
	}
}

func (e *assignmentPointEmitter) predicateLinkForIdent(i int) *flow.PredicateLink {
	callInfo, retIndex := e.info.CallForTarget(i)
	if callInfo == nil {
		return nil
	}
	link := cond.ExtractPredicateLinkFromCallInfo(
		callInfo,
		retIndex,
		e.p,
		e.sc,
		e.state.inputs,
		e.state.derived.TypeKeyRes,
		e.state.wrappedSynth,
		e.state.derived.RefinementBySym,
		e.state.symResolver,
		e.state.fc.Graph,
		e.state.fc.ModuleBindings,
	)
	if link == nil {
		return nil
	}
	e.addTypeCheckInversePredicate(callInfo, retIndex, i, link)
	return link
}

func (e *assignmentPointEmitter) addTypeCheckInversePredicate(callInfo *cfg.CallInfo, retIndex, targetIndex int, link *flow.PredicateLink) {
	if retIndex != 1 || !callInfo.IsTypeCheck || callInfo.Method != "is" || callInfo.Receiver == nil || e.state.derived.TypeKeyRes == nil {
		return
	}
	typeKey, ok := e.state.derived.TypeKeyRes(callInfo.TypeCheckName, e.sc)
	if !ok || typeKey.IsZero() {
		return
	}
	valIndex := targetIndex - retIndex
	if valIndex < 0 || valIndex >= len(e.info.Targets) {
		return
	}
	valTarget := e.info.Targets[valIndex]
	if valTarget.Kind != cfg.TargetIdent || valTarget.Name == "" || valTarget.Symbol == 0 {
		return
	}
	valuePath := constraint.Path{
		Root:   resolve.RootName(e.state.fc.Graph, valTarget.Symbol, valTarget.Name),
		Symbol: valTarget.Symbol,
	}
	link.OnFalsy = constraint.And(link.OnFalsy, constraint.FromConstraints(constraint.HasType{Path: valuePath, Type: typeKey}))
	link.OnTruthy = constraint.And(link.OnTruthy, constraint.FromConstraints(constraint.NotHasType{Path: valuePath, Type: typeKey}))
	link.OnTruthy = constraint.And(link.OnTruthy, constraint.FromConstraints(constraint.IsNil{Path: valuePath}))
	if e.typeCheckImpliesNonNil(callInfo.TypeCheckName) {
		link.OnFalsy = constraint.And(link.OnFalsy, constraint.FromConstraints(constraint.NotNil{Path: valuePath}))
	}
}

func (e *assignmentPointEmitter) typeCheckImpliesNonNil(name string) bool {
	if e.sc == nil {
		return false
	}
	resolved, ok := e.sc.LookupType(name)
	if !ok || resolved == nil {
		return false
	}
	checkType := resolve.Ref(resolved, e.sc)
	baseType := unwrap.Alias(checkType)
	return baseType != nil && !baseType.Kind().IsPlaceholder() && !unwrap.IsOptionalLike(baseType)
}

func (e *assignmentPointEmitter) emitKeysProvenance(i int, sym cfg.SymbolID) {
	call, retIndex := e.info.CallForTarget(i)
	if call == nil {
		return
	}
	var tableSym cfg.SymbolID
	calleeSymbols := callsite.CallableCalleeSymbolCandidates(call, e.state.fc.Graph, e.state.bindings, e.state.fc.ModuleBindings)
	if e.keysCollector != nil {
		tableSym = e.keysCollector(call, e.p, retIndex)
	}
	if tableSym == 0 && e.state.derived.RefinementBySym != nil {
		for _, calleeSym := range calleeSymbols {
			eff := e.state.derived.RefinementBySym(calleeSym)
			if eff == nil {
				continue
			}
			if paramIdx, keyReturnIdx, ok := eff.KeysCollectorInfo(); ok && retIndex == keyReturnIdx {
				tableSym = callsite.SymbolOrCreateFieldFromExpr(callsite.RuntimeArgAt(call, paramIdx), e.state.bindings)
				break
			}
		}
	}
	if tableSym == 0 {
		return
	}
	if e.state.inputs.KeysProvenance == nil {
		e.state.inputs.KeysProvenance = make(map[cfg.SymbolID]cfg.SymbolID)
	}
	e.state.inputs.KeysProvenance[sym] = tableSym
}

func (e *assignmentPointEmitter) containerElementSource(i int) flow.AssignmentSource {
	call, retIndex := e.info.CallForTarget(i)
	if call == nil {
		return flow.AssignmentSource{}
	}
	assignmentTypesResolver := resolve.BuildAssignmentTypeResolver(e.state.inputs)
	elemInfo := calleffect.ContainerElementReturnFromCall(
		call,
		e.p,
		e.state.wrappedSynth,
		e.state.resolverWithSpec,
		assignmentTypesResolver,
		e.state.fc.Graph,
		e.state.bindings,
		e.state.fc.ModuleBindings,
	)
	if elemInfo == nil || elemInfo.ReturnIndex != retIndex || !callsite.IsMethodCallInfo(call) || elemInfo.SourceRef.Index != 0 {
		return flow.AssignmentSource{}
	}
	recvPath := path.FromExprWithBindings(call.Receiver, e.constResolver, e.state.bindings)
	if recvPath.IsEmpty() || recvPath.Symbol == 0 {
		return flow.AssignmentSource{}
	}
	return flow.AssignmentSource{
		Kind: flow.AssignmentSourceContainerElement,
		ContainerPath: constraint.Path{
			Root:     resolve.RootNameFromBindings(e.state.bindings, recvPath.Symbol, recvPath.Root),
			Symbol:   recvPath.Symbol,
			Segments: recvPath.Segments,
		},
		ReturnIndex: elemInfo.ReturnIndex,
	}
}

func (e *assignmentPointEmitter) emitTableLiteralFields(sym cfg.SymbolID, name string, source ast.Expr) {
	tbl, ok := source.(*ast.TableExpr)
	if !ok || tblutil.TableHasFunctionField(tbl) {
		return
	}
	EmitTableLiteralFieldAssignments(
		tbl,
		sym,
		resolve.RootName(e.state.fc.Graph, sym, name),
		e.p,
		e.state.bindings,
		e.constResolver,
		e.state.wrappedSynth,
		e.sc,
		e.state.inputs,
	)
}

func (e *assignmentPointEmitter) emitFieldTarget(i int, target cfg.AssignTarget, source ast.Expr) {
	if target.BaseName == "" || len(target.FieldPath) == 0 {
		return
	}
	segments := make([]constraint.Segment, len(target.FieldPath))
	for j, field := range target.FieldPath {
		segments[j] = constraint.Segment{Kind: constraint.SegmentField, Name: field}
	}
	e.state.inputs.Assignments = append(e.state.inputs.Assignments, flow.UnifiedAssignment{
		Point: e.p,
		TargetPath: constraint.Path{
			Root:     resolve.RootName(e.state.fc.Graph, target.BaseSymbol, target.BaseName),
			Symbol:   target.BaseSymbol,
			Segments: segments,
		},
		Source: flow.AssignmentSource{
			Kind: flow.AssignmentSourcePath,
			Path: e.sourcePath(source),
		},
		Type: resolve.Ref(e.assignedAggregateTargetType(i, target, source), e.sc),
	})
}

func (e *assignmentPointEmitter) emitIndexTarget(i int, target cfg.AssignTarget, source ast.Expr) {
	basePath := e.indexBasePath(target)
	assignedType := e.assignedAggregateTargetType(i, target, source)
	keySeg, hasStaticKeySeg, keyType := e.indexKeyInfo(target)

	if basePath.IsEmpty() {
		if lifted, ok := buildLiftedDynamicMapMutatorAssignment(
			target,
			source,
			assignedType,
			e.p,
			e.sc,
			e.state.fc.Graph,
			e.state.fc.Evidence.Assignments,
			e.state.bindings,
			e.constResolver,
			e.state.wrappedSynth,
			e.state.resolverWithSpec,
			e.state.truthyGuards,
			e.state.typeGuards,
		); ok {
			e.state.inputs.MapMutatorAssignments = append(e.state.inputs.MapMutatorAssignments, lifted)
		}
		return
	}

	if !hasStaticKeySeg {
		e.emitDynamicIndexTarget(target, source, basePath, assignedType, keyType)
		return
	}
	e.emitStaticIndexTarget(source, basePath, keySeg, assignedType)
}

func (e *assignmentPointEmitter) assignedAggregateTargetType(i int, target cfg.AssignTarget, source ast.Expr) typ.Type {
	assignedType := typ.Unknown
	if expected := assignmentTargetExpectedType(target, e.p, e.state.wrappedSynth); expected != nil {
		if expectedType := synthAssignmentSourceWithExpected(e.state.fc.API, source, e.p, expected); expectedType != nil {
			assignedType = expectedType
		}
	}
	if typ.IsAbsentOrUnknown(assignedType) {
		e.ensureValues()
		if value := assignValueAt(e.values, i); !typ.IsAbsentOrUnknown(value) {
			assignedType = value
		} else if source != nil && e.state.wrappedSynth != nil {
			assignedType = e.state.wrappedSynth(source, e.p)
		}
	}
	if assignedType == nil {
		return typ.Unknown
	}
	return assignedType
}

func (e *assignmentPointEmitter) indexBasePath(target cfg.AssignTarget) constraint.Path {
	if target.BaseName != "" {
		return constraint.Path{
			Root:   resolve.RootNameFromBindings(e.state.bindings, target.BaseSymbol, target.BaseName),
			Symbol: target.BaseSymbol,
		}
	}
	if target.Base == nil {
		return constraint.Path{}
	}
	bp := path.FromExprWithBindings(target.Base, e.constResolver, e.state.bindings)
	if bp.IsEmpty() || bp.Symbol == 0 {
		return constraint.Path{}
	}
	return constraint.Path{
		Root:     resolve.RootNameFromBindings(e.state.bindings, bp.Symbol, bp.Root),
		Symbol:   bp.Symbol,
		Segments: bp.Segments,
	}
}

func (e *assignmentPointEmitter) indexKeyInfo(target cfg.AssignTarget) (constraint.Segment, bool, typ.Type) {
	switch k := target.Key.(type) {
	case *ast.StringExpr:
		seg, ok := path.StaticKeySegment(k)
		return seg, ok, nil
	case *ast.IdentExpr:
		if e.constResolver == nil {
			return constraint.Segment{}, false, nil
		}
		val := e.constResolver(k.Value)
		if val == nil {
			return constraint.Segment{}, false, nil
		}
		switch val.Kind {
		case flow.ConstString:
			seg, ok := path.StaticKeySegment(&ast.StringExpr{Value: val.Str})
			return seg, ok, nil
		case flow.ConstInt:
			return constraint.Segment{}, false, typ.Integer
		case flow.ConstFloat:
			return constraint.Segment{}, false, typ.Number
		}
	case *ast.NumberExpr:
		if val := constValueFromIndexKey(target.Key); val != nil {
			switch val.Kind {
			case flow.ConstInt:
				return constraint.Segment{}, false, typ.Integer
			case flow.ConstFloat:
				return constraint.Segment{}, false, typ.Number
			}
		}
	}
	return constraint.Segment{}, false, nil
}

func constValueFromIndexKey(key ast.Expr) *flow.ConstValue {
	return constprop.ConstValueFromExpr(key)
}

func (e *assignmentPointEmitter) emitDynamicIndexTarget(
	target cfg.AssignTarget,
	source ast.Expr,
	basePath constraint.Path,
	assignedType typ.Type,
	keyType typ.Type,
) {
	var keyVar string
	var keySym cfg.SymbolID
	if keyIdent, ok := target.Key.(*ast.IdentExpr); ok && e.state.bindings != nil {
		keySym, _ = e.state.bindings.SymbolOf(keyIdent)
		keyVar = resolve.RootNameFromBindings(e.state.bindings, keySym, keyIdent.Value)
	}
	if keyType == nil && target.Key != nil && e.state.wrappedSynth != nil {
		keyType = e.state.wrappedSynth(target.Key, e.p)
	}
	keyType = canonicalDynamicKeyType(keyType)

	valType := assignedType
	if source != nil && e.state.bindings != nil && e.state.truthyGuards != nil {
		if tbl, ok := source.(*ast.TableExpr); ok {
			valType = guardNarrowTableFieldsByPoint(valType, tbl, e)
		}
	}
	e.state.inputs.MapMutatorAssignments = append(e.state.inputs.MapMutatorAssignments, flow.MapMutatorAssignment{
		Point:     e.p,
		Target:    basePath,
		KeyVar:    keyVar,
		KeySymbol: keySym,
		KeyType:   keyType,
		ValuePath: mutatorValuePathFromExpr(source, e.p, e.constResolver, e.state.bindings, e.state.wrappedSynth),
		ValueType: resolve.Ref(valType, e.sc),
	})
}

func guardNarrowTableFieldsByPoint(valType typ.Type, tbl *ast.TableExpr, e *assignmentPointEmitter) typ.Type {
	return guard.NarrowTableFieldsByGuard(valType, tbl, e.p, e.state.bindings, e.state.truthyGuards, e.state.typeGuards)
}

func (e *assignmentPointEmitter) emitStaticIndexTarget(source ast.Expr, basePath constraint.Path, keySeg constraint.Segment, assignedType typ.Type) {
	e.state.inputs.Assignments = append(e.state.inputs.Assignments, flow.UnifiedAssignment{
		Point: e.p,
		TargetPath: constraint.Path{
			Root:     basePath.Root,
			Symbol:   basePath.Symbol,
			Segments: append(append([]constraint.Segment{}, basePath.Segments...), keySeg),
		},
		Source: flow.AssignmentSource{
			Kind: flow.AssignmentSourcePath,
			Path: e.sourcePath(source),
		},
		Type: resolve.Ref(assignedType, e.sc),
	})
}

func (e *assignmentPointEmitter) sourcePath(source ast.Expr) constraint.Path {
	if source == nil {
		return constraint.Path{}
	}
	sp := path.FromExprWithBindings(source, e.constResolver, e.state.bindings)
	if sp.IsEmpty() {
		return constraint.Path{}
	}
	return constraint.Path{
		Root:     resolve.RootNameFromBindings(e.state.bindings, sp.Symbol, sp.Root),
		Symbol:   sp.Symbol,
		Segments: sp.Segments,
	}
}

func (e *assignmentPointEmitter) emitExpandingSourceSiblings() {
	sourceCall, start := e.info.ExpandingSourceCall()
	if sourceCall == nil {
		return
	}
	count := len(e.info.Targets) - start
	symbols := make([]cfg.SymbolID, count)
	names := make([]string, count)
	types := make([]typ.Type, count)
	e.ensureValues()
	for i := 0; i < count; i++ {
		target, ok := e.info.TargetAt(start + i)
		if !ok {
			continue
		}
		if target.Kind == cfg.TargetIdent && target.Name != "" {
			names[i] = target.Name
			symbols[i] = target.Symbol
		}
		if value := assignValueAt(e.values, start+i); value != nil {
			types[i] = value
		}
	}
	correlations, coCorrelations, guardedCorrelations := extractCallCorrelations(
		sourceCall,
		e.state.wrappedSynth,
		e.p,
		e.state.resolverWithSpec,
		e.state.fc.Graph,
		e.state.bindings,
		e.state.fc.ModuleBindings,
	)
	for _, corr := range guardedCorrelations {
		decl.AddTypeKey(e.state.inputs, corr.TargetType)
	}
	sibling := &flow.SiblingAssignment{
		Symbols:             symbols,
		Names:               names,
		Types:               types,
		Correlations:        correlations,
		CoCorrelations:      coCorrelations,
		GuardedCorrelations: guardedCorrelations,
	}
	for i, sym := range symbols {
		if sym == 0 || names[i] == "" {
			continue
		}
		ver := e.state.fc.Graph.VisibleVersion(e.p, sym)
		if ver.ID == 0 {
			continue
		}
		e.state.inputs.SiblingAssignments[flow.SiblingKey{Symbol: sym, VersionID: ver.ID}] = sibling
	}
}
