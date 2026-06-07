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
