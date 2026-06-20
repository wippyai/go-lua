// Package transferfacts lowers Lua semantic sidecars into generic transfer facts.
package transferfacts

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Lower converts Lua semantic facts into the generic transfer fact DTOs consumed
// by the engine. It intentionally lowers only syntax facts already represented
// by factflow.Facts; higher semantic layers add branch, iterator, interproc,
// and diagnostic facts separately.
type Config struct {
	Registry     *axis.Registry
	Bindings     *bind.Result
	TypeResolver *typeresolve.Resolver
	TypeValues   *typevalue.Cache
}

func Lower(result *semantics.Result, graph cfg.Graph, config Config) factflow.Facts {
	if config.Registry == nil {
		panic("transferfacts: Config.Registry is required")
	}
	if result == nil || graph == nil {
		return factflow.NewFacts(factflow.FactsInput{})
	}
	typeResolver := config.TypeResolver
	if typeResolver == nil {
		typeResolver = typeresolve.New(config.Bindings)
	}
	l := lowerer{
		registry:             config.Registry,
		bindings:             config.Bindings,
		graphID:              graph.ID(),
		typeResolver:         typeResolver,
		typeValues:           config.TypeValues,
		callPoints:           callPointsByExpr(builtCallFacts(graph, result)),
		symbolTypes:          lowerSymbolTypes(config.Bindings, graph, result, typeResolver),
		exprs:                make(map[any]factflow.ExprRef),
		types:                make(map[any]factflow.TypeRef),
		expressionValues:     make(map[factflow.ExprRef]product.Value),
		expressionOperations: make(map[factflow.ExprRef]factflow.ExpressionOperation),
		expressionFunctions:  make(map[factflow.ExprRef]symbol.ID),
		expressionPaths:      make(map[factflow.ExprRef]pathdom.Path),
		expressionConditions: make(map[factflow.ExprRef]factflow.ExpressionCondition),
	}
	input := factflow.FactsInput{
		RootAssignments:             make(map[cfg.Point]factflow.RootAssignment),
		PathAssignments:             make(map[cfg.Point]factflow.PathAssignment),
		PathStaticMemberWrites:      make(map[cfg.Point]factflow.PathStaticMemberWrite),
		DynamicIndexWrites:          make(map[cfg.Point]factflow.DynamicIndexWrite),
		PathDescendantInvalidations: make(map[cfg.Point]factflow.PathDescendantInvalidation),
		BranchRefinements:           make(map[cfg.Point]factflow.BranchRefinementSet),
		BranchPresenceRelations:     make(map[cfg.Point]factflow.BranchPresenceRelationSet),
		BranchPathRelations:         make(map[cfg.Point]factflow.BranchPathRelationSet),
		BranchPathEvidence:          make(map[cfg.Point]factflow.BranchPathEvidenceSet),
		ChannelSelects:              make(map[cfg.Point]factflow.ChannelSelectSet),
		PostconditionRefinements:    make(map[cfg.Point]factflow.PostconditionRefinementSet),
		CallResultValues:            make(map[cfg.Point]factflow.CallResultValueSet),
		ReturnPresenceRelations:     make(map[cfg.Point]factflow.ReturnPresenceRelationSet),
		Returns:                     make(map[cfg.Point]factflow.Return),
		CallSites:                   make(map[cfg.Point]factflow.CallSite),
		ObjectLiterals:              make(map[factflow.ExprRef]factflow.ObjectLiteral),
		ExpressionValues:            make(map[factflow.ExprRef]product.Value),
		ExpressionOperations:        make(map[factflow.ExprRef]factflow.ExpressionOperation),
		ExpressionFunctions:         make(map[factflow.ExprRef]symbol.ID),
		ExpressionRefinements:       make(map[factflow.ExprRef]factflow.ExpressionRefinement),
	}
	for _, point := range graph.RPO() {
		if fact, ok := result.LocalAssignment(point); ok {
			if lowered, ok := l.localAssignment(fact); ok {
				input.RootAssignments[point] = lowered
				l.addAssertionRefinementsForSource(&input, fact.Source)
				l.addObjectLiteral(&input, result, fact.Source)
				l.addObjectLiteralExpectedType(&input, fact)
			}
		}
		if fact, ok := result.OrdinaryAssignment(point); ok {
			if lowered, ok := l.pathAssignment(fact); ok {
				input.PathAssignments[point] = lowered
				if lowered, ok := l.pathStaticMemberWrite(fact); ok {
					input.PathStaticMemberWrites[point] = lowered
				}
				l.addAssertionRefinementsForSource(&input, fact.Source)
				l.addObjectLiteral(&input, result, fact.Source)
			} else if lowered, ok := l.ordinaryAssignment(fact); ok {
				input.RootAssignments[point] = lowered
				l.addAssertionRefinementsForSource(&input, fact.Source)
				l.addObjectLiteral(&input, result, fact.Source)
				l.addOrdinaryObjectLiteralExpectedType(&input, fact)
			}
			if lowered, ok := l.dynamicIndexWrite(fact); ok {
				input.DynamicIndexWrites[point] = lowered
			}
			if lowered, ok := l.pathDescendantInvalidation(fact); ok {
				input.PathDescendantInvalidations[point] = lowered
			}
		}
		if fact, ok := result.Return(point); ok {
			input.Returns[point] = factflow.NewReturn(l.returnValueSources(fact.Sources, result))
			if relations := l.typeIsReturnPresenceRelations(fact.Sources, result); len(relations) != 0 {
				appendReturnPresenceRelations(input.ReturnPresenceRelations, point, relations...)
			}
			for _, source := range fact.Sources {
				l.addAssertionRefinementsForSource(&input, source)
				l.addObjectLiteral(&input, result, source)
			}
			l.addReturnObjectLiteralExpectedTypes(&input, result, fact)
		}
		if fact, ok := result.Call(point); ok {
			input.CallSites[point] = l.callSite(fact)
			if lowered := l.channelSelects(point, result); len(lowered) != 0 {
				input.ChannelSelects[point] = factflow.NewChannelSelectSet(lowered...)
			}
			for _, source := range fact.ArgumentSources {
				l.addAssertionRefinementsForSource(&input, source)
				l.addObjectLiteral(&input, result, source)
			}
			if lowered, ok := l.assertPostconditionRefinement(fact); ok {
				input.PostconditionRefinements[point] = factflow.NewPostconditionRefinementSet(lowered)
			}
			if lowered, ok := l.typeCastPostconditionRefinement(fact); ok {
				appendPostconditionRefinements(input.PostconditionRefinements, point, lowered)
			}
			if lowered, ok := l.typeCastCallResultValue(fact); ok {
				appendCallResultValues(input.CallResultValues, point, lowered)
			}
			if lowered := l.typeIsCallResultValues(fact); len(lowered) != 0 {
				appendCallResultValues(input.CallResultValues, point, lowered...)
			}
		}
		if fact, ok := result.BranchCondition(point); ok {
			if lowered := l.branchRefinements(fact); len(lowered) != 0 {
				appendBranchRefinement(input.BranchRefinements, point, lowered...)
			}
			if lowered := l.branchLenRefinements(fact); len(lowered) != 0 {
				appendBranchLenRefinement(input.BranchRefinements, point, lowered...)
			}
			if lowered := l.branchNumFloorRefinements(fact); len(lowered) != 0 {
				appendBranchNumFloorRefinement(input.BranchRefinements, point, lowered...)
			}
			if lowered := l.branchDiffConstraints(fact); len(lowered) != 0 {
				appendBranchDiffConstraint(input.BranchRefinements, point, lowered...)
			}
			if lowered, ok := l.branchPathRelations(fact); ok {
				input.BranchPathRelations[point] = lowered
			}
			if lowered := l.branchPathEvidence(fact); len(lowered) != 0 {
				appendBranchPathEvidence(input.BranchPathEvidence, point, lowered...)
			}
			l.addAssertionRefinementsForSource(&input, fact.Source)
		}
		if fact, ok := result.NumericFor(point); ok {
			if lowered, ok := l.numericForBranchNumFloorRefinement(fact); ok {
				appendBranchNumFloorRefinement(input.BranchRefinements, point, lowered)
			}
			if lowered := l.numericForBranchPathEvidence(fact); len(lowered) != 0 {
				appendBranchPathEvidence(input.BranchPathEvidence, point, lowered...)
			}
		}
	}
	l.addTypeIsBranchRefinements(&input, graph, result)
	l.addReturnPresenceRelations(&input, graph, result)
	input.ExpressionValues = l.expressionValues
	input.ExpressionOperations = l.expressionOperations
	input.ExpressionFunctions = l.expressionFunctions
	input.ExpressionPaths = l.expressionPaths
	input.ExpressionConditions = l.expressionConditions
	return factflow.NewFacts(input)
}

type lowerer struct {
	registry             *axis.Registry
	bindings             *bind.Result
	graphID              uint64
	typeResolver         *typeresolve.Resolver
	typeValues           *typevalue.Cache
	callPoints           map[*ast.FuncCallExpr]cfg.Point
	symbolTypes          map[symbol.ID]typ.Type
	exprs                map[any]factflow.ExprRef
	types                map[any]factflow.TypeRef
	expressionValues     map[factflow.ExprRef]product.Value
	expressionOperations map[factflow.ExprRef]factflow.ExpressionOperation
	expressionFunctions  map[factflow.ExprRef]symbol.ID
	expressionPaths      map[factflow.ExprRef]pathdom.Path
	expressionConditions map[factflow.ExprRef]factflow.ExpressionCondition
}

func (l *lowerer) valueFromType(t typ.Type) product.Value {
	return l.typeValues.FromType(l.registry, t)
}

func (l *lowerer) valueFromTypeWithWitness(t typ.Type) product.Value {
	return l.typeValues.FromTypeWithWitness(l.registry, t)
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

func builtCallFacts(graph cfg.Graph, result *semantics.Result) map[*ast.FuncCallExpr]cfg.Point {
	if graph == nil || result == nil {
		return nil
	}
	out := make(map[*ast.FuncCallExpr]cfg.Point)
	for _, point := range graph.RPO() {
		fact, ok := result.Call(point)
		if !ok || fact.Call == nil {
			continue
		}
		out[fact.Call] = point
	}
	return out
}

func (l *lowerer) callPointForExpr(_ int, call *ast.FuncCallExpr) (cfg.Point, bool) {
	if l == nil || call == nil || len(l.callPoints) == 0 {
		return 0, false
	}
	point, ok := l.callPoints[call]
	return point, ok && point != 0
}
