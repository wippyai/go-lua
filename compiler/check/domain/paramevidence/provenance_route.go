package paramevidence

import (
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
// back to obligations on the route's source path. Flow owns route discovery;
// paramevidence owns the contract algebra for identity, iterator, and append
// routes.
func ProvenanceRouteContractTargets(route flow.ProvenanceRoute, contract ParamContract) []ProvenanceRouteContractTarget {
	if route.Source.Symbol == 0 || isContractBottom(contract) {
		return nil
	}
	switch route.Kind {
	case flow.ProvenanceRouteIdentityAlias:
		return []ProvenanceRouteContractTarget{{Path: route.Source, Contract: contract}}
	case flow.ProvenanceRouteIndexedIterator, flow.ProvenanceRouteKeyedIterator:
		local := DemandFromPathContract(route.Remainder, contract)
		source := iteratorRouteContract(route, local)
		if isContractBottom(source) {
			return nil
		}
		return []ProvenanceRouteContractTarget{{Path: route.Source, Contract: source}}
	case flow.ProvenanceRouteAppendElementField:
		return appendElementFieldRouteContractTargets(route, contract)
	default:
		return nil
	}
}

func iteratorRouteContract(route flow.ProvenanceRoute, local ParamContract) ParamContract {
	switch route.Kind {
	case flow.ProvenanceRouteIndexedIterator:
		return IndexedIteratorContract(route.VarIndex, local)
	case flow.ProvenanceRouteKeyedIterator:
		return KeyedIteratorContract(route.VarIndex, local)
	default:
		return paramContractBottom()
	}
}

func appendElementFieldRouteContractTargets(route flow.ProvenanceRoute, contract ParamContract) []ProvenanceRouteContractTarget {
	if len(route.SourceField) > 0 {
		sourceField := append([]constraint.Segment(nil), route.SourceField...)
		sourceField = append(sourceField, route.FieldRemainder...)
		source := DemandFromSequenceElement(DemandFromPathContract(sourceField, contract))
		if isContractBottom(source) {
			return nil
		}
		return []ProvenanceRouteContractTarget{{Path: route.Source, Contract: source}}
	}
	source := route.Source
	for _, seg := range route.FieldRemainder {
		source = source.Append(seg)
	}
	return []ProvenanceRouteContractTarget{{Path: source, Contract: contract}}
}
