package readmodel

import (
	"sort"
	"strings"

	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	typeformat "github.com/wippyai/go-lua/analysis/type/format"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

type RegistrationExhaustiveness = readapi.RegistrationExhaustiveness

type registrationReadCall struct {
	point    cfg.Point
	registry path.Path
	key      string
	span     SourceSpan
}

type openRegistrationMutation struct {
	point          cfg.Point
	path           path.Path
	registry       path.Path
	key            string
	hasKey         bool
	opensAll       bool
	aliasSensitive bool
	mayRegister    bool
}

type dispatchReadCall struct {
	point        cfg.Point
	registry     path.Path
	discriminant path.Path
	cases        []registrationDiscriminantCase
	span         SourceSpan
}

type registrationDiscriminantCase struct {
	index int
	name  string
	key   string
}

// ForEachRegistrationExhaustiveness visits callback registries that are
// dispatched over a discriminated union without registering every case.
func (r Reader) ForEachRegistrationExhaustiveness(visit func(RegistrationExhaustiveness) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	registrations, open := r.registrationCalls()
	if len(registrations) == 0 {
		return false
	}
	visited := false
	for _, dispatch := range r.dispatchCalls() {
		seen := make(map[string]registrationReadCall)
		for _, reg := range registrations {
			if !r.result.PathsAliasAtBoundary(reg.point, reg.registry, dispatch.registry) ||
				!r.result.PointCanReach(reg.point, dispatch.point) {
				continue
			}
			if r.registrationInvalidatedBeforeDispatch(open, reg, dispatch) {
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
		item, ok := r.registrationExhaustiveness(dispatch, open, seen)
		if !ok {
			continue
		}
		visited = true
		if !visit(item) {
			return true
		}
	}
	return visited
}

func (r Reader) registrationCalls() ([]registrationReadCall, []openRegistrationMutation) {
	var registrations []registrationReadCall
	var open []openRegistrationMutation
	for _, point := range cfg.RPOReadOnly(r.result.Graph()) {
		call, ok := r.result.Call(point)
		if !ok || call.Call == nil {
			if assignment, ok := r.result.OrdinaryAssignment(point); ok {
				if reg, ok := r.registrationAssignment(assignment, point); ok {
					registrations = append(registrations, reg)
					continue
				}
				if mutation, ok := r.openRegistrationAssignment(point, assignment); ok {
					mutation.point = point
					open = append(open, mutation)
				}
			}
			continue
		}
		site, ok := r.result.CallSite(point)
		if !ok {
			continue
		}
		if reg, ok := r.registrationCall(site, call, point); ok {
			registrations = append(registrations, reg)
			continue
		}
		if mutation, ok := r.openRegistrationMutation(site, point, call); ok {
			mutation.point = point
			open = append(open, mutation)
		}
	}
	return registrations, open
}

func (r Reader) registrationAssignment(fact semantics.OrdinaryAssignmentFact, point cfg.Point) (registrationReadCall, bool) {
	registry, key, ok := r.registrationAssignmentTarget(point, fact)
	if !ok || !r.registrationCallbackExpr(point, fact.Value) {
		return registrationReadCall{}, false
	}
	return registrationReadCall{
		point:    point,
		registry: registry,
		key:      key,
		span:     sourceSpanFromAST(ast.SpanOf(fact.Target)),
	}, true
}

func (r Reader) openRegistrationAssignment(point cfg.Point, fact semantics.OrdinaryAssignmentFact) (openRegistrationMutation, bool) {
	if registry, key, ok := r.registrationAssignmentTarget(point, fact); ok {
		return openRegistrationMutation{
			path:           registry,
			registry:       registry,
			key:            key,
			hasKey:         true,
			aliasSensitive: true,
			mayRegister:    r.registrationValueMayRegister(point, fact.Value),
		}, true
	}
	if fact.HasPath && fact.Path.Symbol != 0 {
		mutation := openRegistrationMutation{
			path:           fact.Path,
			aliasSensitive: true,
			mayRegister:    r.registrationValueMayRegister(point, fact.Value),
		}
		if key, ok := fact.Path.DirectFieldName(); ok {
			mutation.registry = path.Path{Root: fact.Path.Root, Symbol: fact.Path.Symbol, Version: fact.Path.Version}
			mutation.key = key
			mutation.hasKey = true
			return mutation, true
		}
		if seg, ok := fact.Path.LastSegment(); ok {
			if key, keyOK := registrationSegmentStringKey(seg); keyOK {
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
		return openRegistrationMutation{path: path.Path{Symbol: fact.Symbol}, opensAll: true, mayRegister: true}, true
	}
	return openRegistrationMutation{}, false
}

func (r Reader) registrationAssignmentTarget(point cfg.Point, fact semantics.OrdinaryAssignmentFact) (path.Path, string, bool) {
	if fact.HasPath && fact.Path.Symbol != 0 {
		if seg, ok := fact.Path.LastSegment(); ok {
			if key, keyOK := registrationSegmentStringKey(seg); keyOK {
				return fact.Path.Parent(), key, true
			}
		}
	}
	attr, ok := fact.Target.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return path.Path{}, "", false
	}
	registry, ok := r.result.ExpressionPath(attr.Object)
	if !ok || registry.Symbol == 0 {
		return path.Path{}, "", false
	}
	key, ok := r.result.StaticStringExprValueAtBoundary(point, attr.Key)
	return registry, key, ok
}

func (r Reader) registrationCall(site factflow.CallSite, fact semantics.CallFact, point cfg.Point) (registrationReadCall, bool) {
	registry, keyIndex, ok := registrationRegistryAndKeyIndex(r.result, site, fact)
	if !ok || keyIndex < 0 || keyIndex >= len(fact.Args)-1 {
		return registrationReadCall{}, false
	}
	key, ok := r.result.StaticStringExprValueAtBoundary(point, fact.Args[keyIndex])
	if !ok || !r.registrationCallbackExpr(point, fact.Args[keyIndex+1]) {
		return registrationReadCall{}, false
	}
	return registrationReadCall{
		point:    point,
		registry: registry,
		key:      key,
		span:     sourceSpanFromAST(ast.SpanOf(fact.Call)),
	}, true
}

func registrationRegistryAndKeyIndex(result resultPathResolver, site factflow.CallSite, fact semantics.CallFact) (path.Path, int, bool) {
	if fact.Call == nil {
		return path.Path{}, 0, false
	}
	if registry, ok := callSiteMemberReceiverPath(site); ok && len(fact.Args) >= 2 {
		return registry, 0, true
	}
	if len(fact.Args) >= 3 {
		if registry, ok := result.ExpressionPath(fact.Args[0]); ok && registry.Symbol != 0 {
			return registry, 1, true
		}
	}
	return path.Path{}, 0, false
}

type resultPathResolver interface {
	ExpressionPath(ast.Expr) (path.Path, bool)
}

func (r Reader) openRegistrationMutation(site factflow.CallSite, point cfg.Point, fact semantics.CallFact) (openRegistrationMutation, bool) {
	registry, keyIndex, ok := registrationRegistryAndKeyIndex(r.result, site, fact)
	if ok && keyIndex >= 0 && keyIndex < len(fact.Args)-1 {
		if _, ok := r.result.StaticStringExprValueAtBoundary(point, fact.Args[keyIndex]); ok && r.registrationCallbackExpr(point, fact.Args[keyIndex+1]) {
			return openRegistrationMutation{}, false
		}
		if r.registrationCallbackExpr(point, fact.Args[keyIndex+1]) {
			return openRegistrationMutation{path: registry, opensAll: true, aliasSensitive: true, mayRegister: true}, true
		}
	}
	if receiver, ok := callSiteMemberReceiverPath(site); ok && r.result.CallMayInvalidateTrackedPath(point, receiver) {
		return openRegistrationMutation{path: receiver, opensAll: true, aliasSensitive: true, mayRegister: true}, true
	}
	for _, arg := range fact.Args {
		argPath, ok := r.result.ExpressionPath(arg)
		if ok && r.result.CallMayInvalidateTrackedPath(point, argPath) {
			return openRegistrationMutation{path: argPath, opensAll: true, aliasSensitive: true, mayRegister: true}, true
		}
	}
	return openRegistrationMutation{}, false
}

func (r Reader) registrationValueMayRegister(point cfg.Point, expr ast.Expr) bool {
	return r.result.ExpressionMayBeFunctionBeforeBoundary(point, expr)
}

func (r Reader) registrationCallbackExpr(point cfg.Point, expr ast.Expr) bool {
	return r.result.ExpressionProvenFunctionAtBoundary(point, expr)
}

func (r Reader) dispatchCalls() []dispatchReadCall {
	var out []dispatchReadCall
	for _, point := range cfg.RPOReadOnly(r.result.Graph()) {
		fact, ok := r.result.Call(point)
		if !ok || fact.Call == nil {
			continue
		}
		site, ok := r.result.CallSite(point)
		if !ok || r.isRegistrationLikeCall(site, point, fact) {
			continue
		}
		registry, args, ok := dispatchRegistryAndArgs(r.result, site, fact)
		if !ok {
			continue
		}
		for _, arg := range args {
			argPath, ok := r.result.ExpressionPath(arg)
			if !ok || argPath.Symbol == 0 {
				continue
			}
			discriminant, cases, ok := r.registrationStringDiscriminantCasesForArgument(point, argPath)
			if !ok {
				continue
			}
			out = append(out, dispatchReadCall{
				point:        point,
				registry:     registry,
				discriminant: discriminant,
				cases:        cases,
				span:         sourceSpanFromAST(ast.SpanOf(fact.Call)),
			})
			break
		}
	}
	return out
}

func (r Reader) isRegistrationLikeCall(site factflow.CallSite, point cfg.Point, fact semantics.CallFact) bool {
	if _, ok := r.registrationCall(site, fact, point); ok {
		return true
	}
	_, keyIndex, ok := registrationRegistryAndKeyIndex(r.result, site, fact)
	if !ok || keyIndex < 0 || keyIndex >= len(fact.Args)-1 {
		return false
	}
	return r.registrationCallbackExpr(point, fact.Args[keyIndex+1])
}

func dispatchRegistryAndArgs(result resultPathResolver, site factflow.CallSite, fact semantics.CallFact) (path.Path, []ast.Expr, bool) {
	if registry, ok := callSiteMemberReceiverPath(site); ok && len(fact.Args) > 0 {
		return registry, fact.Args, true
	}
	if len(fact.Args) >= 2 {
		registry, ok := result.ExpressionPath(fact.Args[0])
		if ok && registry.Symbol != 0 {
			return registry, fact.Args[1:], true
		}
	}
	return path.Path{}, nil, false
}

func callSiteMemberReceiverPath(site factflow.CallSite) (path.Path, bool) {
	receiver, _, ok := site.CalleeMemberAccessPath()
	if !ok || receiver.IsEmpty() {
		return path.Path{}, false
	}
	return receiver, true
}

func (r Reader) registrationInvalidatedBeforeDispatch(open []openRegistrationMutation, reg registrationReadCall, dispatch dispatchReadCall) bool {
	for _, mutation := range open {
		if !mutation.hasKey || mutation.key != reg.key {
			continue
		}
		if mutation.point == reg.point || mutation.point == dispatch.point ||
			!r.result.PointCanReach(reg.point, mutation.point) ||
			!r.result.PointCanReach(mutation.point, dispatch.point) {
			continue
		}
		if r.result.PathsAliasAtBoundary(mutation.point, mutation.registry, reg.registry) {
			return true
		}
	}
	return false
}

func (r Reader) openRegistrationCanCoverCase(open []openRegistrationMutation, dispatch dispatchReadCall, c registrationDiscriminantCase) bool {
	for _, mutation := range open {
		if !mutation.mayRegister {
			continue
		}
		if mutation.point == dispatch.point || !r.result.PointCanReach(mutation.point, dispatch.point) {
			continue
		}
		if mutation.hasKey {
			if r.result.PathsAliasAtBoundary(mutation.point, mutation.registry, dispatch.registry) &&
				mutation.key == c.key {
				return true
			}
			continue
		}
		if mutation.opensAll {
			if mutation.path.Overlaps(dispatch.registry) {
				return true
			}
			if mutation.aliasSensitive && r.result.PathsAliasAtBoundary(mutation.point, mutation.path, dispatch.registry) {
				return true
			}
			continue
		}
		if dispatch.registry.HasPrefix(mutation.path) {
			return true
		}
	}
	return false
}

func (r Reader) registrationExhaustiveness(dispatch dispatchReadCall, open []openRegistrationMutation, registrations map[string]registrationReadCall) (RegistrationExhaustiveness, bool) {
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
		if r.openRegistrationCanCoverCase(open, dispatch, c) {
			continue
		}
		missing = append(missing, registrationCaseName(dispatch.registry.String(), c.key))
		missingFor = append(missingFor, c.name)
	}
	if !matched || len(missing) == 0 {
		return RegistrationExhaustiveness{}, false
	}
	sort.Strings(registered)
	return RegistrationExhaustiveness{
		Point:            dispatch.point,
		Registry:         dispatch.registry.String(),
		Target:           dispatch.discriminant.String(),
		Possible:         possible,
		Registered:       registered,
		Missing:          missing,
		MissingFor:       missingFor,
		RegistrationSpan: firstRegistrationSpan(registrations),
		DispatchSpan:     dispatch.span,
	}, true
}

func firstRegistrationSpan(registrations map[string]registrationReadCall) SourceSpan {
	var span SourceSpan
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

func (r Reader) registrationStringDiscriminantCasesForArgument(point cfg.Point, argPath path.Path) (path.Path, []registrationDiscriminantCase, bool) {
	if len(argPath.Segments) > 0 {
		cases, ok := r.registrationStringDiscriminantCases(point, argPath)
		return argPath, cases, ok
	}
	for _, domain := range r.registrationStringDiscriminantDomainsForRoot(point, argPath) {
		return domain.target, domain.cases, true
	}
	return path.Path{}, nil, false
}

type registrationStringDiscriminantDomain struct {
	target path.Path
	cases  []registrationDiscriminantCase
}

func (r Reader) registrationStringDiscriminantDomainsForRoot(point cfg.Point, root path.Path) []registrationStringDiscriminantDomain {
	rootType, ok := r.discriminatedUnionRootType(point, root)
	if !ok {
		return nil
	}
	out := r.registrationStringDiscriminantDomainsForType(root, nil, rootType, 0)
	sort.Slice(out, func(i, j int) bool {
		return out[i].target.String() < out[j].target.String()
	})
	return out
}

func (r Reader) registrationStringDiscriminantDomainsForType(root path.Path, prefix []segment.Segment, t typ.Type, depth int) []registrationStringDiscriminantDomain {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil
	}
	if _, cases, ok := variant.OriginCasesOfType(t); ok && len(cases) >= 2 {
		return registrationStringDiscriminantDomainsForCases(root, prefix, cases)
	}
	var out []registrationStringDiscriminantDomain
	for _, child := range r.registrationStaticDiscriminantChildren(t, depth) {
		nextPrefix := appendSegment(prefix, child.segment)
		out = append(out, r.registrationStringDiscriminantDomainsForType(root, nextPrefix, child.typ, depth+1)...)
	}
	return out
}

func registrationStringDiscriminantDomainsForCases(root path.Path, prefix []segment.Segment, cases []variant.OriginCase) []registrationStringDiscriminantDomain {
	var out []registrationStringDiscriminantDomain
	domains, ok := variant.LiteralDiscriminantDomainsForCases(cases)
	if !ok {
		return nil
	}
	for _, domain := range domains {
		suffix := domain.Suffix
		target := root.AppendSegments(prefix).AppendSegments(suffix)
		domainCases, ok := registrationStringDiscriminantCasesFor(target, suffix, cases)
		if !ok {
			continue
		}
		out = append(out, registrationStringDiscriminantDomain{target: target, cases: domainCases})
	}
	return out
}

type registrationStaticDiscriminantChild struct {
	segment segment.Segment
	typ     typ.Type
}

func (r Reader) registrationStaticDiscriminantChildren(t typ.Type, depth int) []registrationStaticDiscriminantChild {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Alias:
		return r.registrationStaticDiscriminantChildren(v.UnaliasedTarget(), depth+1)
	case *typ.Optional:
		return r.registrationStaticDiscriminantChildren(v.Inner, depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return nil
		}
		return r.registrationStaticDiscriminantChildren(v.Body, depth+1)
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return nil
		}
		return r.registrationStaticDiscriminantChildren(expanded, depth+1)
	case *typ.Record:
		out := make([]registrationStaticDiscriminantChild, 0, len(v.Fields)+len(v.StaticMembers))
		for _, field := range v.Fields {
			out = append(out, registrationStaticDiscriminantChild{
				segment: segment.Segment{Kind: segment.SegmentField, Name: field.Name},
				typ:     field.Type,
			})
		}
		for _, member := range v.StaticMembers {
			if member.Kind != typ.StaticMemberStringIndex {
				continue
			}
			out = append(out, registrationStaticDiscriminantChild{
				segment: segment.Segment{Kind: segment.SegmentIndexString, Name: member.Name},
				typ:     member.Type,
			})
		}
		return out
	default:
		return nil
	}
}

func (r Reader) registrationStringDiscriminantCases(point cfg.Point, target path.Path) ([]registrationDiscriminantCase, bool) {
	for _, anchor := range r.discriminatedUnionAnchors(point, target) {
		_, cases, ok := variant.OriginCasesOfType(anchor.anchorType)
		if !ok || len(cases) < 2 {
			continue
		}
		domainCases, ok := registrationStringDiscriminantCasesFor(target, anchor.suffix, cases)
		if !ok {
			continue
		}
		return domainCases, true
	}
	return nil, false
}

func registrationStringDiscriminantCasesFor(target path.Path, suffix []segment.Segment, cases []variant.OriginCase) ([]registrationDiscriminantCase, bool) {
	out := make([]registrationDiscriminantCase, 0, len(cases))
	seen := make(map[string]struct{}, len(cases))
	for _, c := range cases {
		key, ok := registrationDiscriminantCaseStringKey(c.Type, suffix)
		if !ok {
			return nil, false
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, false
		}
		seen[key] = struct{}{}
		out = append(out, registrationDiscriminantCase{
			index: c.Index,
			name:  registrationDiscriminantCaseName(target, suffix, c.Type),
			key:   key,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].name != out[j].name {
			return out[i].name < out[j].name
		}
		return out[i].index < out[j].index
	})
	return out, true
}

func registrationDiscriminantCaseName(target path.Path, suffix []segment.Segment, caseType typ.Type) string {
	if field, ok := variant.FieldAtPath(caseType, suffix); ok {
		return target.String() + " == " + typeformat.Short(field)
	}
	return typeformat.Short(caseType)
}

func registrationDiscriminantCaseStringKey(caseType typ.Type, suffix []segment.Segment) (string, bool) {
	field, ok := variant.FieldAtPath(caseType, suffix)
	if !ok {
		return "", false
	}
	lit, ok := field.(*typ.Literal)
	if !ok {
		return "", false
	}
	value, ok := lit.Value.(string)
	return value, ok
}

func registrationSegmentStringKey(seg segment.Segment) (string, bool) {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return seg.Name, seg.Name != ""
	default:
		return "", false
	}
}

func registrationCaseName(registry, key string) string {
	if identifierName(key) {
		return registry + "." + key
	}
	return registry + "[" + typeformat.Short(typ.LiteralString(key)) + "]"
}

func identifierName(s string) bool {
	if s == "" {
		return false
	}
	if !((s[0] >= 'A' && s[0] <= 'Z') || (s[0] >= 'a' && s[0] <= 'z') || s[0] == '_') {
		return false
	}
	for i := 1; i < len(s); i++ {
		ch := s[i]
		if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_') {
			return false
		}
	}
	return true
}

func appendSegment(prefix []segment.Segment, seg segment.Segment) []segment.Segment {
	next := make([]segment.Segment, 0, len(prefix)+1)
	next = append(next, prefix...)
	next = append(next, seg)
	return next
}

func caseListKey(cases []string) string {
	return strings.Join(cases, "\x1f")
}
