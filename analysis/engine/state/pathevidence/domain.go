package pathevidence

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/internal/registrycache"
)

var domainCache registrycache.Cache[lattice.Lattice[Lane]]

// Domain builds the lattice for the coupled path-evidence lane.
func Domain(reg *axis.Registry) lattice.Lattice[Lane] {
	return domainCache.GetFor(reg, domainForRegistry)
}

func domainForRegistry(reg *axis.Registry) lattice.Lattice[Lane] {
	valueDomain := product.Domain(reg)
	ops := domainOps{
		refinements:              lift.MustMap[keyspace.Key, product.Value](valueDomain),
		staticMembers:            lift.MustMap[keyspace.Key, product.Value](valueDomain),
		proofs:                   lift.MustSet[BranchProof](),
		pathPresenceImplications: lift.MustSet[PathPresenceImplication](),
	}
	return lattice.Lattice[Lane]{
		Bottom: func() Lane {
			return Lane{
				refinementsBottom:              true,
				staticMembersBottom:            true,
				proofsBottom:                   true,
				pathPresenceImplicationsBottom: true,
			}
		},
		Top: func() Lane {
			return Lane{}
		},
		Equal: func(a, b Lane) bool {
			return ops.refinements.Equal(ops.refinementLane(a), ops.refinementLane(b)) &&
				ops.staticMembers.Equal(ops.staticMemberLane(a), ops.staticMemberLane(b)) &&
				ops.proofs.Equal(ops.proofLane(a), ops.proofLane(b)) &&
				ops.pathPresenceImplications.Equal(ops.pathPresenceImplicationLane(a), ops.pathPresenceImplicationLane(b))
		},
		Same: func(a, b Lane) bool {
			return ops.refinements.Same != nil && ops.refinements.Same(ops.refinementLane(a), ops.refinementLane(b)) &&
				ops.staticMembers.Same != nil && ops.staticMembers.Same(ops.staticMemberLane(a), ops.staticMemberLane(b)) &&
				ops.proofs.Same != nil && ops.proofs.Same(ops.proofLane(a), ops.proofLane(b)) &&
				ops.pathPresenceImplications.Same != nil && ops.pathPresenceImplications.Same(ops.pathPresenceImplicationLane(a), ops.pathPresenceImplicationLane(b))
		},
		LessOrEq: func(a, b Lane) bool {
			return ops.refinements.LessOrEq(ops.refinementLane(a), ops.refinementLane(b)) &&
				ops.staticMembers.LessOrEq(ops.staticMemberLane(a), ops.staticMemberLane(b)) &&
				ops.proofs.LessOrEq(ops.proofLane(a), ops.proofLane(b)) &&
				ops.pathPresenceImplications.LessOrEq(ops.pathPresenceImplicationLane(a), ops.pathPresenceImplicationLane(b))
		},
		Join: func(a, b Lane) Lane {
			return ops.fromLanes(
				ops.refinements.Join(ops.refinementLane(a), ops.refinementLane(b)),
				ops.staticMembers.Join(ops.staticMemberLane(a), ops.staticMemberLane(b)),
				ops.proofs.Join(ops.proofLane(a), ops.proofLane(b)),
				ops.pathPresenceImplications.Join(ops.pathPresenceImplicationLane(a), ops.pathPresenceImplicationLane(b)),
			)
		},
		Meet: func(a, b Lane) Lane {
			return ops.fromLanes(
				ops.refinements.Meet(ops.refinementLane(a), ops.refinementLane(b)),
				ops.staticMembers.Meet(ops.staticMemberLane(a), ops.staticMemberLane(b)),
				ops.proofs.Meet(ops.proofLane(a), ops.proofLane(b)),
				ops.pathPresenceImplications.Meet(ops.pathPresenceImplicationLane(a), ops.pathPresenceImplicationLane(b)),
			)
		},
		Widen: func(prev, next Lane) Lane {
			return ops.fromLanes(
				ops.refinements.Widen(ops.refinementLane(prev), ops.refinementLane(next)),
				ops.staticMembers.Widen(ops.staticMemberLane(prev), ops.staticMemberLane(next)),
				ops.proofs.Widen(ops.proofLane(prev), ops.proofLane(next)),
				ops.pathPresenceImplications.Widen(ops.pathPresenceImplicationLane(prev), ops.pathPresenceImplicationLane(next)),
			)
		},
	}
}

type domainOps struct {
	refinements              lattice.Lattice[lift.MustMapLane[keyspace.Key, product.Value]]
	staticMembers            lattice.Lattice[lift.MustMapLane[keyspace.Key, product.Value]]
	proofs                   lattice.Lattice[lift.MustSetLane[BranchProof]]
	pathPresenceImplications lattice.Lattice[lift.MustSetLane[PathPresenceImplication]]
}

func (o domainOps) refinementLane(l Lane) lift.MustMapLane[keyspace.Key, product.Value] {
	if l.refinementsBottom {
		return lift.MustMapBottom[keyspace.Key, product.Value]()
	}
	return lift.MustMapValues(l.refinements)
}

func (o domainOps) staticMemberLane(l Lane) lift.MustMapLane[keyspace.Key, product.Value] {
	if l.staticMembersBottom {
		return lift.MustMapBottom[keyspace.Key, product.Value]()
	}
	return lift.MustMapValues(l.staticMembers)
}

func (o domainOps) proofLane(l Lane) lift.MustSetLane[BranchProof] {
	if l.proofsBottom {
		return lift.MustSetBottom[BranchProof]()
	}
	return lift.MustSetValues(l.proofs)
}

func (o domainOps) pathPresenceImplicationLane(l Lane) lift.MustSetLane[PathPresenceImplication] {
	if l.pathPresenceImplicationsBottom {
		return lift.MustSetBottom[PathPresenceImplication]()
	}
	return lift.MustSetValues(l.pathPresenceImplications)
}

func (o domainOps) fromLanes(
	refinements lift.MustMapLane[keyspace.Key, product.Value],
	staticMembers lift.MustMapLane[keyspace.Key, product.Value],
	proofs lift.MustSetLane[BranchProof],
	pathPresenceImplications lift.MustSetLane[PathPresenceImplication],
) Lane {
	out := Lane{}
	out.refinements = refinements.Values()
	out.refinementsBottom = refinements.Bottom()
	out.staticMembers = staticMembers.Values()
	out.staticMembersBottom = staticMembers.Bottom()
	out.proofs = proofs.Values()
	out.proofsBottom = proofs.Bottom()
	for proof := range out.proofs {
		out.equalityRootMask.merge(equalityProofRootMask(proof))
	}
	out.pathPresenceImplications = pathPresenceImplications.Values()
	out.pathPresenceImplicationsBottom = pathPresenceImplications.Bottom()
	return out
}
