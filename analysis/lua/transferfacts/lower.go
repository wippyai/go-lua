// Package transferfacts lowers WIR descriptors into generic transfer facts.
package transferfacts

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Lower converts WIR descriptors into the generic transfer fact DTOs consumed by
// the engine. Higher semantic layers add interproc and diagnostic facts
// separately.
type Config struct {
	Registry      *axis.Registry
	TypeResolver  *typeresolve.Resolver
	TypeValues    *typevalue.Cache
	ModuleExports importlookup.Source
	WIR           *wir.Body

	// NoNormalReturnCall reports whether a lowered call site cannot complete
	// normally. The predicate is supplied by higher layers that own declared
	// signature/effect lookup; transferfacts only attaches the proven fact during
	// canonical FactsInput construction.
	NoNormalReturnCall func(point cfg.Point, site factflow.CallSiteView) bool
}

type Lowered struct {
	Facts       factflow.Facts
	Plan        *operationplan.Plan
	SymbolTypes map[symbol.ID]typ.Type
}

func LowerDetailed(graph cfg.Graph, config Config) Lowered {
	if config.Registry == nil {
		panic("transferfacts: Config.Registry is required")
	}
	if config.WIR == nil {
		panic("transferfacts: Config.WIR is required")
	}
	if graph == nil {
		plan := operationplan.New(nil, factflow.FactsInput{})
		return Lowered{Facts: plan.Facts(), Plan: plan}
	}
	typeResolver := config.TypeResolver
	symbolTypes := lowerSymbolTypesFromWIR(config.WIR, config.ModuleExports)
	returnLocalTypes := lowerReturnLocalTypesFromWIR(graph, config.WIR)
	symbolTypes = mergeSymbolTypes(symbolTypes, returnLocalTypes)
	expressionRefinements := make(map[factflow.ExprRef]factflow.ExpressionRefinement)
	l := lowerer{
		registry:                config.Registry,
		graph:                   graph,
		graphID:                 graph.ID(),
		typeResolver:            typeResolver,
		typeValues:              config.TypeValues,
		wir:                     config.WIR,
		symbolTypes:             symbolTypes,
		returnLocalTypes:        returnLocalTypes,
		exprs:                   make(map[any]factflow.ExprRef),
		types:                   make(map[any]factflow.TypeRef),
		expressionValues:        make(map[factflow.ExprRef]product.Value),
		expressionOperations:    make(map[factflow.ExprRef]factflow.ExpressionOperation),
		expressionFunctions:     make(map[factflow.ExprRef]symbol.ID),
		expressionRefinements:   expressionRefinements,
		expressionPaths:         make(map[factflow.ExprRef]pathdom.Path),
		dynamicIndexExpressions: make(map[factflow.ExprRef]factflow.DynamicIndexExpression),
		expressionConditions:    make(map[factflow.ExprRef]factflow.ExpressionCondition),
	}
	input := factflow.FactsInput{
		RootAssignments:               make(map[cfg.Point]factflow.RootAssignment),
		PathAssignments:               make(map[cfg.Point]factflow.PathAssignment),
		PathStaticMemberWrites:        make(map[cfg.Point]factflow.PathStaticMemberWrite),
		DynamicIndexWrites:            make(map[cfg.Point]factflow.DynamicIndexWrite),
		PathDescendantInvalidations:   make(map[cfg.Point]factflow.PathDescendantInvalidation),
		BranchEdgeReachability:        make(map[cfg.Point]factflow.BranchEdgeReachability),
		NoNormalReturns:               make(map[cfg.Point]struct{}),
		BranchConditionSources:        make(map[cfg.Point]factflow.ValueSource),
		BranchRefinements:             make(map[cfg.Point]factflow.BranchRefinementSet),
		BranchPresenceRelations:       make(map[cfg.Point]factflow.BranchPresenceRelationSet),
		BranchPathRelations:           make(map[cfg.Point]factflow.BranchPathRelationSet),
		BranchPathEvidence:            make(map[cfg.Point]factflow.BranchPathEvidenceSet),
		BranchSufficientLiteralCases:  make(map[cfg.Point]factflow.BranchSufficientLiteralCaseSet),
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
	l.addObjectLiteralsFromWIR(&input)
	for _, point := range graph.RPO() {
		l.addAssignmentWritesFromWIR(&input, point)
		if sources, ok := l.returnValueSourcesFromWIR(point); ok {
			input.Returns[point] = factflow.NewReturn(sources)
			l.addReturnObjectLiteralExpectedTypesFromWIR(&input, sources)
			l.addTypeIsReturnPresenceRelationsFromSources(&input, point, sources)
		}
		if lowered := l.channelSelectsFromWIR(point); len(lowered) != 0 {
			input.ChannelSelects[point] = factflow.NewChannelSelectSet(lowered...)
		}
		if site, ok := l.callSiteFromWIR(point); ok {
			input.CallSites[point] = site
			if config.NoNormalReturnCall != nil && config.NoNormalReturnCall(point, site.View()) {
				input.NoNormalReturns[point] = struct{}{}
			}
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
		l.addBranchFactsFromWIR(&input, point)
		l.addNumericForFactsFromWIR(&input, point)
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
	plan := operationplan.New(graph, input)
	return Lowered{
		Facts:       plan.Facts(),
		Plan:        plan,
		SymbolTypes: copySymbolTypes(symbolTypes),
	}
}

func (l *lowerer) addTypeIsReturnPresenceRelationsFromSources(input *factflow.FactsInput, point cfg.Point, sources []factflow.ValueSource) {
	if input == nil {
		return
	}
	if relations := l.typeIsReturnPresenceRelationsFromSources(sources, input.CallSites); len(relations) != 0 {
		appendReturnPresenceRelations(input.ReturnPresenceRelations, point, relations...)
	}
}

func (l *lowerer) addNumericForFactsFromWIR(input *factflow.FactsInput, point cfg.Point) {
	if input == nil {
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
	if input == nil || !l.wir.HasInstruction(point, wir.OpBranch) {
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
	if lowered := l.branchNumCeilRefinementsFromWIR(point); len(lowered) != 0 {
		appendBranchNumCeilRefinement(input.BranchRefinements, point, lowered...)
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
	if lowered := l.branchSufficientLiteralCasesFromWIR(point); len(lowered) != 0 {
		appendBranchSufficientLiteralCases(input.BranchSufficientLiteralCases, point, lowered...)
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
	registry                *axis.Registry
	graph                   cfg.Graph
	graphID                 uint64
	typeResolver            *typeresolve.Resolver
	typeValues              *typevalue.Cache
	wir                     *wir.Body
	wirTempDefinitions      map[uint32]wir.Instruction
	wirTempDefinitionSets   map[uint32][]wir.Instruction
	wirStaticReachable      map[cfg.Point]bool
	wirReachability         *cfg.Reachability
	symbolTypes             map[symbol.ID]typ.Type
	returnLocalTypes        map[symbol.ID]typ.Type
	exprs                   map[any]factflow.ExprRef
	types                   map[any]factflow.TypeRef
	expressionValues        map[factflow.ExprRef]product.Value
	expressionOperations    map[factflow.ExprRef]factflow.ExpressionOperation
	expressionFunctions     map[factflow.ExprRef]symbol.ID
	expressionPaths         map[factflow.ExprRef]pathdom.Path
	dynamicIndexExpressions map[factflow.ExprRef]factflow.DynamicIndexExpression
	expressionConditions    map[factflow.ExprRef]factflow.ExpressionCondition
	wirResultSources        map[uint32]wirResultSource
	expressionRefinements   map[factflow.ExprRef]factflow.ExpressionRefinement
	localConditionAliases   map[symbol.ID]factflow.ExpressionCondition
	presentAssignmentStats  *presentAssignmentTransferStats
}

func (l *lowerer) valueFromType(t typ.Type) product.Value {
	return l.typeValues.FromType(l.registry, t)
}

func (l *lowerer) valueFromTypeWithWitness(t typ.Type) product.Value {
	return l.typeValues.FromTypeWithWitness(l.registry, t)
}
