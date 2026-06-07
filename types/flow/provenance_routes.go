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
	var out []ProvenanceRoute
	if q.IdentityAliases {
		policy := q.IdentityAliasPolicy
		if !policy.PathAliases && !policy.AssignmentOrigins {
			policy = IdentityAliasReadPolicy
		}
		for _, source := range IdentityAliasSourcesWithPolicy(f.state, target, policy) {
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
		for _, use := range f.state.ValueOrigins.OriginsCoveringAddress(target) {
			switch use.Origin.Kind {
			case ValueOriginIndexedIterator:
				if q.AppendElementFieldOrigins {
					out = appendAppendFieldProvenanceRoutes(out, f.state, use)
				}
				if q.ValueOrigins {
					out = appendValueOriginProvenanceRoute(out, use, ProvenanceRouteIndexedIterator)
				}
			case ValueOriginKeyedIterator:
				if q.ValueOrigins {
					out = appendValueOriginProvenanceRoute(out, use, ProvenanceRouteKeyedIterator)
				}
			}
		}
	}
	return out
}

func appendValueOriginProvenanceRoute(out []ProvenanceRoute, use ValueOriginUse, kind ProvenanceRouteKind) []ProvenanceRoute {
	source, ok := use.SourcePath()
	if !ok || source.IsEmpty() || source.Symbol == 0 {
		return out
	}
	return append(out, ProvenanceRoute{
		Kind:      kind,
		Source:    source,
		Remainder: cloneAddressSegments(use.Remainder),
		VarIndex:  use.Origin.VarIndex,
	})
}

func appendAppendFieldProvenanceRoutes(out []ProvenanceRoute, state PointState, use ValueOriginUse) []ProvenanceRoute {
	if len(use.Remainder) == 0 {
		return out
	}
	source, ok := use.SourceAddress()
	if !ok {
		return out
	}
	return appendAppendFieldSourceRoutes(out, state.KeyPresence.AppendElementFieldOriginUses(AppendElementFieldSourceQuery{
		Array: source,
		Field: use.Remainder,
	}))
}

// AppendElementFieldSourceRoutes returns source routes for a structural demand
// on an appended element field.
func (f PointFacts) AppendElementFieldSourceRoutes(q AppendElementFieldRouteQuery) []ProvenanceRoute {
	array, ok := StableAddressOfPath(q.ArrayPath)
	if !ok {
		return nil
	}
	uses := f.state.KeyPresence.AppendElementFieldOriginUses(AppendElementFieldSourceQuery{
		Array: array,
		Field: q.Field,
	})
	return appendAppendFieldSourceRoutes(nil, uses)
}

func appendAppendFieldSourceRoutes(out []ProvenanceRoute, uses []AppendElementFieldOriginUse) []ProvenanceRoute {
	for _, use := range uses {
		out = appendAppendFieldSourceRoute(out, use)
	}
	return out
}

func appendAppendFieldSourceRoute(out []ProvenanceRoute, use AppendElementFieldOriginUse) []ProvenanceRoute {
	source, ok := use.SourcePath()
	if !ok || source.IsEmpty() || source.Symbol == 0 {
		return out
	}
	out = append(out, ProvenanceRoute{
		Kind:           ProvenanceRouteAppendElementField,
		Source:         source,
		SourceField:    cloneAddressSegments(use.SourceField),
		FieldRemainder: cloneAddressSegments(use.FieldRemainder),
	})
	return out
}
