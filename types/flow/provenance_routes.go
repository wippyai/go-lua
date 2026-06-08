package flow

import "github.com/wippyai/go-lua/types/constraint"

// ProvenanceRouteQuery selects the provenance edges to expose for a path.
type ProvenanceRouteQuery struct {
	Path                      constraint.Path
	IdentityAliases           bool
	IdentityAliasPolicy       IdentityAliasRoutePolicy
	ValueOrigins              bool
	AppendElementFieldOrigins bool
}

// AppendElementFieldRouteQuery asks for source routes for one element field of a
// tracked append-history array.
type AppendElementFieldRouteQuery struct {
	ArrayPath constraint.Path
	Field     []constraint.Segment
}

// ProvenanceRoutes returns one-step source routes that may explain a read of
// path. It composes the currently separate alias, value-origin, and append-field
// origin domains into one normalized route boundary for checker layers.
func (f PointFacts) ProvenanceRoutes(path constraint.Path) []ProvenanceRoute {
	return f.ProvenanceRoutesFor(ProvenanceRouteQuery{
		Path:                      path,
		IdentityAliases:           true,
		IdentityAliasPolicy:       IdentityAliasReadPolicy,
		ValueOrigins:              true,
		AppendElementFieldOrigins: true,
	})
}

// ProvenanceRoutesFor returns one-step source routes selected by query. Flow
// owns the storage-domain composition; caller layers choose which semantic edge
// classes are meaningful for their operation.
func (f PointFacts) ProvenanceRoutesFor(q ProvenanceRouteQuery) []ProvenanceRoute {
	target, ok := StableAddressOfPath(q.Path)
	if !ok {
		return nil
	}
	relations := relationIndexOf(f.state)
	var out []ProvenanceRoute
	if q.IdentityAliases {
		for _, route := range relations.SourceRoutes(relationSourceQuery{
			Target:         target,
			Kind:           relationSourceIdentityAlias,
			IdentityPolicy: q.IdentityAliasPolicy,
		}) {
			source, ok := route.appendedSource()
			if !ok {
				continue
			}
			sourcePath, ok := source.Path()
			if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
				continue
			}
			out = append(out, ProvenanceRoute{
				Kind:   ProvenanceRouteIdentityAlias,
				Source: sourcePath,
			})
		}
	}
	if q.ValueOrigins || q.AppendElementFieldOrigins {
		for _, route := range relations.SourceRoutes(relationSourceQuery{
			Target:    target,
			Kind:      relationSourceIndexedIteratorValue,
			Remainder: valueOriginRouteAnyRemainder,
		}) {
			if q.AppendElementFieldOrigins {
				out = appendAppendFieldProvenanceRoutes(out, f.state, route)
			}
			if q.ValueOrigins {
				out = appendValueOriginProvenanceRoute(out, route)
			}
		}
		if q.ValueOrigins {
			for _, route := range relations.SourceRoutes(relationSourceQuery{
				Target: target,
				Kind:   relationSourceKeyedIterator,
			}) {
				out = appendValueOriginProvenanceRoute(out, route)
			}
		}
	}
	return out
}

func appendValueOriginProvenanceRoute(out []ProvenanceRoute, route sourceRoute) []ProvenanceRoute {
	source, ok := route.source.Path()
	if !ok || source.IsEmpty() || source.Symbol == 0 {
		return out
	}
	return append(out, ProvenanceRoute{
		Kind:      route.kind,
		Source:    source,
		Remainder: cloneAddressSegments(route.remainder),
		VarIndex:  route.varIndex,
	})
}

func appendAppendFieldProvenanceRoutes(out []ProvenanceRoute, state PointState, route sourceRoute) []ProvenanceRoute {
	if len(route.remainder) == 0 || route.source.Key() == "" {
		return out
	}
	return appendAppendFieldSourceRoutes(out, state.KeyPresence.AppendElementFieldSourceAddresses(AppendElementFieldSourceQuery{
		Array: route.source,
		Field: route.remainder,
	}))
}

// AppendElementFieldSourceRoutes returns source routes for a structural demand
// on an appended element field.
func (f PointFacts) AppendElementFieldSourceRoutes(q AppendElementFieldRouteQuery) []ProvenanceRoute {
	array, ok := StableAddressOfPath(q.ArrayPath)
	if !ok {
		return nil
	}
	uses := f.state.KeyPresence.AppendElementFieldSourceAddresses(AppendElementFieldSourceQuery{
		Array: array,
		Field: q.Field,
	})
	return appendAppendFieldSourceRoutes(nil, uses)
}

func appendAppendFieldSourceRoutes(out []ProvenanceRoute, sources []AppendElementFieldSourceAddress) []ProvenanceRoute {
	for _, source := range sources {
		out = appendAppendFieldSourceRoute(out, source)
	}
	return out
}

func appendAppendFieldSourceRoute(out []ProvenanceRoute, sourceRoute AppendElementFieldSourceAddress) []ProvenanceRoute {
	source, ok := sourceRoute.SourcePath()
	if !ok || source.IsEmpty() || source.Symbol == 0 {
		return out
	}
	out = append(out, ProvenanceRoute{
		Kind:           ProvenanceRouteAppendElementField,
		Source:         source,
		SourceField:    cloneAddressSegments(sourceRoute.SourceField),
		FieldRemainder: cloneAddressSegments(sourceRoute.FieldRemainder),
	})
	return out
}
