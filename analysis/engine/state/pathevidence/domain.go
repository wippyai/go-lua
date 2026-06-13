package pathevidence

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// Domain builds the lattice for the coupled path-evidence lane.
func Domain(reg *axis.Registry) lattice.Lattice[Lane] {
	valueDomain := product.Domain(reg)
	ops := domainOps{
		refinements:   lift.Map[pathdom.PathKey, product.Value](valueDomain),
		staticMembers: lift.MustMap[pathdom.PathKey, product.Value](valueDomain),
		proofs:        lift.MustSet[BranchProof](),
	}
	return lattice.Lattice[Lane]{
		Bottom: func() Lane {
			return Lane{
				staticMembersBottom: true,
				proofsBottom:        true,
			}
		},
		Top: func() Lane {
			return Lane{refinementsTop: true}
		},
		Equal: func(a, b Lane) bool {
			return ops.refinements.Equal(ops.refinementLane(a), ops.refinementLane(b)) &&
				ops.staticMembers.Equal(ops.staticMemberLane(a), ops.staticMemberLane(b)) &&
				ops.proofs.Equal(ops.proofLane(a), ops.proofLane(b))
		},
		LessOrEq: func(a, b Lane) bool {
			return ops.refinements.LessOrEq(ops.refinementLane(a), ops.refinementLane(b)) &&
				ops.staticMembers.LessOrEq(ops.staticMemberLane(a), ops.staticMemberLane(b)) &&
				ops.proofs.LessOrEq(ops.proofLane(a), ops.proofLane(b))
		},
		Join: func(a, b Lane) Lane {
			return ops.fromLanes(
				ops.refinements.Join(ops.refinementLane(a), ops.refinementLane(b)),
				ops.staticMembers.Join(ops.staticMemberLane(a), ops.staticMemberLane(b)),
				ops.proofs.Join(ops.proofLane(a), ops.proofLane(b)),
			)
		},
		Widen: func(prev, next Lane) Lane {
			return ops.fromLanes(
				ops.refinements.Widen(ops.refinementLane(prev), ops.refinementLane(next)),
				ops.staticMembers.Widen(ops.staticMemberLane(prev), ops.staticMemberLane(next)),
				ops.proofs.Widen(ops.proofLane(prev), ops.proofLane(next)),
			)
		},
	}
}

type domainOps struct {
	refinements   lattice.Lattice[map[pathdom.PathKey]product.Value]
	staticMembers lattice.Lattice[lift.MustMapLane[pathdom.PathKey, product.Value]]
	proofs        lattice.Lattice[lift.MustSetLane[BranchProof]]
}

func (o domainOps) refinementLane(l Lane) map[pathdom.PathKey]product.Value {
	if l.refinementsTop {
		return o.refinements.Top()
	}
	return l.refinements
}

func (o domainOps) staticMemberLane(l Lane) lift.MustMapLane[pathdom.PathKey, product.Value] {
	if l.staticMembersBottom {
		return lift.MustMapBottom[pathdom.PathKey, product.Value]()
	}
	return lift.MustMapValues(l.staticMembers)
}

func (o domainOps) proofLane(l Lane) lift.MustSetLane[BranchProof] {
	if l.proofsBottom {
		return lift.MustSetBottom[BranchProof]()
	}
	return lift.MustSetValues(l.proofs)
}

func (o domainOps) fromLanes(
	refinements map[pathdom.PathKey]product.Value,
	staticMembers lift.MustMapLane[pathdom.PathKey, product.Value],
	proofs lift.MustSetLane[BranchProof],
) Lane {
	out := Lane{}
	if o.refinements.Equal(refinements, o.refinements.Top()) {
		out.refinementsTop = true
	} else {
		out.refinements = refinements
	}
	out.staticMembers = staticMembers.Values()
	out.staticMembersBottom = staticMembers.Bottom()
	out.proofs = proofs.Values()
	out.proofsBottom = proofs.Bottom()
	return out
}
