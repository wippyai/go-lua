package paramevidence

import (
	"fmt"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/typ"
)

// contractSample is a structural cross-section of the Contracts lattice: the
// Bottom (empty) map, the Top sentinel, single-index cells at varied parameter
// demands, multi-index cells with overlapping and disjoint indices, and a cell
// whose element is the value-domain Bottom (which must canonicalize to absence).
// The demands cover the obligation kinds the domain models: a declared scalar, a
// required record field, a numeric bound, an optional (nilable) demand, a union
// of shapes, and the dynamic top element.
//
// Each index carries a single structural family of demands so the element
// lattice joins it associatively, matching the record-handling envelope of the
// value domain's own law sample (a single record shape per slot, plus scalars,
// optionals, unions, and the dynamic top). Index 1 carries the record family
// {id}; the wider record {id, name} lives only at index 3, so the element shape
// axis never width-joins two distinct record shapes at one index. See the report
// note on the upstream value/product shape-axis width-join asymmetry.
func contractSample(d lattice.Lattice[Contracts]) []Contracts {
	rec := typ.NewRecord().ReadonlyField("id", typ.String).Build()
	recWider := typ.NewRecord().ReadonlyField("id", typ.String).ReadonlyField("name", typ.String).Build()

	d0 := DemandFromType(typ.String)
	d0b := DemandFromType(typ.Number)
	d0Top := DemandFromType(typ.Any)
	dRec := DemandFromType(rec)
	dRecWider := DemandFromType(recWider)
	dOpt := DemandFromType(typ.NewOptional(typ.Integer))
	dUnion := DemandFromType(typ.NewUnion(typ.String, typ.Number))
	dNum := DemandFromType(typ.Integer)
	dBot := ParamContractDomain.Bottom()

	return []Contracts{
		d.Bottom(),
		d.Top(),
		{},                                 // explicit empty == Bottom
		{0: d0},                            // single index, scalar demand
		{0: d0b},                           // same index, different scalar
		{1: dRec},                          // record family at its dedicated index
		{1: dOpt},                          // same index, optional demand (record ⊔ optional)
		{0: d0Top},                         // dynamic top at an index
		{2: dUnion},                        // union demand at its own index
		{3: dRecWider},                     // wider record family at its dedicated index
		{0: d0, 1: dRec},                   // two indices
		{0: d0b, 1: dOpt, 2: dNum},         // three indices, overlapping at 0/1
		{1: dRec, 2: dUnion, 3: dRecWider}, // partial overlap with above
		{0: dBot},                          // explicit element bottom -> canonical absence
		{0: d0, 1: dBot},                   // mixed explicit bottom
	}
}

func formatContracts(c Contracts) string {
	if len(c) == 0 {
		return "{}"
	}
	keys := make([]int, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	out := "{"
	for i, k := range keys {
		if i > 0 {
			out += ","
		}
		t := c[k].ProjectValue()
		if t == nil {
			out += fmt.Sprintf("%d:<bot>", k)
			continue
		}
		out += fmt.Sprintf("%d:%s", k, t.String())
	}
	return out + "}"
}

// TestContractDomain_Laws drives the full lattice law suite over the Contracts
// component: idempotency, commutativity, associativity, partial order,
// least-upper-bound, widening over-approximation, and ascending-chain
// termination under Widen (ACC). The element lattice (value/product.Domain) is
// forward-only (nil Meet), so the map lattice is forward-only and LawSuite skips
// the meet-side laws.
func TestContractDomain_Laws(t *testing.T) {
	suite := lattice.LawSuite[Contracts]{
		Name:   "paramevidence.ContractDomain",
		Domain: ContractDomain,
		Sample: contractSample(ContractDomain),
		Format: formatContracts,
	}
	suite.Run(t)
}

// TestContractDomain_ForwardOnly pins that the Contracts lattice inherits the
// value domain's forward-only character: the element provides no greatest lower
// bound, so the map lattice has nil Meet.
func TestContractDomain_ForwardOnly(t *testing.T) {
	if ParamContractDomain.Meet != nil {
		t.Fatalf("value domain element must be forward-only (nil Meet)")
	}
	if ContractDomain.Meet != nil {
		t.Fatalf("Contracts map lattice over a forward-only element must have nil Meet")
	}
}

// TestContractDomain_AbsentIndexIsBottom pins the central MapLattice invariant
// for the Contracts component: an absent parameter index denotes no obligation
// (the element Bottom), so a map with an explicit element-Bottom cell is Equal to
// the same map without that index, and the accumulation update drops a cell that
// joins to Bottom.
func TestContractDomain_AbsentIndexIsBottom(t *testing.T) {
	bot := ParamContractDomain.Bottom()

	withExplicit := Contracts{0: DemandFromType(typ.String), 1: bot}
	withoutIndex := Contracts{0: DemandFromType(typ.String)}
	if !ContractDomain.Equal(withExplicit, withoutIndex) {
		t.Errorf("explicit element-bottom cell must Equal absence: %s != %s",
			formatContracts(withExplicit), formatContracts(withoutIndex))
	}

	allBottom := Contracts{0: bot, 1: bot}
	if !ContractDomain.Equal(allBottom, ContractDomain.Bottom()) {
		t.Errorf("all-bottom map must Equal Bottom: %s", formatContracts(allBottom))
	}
}

// TestContractDomain_AccumulationIsUpperBound pins the locked update semantics:
// JoinDemand only ever moves a cell UP the order. The non-monotone "prefer a
// narrower refined type" replacement is rejected — joining a broad obligation
// with a narrower one yields their least upper bound (the broad one covers the
// narrow), never a downward replacement.
func TestContractDomain_AccumulationIsUpperBound(t *testing.T) {
	broad := DemandFromType(typ.NewUnion(typ.String, typ.False))
	narrow := DemandFromType(typ.String)

	c := Contracts{0: broad}
	got := JoinDemand(c, 0, narrow)

	// The accumulated cell must be an upper bound of both: broad ⊑ cell and
	// narrow ⊑ cell. A "prefer narrow" replacement would make broad ⋢ cell.
	cellMap := Contracts{0: got[0]}
	if !ContractDomain.LessOrEq(Contracts{0: broad}, cellMap) {
		t.Errorf("accumulation dropped below prior obligation: prev=%s cell=%s",
			formatContracts(Contracts{0: broad}), formatContracts(cellMap))
	}
	if !ContractDomain.LessOrEq(Contracts{0: narrow}, cellMap) {
		t.Errorf("accumulation does not cover new demand: demand=%s cell=%s",
			formatContracts(Contracts{0: narrow}), formatContracts(cellMap))
	}
	// broad already covers narrow, so the least upper bound equals broad.
	if !ParamContractDomain.Equal(got[0], product.Join(broad, narrow)) {
		t.Errorf("cell is not the element least upper bound: cell=%s",
			formatContracts(cellMap))
	}

	// Accumulation is idempotent and commutative: re-applying either order is
	// stable.
	again := JoinDemand(got, 0, narrow)
	if !ContractDomain.Equal(got, again) {
		t.Errorf("re-applying a covered demand changed the cell: %s -> %s",
			formatContracts(got), formatContracts(again))
	}
}

// TestContractDomain_BottomDemandIsIdentity pins that admitting a nil/no
// requirement (element Bottom) leaves the contract unchanged and does not create
// a spurious cell.
func TestContractDomain_BottomDemandIsIdentity(t *testing.T) {
	c := Contracts{0: DemandFromType(typ.String)}
	got := JoinDemand(c, 1, DemandFromType(nil))
	if !ContractDomain.Equal(c, got) {
		t.Errorf("Bottom demand created an obligation: %s -> %s",
			formatContracts(c), formatContracts(got))
	}
	if _, ok := got[1]; ok {
		t.Errorf("Bottom demand left a spurious cell at index 1: %s", formatContracts(got))
	}
}
