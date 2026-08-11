package suspension

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/materialization"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/lattice"
)

func (schema Schema) Lattice() (lattice.Lattice[Value], bool) {
	if !schema.Valid() {
		return lattice.Lattice[Value]{}, false
	}
	bottom, _ := schema.Bottom()
	top, _ := schema.Top()
	return lattice.Lattice[Value]{
		Bottom: func() Value { return bottom }, Top: func() Value { return top },
		Equal: schema.Equal, LessOrEq: schema.LessOrEq,
		Join: func(left, right Value) Value {
			value, ok := schema.Join(left, right)
			if !ok {
				panic("suspension: foreign generation schema")
			}
			return value
		},
		Meet: func(left, right Value) Value {
			value, ok := schema.Meet(left, right)
			if !ok {
				panic("suspension: foreign generation schema")
			}
			return value
		},
		Widen: func(previous, next Value) Value {
			value, ok := schema.Widen(previous, next)
			if !ok {
				panic("suspension: foreign generation schema")
			}
			return value
		},
	}, true
}

func (schema Schema) Equal(left, right Value) bool {
	if !schema.owns(left) || !schema.owns(right) {
		return false
	}
	if left.top || right.top {
		return left.top == right.top
	}
	if len(left.lifecycles) != len(right.lifecycles) {
		return false
	}
	for index := range left.lifecycles {
		if !sameLifecycle(left.lifecycles[index], right.lifecycles[index]) {
			return false
		}
	}
	return true
}

func (schema Schema) LessOrEq(left, right Value) bool {
	if !schema.owns(left) || !schema.owns(right) {
		return false
	}
	if right.top || left.IsBottom() {
		return true
	}
	if left.top {
		return false
	}
	for _, current := range left.lifecycles {
		covered, found := lifecycleForRole(right.lifecycles, current.role)
		if !found || !lessLifecycle(schema, current, covered) {
			return false
		}
	}
	return true
}

func (schema Schema) Join(left, right Value) (Value, bool) {
	if !schema.owns(left) || !schema.owns(right) {
		return Value{}, false
	}
	if left.top || right.top {
		return schema.Top()
	}
	items := append(cloneLifecycles(left.lifecycles), cloneLifecycles(right.lifecycles)...)
	return schema.normalize(items), true
}

func (schema Schema) Meet(left, right Value) (Value, bool) {
	if !schema.owns(left) || !schema.owns(right) {
		return Value{}, false
	}
	if left.top {
		return schema.clone(right), true
	}
	if right.top {
		return schema.clone(left), true
	}
	items := make([]lifecycle, 0, len(left.lifecycles))
	for _, first := range left.lifecycles {
		second, found := lifecycleForRole(right.lifecycles, first.role)
		if !found {
			continue
		}
		items = append(items, lifecycle{role: first.role, live: first.live && second.live, consumed: first.consumed && second.consumed, retained: intersectRetentions(schema, first.retained, second.retained)})
	}
	return schema.normalize(items), true
}

func (schema Schema) Widen(previous, next Value) (Value, bool) { return schema.Join(previous, next) }

func (schema Schema) Fingerprint(value Value) (uint64, bool) {
	if !schema.owns(value) {
		return 0, false
	}
	hash := internal.MixHash(0x1e4e32fe5095b7e3, 0)
	for _, word := range schema.owner.linkID {
		hash = internal.MixHash(hash, uint64(word))
	}
	if value.top {
		return internal.MixHash(hash, 1), true
	}
	for _, lifecycle := range value.lifecycles {
		hash = internal.MixHash(hash, uint64(lifecycle.role))
		if lifecycle.live {
			hash = internal.MixHash(hash, 1)
		}
		if lifecycle.consumed {
			hash = internal.MixHash(hash, 2)
		}
		for _, retention := range lifecycle.retained {
			hash = internal.MixHash(hash, uint64(retention.subject.kind))
			if schema.owner.source.Boundary() == nil {
				return 0, false
			}
			id, ok := schema.owner.source.Boundary().Values().ID(retention.subject.value)
			if !ok {
				return 0, false
			}
			for _, word := range id {
				hash = internal.MixHash(hash, uint64(word))
			}
			hash = internal.MixHash(hash, uint64(retention.roles))
		}
	}
	return hash, true
}

func (schema Schema) clone(value Value) Value {
	if value.top {
		copy, _ := schema.Top()
		return copy
	}
	return Value{owner: schema.owner, lifecycles: cloneLifecycles(value.lifecycles)}
}

func sameLifecycle(left, right lifecycle) bool {
	if left.role != right.role || left.live != right.live || left.consumed != right.consumed || len(left.retained) != len(right.retained) {
		return false
	}
	for index := range left.retained {
		if left.retained[index] != right.retained[index] {
			return false
		}
	}
	return true
}

func lifecycleForRole(values []lifecycle, role materialization.Role) (lifecycle, bool) {
	index := sort.Search(len(values), func(index int) bool { return values[index].role >= role })
	return lifecycleAt(values, index, role)
}

func lifecycleAt(values []lifecycle, index int, role materialization.Role) (lifecycle, bool) {
	if index >= len(values) || values[index].role != role {
		return lifecycle{}, false
	}
	return values[index], true
}

func lessLifecycle(schema Schema, left, right lifecycle) bool {
	if left.live && !right.live || left.consumed && !right.consumed {
		return false
	}
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left.retained) && rightIndex < len(right.retained) {
		current, cover := left.retained[leftIndex], right.retained[rightIndex]
		if current.subject == cover.subject {
			if current.roles&cover.roles != current.roles {
				return false
			}
			leftIndex++
			rightIndex++
			continue
		}
		order, comparable := schema.owner.compareSubjects(current.subject, cover.subject)
		if !comparable {
			return false
		}
		if order > 0 {
			rightIndex++
			continue
		}
		return false
	}
	return leftIndex == len(left.retained)
}

func intersectRetentions(schema Schema, left, right []Retention) []Retention {
	items := make([]Retention, 0, len(left))
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) && rightIndex < len(right) {
		first, second := left[leftIndex], right[rightIndex]
		order, comparable := schema.owner.compareSubjects(first.subject, second.subject)
		if !comparable {
			return nil
		}
		if order < 0 {
			leftIndex++
			continue
		}
		if order > 0 {
			rightIndex++
			continue
		}
		if roles := first.roles & second.roles; roles != 0 {
			items = append(items, Retention{subject: first.subject, roles: roles})
		}
		leftIndex++
		rightIndex++
	}
	return items
}
