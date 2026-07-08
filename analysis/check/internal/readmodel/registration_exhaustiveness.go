package readmodel

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
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
		site, hasSite := r.result.CallSiteView(point)
		if !hasSite {
			if write, ok := r.result.LoweredAssignmentWrite(point); ok {
				if reg, ok := r.registrationAssignment(write, point); ok {
					registrations = append(registrations, reg)
					continue
				}
				if mutation, ok := r.openRegistrationAssignment(point, write); ok {
					mutation.point = point
					open = append(open, mutation)
				}
			}
			continue
		}
		shape, hasShape := r.result.RegistryKeyCallShape(point, site)
		if hasShape {
			if reg, ok := r.registrationCall(shape, point); ok {
				registrations = append(registrations, reg)
				continue
			}
		}
		args := r.result.CallArgumentInfos(point, site)
		receiver, hasReceiver := body.CallSiteMemberReceiverPath(site)
		if mutation, ok := r.openRegistrationMutation(point, shape, hasShape, receiver, hasReceiver, args); ok {
			mutation.point = point
			open = append(open, mutation)
		}
	}
	return registrations, open
}

func (r Reader) registrationAssignment(write body.LoweredAssignmentWrite, point cfg.Point) (registrationReadCall, bool) {
	target, ok := write.StaticStringTarget()
	if !ok || !r.result.AssignmentSourceProvenFunctionAtBoundary(point, write.Source) {
		return registrationReadCall{}, false
	}
	return registrationReadCall{
		point:    point,
		registry: target.Container,
		key:      target.Key,
		span:     sourceSpanFromBody(writeSpanOrTarget(target, write)),
	}, true
}

func (r Reader) openRegistrationAssignment(point cfg.Point, write body.LoweredAssignmentWrite) (openRegistrationMutation, bool) {
	if target, ok := write.StaticStringTarget(); ok {
		return openRegistrationMutation{
			path:           target.Container,
			registry:       target.Container,
			key:            target.Key,
			hasKey:         true,
			aliasSensitive: true,
			mayRegister:    r.result.AssignmentSourceMayBeFunctionBeforeBoundary(point, write.Source),
		}, true
	}
	if !write.Target.IsEmpty() && write.Target.Symbol != 0 {
		mutation := openRegistrationMutation{
			path:           write.Target,
			aliasSensitive: true,
			mayRegister:    r.result.AssignmentSourceMayBeFunctionBeforeBoundary(point, write.Source),
		}
		if key, ok := write.Target.DirectFieldName(); ok {
			mutation.registry = path.Path{Root: write.Target.Root, Symbol: write.Target.Symbol, Version: write.Target.Version}
			mutation.key = key
			mutation.hasKey = true
			return mutation, true
		}
		if seg, ok := write.Target.LastSegment(); ok {
			if key, keyOK := registrationSegmentStringKey(seg); keyOK {
				mutation.registry = write.Target.Parent()
				mutation.key = key
				mutation.hasKey = true
				return mutation, true
			}
		}
		mutation.opensAll = true
		return mutation, true
	}
	return openRegistrationMutation{}, false
}

func writeSpanOrTarget(target body.StaticStringAssignmentTarget, write body.LoweredAssignmentWrite) body.SourceSpan {
	if write.HasSpan {
		return write.Span
	}
	return target.Span
}

func (r Reader) registrationCall(shape body.RegistryKeyCallShape, point cfg.Point) (registrationReadCall, bool) {
	keyIndex := shape.KeyIndex
	if keyIndex < 0 || keyIndex >= len(shape.Args)-1 {
		return registrationReadCall{}, false
	}
	keyArg := shape.Args[keyIndex]
	callbackArg := shape.Args[keyIndex+1]
	if !keyArg.HasStaticString || !callbackArg.ProvenFunction {
		return registrationReadCall{}, false
	}
	return registrationReadCall{
		point:    point,
		registry: shape.Registry,
		key:      keyArg.StaticString,
		span:     sourceSpanFromBody(shape.Span),
	}, true
}

func (r Reader) openRegistrationMutation(point cfg.Point, shape body.RegistryKeyCallShape, hasShape bool, receiver path.Path, hasReceiver bool, args []body.CallArgumentInfo) (openRegistrationMutation, bool) {
	if hasShape {
		keyIndex := shape.KeyIndex
		if keyIndex >= 0 && keyIndex < len(shape.Args)-1 {
			keyArg := shape.Args[keyIndex]
			callbackArg := shape.Args[keyIndex+1]
			if keyArg.HasStaticString && callbackArg.ProvenFunction {
				return openRegistrationMutation{}, false
			}
			if callbackArg.ProvenFunction {
				return openRegistrationMutation{path: shape.Registry, opensAll: true, aliasSensitive: true, mayRegister: true}, true
			}
		}
	}
	if hasReceiver && r.result.CallMayInvalidateTrackedPath(point, receiver) {
		return openRegistrationMutation{path: receiver, opensAll: true, aliasSensitive: true, mayRegister: true}, true
	}
	for _, arg := range args {
		if arg.HasPath && r.result.CallMayInvalidateTrackedPath(point, arg.Path) {
			return openRegistrationMutation{path: arg.Path, opensAll: true, aliasSensitive: true, mayRegister: true}, true
		}
	}
	return openRegistrationMutation{}, false
}

func (r Reader) dispatchCalls() []dispatchReadCall {
	var out []dispatchReadCall
	for _, point := range cfg.RPOReadOnly(r.result.Graph()) {
		site, ok := r.result.CallSiteView(point)
		if !ok || r.isRegistrationLikeCall(point, site) {
			continue
		}
		shape, ok := r.result.DispatchCallShape(point, site)
		if !ok {
			continue
		}
		for _, arg := range shape.Args {
			if !arg.HasPath || arg.Path.Symbol == 0 {
				continue
			}
			discriminant, cases, ok := r.registrationStringDiscriminantCasesForArgument(point, arg.Path)
			if !ok {
				continue
			}
			out = append(out, dispatchReadCall{
				point:        point,
				registry:     shape.Registry,
				discriminant: discriminant,
				cases:        cases,
				span:         sourceSpanFromBody(shape.Span),
			})
			break
		}
	}
	return out
}

func (r Reader) isRegistrationLikeCall(point cfg.Point, site factflow.CallSiteView) bool {
	shape, ok := r.result.RegistryKeyCallShape(point, site)
	if !ok {
		return false
	}
	if _, ok := r.registrationCall(shape, point); ok {
		return true
	}
	keyIndex := shape.KeyIndex
	return keyIndex >= 0 && keyIndex < len(shape.Args)-1 && shape.Args[keyIndex+1].ProvenFunction
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
		Point:             dispatch.point,
		Registry:          dispatch.registry.String(),
		Target:            dispatch.discriminant.String(),
		Possible:          possible,
		Registered:        registered,
		Missing:           missing,
		MissingFor:        missingFor,
		RegistrationSpan:  firstRegistrationSpan(registrations),
		RegistrationSpans: registrationSpans(registrations),
		DispatchSpan:      dispatch.span,
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

func registrationSpans(registrations map[string]registrationReadCall) []SourceSpan {
	items := make([]registrationReadCall, 0, len(registrations))
	for _, reg := range registrations {
		items = append(items, reg)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].point < items[j].point
	})
	out := make([]SourceSpan, 0, len(items))
	for _, item := range items {
		if item.span.Valid() {
			out = append(out, item.span)
		}
	}
	return out
}
