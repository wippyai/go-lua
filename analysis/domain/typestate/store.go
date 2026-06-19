// Package typestate provides the canonical abstract domain for protocols whose
// values move through named states and may carry an exit obligation.
package typestate

import (
	"sort"
)

// Protocol identifies one state-machine namespace. It is intentionally a plain
// value supplied by lowering, not inferred from a variable or method name.
type Protocol string

// State identifies one protocol state.
type State string

// Resource identifies one tracked abstract resource. The analyzer should build
// this from canonical identity/path facts, not from source spelling.
type Resource struct {
	ID       string
	Protocol Protocol
}

// Obligation describes the state a resource must reach before local ownership
// ends.
type Obligation struct {
	Final State
}

// Locality describes whether an obligation may still be locally owned.
type Locality uint8

const (
	LocalityBottom Locality = iota
	LocalityClosed
	LocalityEscaped
	LocalityOpen
	LocalityUnknown
)

// Slot is the abstract typestate for one resource.
type Slot struct {
	Current    State
	Obligation Obligation
	Locality   Locality
}

// Open reports whether this resource may still be locally owned with an
// unsatisfied obligation.
func (s Slot) Open() bool {
	return s.Locality == LocalityOpen || s.Locality == LocalityUnknown
}

// Store is a normalized map from resource identity to typestate slot.
type Store struct {
	top   bool
	slots map[Resource]Slot
}

// Empty returns the bottom store.
func Empty() Store {
	return Store{}
}

// Acquire records a freshly owned resource with an exit obligation.
func (s Store) Acquire(resource Resource, current State, obligation Obligation) Store {
	if s.top {
		return s
	}
	if resource.ID == "" || resource.Protocol == "" {
		return s.Clone()
	}
	next := s.Clone()
	next.set(resource, Slot{Current: current, Obligation: obligation, Locality: LocalityOpen})
	return next
}

// Transition moves a resource from one protocol state to another. The
// transition is ignored when the current abstract state proves a different
// state; callers must explicitly join ambiguous paths before applying facts.
func (s Store) Transition(resource Resource, from State, to State) Store {
	if s.top {
		return s
	}
	next := s.Clone()
	slot, ok := next.slots[resource]
	if !ok || slot.Locality == LocalityBottom || slot.Locality == LocalityClosed || slot.Locality == LocalityEscaped {
		return next
	}
	if slot.Locality != LocalityUnknown && from != "" && slot.Current != from {
		return next
	}
	slot.Current = to
	if slot.Obligation.Final != "" && to == slot.Obligation.Final {
		slot.Locality = LocalityClosed
	}
	next.set(resource, slot)
	return next
}

// Escape transfers the local obligation to an external owner.
func (s Store) Escape(resource Resource) Store {
	if s.top {
		return s
	}
	next := s.Clone()
	slot, ok := next.slots[resource]
	if !ok || slot.Locality == LocalityBottom || slot.Locality == LocalityClosed {
		return next
	}
	slot.Locality = LocalityEscaped
	next.set(resource, slot)
	return next
}

// OpenObligations returns locally owned obligations that are not proven closed
// or escaped.
func (s Store) OpenObligations() []OpenObligation {
	if s.top {
		return []OpenObligation{{Locality: LocalityUnknown}}
	}
	if len(s.slots) == 0 {
		return nil
	}
	var out []OpenObligation
	for resource, slot := range s.slots {
		if slot.Open() {
			out = append(out, OpenObligation{Resource: resource, Current: slot.Current, Obligation: slot.Obligation, Locality: slot.Locality})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return resourceLess(out[i].Resource, out[j].Resource)
	})
	return out
}

type OpenObligation struct {
	Resource   Resource
	Current    State
	Obligation Obligation
	Locality   Locality
}

// Clone returns an independent copy.
func (s Store) Clone() Store {
	if s.top {
		return Store{top: true}
	}
	if len(s.slots) == 0 {
		return Store{}
	}
	out := Store{slots: make(map[Resource]Slot, len(s.slots))}
	for resource, slot := range s.slots {
		out.slots[resource] = slot
	}
	return out
}

func (s *Store) set(resource Resource, slot Slot) {
	if s.top {
		return
	}
	if s.slots == nil {
		s.slots = make(map[Resource]Slot, 1)
	}
	if slot.Locality == LocalityBottom {
		delete(s.slots, resource)
		return
	}
	s.slots[resource] = slot
}

// Equal reports semantic store equality.
func Equal(a, b Store) bool {
	if a.top || b.top {
		return a.top == b.top
	}
	if len(a.slots) != len(b.slots) {
		return false
	}
	for resource, left := range a.slots {
		right, ok := b.slots[resource]
		if !ok || left != right {
			return false
		}
	}
	return true
}

// LessOrEq reports whether a is at least as precise as b.
func LessOrEq(a, b Store) bool {
	if b.top {
		return true
	}
	if a.top {
		return b.top
	}
	for resource, left := range a.slots {
		right, ok := b.slots[resource]
		if !ok {
			right = Slot{}
		}
		if !slotLessOrEq(left, right) {
			return false
		}
	}
	for resource, right := range b.slots {
		if _, ok := a.slots[resource]; ok {
			continue
		}
		if !slotLessOrEq(Slot{}, right) {
			return false
		}
	}
	return true
}

// Join returns the least upper bound of two stores.
func Join(a, b Store) Store {
	if a.top || b.top {
		return Store{top: true}
	}
	if len(a.slots) == 0 {
		return b.Clone()
	}
	if len(b.slots) == 0 {
		return a.Clone()
	}
	out := Store{slots: make(map[Resource]Slot, len(a.slots)+len(b.slots))}
	for resource, left := range a.slots {
		if right, ok := b.slots[resource]; ok {
			out.set(resource, joinSlot(left, right))
			continue
		}
		out.set(resource, left)
	}
	for resource, right := range b.slots {
		if _, ok := a.slots[resource]; ok {
			continue
		}
		out.set(resource, right)
	}
	return out
}

// Meet returns the greatest lower bound of two stores.
func Meet(a, b Store) Store {
	if a.top {
		return b.Clone()
	}
	if b.top {
		return a.Clone()
	}
	if len(a.slots) == 0 || len(b.slots) == 0 {
		return Store{}
	}
	out := Store{slots: make(map[Resource]Slot)}
	for resource, left := range a.slots {
		right, ok := b.slots[resource]
		if !ok {
			continue
		}
		out.set(resource, meetSlot(left, right))
	}
	if len(out.slots) == 0 {
		return Store{}
	}
	return out
}

// Widen is Join because this domain has finite height.
func Widen(prev, next Store) Store {
	return Join(prev, next)
}

func joinSlot(a, b Slot) Slot {
	if a == b {
		return a
	}
	return Slot{
		Current:    joinState(a.Current, b.Current),
		Obligation: joinObligation(a.Obligation, b.Obligation),
		Locality:   joinLocality(a.Locality, b.Locality),
	}
}

func joinState(a, b State) State {
	if a == b {
		return a
	}
	return ""
}

func joinObligation(a, b Obligation) Obligation {
	if a == b {
		return a
	}
	return Obligation{}
}

func joinLocality(a, b Locality) Locality {
	if a == b {
		return a
	}
	if a == LocalityBottom {
		return b
	}
	if b == LocalityBottom {
		return a
	}
	if a == LocalityUnknown || b == LocalityUnknown {
		return LocalityUnknown
	}
	if a == LocalityOpen || b == LocalityOpen {
		return LocalityOpen
	}
	return LocalityClosed
}

func meetSlot(a, b Slot) Slot {
	if a == b {
		return a
	}
	if a.Locality == LocalityBottom || b.Locality == LocalityBottom {
		return Slot{}
	}
	current, ok := meetState(a.Current, b.Current)
	if !ok {
		return Slot{}
	}
	obligation, ok := meetObligation(a.Obligation, b.Obligation)
	if !ok {
		return Slot{}
	}
	locality := meetLocality(a.Locality, b.Locality)
	if locality == LocalityBottom {
		return Slot{}
	}
	return Slot{Current: current, Obligation: obligation, Locality: locality}
}

func meetState(a, b State) (State, bool) {
	if a == b {
		return a, true
	}
	if a == "" {
		return b, true
	}
	if b == "" {
		return a, true
	}
	return "", false
}

func meetObligation(a, b Obligation) (Obligation, bool) {
	if a == b {
		return a, true
	}
	if a == (Obligation{}) {
		return b, true
	}
	if b == (Obligation{}) {
		return a, true
	}
	return Obligation{}, false
}

func meetLocality(a, b Locality) Locality {
	for _, candidate := range []Locality{
		LocalityUnknown,
		LocalityOpen,
		LocalityClosed,
		LocalityEscaped,
		LocalityBottom,
	} {
		if localityLessOrEq(candidate, a) && localityLessOrEq(candidate, b) {
			return candidate
		}
	}
	return LocalityBottom
}

func slotLessOrEq(a, b Slot) bool {
	if a.Locality == LocalityBottom {
		return true
	}
	if b.Locality == LocalityBottom {
		return false
	}
	return stateLessOrEq(a.Current, b.Current) &&
		obligationLessOrEq(a.Obligation, b.Obligation) &&
		localityLessOrEq(a.Locality, b.Locality)
}

func stateLessOrEq(a, b State) bool {
	return a == b || b == ""
}

func obligationLessOrEq(a, b Obligation) bool {
	return a == b || b == (Obligation{})
}

func localityLessOrEq(a, b Locality) bool {
	if a == b || a == LocalityBottom || b == LocalityUnknown {
		return true
	}
	if b == LocalityOpen {
		return a == LocalityClosed || a == LocalityEscaped
	}
	if b == LocalityClosed {
		return a == LocalityEscaped
	}
	return false
}

func resourceLess(a, b Resource) bool {
	if a.Protocol != b.Protocol {
		return a.Protocol < b.Protocol
	}
	return a.ID < b.ID
}
