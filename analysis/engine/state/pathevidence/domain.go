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
		staticMembers: mustMapDomain[pathdom.PathKey, product.Value](valueDomain),
		proofs:        mustSetDomain[BranchProof](),
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
	staticMembers lattice.Lattice[mustMapLane[pathdom.PathKey, product.Value]]
	proofs        lattice.Lattice[mustSetLane[BranchProof]]
}

func (o domainOps) refinementLane(l Lane) map[pathdom.PathKey]product.Value {
	if l.refinementsTop {
		return o.refinements.Top()
	}
	return l.refinements
}

func (o domainOps) staticMemberLane(l Lane) mustMapLane[pathdom.PathKey, product.Value] {
	return mustMapLane[pathdom.PathKey, product.Value]{
		bottom: l.staticMembersBottom,
		values: l.staticMembers,
	}
}

func (o domainOps) proofLane(l Lane) mustSetLane[BranchProof] {
	return mustSetLane[BranchProof]{
		bottom: l.proofsBottom,
		values: l.proofs,
	}
}

func (o domainOps) fromLanes(
	refinements map[pathdom.PathKey]product.Value,
	staticMembers mustMapLane[pathdom.PathKey, product.Value],
	proofs mustSetLane[BranchProof],
) Lane {
	out := Lane{}
	if o.refinements.Equal(refinements, o.refinements.Top()) {
		out.refinementsTop = true
	} else {
		out.refinements = refinements
	}
	out.staticMembers = staticMembers.values
	out.staticMembersBottom = staticMembers.bottom
	out.proofs = proofs.values
	out.proofsBottom = proofs.bottom
	return out
}

type mustMapLane[K comparable, V any] struct {
	bottom bool
	values map[K]V
}

type mustSetLane[T comparable] struct {
	bottom bool
	values map[T]struct{}
}

func mustMapDomain[K comparable, V any](elem lattice.Lattice[V]) lattice.Lattice[mustMapLane[K, V]] {
	return lattice.Lattice[mustMapLane[K, V]]{
		Bottom: func() mustMapLane[K, V] {
			return mustMapLane[K, V]{bottom: true}
		},
		Top: func() mustMapLane[K, V] {
			return mustMapLane[K, V]{}
		},
		Equal: func(a, b mustMapLane[K, V]) bool {
			if a.bottom || b.bottom {
				return a.bottom && b.bottom
			}
			return finiteMapEqual(a.values, b.values, elem.Equal)
		},
		LessOrEq: func(a, b mustMapLane[K, V]) bool {
			switch {
			case a.bottom:
				return true
			case b.bottom:
				return false
			default:
				return finiteMustMapLessOrEq(a.values, b.values, elem.LessOrEq)
			}
		},
		Join: func(a, b mustMapLane[K, V]) mustMapLane[K, V] {
			if a.bottom {
				return b
			}
			if b.bottom {
				return a
			}
			return mustMapLane[K, V]{values: finiteMustMapJoin(a.values, b.values, elem.Join)}
		},
		Widen: func(prev, next mustMapLane[K, V]) mustMapLane[K, V] {
			if prev.bottom {
				return next
			}
			if next.bottom {
				return prev
			}
			return mustMapLane[K, V]{values: finiteMustMapJoin(prev.values, next.values, elem.Widen)}
		},
	}
}

func mustSetDomain[T comparable]() lattice.Lattice[mustSetLane[T]] {
	return lattice.Lattice[mustSetLane[T]]{
		Bottom: func() mustSetLane[T] {
			return mustSetLane[T]{bottom: true}
		},
		Top: func() mustSetLane[T] {
			return mustSetLane[T]{}
		},
		Equal: func(a, b mustSetLane[T]) bool {
			if a.bottom || b.bottom {
				return a.bottom && b.bottom
			}
			return finiteSetEqual(a.values, b.values)
		},
		LessOrEq: func(a, b mustSetLane[T]) bool {
			switch {
			case a.bottom:
				return true
			case b.bottom:
				return false
			default:
				return finiteMustSetLessOrEq(a.values, b.values)
			}
		},
		Join: func(a, b mustSetLane[T]) mustSetLane[T] {
			if a.bottom {
				return b
			}
			if b.bottom {
				return a
			}
			return mustSetLane[T]{values: finiteSetIntersection(a.values, b.values)}
		},
		Widen: func(prev, next mustSetLane[T]) mustSetLane[T] {
			if prev.bottom {
				return next
			}
			if next.bottom {
				return prev
			}
			return mustSetLane[T]{values: finiteSetIntersection(prev.values, next.values)}
		},
	}
}

func finiteMapEqual[K comparable, V any](
	a map[K]V,
	b map[K]V,
	equal func(V, V) bool,
) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || !equal(av, bv) {
			return false
		}
	}
	return true
}

func finiteMustMapLessOrEq[K comparable, V any](
	a map[K]V,
	b map[K]V,
	lessOrEq func(V, V) bool,
) bool {
	for k, bv := range b {
		av, ok := a[k]
		if !ok || !lessOrEq(av, bv) {
			return false
		}
	}
	return true
}

func finiteMustMapJoin[K comparable, V any](
	a map[K]V,
	b map[K]V,
	join func(V, V) V,
) map[K]V {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	out := make(map[K]V)
	for k, av := range a {
		if bv, ok := b[k]; ok {
			out[k] = join(av, bv)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func finiteSetEqual[T comparable](a, b map[T]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

func finiteMustSetLessOrEq[T comparable](a, b map[T]struct{}) bool {
	for k := range b {
		if _, ok := a[k]; !ok {
			return false
		}
	}
	return true
}

func finiteSetIntersection[T comparable](a, b map[T]struct{}) map[T]struct{} {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	out := make(map[T]struct{})
	for k := range a {
		if _, ok := b[k]; ok {
			out[k] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
