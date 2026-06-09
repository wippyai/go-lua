package flow

// pointRelationIndex is the read-side semantic program over point-state
// relations. Storage carriers keep their row formats private; consumers ask this
// index for source routes by relation kind.
type pointRelationIndex struct {
	aliases PathAliasFacts
	origins ValueOriginFacts
}

type relationSourceKind uint8

const (
	relationSourceIdentityAlias relationSourceKind = iota + 1
	relationSourceAssignmentAlias
	relationSourceIndexedIteratorValue
	relationSourceExactIndexedIteratorValue
	relationSourceKeyedIterator
)

type relationSourceQuery struct {
	Target         StableAddress
	Kind           relationSourceKind
	IdentityPolicy IdentityAliasRoutePolicy
	VarIndex       int
	Remainder      valueOriginRouteRemainder
}

func relationIndexOf(state PointState) pointRelationIndex {
	return pointRelationIndex{
		aliases: PathAliasesOf(state),
		origins: ValueOriginsOf(state),
	}
}

func relationIndexForOrigins(origins ValueOriginFacts) pointRelationIndex {
	return pointRelationIndex{origins: origins}
}

func (idx pointRelationIndex) SourceRoutes(q relationSourceQuery) []sourceRoute {
	if q.Target.Key() == "" {
		return nil
	}
	switch q.Kind {
	case relationSourceIdentityAlias:
		policy := q.IdentityPolicy
		if !policy.PathAliasFacts && !policy.AssignmentOrigins {
			policy = IdentityAliasReadPolicy
		}
		return idx.identityRoutes(q.Target, policy)
	case relationSourceAssignmentAlias:
		return idx.valueOriginRoutes(valueOriginRouteQuery{
			value: q.Target,
			kind:  ValueOriginAssignmentAlias,
		})
	case relationSourceIndexedIteratorValue:
		return idx.valueOriginRoutes(valueOriginRouteQuery{
			value:     q.Target,
			kind:      ValueOriginIndexedIterator,
			varIndex:  1,
			hasVar:    true,
			remainder: q.Remainder,
		})
	case relationSourceExactIndexedIteratorValue:
		return idx.valueOriginRoutes(valueOriginRouteQuery{
			value:     q.Target,
			kind:      ValueOriginIndexedIterator,
			varIndex:  q.VarIndex,
			hasVar:    true,
			remainder: valueOriginRouteForbidRemainder,
		})
	case relationSourceKeyedIterator:
		return idx.valueOriginRoutes(valueOriginRouteQuery{
			value: q.Target,
			kind:  ValueOriginKeyedIterator,
		})
	default:
		return nil
	}
}

func (idx pointRelationIndex) identityRoutes(target StableAddress, policy IdentityAliasRoutePolicy) []sourceRoute {
	var out []sourceRoute
	appendRoutes := func(routes []sourceRoute) {
		for _, route := range routes {
			if policy.RequireRemainder && len(route.remainder) == 0 {
				continue
			}
			out = append(out, route)
		}
	}
	if policy.PathAliasFacts {
		appendRoutes(idx.pathAliasRoutes(target))
	}
	if policy.AssignmentOrigins {
		appendRoutes(idx.valueOriginRoutes(valueOriginRouteQuery{
			value: target,
			kind:  ValueOriginAssignmentAlias,
		}))
	}
	return out
}

func (idx pointRelationIndex) pathAliasRoutes(target StableAddress) []sourceRoute {
	uses := idx.aliases.AliasesCoveringAddress(target)
	if len(uses) == 0 {
		return nil
	}
	var out []sourceRoute
	for _, use := range uses {
		route, ok := use.SourceRoute()
		if !ok {
			continue
		}
		out = append(out, route)
	}
	return out
}

func (idx pointRelationIndex) valueOriginRoutes(q valueOriginRouteQuery) []sourceRoute {
	return idx.origins.sourceRoutes(q)
}

func provenanceRouteKindForValueOrigin(kind ValueOriginKind) ProvenanceRouteKind {
	switch kind {
	case ValueOriginIndexedIterator:
		return ProvenanceRouteIndexedIterator
	case ValueOriginKeyedIterator:
		return ProvenanceRouteKeyedIterator
	case ValueOriginAssignmentAlias:
		return ProvenanceRouteIdentityAlias
	default:
		return 0
	}
}
