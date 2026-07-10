// Package typestate provides the canonical abstract domain for protocols whose
// values move through named states and may carry an exit obligation.
package typestate

import (
	"sort"
	"strconv"
	"strings"
)

// Protocol identifies one state-machine namespace. It is intentionally a plain
// value supplied by lowering, not inferred from a variable or method name.
type Protocol string

// State identifies one protocol state.
type State string

// ResourceID is the opaque identity for one tracked resource. Engine layers may
// derive it from canonical state keys, heap identities, or future resource
// identities; the typestate lattice only requires stable comparability.
type ResourceID string

func (id ResourceID) String() string {
	return string(id)
}

// Resource identifies one tracked abstract resource. The analyzer should build
// this from canonical identity/path facts, not from source spelling.
type Resource struct {
	ID       ResourceID
	Protocol Protocol
}

// FinalStates is a canonical, comparable set of states that satisfy an
// obligation. It is string-backed so Obligation remains comparable inside
// typestate map values and summary equality checks.
type FinalStates string

// NewFinalStates returns a deterministic set of non-empty final states.
func NewFinalStates(states ...State) FinalStates {
	if len(states) == 0 {
		return ""
	}
	unique := uniqueStates(states)
	if len(unique) == 0 {
		return ""
	}
	var b strings.Builder
	for _, state := range unique {
		raw := state.String()
		b.WriteString(strconv.Itoa(len(raw)))
		b.WriteByte(':')
		b.WriteString(raw)
	}
	return FinalStates(b.String())
}

// States decodes the final states in deterministic order.
func (s FinalStates) States() []State {
	if s == "" {
		return nil
	}
	raw := string(s)
	out := make([]State, 0, 2)
	for len(raw) > 0 {
		colon := strings.IndexByte(raw, ':')
		if colon <= 0 {
			return nil
		}
		n, err := strconv.Atoi(raw[:colon])
		if err != nil || n <= 0 || colon+1+n > len(raw) {
			return nil
		}
		out = append(out, State(raw[colon+1:colon+1+n]))
		raw = raw[colon+1+n:]
	}
	return out
}

// Contains reports whether state is one of the satisfying states.
func (s FinalStates) Contains(state State) bool {
	if state == "" || s == "" {
		return false
	}
	for _, candidate := range s.States() {
		if candidate == state {
			return true
		}
	}
	return false
}

// Obligation describes the states a resource must reach before local ownership
// ends. Final is the legacy single-state form; Finals carries the generalized
// finite set. When Finals is non-empty it is authoritative.
type Obligation struct {
	Final  State
	Finals FinalStates
}

// SatisfiedBy reports whether state discharges this obligation.
func (o Obligation) SatisfiedBy(state State) bool {
	if state == "" {
		return false
	}
	if o.Finals != "" {
		return o.Finals.Contains(state)
	}
	return o.Final != "" && o.Final == state
}

// Empty reports whether the obligation has no known final state.
func (o Obligation) Empty() bool {
	return o.Final == "" && o.Finals == ""
}

// FinalStateList returns every state that can discharge this obligation.
func (o Obligation) FinalStateList() []State {
	if o.Finals != "" {
		return o.Finals.States()
	}
	if o.Final == "" {
		return nil
	}
	return []State{o.Final}
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

// InvalidTransition is a proven lifecycle operation whose declared source
// state does not match the resource state at the call site. Site is an opaque
// CFG-point number supplied by the engine; keeping it in the solved typestate
// domain lets the diagnostic layer report the exact offending operation after
// fixpoint joins.
type InvalidTransition struct {
	Resource Resource
	Expected State
	Found    State
	Site     uint32
}

// Open reports whether this resource may still be locally owned with an
// unsatisfied obligation.
func (s Slot) Open() bool {
	return s.Locality == LocalityOpen || s.Locality == LocalityUnknown
}

// Store is a normalized map from resource identity to typestate slot.
type Store struct {
	top      bool
	slots    map[Resource]Slot
	invalids map[InvalidTransition]struct{}
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

// Transition moves a resource from one protocol state to another. A proven
// source-state mismatch is retained as an InvalidTransition with no source
// site. Engine callers should prefer TransitionAt so diagnostics can point at
// the offending operation.
func (s Store) Transition(resource Resource, from State, to State) Store {
	return s.transitionAt(resource, from, to, 0)
}

// TransitionAt is Transition with an opaque call-site identity for a proven
// invalid transition. Missing, escaped, or abstractly unknown resources remain
// silent: they do not prove that the declared precondition was violated.
func (s Store) TransitionAt(resource Resource, from State, to State, site uint32) Store {
	return s.transitionAt(resource, from, to, site)
}

func (s Store) transitionAt(resource Resource, from State, to State, site uint32) Store {
	if s.top {
		return s
	}
	next := s.Clone()
	slot, ok := next.slots[resource]
	if !ok || slot.Locality == LocalityBottom || slot.Locality == LocalityClosed || slot.Locality == LocalityEscaped {
		if ok && slot.Locality == LocalityClosed && from != "" && slot.Current != "" {
			next.setInvalidTransition(InvalidTransition{Resource: resource, Expected: from, Found: slot.Current, Site: site})
		}
		return next
	}
	if slot.Locality != LocalityUnknown && from != "" && slot.Current != from {
		if slot.Current != "" {
			next.setInvalidTransition(InvalidTransition{Resource: resource, Expected: from, Found: slot.Current, Site: site})
		}
		return next
	}
	slot.Current = to
	if slot.Obligation.SatisfiedBy(to) {
		slot.Locality = LocalityClosed
	}
	next.set(resource, slot)
	return next
}

// InvalidTransitions returns every proven transition-precondition failure in
// deterministic order. They are may-facts: a failure observed on any reachable
// normal path remains reportable after control-flow joins.
func (s Store) InvalidTransitions() []InvalidTransition {
	if s.top || len(s.invalids) == 0 {
		return nil
	}
	out := make([]InvalidTransition, 0, len(s.invalids))
	for invalid := range s.invalids {
		out = append(out, invalid)
	}
	sort.Slice(out, func(i, j int) bool {
		if resourceLess(out[i].Resource, out[j].Resource) {
			return true
		}
		if resourceLess(out[j].Resource, out[i].Resource) {
			return false
		}
		if out[i].Expected != out[j].Expected {
			return out[i].Expected < out[j].Expected
		}
		if out[i].Found != out[j].Found {
			return out[i].Found < out[j].Found
		}
		return out[i].Site < out[j].Site
	})
	return out
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

// Lookup returns the exact slot for resource when the store is not top.
// Missing slots and top stores are both unknown to callers.
func (s Store) Lookup(resource Resource) (Slot, bool) {
	if s.top || resource.ID == "" || resource.Protocol == "" {
		return Slot{}, false
	}
	slot, ok := s.slots[resource]
	return slot, ok
}

type OpenObligation struct {
	Resource   Resource
	Current    State
	Obligation Obligation
	Locality   Locality
}

// Resources returns every tracked resource in deterministic order. It is the
// narrow transport surface used by interprocedural outcome projection: callers
// can retain only the resources that existed at a callback boundary without
// exposing the store's mutable representation.
func (s Store) Resources() []Resource {
	if s.top || len(s.slots) == 0 {
		return nil
	}
	out := make([]Resource, 0, len(s.slots))
	for resource := range s.slots {
		out = append(out, resource)
	}
	sort.Slice(out, func(i, j int) bool { return resourceLess(out[i], out[j]) })
	return out
}

// Restrict returns the exact slots for resources. It intentionally omits
// invalid-transition site facts: those facts belong to the callback body that
// established them and must not be replayed at a protected caller boundary.
func (s Store) Restrict(resources []Resource) Store {
	if s.top || len(resources) == 0 {
		return Store{}
	}
	out := Store{}
	for _, resource := range resources {
		if slot, ok := s.slots[resource]; ok {
			out.set(resource, slot)
		}
	}
	return out
}

// Overlay replaces the slots present in snapshot while retaining every other
// resource. It is used to materialize one protected callback outcome against
// the caller's entry state before normal and exceptional outcomes are joined.
// Invalid-transition facts are deliberately not imported across that boundary.
func (s Store) Overlay(snapshot Store) Store {
	if s.top || snapshot.top || len(snapshot.slots) == 0 {
		return s.Clone()
	}
	next := s.Clone()
	for resource, slot := range snapshot.slots {
		next.set(resource, slot)
	}
	return next
}

// Clone returns an independent copy.
func (s Store) Clone() Store {
	if s.top {
		return Store{top: true}
	}
	if len(s.slots) == 0 && len(s.invalids) == 0 {
		return Store{}
	}
	out := Store{}
	if len(s.slots) != 0 {
		out.slots = make(map[Resource]Slot, len(s.slots))
	}
	for resource, slot := range s.slots {
		out.slots[resource] = slot
	}
	if len(s.invalids) != 0 {
		out.invalids = make(map[InvalidTransition]struct{}, len(s.invalids))
		for invalid := range s.invalids {
			out.invalids[invalid] = struct{}{}
		}
	}
	return out
}

// MapResources rewrites resource identities while preserving slots. Collisions
// are joined, matching control-flow joins that prove two resource paths are the
// same abstract resource.
func (s Store) MapResources(mapper func(Resource) Resource) Store {
	if s.top {
		return Store{top: true}
	}
	if (len(s.slots) == 0 && len(s.invalids) == 0) || mapper == nil {
		return s.Clone()
	}
	out := Store{}
	if len(s.slots) != 0 {
		out.slots = make(map[Resource]Slot, len(s.slots))
	}
	for resource, slot := range s.slots {
		nextResource := mapper(resource)
		if nextResource.ID == "" || nextResource.Protocol == "" {
			nextResource = resource
		}
		if existing, ok := out.slots[nextResource]; ok {
			out.set(nextResource, joinSlot(existing, slot))
			continue
		}
		out.set(nextResource, slot)
	}
	for invalid := range s.invalids {
		nextResource := mapper(invalid.Resource)
		if nextResource.ID == "" || nextResource.Protocol == "" {
			nextResource = invalid.Resource
		}
		invalid.Resource = nextResource
		out.setInvalidTransition(invalid)
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

func (s *Store) setInvalidTransition(invalid InvalidTransition) {
	if s.top || invalid.Resource.ID == "" || invalid.Resource.Protocol == "" || invalid.Expected == "" || invalid.Found == "" {
		return
	}
	if s.invalids == nil {
		s.invalids = make(map[InvalidTransition]struct{}, 1)
	}
	s.invalids[invalid] = struct{}{}
}

// Equal reports semantic store equality.
func Equal(a, b Store) bool {
	if a.top || b.top {
		return a.top == b.top
	}
	if len(a.slots) != len(b.slots) || len(a.invalids) != len(b.invalids) {
		return false
	}
	for resource, left := range a.slots {
		right, ok := b.slots[resource]
		if !ok || left != right {
			return false
		}
	}
	for invalid := range a.invalids {
		if _, ok := b.invalids[invalid]; !ok {
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
	for invalid := range a.invalids {
		if _, ok := b.invalids[invalid]; !ok {
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
	if len(a.slots) == 0 && len(a.invalids) == 0 {
		return b.Clone()
	}
	if len(b.slots) == 0 && len(b.invalids) == 0 {
		return a.Clone()
	}
	out := Store{}
	if len(a.slots)+len(b.slots) != 0 {
		out.slots = make(map[Resource]Slot, len(a.slots)+len(b.slots))
	}
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
	for invalid := range a.invalids {
		out.setInvalidTransition(invalid)
	}
	for invalid := range b.invalids {
		out.setInvalidTransition(invalid)
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
	out := Store{}
	if len(a.slots) != 0 && len(b.slots) != 0 {
		out.slots = make(map[Resource]Slot)
	}
	for resource, left := range a.slots {
		right, ok := b.slots[resource]
		if !ok {
			continue
		}
		out.set(resource, meetSlot(left, right))
	}
	for invalid := range a.invalids {
		if _, ok := b.invalids[invalid]; ok {
			out.setInvalidTransition(invalid)
		}
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
