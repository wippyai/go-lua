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

// ProvenanceRouteContractClosure follows provenance routes backward from a local
// path obligation to every source-path obligation implied by the current route
// graph. The first target is the seed obligation. Contracts that reach the same
// stable path identity are joined and revisited when the joined demand grows.
func ProvenanceRouteContractClosure(
	path constraint.Path,
	contract ParamContract,
	routes provenance.RouteResolver,
) []ProvenanceRouteContractTarget {
	if path.Symbol == 0 || isContractBottom(contract) {
		return nil
	}
	closure := provenance.RouteClosure(provenance.RouteClosureConfig[ParamContract]{
		Seed: provenance.RouteClosureTarget[ParamContract]{
			Path:    path,
			Payload: contract,
		},
		Routes:   routes,
		Targets:  routeContractClosureTargets,
		IsBottom: isContractBottom,
		Join:     ParamContractDomain.Join,
		Equal:    ParamContractDomain.Equal,
	})
	out := make([]ProvenanceRouteContractTarget, 0, len(closure))
	for _, target := range closure {
		out = append(out, ProvenanceRouteContractTarget{
			Path:     target.Path,
			Contract: target.Payload,
		})
	}
	return out
}

func routeContractClosureTargets(route flow.ProvenanceRoute, contract ParamContract) []provenance.RouteClosureTarget[ParamContract] {
	targets := ProvenanceRouteContractTargets(route, contract)
	if len(targets) == 0 {
		return nil
	}
	out := make([]provenance.RouteClosureTarget[ParamContract], 0, len(targets))
	for _, target := range targets {
		out = append(out, provenance.RouteClosureTarget[ParamContract]{
			Path:    target.Path,
			Payload: target.Contract,
		})
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
	return provenance.ApplyRouteProjection(query.Projection, local, routeContractProjection)
}

var routeContractProjection = provenance.RouteProjectionAlgebra[ParamContract]{
	IndexedIteratorValue: func(local ParamContract) ParamContract { return IndexedIteratorContract(1, local) },
	KeyedIteratorKey:     func(local ParamContract) ParamContract { return KeyedIteratorContract(0, local) },
	KeyedIteratorValue:   func(local ParamContract) ParamContract { return KeyedIteratorContract(1, local) },
	SequenceElement:      DemandFromSequenceElement,
}
