package diagnostics

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
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
	mayRegister    bool
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
		seen := make(map[string]registrationCall)
		for _, reg := range registrations {
			if !result.PathsAliasAtBoundary(reg.point, reg.registry, dispatch.registry) ||
				!result.PointCanReach(reg.point, dispatch.point) {
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
		if diag, ok := p.registrationExhaustivenessDiagnosticFor(result, graph, openRegistries, dispatch, seen); ok {
			out = append(out, diag)
		}
	}
	return out
}

func (p discriminatedUnionExhaustiveness) registrationExhaustivenessDiagnosticFor(result *body.Result, graph cfg.Graph, open []openRegistrationMutation, dispatch dispatchCall, registrations map[string]registrationCall) (diagnostic.Diagnostic, bool) {
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
		if p.openRegistrationCanCoverCase(result, graph, open, dispatch, c) {
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
