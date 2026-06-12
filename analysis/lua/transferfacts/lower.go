// Package transferfacts lowers Lua semantic sidecars into generic transfer facts.
package transferfacts

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
)

// Lower converts Lua semantic facts into the generic transfer fact DTOs consumed
// by the engine. It intentionally lowers only syntax facts already represented
// by factflow.Facts; higher semantic layers add branch, iterator, interproc,
// and diagnostic facts separately.
type Config struct {
	Registry *axis.Registry
	Bindings *bind.Result
}

func Lower(result *semantics.Result, graph cfg.Graph, config Config) factflow.Facts {
	if config.Registry == nil {
		panic("transferfacts: Config.Registry is required")
	}
	if result == nil || graph == nil {
		return factflow.NewFacts(factflow.FactsInput{})
	}
	l := lowerer{
		registry:             config.Registry,
		bindings:             config.Bindings,
		exprs:                make(map[any]factflow.ExprRef),
		types:                make(map[any]factflow.TypeRef),
		expressionPaths:      make(map[factflow.ExprRef]pathdom.Path),
		expressionConditions: make(map[factflow.ExprRef]factflow.ExpressionCondition),
	}
	input := factflow.FactsInput{
		LocalAssignments:            make(map[cfg.Point]factflow.RootAssignment),
		OrdinaryAssignments:         make(map[cfg.Point]factflow.RootAssignment),
		PathAssignments:             make(map[cfg.Point]factflow.PathAssignment),
		PathDescendantInvalidations: make(map[cfg.Point]factflow.PathDescendantInvalidation),
		BranchRefinements:           make(map[cfg.Point]factflow.BranchRefinementSet),
		BranchPresenceRelations:     make(map[cfg.Point]factflow.BranchPresenceRelationSet),
		BranchPathRelations:         make(map[cfg.Point]factflow.BranchPathRelationSet),
		PostconditionRefinements:    make(map[cfg.Point]factflow.PostconditionRefinementSet),
		CallResultValues:            make(map[cfg.Point]factflow.CallResultValueSet),
		ReturnPresenceRelations:     make(map[cfg.Point]factflow.ReturnPresenceRelationSet),
		Returns:                     make(map[cfg.Point]factflow.Return),
		Calls:                       make(map[cfg.Point]factflow.CallProducer),
		CallSites:                   make(map[cfg.Point]factflow.CallSite),
		ObjectLiterals:              make(map[factflow.ExprRef]factflow.ObjectLiteral),
		ExpressionRefinements:       make(map[factflow.ExprRef]factflow.ExpressionRefinement),
	}
	for _, point := range graph.RPO() {
		if fact, ok := result.LocalAssignment(point); ok {
			if lowered, ok := l.localAssignment(fact); ok {
				input.LocalAssignments[point] = lowered
				l.addAssertionRefinementsForSource(&input, fact.Source)
				l.addObjectLiteral(&input, result, fact.Source)
			}
		}
		if fact, ok := result.OrdinaryAssignment(point); ok {
			if lowered, ok := l.pathAssignment(fact); ok {
				input.PathAssignments[point] = lowered
				l.addAssertionRefinementsForSource(&input, fact.Source)
				l.addObjectLiteral(&input, result, fact.Source)
			} else if lowered, ok := l.pathDescendantInvalidation(fact); ok {
				input.PathDescendantInvalidations[point] = lowered
			} else if lowered, ok := l.ordinaryAssignment(fact); ok {
				input.OrdinaryAssignments[point] = lowered
				l.addAssertionRefinementsForSource(&input, fact.Source)
				l.addObjectLiteral(&input, result, fact.Source)
			}
		}
		if fact, ok := result.Return(point); ok {
			input.Returns[point] = factflow.NewReturn(l.returnValueSources(fact.Sources, result))
			if relations := l.typeIsReturnPresenceRelations(fact.Sources, result); len(relations) != 0 {
				input.ReturnPresenceRelations[point] = factflow.NewReturnPresenceRelationSet(relations...)
			}
			for _, source := range fact.Sources {
				l.addAssertionRefinementsForSource(&input, source)
			}
		}
		if fact, ok := result.Call(point); ok {
			input.CallSites[point] = l.callSite(fact)
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
			if lowered, ok := l.callProducer(fact); ok {
				input.Calls[point] = lowered
			}
		}
		if fact, ok := result.BranchCondition(point); ok {
			if lowered, ok := l.branchRefinement(fact); ok {
				appendBranchRefinement(input.BranchRefinements, point, lowered)
			}
			if lowered, ok := l.branchPathRelations(fact); ok {
				input.BranchPathRelations[point] = lowered
			}
			l.addAssertionRefinementsForSource(&input, fact.Source)
		}
	}
	l.addTypeIsBranchRefinements(&input, graph, result)
	input.ExpressionPaths = l.expressionPaths
	input.ExpressionConditions = l.expressionConditions
	return factflow.NewFacts(input)
}

type lowerer struct {
	registry             *axis.Registry
	bindings             *bind.Result
	exprs                map[any]factflow.ExprRef
	types                map[any]factflow.TypeRef
	expressionPaths      map[factflow.ExprRef]pathdom.Path
	expressionConditions map[factflow.ExprRef]factflow.ExpressionCondition
}
