package paramevidence

import (
	"maps"

	"github.com/wippyai/go-lua/types/domain/value/product"
	latticeproduct "github.com/wippyai/go-lua/types/lattice/product"
	"github.com/wippyai/go-lua/types/typ"
)

// ParamContract is one parameter's accumulated contract demand.
//
// The carrier is the interned value-domain reduced product
// product.AbstractValue. It is the same carrier api.FunctionFact already stores
// per slot for Params/BodyParams/EntryParams, so the Contracts component shares
// the function-fact representation rather than introducing a parallel one. A
// parameter's contract records the obligations the body imposes on the value an
// entry must supply: the shape axis carries required field presence and the
// declared/inferred type, the numeric axis carries numeric bounds, and the
// presence axis carries required non-nilability. Bottom is the empty value: no
// obligation. Top is the dynamic value: an over-demand satisfiable by nothing
// more specific.
type ParamContract = product.AbstractValue

// Contracts is the per-function Contracts component of the canonical
// FunctionState = Points x Contracts.
//
// It is a total function from parameter index to ParamContract: an absent index
// denotes the element Bottom (no obligation on that parameter), matching the
// MapLattice convention that absence is the element least element. The cell at
// index i accumulates every contract demand observed for parameter i.
type Contracts = map[int]ParamContract

// ParamContractDomain is the per-parameter contract lattice.
//
// It is the value-domain Domain: the obligation a single parameter accumulates
// is a value-domain converged fact, so its least upper bound, order, and
// widening are exactly the value domain's. Reusing value/product.Domain keeps a
// single sound join for parameter facts rather than a second implementation
// alongside the typ.Type merges this package uses for body interpretation.
var ParamContractDomain = product.Domain

// ContractDomain is the Contracts component lattice: ParamContractDomain lifted
// pointwise over the parameter index by MapLattice.
//
// The update law is accumulation: applying an observed requirement r to the
// contract for parameter i is Contracts[i] = ContractDomain element-Join of the
// current cell with r. Join is the value domain's least upper bound, so the cell
// only ever moves up the order — it is a true upper bound, never a non-monotone
// "prefer a refined type" replacement. Cells on the entry->body->entry cycle use
// ContractDomain.Widen (the value domain's ACC widening); acyclic cells use exact
// Join. Bottom is the empty map (no parameter constrained); Top is the MapLattice
// top sentinel (every parameter over-demanded).
var ContractDomain = latticeproduct.MapLattice[int](ParamContractDomain)

// DemandFromType admits an observed parameter requirement, expressed as a
// structural type, into the per-parameter contract carrier.
//
// It is the admission boundary for an obligation produced by body usage (a
// required field read, a declared/inferred type, a numeric bound). The result is
// Joined into the cell with ParamContractDomain.Join; a nil requirement carries
// no obligation and admits the element Bottom.
func DemandFromType(t typ.Type) ParamContract {
	if t == nil {
		return ParamContractDomain.Bottom()
	}
	return product.FromType(t)
}

// ContractTypes projects the solved Contracts carrier to caller-visible concrete
// types keyed by parameter index. It is a projection boundary only: the abstract
// interpreter keeps Contracts as product-domain values, while external bridges
// that need concrete types read them through this function.
func ContractTypes(contracts Contracts) map[int]typ.Type {
	if len(contracts) == 0 {
		return nil
	}
	out := make(map[int]typ.Type, len(contracts))
	for idx, av := range contracts {
		if idx < 0 || ParamContractDomain.Equal(av, ParamContractDomain.Bottom()) {
			continue
		}
		out[idx] = product.ProjectValueOrUnknown(av)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// JoinDemand accumulates one observed requirement into the contract for a
// parameter index, returning the updated Contracts cell map.
//
// This is the locked update law: Contracts[idx] = Join(Contracts[idx], demand).
// The cell only grows up the order. An index whose joined cell equals the element
// Bottom is dropped so absence and an explicit Bottom denote the same total
// function (the MapLattice canonicalization).
func JoinDemand(c Contracts, idx int, demand ParamContract) Contracts {
	if idx < 0 {
		return c
	}
	cur := ParamContractDomain.Bottom()
	if existing, ok := c[idx]; ok {
		cur = existing
	}
	joined := ParamContractDomain.Join(cur, demand)
	if ParamContractDomain.Equal(joined, ParamContractDomain.Bottom()) {
		if _, ok := c[idx]; !ok {
			return c
		}
		out := cloneContracts(c)
		delete(out, idx)
		return out
	}
	out := cloneContracts(c)
	out[idx] = joined
	return out
}

func cloneContracts(c Contracts) Contracts {
	out := make(Contracts, len(c)+1)
	maps.Copy(out, c)
	return out
}
