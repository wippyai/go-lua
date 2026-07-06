// Package transferfacts lowers Lua semantic sidecars into generic transfer facts.
package transferfacts

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/expressionid"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Lower converts Lua semantic facts into the generic transfer fact DTOs consumed
// by the engine. It intentionally lowers only syntax facts already represented
// by factflow.Facts; higher semantic layers add branch, iterator, interproc,
// and diagnostic facts separately.
type Config struct {
	Registry      *axis.Registry
	Bindings      *bind.Result
	TypeResolver  *typeresolve.Resolver
	TypeValues    *typevalue.Cache
	ModuleExports importlookup.Source
	Metadata      cfgfacts.Metadata
	WIR           *wir.Body
}

type Lowered struct {
	Facts       factflow.Facts
	SymbolTypes map[symbol.ID]typ.Type
}

func Lower(result *semantics.Result, graph cfg.Graph, config Config) factflow.Facts {
	return LowerWithSidecars(result, graph, config).Facts
}

func LowerWithSidecars(result *semantics.Result, graph cfg.Graph, config Config) Lowered {
	if config.Registry == nil {
		panic("transferfacts: Config.Registry is required")
	}
	if graph == nil || (result == nil && config.WIR == nil) {
		return Lowered{Facts: factflow.NewFacts(factflow.FactsInput{})}
	}
	typeResolver := config.TypeResolver
	if typeResolver == nil {
		typeResolver = typeresolve.New(config.Bindings)
	}
	symbolTypes := lowerSymbolTypes(config.Bindings, graph, config.Metadata, result, typeResolver, config.ModuleExports)
	if config.WIR != nil {
		symbolTypes = mergeSymbolTypes(symbolTypes, lowerSymbolTypesFromWIR(config.WIR))
	}
	var declaredReturnLocalTypes map[symbol.ID]typ.Type
	var returnLocalObjectLiteralTypes map[symbol.ID]typ.Type
	if config.WIR != nil {
		declaredReturnLocalTypes = lowerDeclaredReturnLocalTypesFromWIR(config.Bindings, graph, config.WIR)
		returnLocalObjectLiteralTypes = lowerReturnLocalObjectLiteralTypesFromWIR(config.Bindings, graph, config.WIR)
	} else {
		declaredReturnLocalTypes = lowerDeclaredReturnLocalTypes(config.Bindings, graph, result, typeResolver)
		returnLocalObjectLiteralTypes = lowerReturnLocalObjectLiteralTypes(config.Bindings, graph, result, typeResolver)
	}
	symbolTypes = mergeSymbolTypes(symbolTypes, declaredReturnLocalTypes)
	expressionRefinements := make(map[factflow.ExprRef]factflow.ExpressionRefinement)
	l := lowerer{
		registry:                      config.Registry,
		bindings:                      config.Bindings,
		graph:                         graph,
		graphID:                       graph.ID(),
		typeResolver:                  typeResolver,
		typeValues:                    config.TypeValues,
		metadata:                      config.Metadata,
		wir:                           config.WIR,
		callPoints:                    semanticCallPointsByExpr(graph, result, config.WIR),
		wirCallPoints:                 callPointsByExpressionIDFromWIR(graph, config.WIR),
		symbolTypes:                   symbolTypes,
		declaredReturnLocalTypes:      declaredReturnLocalTypes,
		returnLocalObjectLiteralTypes: returnLocalObjectLiteralTypes,
		exprs:                         make(map[any]factflow.ExprRef),
		types:                         make(map[any]factflow.TypeRef),
		expressionValues:              make(map[factflow.ExprRef]product.Value),
		expressionOperations:          make(map[factflow.ExprRef]factflow.ExpressionOperation),
		expressionFunctions:           make(map[factflow.ExprRef]symbol.ID),
		expressionRefinements:         expressionRefinements,
		expressionPaths:               make(map[factflow.ExprRef]pathdom.Path),
		dynamicIndexExpressions:       make(map[factflow.ExprRef]factflow.DynamicIndexExpression),
		expressionConditions:          make(map[factflow.ExprRef]factflow.ExpressionCondition),
	}
	input := factflow.FactsInput{
		RootAssignments:               make(map[cfg.Point]factflow.RootAssignment),
		PathAssignments:               make(map[cfg.Point]factflow.PathAssignment),
		PathStaticMemberWrites:        make(map[cfg.Point]factflow.PathStaticMemberWrite),
		DynamicIndexWrites:            make(map[cfg.Point]factflow.DynamicIndexWrite),
		PathDescendantInvalidations:   make(map[cfg.Point]factflow.PathDescendantInvalidation),
		BranchEdgeReachability:        make(map[cfg.Point]factflow.BranchEdgeReachability),
		BranchConditionSources:        make(map[cfg.Point]factflow.ValueSource),
		BranchRefinements:             make(map[cfg.Point]factflow.BranchRefinementSet),
		BranchPresenceRelations:       make(map[cfg.Point]factflow.BranchPresenceRelationSet),
		BranchPathRelations:           make(map[cfg.Point]factflow.BranchPathRelationSet),
		BranchPathEvidence:            make(map[cfg.Point]factflow.BranchPathEvidenceSet),
		PathValuePresenceImplications: make(map[cfg.Point]factflow.PathValuePresenceImplicationSet),
		ChannelSelects:                make(map[cfg.Point]factflow.ChannelSelectSet),
		PostconditionRefinements:      make(map[cfg.Point]factflow.PostconditionRefinementSet),
		CallResultValues:              make(map[cfg.Point]factflow.CallResultValueSet),
		ReturnPresenceRelations:       make(map[cfg.Point]factflow.ReturnPresenceRelationSet),
		Returns:                       make(map[cfg.Point]factflow.Return),
		CallSites:                     make(map[cfg.Point]factflow.CallSite),
		ObjectLiterals:                make(map[factflow.ExprRef]factflow.ObjectLiteral),
		ExpressionValues:              make(map[factflow.ExprRef]product.Value),
		ExpressionOperations:          make(map[factflow.ExprRef]factflow.ExpressionOperation),
		ExpressionFunctions:           make(map[factflow.ExprRef]symbol.ID),
		ExpressionRefinements:         expressionRefinements,
		DynamicIndexExpressions:       make(map[factflow.ExprRef]factflow.DynamicIndexExpression),
	}
	if l.wir != nil && result == nil {
		l.addObjectLiteralsFromWIR(&input)
	}
	for _, point := range graph.RPO() {
		if l.wir != nil && result == nil {
			l.addAssignmentWritesFromWIR(&input, point)
		}
		if result != nil {
			if view, ok := result.LocalAssignmentView(point); ok {
				fact, _ := view.Borrowed()
				if l.wir != nil && !l.hasAssignmentWriteFromWIR(point) {
					// WIR mode must not fall back to semantic assignment facts, but
					// other WIR-owned facts can share this CFG point.
				} else if lowered, ok := l.localAssignment(point, fact); ok {
					input.RootAssignments[point] = lowered
					l.addLocalConditionAlias(fact.Symbol, lowered.Source())
					l.addAssignmentAssertionRefinements(&input, point, lowered.TargetPath(), lowered.Source(), fact.Source)
					l.addObjectLiteral(&input, result, fact.Source)
					l.addObjectLiteralExpectedType(&input, fact)
					l.addLocalAliasExposure(&input, point, fact)
					if fact.Source.Kind == sourceprovenance.SourceExpression {
						l.addCastExposure(&input, point, fact.Source.Expr)
					}
					if declared, ok := l.resolveType(fact.Type); ok {
						l.addObjectLiteralFieldExposures(&input, result, point, fact.Source, declared)
					}
				}
			}
			if view, ok := result.OrdinaryAssignmentView(point); ok {
				fact, _ := view.Borrowed()
				if l.wir != nil && !l.hasAssignmentWriteFromWIR(point) {
					// WIR mode must not fall back to semantic assignment facts, but
					// other WIR-owned facts can share this CFG point.
				} else if lowered, ok := l.pathAssignment(point, fact); ok {
					input.PathAssignments[point] = lowered
					if lowered, ok := l.pathStaticMemberWrite(point, fact); ok {
						input.PathStaticMemberWrites[point] = lowered
					}
					if lowered, ok := l.ordinaryAssignment(point, fact); ok {
						input.RootAssignments[point] = lowered
					}
					l.addAssertionRefinementsForLoweredSource(&input, lowered.Source(), fact.Source)
					l.addObjectLiteral(&input, result, fact.Source)
					l.addStoreExposure(&input, point, fact)
				} else if lowered, ok := l.ordinaryAssignment(point, fact); ok {
					input.RootAssignments[point] = lowered
					l.addAssignmentAssertionRefinements(&input, point, lowered.TargetPath(), lowered.Source(), fact.Source)
					l.addObjectLiteral(&input, result, fact.Source)
					l.addOrdinaryObjectLiteralExpectedType(&input, fact)
					l.addReassignExposure(&input, point, fact)
					if declared, ok := l.symbolTypes[fact.Symbol]; ok {
						l.addObjectLiteralFieldExposures(&input, result, point, fact.Source, declared)
					}
				}
				if lowered, ok := l.dynamicIndexWrite(point, fact); ok {
					input.DynamicIndexWrites[point] = lowered
					l.addAssertionRefinementsForLoweredSource(&input, lowered.Source(), fact.Source)
					l.addObjectLiteral(&input, result, fact.Source)
					l.addDynamicIndexObjectLiteralExpectedTypes(&input, fact)
				}
				if lowered, ok := l.pathDescendantInvalidation(fact); ok {
					input.PathDescendantInvalidations[point] = lowered
				}
			}
		}
		if l.wir != nil {
			if sources, ok := l.returnValueSourcesFromWIR(point); ok {
				input.Returns[point] = factflow.NewReturn(sources)
			}
		}
		if result != nil {
			if view, ok := result.ReturnView(point); ok {
				fact, _ := view.Borrowed()
				if _, hasReturn := input.Returns[point]; !hasReturn {
					if l.wir != nil {
						continue
					} else {
						input.Returns[point] = factflow.NewReturn(l.returnValueSources(fact.Sources, result))
					}
				}
				if ret, ok := input.Returns[point]; ok {
					if relations := l.typeIsReturnPresenceRelationsFromSources(ret.Sources(), input.CallSites); len(relations) != 0 {
						appendReturnPresenceRelations(input.ReturnPresenceRelations, point, relations...)
					}
					returnSources := ret.Sources()
					for index, source := range fact.Sources {
						l.addReturnAssertionRefinements(&input, point, index, valueSourceAt(returnSources, index), source)
						l.addObjectLiteral(&input, result, source)
					}
					l.addReturnObjectLiteralExpectedTypes(&input, result, fact)
				}
			}
		}
		if l.wir != nil {
			if lowered := l.channelSelectsFromWIR(point); len(lowered) != 0 {
				input.ChannelSelects[point] = factflow.NewChannelSelectSet(lowered...)
			}
			if site, ok := l.callSiteFromWIR(point); ok {
				input.CallSites[point] = site
			}
			if lowered := l.typeIsCallResultValuesFromWIR(point); len(lowered) != 0 {
				appendCallResultValues(input.CallResultValues, point, lowered...)
			}
			if lowered, ok := l.typeCastPostconditionRefinementFromWIR(point); ok {
				appendPostconditionRefinements(input.PostconditionRefinements, point, lowered)
			}
			if lowered, ok := l.assertPostconditionRefinementFromWIR(point); ok {
				appendPostconditionRefinements(input.PostconditionRefinements, point, lowered)
			}
			if lowered, ok := l.typeCastCallResultValueFromWIR(point); ok {
				appendCallResultValues(input.CallResultValues, point, lowered)
			}
		}
		if result != nil {
			if view, ok := result.CallView(point); ok {
				fact, _ := view.Borrowed()
				site, hasCallSite := input.CallSites[point]
				if !hasCallSite {
					site, hasCallSite = l.callSiteAt(point, fact)
				}
				if !hasCallSite {
					if l.wir != nil {
						continue
					}
				} else {
					input.CallSites[point] = site
				}
				var argumentSources []factflow.ValueSource
				if hasCallSite {
					argumentSources = site.ArgumentSources()
				}
				for index, source := range fact.ArgumentSources {
					l.addCallArgumentAssertionRefinements(&input, point, index, valueSourceAt(argumentSources, index), source)
					l.addObjectLiteral(&input, result, source)
				}
				if l.wir == nil {
					if lowered, ok := l.assertPostconditionRefinement(fact); ok {
						input.PostconditionRefinements[point] = factflow.NewPostconditionRefinementSet(lowered)
					}
				}
			}
		}
		if l.wir != nil {
			l.addBranchFactsFromWIR(&input, point)
		}
		if l.wir != nil {
			l.addNumericForFactsFromWIR(&input, point)
		}
	}
	l.addTypeIsBranchRefinements(&input, graph)
	l.addProtectedCallBranchRefinements(&input, graph)
	l.addReturnPresenceRelations(&input, graph)
	l.addConditionalAssignmentImplications(&input, graph)
	input.ExpressionValues = l.expressionValues
	input.ExpressionOperations = l.expressionOperations
	input.ExpressionFunctions = l.expressionFunctions
	input.ExpressionPaths = l.expressionPaths
	input.DynamicIndexExpressions = l.dynamicIndexExpressions
	input.ExpressionConditions = l.expressionConditions
	return Lowered{
		Facts:       factflow.NewFacts(input),
		SymbolTypes: copySymbolTypes(symbolTypes),
	}
}

func (l *lowerer) addNumericForFactsFromWIR(input *factflow.FactsInput, point cfg.Point) {
	if l == nil || l.wir == nil || input == nil {
		return
	}
	if lowered, ok := l.numericForBranchNumFloorRefinementFromWIR(point); ok {
		appendBranchNumFloorRefinement(input.BranchRefinements, point, lowered)
	}
	if lowered := l.numericForBranchPathEvidenceFromWIR(point); len(lowered) != 0 {
		appendBranchPathEvidence(input.BranchPathEvidence, point, lowered...)
	}
}

func (l *lowerer) addBranchFactsFromWIR(input *factflow.FactsInput, point cfg.Point) {
	if l == nil || l.wir == nil || input == nil || !l.wir.HasInstruction(point, wir.OpBranch) {
		return
	}
	if source, ok := l.branchConditionSourceAtWIR(point); ok {
		input.BranchConditionSources[point] = source
	}
	if reachability, ok := l.branchEdgeReachabilityFromWIR(point); ok {
		input.BranchEdgeReachability[point] = reachability
	}
	if lowered := l.branchRefinementsFromWIR(point); len(lowered) != 0 {
		appendBranchRefinement(input.BranchRefinements, point, lowered...)
	}
	if lowered := l.branchLenRefinementsFromWIR(point); len(lowered) != 0 {
		appendBranchLenRefinement(input.BranchRefinements, point, lowered...)
	}
	if lowered := l.branchNumFloorRefinementsFromWIR(point); len(lowered) != 0 {
		appendBranchNumFloorRefinement(input.BranchRefinements, point, lowered...)
	}
	if lowered := l.branchDiffConstraintsFromWIR(point); len(lowered) != 0 {
		appendBranchDiffConstraint(input.BranchRefinements, point, lowered...)
	}
	if lowered := l.branchAliasRefinementsFromWIR(point); len(lowered) != 0 {
		appendBranchRefinement(input.BranchRefinements, point, lowered...)
	}
	if lowered, ok := l.branchPathRelationsFromWIR(point); ok {
		input.BranchPathRelations[point] = lowered
	}
	if lowered := l.branchAliasPathRelationsFromWIR(point); len(lowered) != 0 {
		appendBranchPathRelations(input.BranchPathRelations, point, lowered...)
	}
	if lowered := l.branchPathEvidenceFromWIR(point); len(lowered) != 0 {
		appendBranchPathEvidence(input.BranchPathEvidence, point, lowered...)
	}
	if lowered := l.branchAliasPathEvidenceFromWIR(point); len(lowered) != 0 {
		appendBranchPathEvidence(input.BranchPathEvidence, point, lowered...)
	}
}

func copySymbolTypes(in map[symbol.ID]typ.Type) map[symbol.ID]typ.Type {
	if len(in) == 0 {
		return nil
	}
	out := make(map[symbol.ID]typ.Type, len(in))
	for id, t := range in {
		out[id] = t
	}
	return out
}

type lowerer struct {
	registry                      *axis.Registry
	bindings                      *bind.Result
	graph                         cfg.Graph
	graphID                       uint64
	typeResolver                  *typeresolve.Resolver
	typeValues                    *typevalue.Cache
	metadata                      cfgfacts.Metadata
	wir                           *wir.Body
	wirTempDefinitions            map[uint32]wir.Instruction
	wirTempDefinitionSets         map[uint32][]wir.Instruction
	wirStaticReachable            map[cfg.Point]bool
	wirReachability               *cfg.Reachability
	callPoints                    map[*ast.FuncCallExpr]cfg.Point
	wirCallPoints                 map[wir.ExpressionID]cfg.Point
	symbolTypes                   map[symbol.ID]typ.Type
	declaredReturnLocalTypes      map[symbol.ID]typ.Type
	returnLocalObjectLiteralTypes map[symbol.ID]typ.Type
	exprs                         map[any]factflow.ExprRef
	types                         map[any]factflow.TypeRef
	expressionValues              map[factflow.ExprRef]product.Value
	expressionOperations          map[factflow.ExprRef]factflow.ExpressionOperation
	expressionFunctions           map[factflow.ExprRef]symbol.ID
	expressionPaths               map[factflow.ExprRef]pathdom.Path
	dynamicIndexExpressions       map[factflow.ExprRef]factflow.DynamicIndexExpression
	expressionConditions          map[factflow.ExprRef]factflow.ExpressionCondition
	wirResultSources              map[uint32]wirResultSource
	expressionRefinements         map[factflow.ExprRef]factflow.ExpressionRefinement
	localConditionAliases         map[symbol.ID]factflow.ExpressionCondition
}

func (l *lowerer) valueFromType(t typ.Type) product.Value {
	return l.typeValues.FromType(l.registry, t)
}

func (l *lowerer) valueFromTypeWithWitness(t typ.Type) product.Value {
	return l.typeValues.FromTypeWithWitness(l.registry, t)
}

func semanticCallPointsByExpr(graph cfg.Graph, result *semantics.Result, body *wir.Body) map[*ast.FuncCallExpr]cfg.Point {
	if body != nil {
		return nil
	}
	return callPointsByExpr(builtCallFacts(graph, result))
}

func callPointsByExpr(facts map[*ast.FuncCallExpr]cfg.Point) map[*ast.FuncCallExpr]cfg.Point {
	if len(facts) == 0 {
		return nil
	}
	out := make(map[*ast.FuncCallExpr]cfg.Point, len(facts))
	for call, point := range facts {
		if call == nil || point == 0 {
			continue
		}
		out[call] = point
	}
	return out
}

func callPointsByExpressionIDFromWIR(graph cfg.Graph, body *wir.Body) map[wir.ExpressionID]cfg.Point {
	if graph == nil || body == nil {
		return nil
	}
	out := make(map[wir.ExpressionID]cfg.Point)
	for _, point := range graph.RPO() {
		for _, inst := range body.PointInstructions(point) {
			if inst.Op != wir.OpCall || inst.ExprID == 0 || point == 0 {
				continue
			}
			out[inst.ExprID] = point
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func builtCallFacts(graph cfg.Graph, result *semantics.Result) map[*ast.FuncCallExpr]cfg.Point {
	if graph == nil || result == nil {
		return nil
	}
	out := make(map[*ast.FuncCallExpr]cfg.Point)
	for _, point := range graph.RPO() {
		view, ok := result.CallView(point)
		if !ok {
			continue
		}
		fact, ok := view.Borrowed()
		if !ok || fact.Call == nil {
			continue
		}
		out[fact.Call] = point
	}
	return out
}

func (l *lowerer) callPointForExpr(_ int, call *ast.FuncCallExpr) (cfg.Point, bool) {
	if l == nil || call == nil {
		return 0, false
	}
	if l.wir != nil {
		point, ok := l.wirCallPoints[wir.ExpressionID(expressionid.Of(call))]
		return point, ok && point != 0
	}
	if len(l.callPoints) == 0 {
		return 0, false
	}
	point, ok := l.callPoints[call]
	return point, ok && point != 0
}

func valueSourceAt(sources []factflow.ValueSource, index int) factflow.ValueSource {
	if index < 0 || index >= len(sources) {
		return factflow.ValueSource{}
	}
	return sources[index]
}
