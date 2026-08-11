package residence

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
)

// Location is semantic residence, never a physical allocation decision.
// Unknown is represented by a lawful union (or Top), not a third axis value.
type Location uint8

const (
	LocationInvalid Location = iota
	ActorLocal
	Shared
	Module
	Global
)

func (location Location) valid() bool { return location >= ActorLocal && location <= Global }

// Retention records whether this exact boundary structurally retains a root.
type Retention uint8

const (
	RetentionInvalid Retention = iota
	NotRetained
	Retained
)

func (retention Retention) valid() bool { return retention == NotRetained || retention == Retained }

// Survival records lifetime at the exact boundary.
type Survival uint8

const (
	SurvivalInvalid Survival = iota
	Dead
	Live
)

func (survival Survival) valid() bool { return survival == Dead || survival == Live }

// LastUse is the semantic eligibility result, not an allocation or release.
type LastUse uint8

const (
	LastUseInvalid LastUse = iota
	LastUseEligible
	LastUseRevoked
)

func (lastUse LastUse) valid() bool { return lastUse == LastUseEligible || lastUse == LastUseRevoked }

// Reference is one exact Heap allocation key with the one shared canonical
// materialization role vocabulary. It names no concrete object ID.
type Reference struct {
	owner *schema
	root  uint32
	role  materialization.Role
}

func (reference Reference) valid() bool {
	return reference.owner != nil && reference.root != 0 && int(reference.root) <= len(reference.owner.allocations) && reference.role.Valid()
}

func (reference Reference) HeapKey() (heap.Key, materialization.Role, bool) {
	if !reference.valid() {
		return heap.Key{}, materialization.Invalid, false
	}
	return reference.owner.allocations[reference.root-1], reference.role, true
}

// Fact is one correlated root/role/residence/retention/survival/last-use
// alternative. There are no independent Unknown fields: joins preserve whole
// alternatives and Top is the explicit opaque family element.
type Fact struct {
	owner     *schema
	reference Reference
	location  Location
	retention Retention
	survival  Survival
	lastUse   LastUse
}

func (fact Fact) valid() bool {
	if fact.owner == nil || !fact.reference.valid() || fact.reference.owner != fact.owner || !fact.location.valid() || !fact.retention.valid() || !fact.survival.valid() || !fact.lastUse.valid() {
		return false
	}
	// A retained root has a future retaining boundary, and Summary can never
	// obtain a singleton last-use privilege.
	return !(fact.lastUse == LastUseEligible && (fact.retention == Retained || fact.reference.role == materialization.Summary))
}

func (fact Fact) Reference() (Reference, bool) {
	if !fact.valid() {
		return Reference{}, false
	}
	return fact.reference, true
}
func (fact Fact) Residence() (Location, Retention, Survival, LastUse, bool) {
	if !fact.valid() {
		return LocationInvalid, RetentionInvalid, SurvivalInvalid, LastUseInvalid, false
	}
	return fact.location, fact.retention, fact.survival, fact.lastUse, true
}

// Value is one immutable normalized may-relation in the one Residence family.
// The zero value is unavailable; Bottom is a single constant sparse default.
type Value struct {
	owner *schema
	top   bool
	facts []Fact
}

func (value Value) valid() bool { return value.owner != nil }

func (schema Schema) Bottom() Value {
	if !schema.valid() {
		return Value{}
	}
	return schema.owner.bottom
}
func (schema Schema) Default() Value { return schema.Bottom() }
func (schema Schema) Top() Value {
	if !schema.valid() {
		return Value{}
	}
	return schema.owner.top
}

// Reference admits exactly one Heap allocation key/role source.
func (schema Schema) Reference(root heap.Key, role materialization.Role) (Reference, bool) {
	if !schema.valid() || !schema.owner.heap.OwnsKey(root) || root.Kind() != heap.RootAllocation || !role.Valid() {
		return Reference{}, false
	}
	id := schema.owner.allocationIndex[root]
	if id == 0 {
		return Reference{}, false
	}
	return Reference{owner: schema.owner, root: id, role: role}, true
}

// Observation admits one correlated residence alternative. Key-specific
// boundary admission remains on Key/Rule capabilities; this homogeneous Value
// never receives a per-key schema or a global raw-root fallback.
func (schema Schema) Observation(reference Reference, location Location, retention Retention, survival Survival, lastUse LastUse) (Fact, bool) {
	if !schema.valid() {
		return Fact{}, false
	}
	fact := Fact{owner: schema.owner, reference: reference, location: location, retention: retention, survival: survival, lastUse: lastUse}
	if !fact.valid() {
		return Fact{}, false
	}
	return fact, true
}

func (schema Schema) Of(facts ...Fact) (Value, bool) {
	if !schema.valid() {
		return Value{}, false
	}
	if len(facts) == 0 {
		return schema.Bottom(), true
	}
	result := append([]Fact(nil), facts...)
	for _, fact := range result {
		if !fact.valid() || fact.owner != schema.owner {
			return Value{}, false
		}
	}
	sort.Slice(result, func(left, right int) bool { return lessFact(result[left], result[right]) })
	end := 1
	for index := 1; index < len(result); index++ {
		if result[index] != result[end-1] {
			result[end] = result[index]
			end++
		}
	}
	return Value{owner: schema.owner, facts: result[:end]}, true
}

func (value Value) Valid() bool { return value.valid() }
func (value Value) IsBottom() bool {
	return value.valid() && !value.top && len(value.facts) == 0
}
func (value Value) IsTop() bool { return value.valid() && value.top }
func (value Value) Facts() []Fact {
	if !value.valid() || value.top {
		return nil
	}
	return append([]Fact(nil), value.facts...)
}

// Materialize advances Recent only for one selected root. It never rewrites a
// Key's actor/boundary relation. Summary also revokes a singleton last-use
// privilege as required by the Residence law.
func (schema Schema) Materialize(value Value, root heap.Key) (Value, bool) {
	if !schema.owns(value) {
		return Value{}, false
	}
	selected := schema.owner.allocationIndex[root]
	if selected == 0 {
		return Value{}, false
	}
	if value.top || value.IsBottom() {
		return value, true
	}
	mapped := make([]Fact, len(value.facts))
	changed := false
	for index, fact := range value.facts {
		mapped[index] = materializeFact(fact, selected)
		changed = changed || mapped[index] != fact
	}
	if !changed {
		return value, true
	}
	sort.Slice(mapped, func(left, right int) bool { return lessFact(mapped[left], mapped[right]) })
	end := 1
	for index := 1; index < len(mapped); index++ {
		if mapped[index] != mapped[end-1] {
			mapped[end] = mapped[index]
			end++
		}
	}
	return Value{owner: schema.owner, facts: mapped[:end]}, true
}

func materializeFact(fact Fact, selected uint32) Fact {
	if fact.reference.root != selected {
		return fact
	}
	role, advanced := materialization.RecentToSummary(fact.reference.role)
	if !advanced {
		return fact
	}
	fact.reference.role = role
	if fact.lastUse == LastUseEligible {
		fact.lastUse = LastUseRevoked
	}
	return fact
}

func (schema Schema) owns(value Value) bool {
	return schema.valid() && value.valid() && value.owner == schema.owner
}

// Admits reports homogeneous Value-family membership without traversing its
// facts. Values are owner-issued and all constructors preserve normalization.
func (schema Schema) Admits(value Value) bool { return schema.owns(value) }

func lessFact(left, right Fact) bool {
	if left.reference.root != right.reference.root {
		return left.reference.root < right.reference.root
	}
	if left.reference.role != right.reference.role {
		return left.reference.role < right.reference.role
	}
	if left.location != right.location {
		return left.location < right.location
	}
	if left.retention != right.retention {
		return left.retention < right.retention
	}
	if left.survival != right.survival {
		return left.survival < right.survival
	}
	return left.lastUse < right.lastUse
}
