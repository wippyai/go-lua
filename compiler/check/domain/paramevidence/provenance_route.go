package paramevidence

import (
	"github.com/wippyai/go-lua/compiler/check/domain/provenance"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

// ProvenanceRouteContractTarget is one source-path obligation produced by
// interpreting a flow provenance route backward through the parameter-contract
// domain.
type ProvenanceRouteContractTarget struct {
	Path     constraint.Path
	Contract ParamContract
}

// ProvenanceRouteResolver returns one-step provenance routes for path.
type ProvenanceRouteResolver func(path constraint.Path) []flow.ProvenanceRoute

// ProvenanceRouteContractClosure follows provenance routes backward from a local
// path obligation to every source-path obligation implied by the current route
// graph. The first target is the seed obligation. Paths are visited once by
// stable identity, matching flow's point-state route semantics.
func ProvenanceRouteContractClosure(
	path constraint.Path,
	contract ParamContract,
	routes ProvenanceRouteResolver,
) []ProvenanceRouteContractTarget {
	if path.Symbol == 0 || isContractBottom(contract) {
		return nil
	}
	queue := []ProvenanceRouteContractTarget{{Path: path, Contract: contract}}
	seen := map[constraint.PathKey]struct{}{}
	var out []ProvenanceRouteContractTarget
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		key := flow.PathIdentityKey(cur.Path)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cur)
		if routes == nil {
			continue
		}
		for _, route := range routes(cur.Path) {
			queue = append(queue, ProvenanceRouteContractTargets(route, cur.Contract)...)
		}
	}
	return out
}

// ProvenanceRouteContractTargets translates a local obligation on a routed value
// back to obligations on the route's source path. Provenance owns route shape;
// paramevidence owns inversion into the parameter-contract algebra.
func ProvenanceRouteContractTargets(route flow.ProvenanceRoute, contract ParamContract) []ProvenanceRouteContractTarget {
	if route.Source.Symbol == 0 || isContractBottom(contract) {
		return nil
	}
	var out []ProvenanceRouteContractTarget
	for _, query := range provenance.RouteSourceQueries(route, nil) {
		source := sourceContractFromRouteQuery(query, contract)
		if isContractBottom(source) {
			continue
		}
		out = append(out, ProvenanceRouteContractTarget{Path: query.Path, Contract: source})
	}
	return out
}

func sourceContractFromRouteQuery(query provenance.RouteSourceQuery, contract ParamContract) ParamContract {
	local := DemandFromPathContract(query.Segments, contract)
	switch query.Projection {
	case provenance.RouteProjectionDirect:
		return local
	case provenance.RouteProjectionIndexedIteratorValue:
		return IndexedIteratorContract(1, local)
	case provenance.RouteProjectionKeyedIteratorKey:
		return KeyedIteratorContract(0, local)
	case provenance.RouteProjectionKeyedIteratorValue:
		return KeyedIteratorContract(1, local)
	case provenance.RouteProjectionSequenceElement:
		return DemandFromSequenceElement(local)
	default:
		return paramContractBottom()
	}
}
