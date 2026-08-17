// Package keymatch projects one correlated Value relation into Heap's typed
// table-key operands. It owns no state, identity, or equality vocabulary:
// Link owns exact literals, Value owns alternatives, and Heap owns selectors
// and contained-root references.
package keymatch

import (
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Alternative is one valid Heap table-key alternative. Selector is the one
// typed Heap selection input. Containment keeps the independent proof that
// the key has no reference edge, one exact child edge, or an opaque edge.
type Alternative struct {
	selector    heapdomain.KeySelector
	containment heapdomain.Containment
}

// Selector returns the one owner-issued Heap selector for this alternative.
func (alternative Alternative) Selector() heapdomain.KeySelector { return alternative.selector }

// Containment returns the owner-issued key child-edge fact. Callers must keep
// Unknown distinct from None: only Exact can project a structural Heap child.
func (alternative Alternative) Containment() heapdomain.Containment { return alternative.containment }

// Reference maps the one existing rooted Value reference to Heap's one root
// and materialization-role operand. It is the sole cross-domain reference
// mapping for both key containment and value containment; endpoint/callable
// references have no Heap root and therefore fail rather than becoming an
// invented identity.
func Reference(heap heapdomain.Schema, values *valuedomain.Schema, reference valuedomain.Reference, role materialization.Role) (heapdomain.Reference, bool) {
	if values == nil || !values.OwnsHeapSchema(heap) || !heap.LinkOwner().Matches(values.LinkOwner()) || !values.LinkOwner().Available() || !values.OwnsReference(reference) || !role.Valid() {
		return heapdomain.Reference{}, false
	}
	if key, ok := reference.AllocationKey(); ok {
		if !heap.OwnsKey(key) {
			return heapdomain.Reference{}, false
		}
		return heap.Reference(key, role)
	}
	if rootID, ok := reference.BootRootID(); ok {
		key, keyOK := heap.KeyForBootID(rootID)
		if !keyOK {
			return heapdomain.Reference{}, false
		}
		return heap.Reference(key, role)
	}
	return heapdomain.Reference{}, false
}

// Project maps one exact correlated Value atom to one Heap key alternative.
// It intentionally accepts no Value relation and performs no traversal or
// deduplication: callers must preserve which atom a coordinate selected so
// repeated coordinate uses such as `[x] = x` remain correlated. Nil and NaN
// have no valid alternative; callers inspect TableKeyValidity separately to
// retain their invalid-key branch.
func Project(heap heapdomain.Schema, values *valuedomain.Schema, atom valuedomain.Atom) (Alternative, bool) {
	if values == nil || !values.OwnsHeapSchema(heap) || !heap.LinkOwner().Matches(values.LinkOwner()) || !values.LinkOwner().Available() || !values.OwnsAtom(atom) || !atom.TableKeyValidity().MayBeValid() {
		return Alternative{}, false
	}
	containment, containmentOK := Containment(heap, values, atom)
	if !containmentOK {
		return Alternative{}, false
	}
	if exact, exactOK := atom.ExactKey(); exactOK {
		selector, selectorOK := heap.ExactSelector(exact)
		if !selectorOK {
			return Alternative{}, false
		}
		return Alternative{selector: selector, containment: containment}, true
	}

	if _, role, rooted := atom.Reference(); rooted {
		child, exactChild := containment.Reference()
		if exactChild && (role == materialization.Exact || role == materialization.Recent) {
			selector, selectorOK := heap.ReferenceSelector(child)
			if !selectorOK {
				return Alternative{}, false
			}
			return Alternative{selector: selector, containment: containment}, true
		}
	}

	kinds := atom.RuntimeKinds() & runtimekind.NonNil
	selector, selectorOK := heap.KindsSelector(kinds)
	if !selectorOK {
		return Alternative{}, false
	}
	return Alternative{selector: selector, containment: containment}, true
}

// Containment maps one exact Value atom to Heap's one owner-fenced child-edge
// fact. It is the sole atom-to-containment mapping for both stored values and
// table keys. Scalar alternatives prove None; tracked rooted alternatives
// produce Exact; untracked or opaque reference families produce Unknown.
func Containment(heap heapdomain.Schema, values *valuedomain.Schema, atom valuedomain.Atom) (heapdomain.Containment, bool) {
	if values == nil || !values.OwnsHeapSchema(heap) || !heap.LinkOwner().Matches(values.LinkOwner()) || !values.LinkOwner().Available() || !values.OwnsAtom(atom) {
		return heapdomain.Containment{}, false
	}
	if reference, role, rooted := atom.Reference(); rooted {
		if child, exact := Reference(heap, values, reference, role); exact {
			containment, contained := heap.ContainmentExact(child)
			return containment, contained
		}
		containment, contained := heap.ContainmentUnknown()
		return containment, contained
	}
	// Any untracked reference-family alternative still carries a possibly
	// retained graph. Heap must preserve that as Unknown, not turn it into a
	// zero reference or a proof of absence.
	if atom.RuntimeKinds()&runtimekind.Reference != 0 {
		containment, contained := heap.ContainmentUnknown()
		return containment, contained
	}
	containment, contained := heap.ContainmentNone()
	return containment, contained
}
