package diagnostics

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/compiler/ast"
)

type registrationEvidence struct {
	registry         string
	target           string
	possible         []string
	registered       []string
	missing          []string
	missingFor       []string
	registrationSpan diagnostic.Span
	dispatchSpan     diagnostic.Span
}

type registrationCall struct {
	point    cfg.Point
	call     *ast.FuncCallExpr
	registry pathdom.Path
	key      string
	span     diagnostic.Span
}

type openRegistrationMutation struct {
	point          cfg.Point
	path           pathdom.Path
	registry       pathdom.Path
	key            string
	hasKey         bool
	opensAll       bool
	aliasSensitive bool
}

type dispatchCall struct {
	point        cfg.Point
	call         *ast.FuncCallExpr
	registry     pathdom.Path
	discriminant pathdom.Path
	cases        []discriminantCase
	span         diagnostic.Span
}

func (p discriminatedUnionExhaustiveness) registrationDiagnostics(result *body.Result, graph cfg.Graph) []diagnostic.Diagnostic {
	registrations, openRegistries := p.registrationCalls(result, graph)
	if len(registrations) == 0 {
		return nil
	}
	var out []diagnostic.Diagnostic
	for _, dispatch := range p.dispatchCalls(result, graph) {
		if p.openRegistrationCanReach(result, graph, openRegistries, dispatch) {
			continue
		}
		seen := make(map[string]registrationCall)
		for _, reg := range registrations {
			if !registrationRegistryMatchesAt(result, reg.point, reg.registry, dispatch.registry) ||
				!diagnosticCanReach(p.flow, graph, reg.point, dispatch.point) {
				continue
			}
			if p.registrationInvalidatedBeforeDispatch(result, graph, openRegistries, reg, dispatch) {
				continue
			}
			if existing, ok := seen[reg.key]; ok && existing.point > reg.point {
				continue
			}
			seen[reg.key] = reg
		}
		if len(seen) == 0 {
			continue
		}
		if diag, ok := registrationExhaustivenessDiagnosticFor(dispatch, seen); ok {
			out = append(out, diag)
		}
	}
	return out
}

func registrationExhaustivenessDiagnosticFor(dispatch dispatchCall, registrations map[string]registrationCall) (diagnostic.Diagnostic, bool) {
	var possible []string
	var registered []string
	var missing []string
	var missingFor []string
	matched := false
	for _, c := range dispatch.cases {
		possible = append(possible, c.name)
		if _, ok := registrations[c.key]; ok {
			matched = true
			registered = append(registered, registrationCaseName(dispatch.registry.String(), c.key))
			continue
		}
		missing = append(missing, registrationCaseName(dispatch.registry.String(), c.key))
		missingFor = append(missingFor, c.name)
	}
	if !matched || len(missing) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	sort.Strings(registered)
	regSpan := firstRegistrationSpan(registrations)
	return newRegistrationExhaustivenessDiagnostic(registrationEvidence{
		registry:         dispatch.registry.String(),
		target:           dispatch.discriminant.String(),
		possible:         possible,
		registered:       registered,
		missing:          missing,
		missingFor:       missingFor,
		registrationSpan: regSpan,
		dispatchSpan:     dispatch.span,
	}), true
}

func firstRegistrationSpan(registrations map[string]registrationCall) diagnostic.Span {
	var span diagnostic.Span
	var point cfg.Point
	first := true
	for _, reg := range registrations {
		if first || reg.point < point {
			span = reg.span
			point = reg.point
			first = false
		}
	}
	return span
}

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
		if reg, ok := registrationCallFromFact(result, call, point); ok {
			registrations = append(registrations, reg)
			continue
		}
		if mutation, ok := openRegistrationMutationFromFact(result, point, call); ok {
			mutation.point = point
			open = append(open, mutation)
		}
	}
	return registrations, open
}

func (p discriminatedUnionExhaustiveness) registrationInvalidatedBeforeDispatch(result *body.Result, graph cfg.Graph, open []openRegistrationMutation, reg registrationCall, dispatch dispatchCall) bool {
	for _, mutation := range open {
		if !mutation.hasKey || mutation.key != reg.key {
			continue
		}
		if mutation.point == reg.point || mutation.point == dispatch.point ||
			!diagnosticCanReach(p.flow, graph, reg.point, mutation.point) ||
			!diagnosticCanReach(p.flow, graph, mutation.point, dispatch.point) {
			continue
		}
		if registrationRegistryMatchesAt(result, mutation.point, mutation.registry, reg.registry) {
			return true
		}
	}
	return false
}

func (p discriminatedUnionExhaustiveness) openRegistrationCanReach(result *body.Result, graph cfg.Graph, open []openRegistrationMutation, dispatch dispatchCall) bool {
	for _, mutation := range open {
		if mutation.point == dispatch.point || !diagnosticCanReach(p.flow, graph, mutation.point, dispatch.point) {
			continue
		}
		if mutation.hasKey {
			continue
		}
		if mutation.opensAll {
			if mutation.path.Overlaps(dispatch.registry) {
				return true
			}
			if mutation.aliasSensitive && registrationRegistryMatchesAt(result, mutation.point, mutation.path, dispatch.registry) {
				return true
			}
			continue
		}
		if dispatch.registry.HasPrefix(mutation.path) {
			return true
		}
		if mutation.hasKey &&
			registrationRegistryMatchesAt(result, mutation.point, mutation.registry, dispatch.registry) &&
			registrationMutationKeyMatchesCase(mutation.key, dispatch.cases) {
			return true
		}
	}
	return false
}

func registrationRegistryMatchesAt(result *body.Result, point cfg.Point, left, right pathdom.Path) bool {
	if left.Equal(right) {
		return true
	}
	return result != nil && result.PathsEquivalentAtBoundary(point, left, right)
}

func registrationMutationKeyMatchesCase(key string, cases []discriminantCase) bool {
	for _, c := range cases {
		if c.key == key {
			return true
		}
	}
	return false
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
		}, true
	}
	if fact.HasPath && fact.Path.Symbol != 0 {
		mutation := openRegistrationMutation{path: fact.Path, aliasSensitive: true}
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
		return openRegistrationMutation{path: fact.ContainerPath, opensAll: true, aliasSensitive: true}, true
	}
	if fact.HasSymbol && fact.Symbol != 0 {
		return openRegistrationMutation{path: pathdom.Path{Symbol: fact.Symbol}, opensAll: true}, true
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
	key, ok := staticStringExprValueAt(result, point, attr.Key)
	return registry, key, ok
}

func registrationCallFromFact(result *body.Result, fact semantics.CallFact, point cfg.Point) (registrationCall, bool) {
	registry, keyIndex, ok := registrationRegistryAndKeyIndex(result, fact)
	if !ok || keyIndex < 0 || keyIndex >= len(fact.Args)-1 {
		return registrationCall{}, false
	}
	key, ok := staticStringExprValueAt(result, point, fact.Args[keyIndex])
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

func registrationRegistryAndKeyIndex(result *body.Result, fact semantics.CallFact) (pathdom.Path, int, bool) {
	if fact.Call == nil {
		return pathdom.Path{}, 0, false
	}
	if fact.HasReceiverPath && fact.Method != "" && len(fact.Args) >= 2 {
		return fact.ReceiverPath, 0, true
	}
	if len(fact.Args) >= 3 {
		if registry, ok := result.ExpressionPath(fact.Args[0]); ok && registry.Symbol != 0 {
			return registry, 1, true
		}
	}
	return pathdom.Path{}, 0, false
}

func openRegistrationMutationFromFact(result *body.Result, point cfg.Point, fact semantics.CallFact) (openRegistrationMutation, bool) {
	registry, keyIndex, ok := registrationRegistryAndKeyIndex(result, fact)
	if ok && keyIndex >= 0 && keyIndex < len(fact.Args)-1 {
		if _, ok := staticStringExprValueAt(result, point, fact.Args[keyIndex]); ok && registrationCallbackExpr(result, point, fact.Args[keyIndex+1]) {
			return openRegistrationMutation{}, false
		}
		if registrationCallbackExpr(result, point, fact.Args[keyIndex+1]) {
			return openRegistrationMutation{path: registry, opensAll: true, aliasSensitive: true}, true
		}
	}
	if fact.HasReceiverPath && callMayInvalidateTrackedPath(result, point, fact.ReceiverPath) {
		return openRegistrationMutation{path: fact.ReceiverPath, opensAll: true, aliasSensitive: true}, true
	}
	for _, arg := range fact.Args {
		argPath, ok := result.ExpressionPath(arg)
		if ok && callMayInvalidateTrackedPath(result, point, argPath) {
			return openRegistrationMutation{path: argPath, opensAll: true, aliasSensitive: true}, true
		}
	}
	return openRegistrationMutation{}, false
}

func registrationCallbackExpr(result *body.Result, point cfg.Point, expr ast.Expr) bool {
	if _, ok := directFunctionExprFromExpr(expr); ok {
		return true
	}
	if _, ok := result.FunctionValueTypeAtBoundary(point, expr); ok {
		return true
	}
	path, ok := result.ExpressionPath(expr)
	if !ok || path.IsEmpty() {
		return false
	}
	return registrationCallbackPathExpr(result, point, path, nil)
}

func registrationCallbackPathExpr(result *body.Result, point cfg.Point, target pathdom.Path, seen map[pathdom.PathKey]struct{}) bool {
	graph := result.Graph()
	if graph == nil || target.IsEmpty() {
		return false
	}
	key := target.Key()
	if _, ok := seen[key]; ok {
		return false
	}
	if seen == nil {
		seen = make(map[pathdom.PathKey]struct{}, 1)
	}
	seen[key] = struct{}{}
	if dominatingFunctionDefinitionForPath(result, point, target) != nil {
		return true
	}
	idom := dominance.ComputeImmediateDominatorInfo(graph).Map()
	visited := make(map[cfg.Point]struct{}, graph.Size())
	for cursor := point; ; {
		if _, ok := visited[cursor]; ok {
			return false
		}
		visited[cursor] = struct{}{}
		if fact, ok := result.LocalAssignment(cursor); ok &&
			len(target.Segments) == 0 &&
			fact.HasSymbol &&
			fact.Symbol == target.Symbol {
			return registrationCallbackSourceExpr(result, cursor, fact.Expr, seen)
		}
		if fact, ok := result.OrdinaryAssignment(cursor); ok {
			if len(target.Segments) == 0 &&
				fact.HasSymbol &&
				fact.Symbol == target.Symbol {
				return registrationCallbackSourceExpr(result, cursor, fact.Value, seen)
			}
			if fact.HasPath && fact.Path.Equal(target) {
				return registrationCallbackSourceExpr(result, cursor, fact.Value, seen)
			}
			if fact.HasPath && target.HasPrefix(fact.Path) {
				return false
			}
		}
		parent, ok := idom[cursor]
		if !ok || parent == cursor {
			return false
		}
		cursor = parent
	}
}

func registrationCallbackSourceExpr(result *body.Result, point cfg.Point, expr ast.Expr, seen map[pathdom.PathKey]struct{}) bool {
	if _, ok := directFunctionExprFromExpr(expr); ok {
		return true
	}
	if _, ok := result.FunctionValueTypeAtBoundary(point, expr); ok {
		return true
	}
	path, ok := result.ExpressionPath(expr)
	if !ok || path.IsEmpty() {
		return false
	}
	return registrationCallbackPathExpr(result, point, path, seen)
}

func (p discriminatedUnionExhaustiveness) dispatchCalls(result *body.Result, graph cfg.Graph) []dispatchCall {
	var out []dispatchCall
	for _, point := range graph.RPO() {
		fact, ok := result.Call(point)
		if !ok || fact.Call == nil || isRegistrationLikeCall(result, fact) {
			continue
		}
		registry, args, ok := dispatchRegistryAndArgs(result, fact)
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

func isRegistrationLikeCall(result *body.Result, fact semantics.CallFact) bool {
	if _, ok := registrationCallFromFact(result, fact, 0); ok {
		return true
	}
	if _, ok := openRegistrationMutationFromFact(result, 0, fact); ok {
		return true
	}
	return false
}

func dispatchRegistryAndArgs(result *body.Result, fact semantics.CallFact) (pathdom.Path, []ast.Expr, bool) {
	if fact.HasReceiverPath && fact.Method != "" && len(fact.Args) > 0 {
		return fact.ReceiverPath, fact.Args, true
	}
	if len(fact.Args) >= 2 {
		registry, ok := result.ExpressionPath(fact.Args[0])
		if ok && registry.Symbol != 0 {
			return registry, fact.Args[1:], true
		}
	}
	return pathdom.Path{}, nil, false
}
