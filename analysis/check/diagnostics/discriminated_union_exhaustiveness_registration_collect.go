package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (p discriminatedUnionExhaustiveness) registrationCalls(result *body.Result, graph cfg.Graph) ([]registrationCall, []openRegistrationMutation) {
	var registrations []registrationCall
	var open []openRegistrationMutation
	for _, point := range graph.RPO() {
		call, ok := result.Call(point)
		if !ok || call.Call == nil {
			if assignment, ok := result.OrdinaryAssignment(point); ok {
				if reg, ok := registrationAssignmentFromFact(result, assignment, point); ok {
					registrations = append(registrations, reg)
					continue
				}
				if mutation, ok := openRegistrationAssignment(result, point, assignment); ok {
					mutation.point = point
					open = append(open, mutation)
				}
			}
			continue
		}
		site, ok := result.CallSite(point)
		if !ok {
			continue
		}
		if reg, ok := registrationCallFromFact(result, site, call, point); ok {
			registrations = append(registrations, reg)
			continue
		}
		if mutation, ok := openRegistrationMutationFromFact(result, site, point, call); ok {
			mutation.point = point
			open = append(open, mutation)
		}
	}
	return registrations, open
}

func registrationAssignmentFromFact(result *body.Result, fact semantics.OrdinaryAssignmentFact, point cfg.Point) (registrationCall, bool) {
	registry, key, ok := registrationAssignmentTarget(result, point, fact)
	if !ok || !registrationCallbackExpr(result, point, fact.Value) {
		return registrationCall{}, false
	}
	return registrationCall{
		point:    point,
		registry: registry,
		key:      key,
		span:     ast.SpanOf(fact.Target),
	}, true
}

func openRegistrationAssignment(result *body.Result, point cfg.Point, fact semantics.OrdinaryAssignmentFact) (openRegistrationMutation, bool) {
	if registry, key, ok := registrationAssignmentTarget(result, point, fact); ok {
		return openRegistrationMutation{
			path:           registry,
			registry:       registry,
			key:            key,
			hasKey:         true,
			aliasSensitive: true,
			mayRegister:    registrationValueMayRegister(result, point, fact.Value),
		}, true
	}
	if fact.HasPath && fact.Path.Symbol != 0 {
		mutation := openRegistrationMutation{
			path:           fact.Path,
			aliasSensitive: true,
			mayRegister:    registrationValueMayRegister(result, point, fact.Value),
		}
		if key, ok := fact.Path.DirectFieldName(); ok {
			mutation.registry = pathdom.Path{Root: fact.Path.Root, Symbol: fact.Path.Symbol, Version: fact.Path.Version}
			mutation.key = key
			mutation.hasKey = true
			return mutation, true
		}
		if seg, ok := fact.Path.LastSegment(); ok {
			if key, keyOK := segmentStringKey(seg); keyOK {
				mutation.registry = fact.Path.Parent()
				mutation.key = key
				mutation.hasKey = true
				return mutation, true
			}
		}
		mutation.opensAll = true
		return mutation, true
	}
	if fact.HasContainerPath && fact.ContainerPath.Symbol != 0 {
		return openRegistrationMutation{path: fact.ContainerPath, opensAll: true, aliasSensitive: true, mayRegister: true}, true
	}
	if fact.HasSymbol && fact.Symbol != 0 {
		return openRegistrationMutation{path: pathdom.Path{Symbol: fact.Symbol}, opensAll: true, mayRegister: true}, true
	}
	return openRegistrationMutation{}, false
}

func registrationAssignmentTarget(result *body.Result, point cfg.Point, fact semantics.OrdinaryAssignmentFact) (pathdom.Path, string, bool) {
	if fact.HasPath && fact.Path.Symbol != 0 {
		if seg, ok := fact.Path.LastSegment(); ok {
			if key, keyOK := segmentStringKey(seg); keyOK {
				return fact.Path.Parent(), key, true
			}
		}
	}
	attr, ok := fact.Target.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return pathdom.Path{}, "", false
	}
	registry, ok := result.ExpressionPath(attr.Object)
	if !ok || registry.Symbol == 0 {
		return pathdom.Path{}, "", false
	}
	key, ok := result.StaticStringExprValueAtBoundary(point, attr.Key)
	return registry, key, ok
}

func registrationCallFromFact(result *body.Result, site factflow.CallSite, fact semantics.CallFact, point cfg.Point) (registrationCall, bool) {
	registry, keyIndex, ok := registrationRegistryAndKeyIndex(result, site, fact)
	if !ok || keyIndex < 0 || keyIndex >= len(fact.Args)-1 {
		return registrationCall{}, false
	}
	key, ok := result.StaticStringExprValueAtBoundary(point, fact.Args[keyIndex])
	if !ok || !registrationCallbackExpr(result, point, fact.Args[keyIndex+1]) {
		return registrationCall{}, false
	}
	return registrationCall{
		point:    point,
		call:     fact.Call,
		registry: registry,
		key:      key,
		span:     ast.SpanOf(fact.Call),
	}, true
}

func registrationRegistryAndKeyIndex(result *body.Result, site factflow.CallSite, fact semantics.CallFact) (pathdom.Path, int, bool) {
	if fact.Call == nil {
		return pathdom.Path{}, 0, false
	}
	if registry, ok := callSiteMemberReceiverPath(site); ok && len(fact.Args) >= 2 {
		return registry, 0, true
	}
	if len(fact.Args) >= 3 {
		if registry, ok := result.ExpressionPath(fact.Args[0]); ok && registry.Symbol != 0 {
			return registry, 1, true
		}
	}
	return pathdom.Path{}, 0, false
}

func openRegistrationMutationFromFact(result *body.Result, site factflow.CallSite, point cfg.Point, fact semantics.CallFact) (openRegistrationMutation, bool) {
	registry, keyIndex, ok := registrationRegistryAndKeyIndex(result, site, fact)
	if ok && keyIndex >= 0 && keyIndex < len(fact.Args)-1 {
		if _, ok := result.StaticStringExprValueAtBoundary(point, fact.Args[keyIndex]); ok && registrationCallbackExpr(result, point, fact.Args[keyIndex+1]) {
			return openRegistrationMutation{}, false
		}
		if registrationCallbackExpr(result, point, fact.Args[keyIndex+1]) {
			return openRegistrationMutation{path: registry, opensAll: true, aliasSensitive: true, mayRegister: true}, true
		}
	}
	if receiver, ok := callSiteMemberReceiverPath(site); ok && result.CallMayInvalidateTrackedPath(point, receiver) {
		return openRegistrationMutation{path: receiver, opensAll: true, aliasSensitive: true, mayRegister: true}, true
	}
	for _, arg := range fact.Args {
		argPath, ok := result.ExpressionPath(arg)
		if ok && result.CallMayInvalidateTrackedPath(point, argPath) {
			return openRegistrationMutation{path: argPath, opensAll: true, aliasSensitive: true, mayRegister: true}, true
		}
	}
	return openRegistrationMutation{}, false
}

func registrationValueMayRegister(result *body.Result, point cfg.Point, expr ast.Expr) bool {
	return result.ExpressionMayBeFunctionBeforeBoundary(point, expr)
}

func registrationCallbackExpr(result *body.Result, point cfg.Point, expr ast.Expr) bool {
	return result.ExpressionProvenFunctionAtBoundary(point, expr)
}

func (p discriminatedUnionExhaustiveness) dispatchCalls(result *body.Result, graph cfg.Graph) []dispatchCall {
	var out []dispatchCall
	for _, point := range graph.RPO() {
		fact, ok := result.Call(point)
		if !ok || fact.Call == nil {
			continue
		}
		site, ok := result.CallSite(point)
		if !ok || isRegistrationLikeCall(result, site, point, fact) {
			continue
		}
		registry, args, ok := dispatchRegistryAndArgs(result, site, fact)
		if !ok {
			continue
		}
		for _, arg := range args {
			argPath, ok := result.ExpressionPath(arg)
			if !ok || argPath.Symbol == 0 {
				continue
			}
			discriminant, cases, ok := p.stringDiscriminantCasesForArgument(result, point, argPath)
			if !ok {
				continue
			}
			out = append(out, dispatchCall{
				point:        point,
				call:         fact.Call,
				registry:     registry,
				discriminant: discriminant,
				cases:        cases,
				span:         ast.SpanOf(fact.Call),
			})
			break
		}
	}
	return out
}

func isRegistrationLikeCall(result *body.Result, site factflow.CallSite, point cfg.Point, fact semantics.CallFact) bool {
	if _, ok := registrationCallFromFact(result, site, fact, point); ok {
		return true
	}
	_, keyIndex, ok := registrationRegistryAndKeyIndex(result, site, fact)
	if !ok || keyIndex < 0 || keyIndex >= len(fact.Args)-1 {
		return false
	}
	return registrationCallbackExpr(result, point, fact.Args[keyIndex+1])
}

func dispatchRegistryAndArgs(result *body.Result, site factflow.CallSite, fact semantics.CallFact) (pathdom.Path, []ast.Expr, bool) {
	if registry, ok := callSiteMemberReceiverPath(site); ok && len(fact.Args) > 0 {
		return registry, fact.Args, true
	}
	if len(fact.Args) >= 2 {
		registry, ok := result.ExpressionPath(fact.Args[0])
		if ok && registry.Symbol != 0 {
			return registry, fact.Args[1:], true
		}
	}
	return pathdom.Path{}, nil, false
}

func callSiteMemberReceiverPath(site factflow.CallSite) (pathdom.Path, bool) {
	receiver, _, ok := site.CalleeMemberAccessPath()
	if !ok || receiver.IsEmpty() {
		return pathdom.Path{}, false
	}
	return receiver, true
}
