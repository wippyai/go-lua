package context

import (
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
)

// Local is the owner-issued local capability event. Its Result is the
// current contextual Reference; Local itself is not part of Reference
// identity.
type Local struct {
	owner  *schemaOwner
	result Reference
}

// Share is the owner-issued share event. The transition and source are event
// evidence; the Result is compared only by canonical current-reference
// identity.
type Share struct {
	owner      *schemaOwner
	source     Reference
	result     Reference
	transition executioncontext.Transition
}

// Move is the owner-issued move event. It has the same current-reference
// projection as Share but remains a distinct event capability.
type Move struct {
	owner      *schemaOwner
	source     Reference
	result     Reference
	transition executioncontext.Transition
}

// Copy is the owner-issued copy event. It carries one bounded source
// Reference rather than a recursive provenance chain; the new Result has a
// fresh Heap allocation key and a new immutable origin. This is one
// allocation/root mapping only: a graph consumer must separately provide
// memoized traversal if it needs deep copy or cycle preservation.
type Copy struct {
	owner      *schemaOwner
	source     Reference
	result     Reference
	transition executioncontext.Transition
}

// Valid reports whether Local is an authenticated event capability.
func (event Local) Valid() bool {
	return event.owner != nil && event.result.valid() && event.result.owner == event.owner
}

// Result returns Local's current contextual allocation reference.
func (event Local) Result() Reference {
	if !event.Valid() {
		return Reference{}
	}
	return event.result
}

// Equal compares Local event results by canonical current-reference identity.
func (event Local) Equal(other Local) bool {
	return event.Valid() && other.Valid() && event.result.Equal(other.result)
}

// Valid reports whether Share is an authenticated event capability.
func (event Share) Valid() bool {
	return event.owner != nil && event.source.valid() && event.result.valid() && event.source.owner == event.owner &&
		event.result.owner == event.owner && event.owner.ownsTransition(event.transition) &&
		event.source.holder.ID() == event.transition.FromContextID() && event.result.holder.ID() == event.transition.ToContextID() &&
		event.source.allocation.Equal(event.result.allocation) && event.source.role == event.result.role &&
		event.source.allocation.origin.Equal(event.result.allocation.origin)
}

// Source returns Share's bounded source current reference.
func (event Share) Source() Reference {
	if !event.Valid() {
		return Reference{}
	}
	return event.source
}

// Result returns Share's current contextual allocation reference.
func (event Share) Result() Reference {
	if !event.Valid() {
		return Reference{}
	}
	return event.result
}

// Transition returns Share's exact typed execution transition evidence.
func (event Share) Transition() executioncontext.Transition {
	if !event.Valid() {
		return executioncontext.Transition{}
	}
	return event.transition
}

// Equal compares Share event evidence and result. Consumers that need only
// current state should compare Result instead.
func (event Share) Equal(other Share) bool {
	return event.Valid() && other.Valid() && event.owner == other.owner && event.source.Equal(other.source) &&
		event.result.Equal(other.result) && event.transition.ID() == other.transition.ID()
}

// Valid reports whether Move is an authenticated event capability.
func (event Move) Valid() bool {
	return event.owner != nil && event.source.valid() && event.result.valid() && event.source.owner == event.owner &&
		event.result.owner == event.owner && event.owner.ownsTransition(event.transition) &&
		event.source.holder.ID() == event.transition.FromContextID() && event.result.holder.ID() == event.transition.ToContextID() &&
		event.source.allocation.Equal(event.result.allocation) && event.source.role == event.result.role &&
		event.source.allocation.origin.Equal(event.result.allocation.origin)
}

// Source returns Move's bounded source current reference.
func (event Move) Source() Reference {
	if !event.Valid() {
		return Reference{}
	}
	return event.source
}

// Result returns Move's current contextual allocation reference.
func (event Move) Result() Reference {
	if !event.Valid() {
		return Reference{}
	}
	return event.result
}

// Transition returns Move's exact typed execution transition evidence.
func (event Move) Transition() executioncontext.Transition {
	if !event.Valid() {
		return executioncontext.Transition{}
	}
	return event.transition
}

// Equal compares Move event evidence and result. Consumers that need only
// current state should compare Result instead.
func (event Move) Equal(other Move) bool {
	return event.Valid() && other.Valid() && event.owner == other.owner && event.source.Equal(other.source) &&
		event.result.Equal(other.result) && event.transition.ID() == other.transition.ID()
}

// Valid reports whether Copy is an authenticated event capability.
func (event Copy) Valid() bool {
	return event.owner != nil && event.source.valid() && event.result.valid() && event.source.owner == event.owner &&
		event.result.owner == event.owner && event.owner.ownsTransition(event.transition) &&
		event.source.holder.ID() == event.transition.FromContextID() && event.result.holder.ID() == event.transition.ToContextID() &&
		event.source.role == event.result.role &&
		Schema{owner: event.owner}.freshKey(event.result.allocation.key) && event.result.allocation.key != event.source.allocation.key &&
		event.result.allocation.origin.valid() && event.result.allocation.origin.kind == OriginExecutionContext &&
		event.result.allocation.origin.context.ID() == event.result.holder.ID() &&
		!event.source.allocation.Equal(event.result.allocation)
}

// CopiedFrom returns the one bounded source reference carried by this Copy
// event; copying a copy does not create an unbounded recursive current-
// reference value.
func (event Copy) CopiedFrom() (Reference, bool) {
	if !event.Valid() {
		return Reference{}, false
	}
	return event.source, true
}

// Result returns Copy's new contextual allocation reference.
func (event Copy) Result() Reference {
	if !event.Valid() {
		return Reference{}
	}
	return event.result
}

// Transition returns Copy's exact typed execution transition evidence.
func (event Copy) Transition() executioncontext.Transition {
	if !event.Valid() {
		return executioncontext.Transition{}
	}
	return event.transition
}

// Equal compares Copy event evidence and result. Current-reference identity
// is still available through Result().Equal and does not include provenance.
func (event Copy) Equal(other Copy) bool {
	return event.Valid() && other.Valid() && event.owner == other.owner && event.source.Equal(other.source) &&
		event.result.Equal(other.result) && event.transition.ID() == other.transition.ID()
}

// Local issues an existing allocation root for one typed holder. Its origin
// is the holder itself; role admission remains Heap's authority.
func (schema Schema) Local(key heap.Key, holder executioncontext.Context, role materialization.Role) (Local, bool) {
	if !schema.Valid() || !schema.OwnsKey(key) || !schema.OwnsContext(holder) || !role.Valid() {
		return Local{}, false
	}
	if _, referenceOK := schema.owner.heap.Reference(key, role); !referenceOK {
		return Local{}, false
	}
	reference := Reference{
		owner: schema.owner,
		allocation: Allocation{
			owner:  schema.owner,
			origin: Origin{owner: schema.owner, kind: OriginExecutionContext, context: holder},
			key:    key,
		},
		holder: holder,
		role:   role,
	}
	event := Local{owner: schema.owner, result: reference}
	return event, event.Valid()
}

// LocalReference admits an already owner-issued Heap Reference. It is the
// typed bridge for consumers that already carry Heap's role-bearing relation.
func (schema Schema) LocalReference(reference heap.Reference, holder executioncontext.Context) (Local, bool) {
	if !schema.Valid() || reference.Kind() == heap.RootInvalid || !schema.OwnsContext(holder) {
		return Local{}, false
	}
	key, role, keyOK := reference.Key()
	if !keyOK || !schema.OwnsKey(key) {
		return Local{}, false
	}
	return schema.Local(key, holder, role)
}

// Share changes Holder along one exact owner-directory Transition while
// preserving the source key, role, and immutable Origin.
func (schema Schema) Share(source Reference, transition executioncontext.Transition) (Share, bool) {
	target, ok := schema.transferTarget(source, transition)
	if !ok {
		return Share{}, false
	}
	result := Reference{owner: schema.owner, allocation: source.allocation, holder: target, role: source.role}
	event := Share{owner: schema.owner, source: source, result: result, transition: transition}
	return event, event.Valid()
}

// Move changes Holder along one exact owner-directory Transition while
// preserving the source key, role, and immutable Origin.
func (schema Schema) Move(source Reference, transition executioncontext.Transition) (Move, bool) {
	target, ok := schema.transferTarget(source, transition)
	if !ok {
		return Move{}, false
	}
	result := Reference{owner: schema.owner, allocation: source.allocation, holder: target, role: source.role}
	event := Move{owner: schema.owner, source: source, result: result, transition: transition}
	return event, event.Valid()
}

// Copy creates a new contextual reference at one owner-issued Target fresh
// allocation. The source key is retained only on this bounded event, not in
// the current Reference identity. Deep graph traversal and cycle mapping are
// intentionally outside this root-level constructor.
func (schema Schema) Copy(source Reference, target heap.Key, transition executioncontext.Transition) (Copy, bool) {
	targetHolder, ok := schema.transferTarget(source, transition)
	if !ok || !schema.freshKey(target) || target == source.allocation.key {
		return Copy{}, false
	}
	if _, referenceOK := schema.owner.heap.Reference(target, source.role); !referenceOK {
		return Copy{}, false
	}
	result := Reference{
		owner: schema.owner,
		allocation: Allocation{
			owner:  schema.owner,
			origin: Origin{owner: schema.owner, kind: OriginExecutionContext, context: targetHolder},
			key:    target,
		},
		holder: targetHolder,
		role:   source.role,
	}
	event := Copy{owner: schema.owner, source: source, result: result, transition: transition}
	return event, event.Valid()
}

func (schema Schema) transferTarget(source Reference, transition executioncontext.Transition) (executioncontext.Context, bool) {
	if !schema.Valid() || !source.valid() || source.owner != schema.owner || !schema.OwnsTransition(transition) {
		return executioncontext.Context{}, false
	}
	if transition.FromContextID() != source.holder.ID() {
		return executioncontext.Context{}, false
	}
	target, ok := schema.owner.directory.Context(transition.ToContextID())
	if !ok || !schema.OwnsContext(target) {
		return executioncontext.Context{}, false
	}
	return target, true
}
