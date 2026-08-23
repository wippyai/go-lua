package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/domain/heap"
	contextdomain "github.com/wippyai/go-lua/domain/heap/context"
)

// HotOwner is Context's Link-local allocation-key Factor owner. Its schema is
// the exact Heap+Directory authority that authenticates every contextual
// Reference row; no current-context default or secondary identity table is
// retained.
type HotOwner struct {
	binding  *engine.SchemaBinding
	fragment *SchemaFragment
	schema   contextdomain.Schema
}

// BindHot attaches Context's exact lattice to its cold Factor slot.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, schema contextdomain.Schema) (*HotOwner, bool) {
	if !bindingOpen(binding) || fragment == nil || !schema.Valid() || !schema.Heap().LinkOwner().Available() {
		return nil, false
	}
	owner := &HotOwner{binding: binding, fragment: fragment, schema: schema}
	spec, ok := owner.FactorSpec()
	if !ok || !engine.BindFactor[coordinate](binding, fragment.slot, spec) {
		if !ok {
			return nil, false
		}
		// BindFactor returns false on duplicate or malformed structural binding.
		return nil, false
	}
	return owner, true
}

// FactorSpec is the exact Context algebra for this owner binding.
func (owner *HotOwner) FactorSpec() (engine.HotFactorSpec[coordinate, contextdomain.Value], bool) {
	if owner == nil || !owner.schema.Valid() {
		return engine.HotFactorSpec[coordinate, contextdomain.Value]{}, false
	}
	keyEnd := owner.schema.Heap().AllocationKeyCount()
	if keyEnd < 0 || uint64(keyEnd) > uint64(^uint32(0)) {
		return engine.HotFactorSpec[coordinate, contextdomain.Value]{}, false
	}
	return engine.HotFactorSpec[coordinate, contextdomain.Value]{
		KeyEnd:      uint64(keyEnd),
		Lattice:     owner.schema.Lattice(),
		Default:     owner.schema.Bottom(),
		AdmitAt:     owner.admits,
		Fingerprint: owner.schema.Fingerprint,
		WidenRank: engine.Measure[coordinate, contextdomain.Value]{
			Width: 1,
			At:    owner.widenRank,
		},
	}, true
}

// Schema returns the exact contextual authority sealed for this Link.
func (owner *HotOwner) Schema() contextdomain.Schema {
	if owner == nil {
		return contextdomain.Schema{}
	}
	return owner.schema
}

// ContextSchema is a descriptive alias for consumers that distinguish the
// contextual authority from other domain schemas.
func (owner *HotOwner) ContextSchema() contextdomain.Schema { return owner.Schema() }

// HeapSchema projects the exact Heap issuer inside Context's authority.
func (owner *HotOwner) HeapSchema() heap.Schema {
	if owner == nil {
		return heap.Schema{}
	}
	return owner.schema.Heap()
}

// MatchesBinding proves exact hot transaction ownership.
func (owner *HotOwner) MatchesBinding(binding *engine.SchemaBinding) bool {
	return owner != nil && owner.binding != nil && owner.binding == binding
}

// LinkOwner returns the detached Link authority captured by the underlying
// Heap schema.
func (owner *HotOwner) LinkOwner() link.OwnerCapability {
	if owner == nil || !owner.schema.Valid() {
		return link.OwnerCapability{}
	}
	return owner.schema.Heap().LinkOwner()
}

// LinkID returns the exact Link identity captured by this owner.
func (owner *HotOwner) LinkID() identity.ContentID { return owner.LinkOwner().ContentID() }

// FactorRef returns Context's sealed Factor surface.
func (owner *HotOwner) FactorRef() engine.FactorRef[contextdomain.Value] {
	if owner == nil || owner.fragment == nil {
		return engine.FactorRef[contextdomain.Value]{}
	}
	return owner.fragment.Ref()
}

func (owner *HotOwner) ExactRead() engine.SchemaReadForm[contextdomain.Value] {
	if owner == nil || owner.fragment == nil {
		return engine.SchemaReadForm[contextdomain.Value]{}
	}
	return owner.fragment.ExactRead()
}

func (owner *HotOwner) ExactWrite() engine.SchemaWriteForm[contextdomain.Value] {
	if owner == nil || owner.fragment == nil {
		return engine.SchemaWriteForm[contextdomain.Value]{}
	}
	return owner.fragment.ExactWrite()
}

// Ref issues the exact Context factor coordinate for one allocation root.
func (owner *HotOwner) Ref(key heap.Key) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.binding == nil || owner.fragment == nil || !owner.schema.OwnsKey(key) {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.schema.Heap().AllocationKeyIndex(key)
	if !ok || index < 0 || uint64(index) > uint64(^uint32(0)) {
		return engine.Ref[coordinate]{}, false
	}
	implementation, ok := engine.FactorImplementationAt[coordinate, contextdomain.Value](owner.binding, owner.fragment.slot)
	if !ok {
		return engine.Ref[coordinate]{}, false
	}
	return implementation.Ref(coordinate(index))
}

// Exact issues an owner-authenticated contextual Factor value for one exact
// Reference. It is the typed construction door for future Context rules.
func (owner *HotOwner) Exact(reference contextdomain.Reference) (contextdomain.Value, bool) {
	if owner == nil || !owner.schema.Valid() {
		return contextdomain.Value{}, false
	}
	return owner.schema.Exact(reference)
}

// Admit proves that a value belongs at a concrete allocation coordinate.
func (owner *HotOwner) Admit(key heap.Key, value contextdomain.Value) bool {
	return owner != nil && owner.schema.Admit(key, value)
}

func (owner *HotOwner) admits(index coordinate, value contextdomain.Value) bool {
	if owner == nil || uint64(index) >= uint64(owner.schema.Heap().AllocationKeyCount()) {
		return false
	}
	key, ok := owner.schema.Heap().AllocationKeyAt(int(index))
	return ok && owner.schema.Admit(key, value)
}

func (owner *HotOwner) widenRank(index coordinate, value contextdomain.Value, component int) uint64 {
	if owner == nil || component != 0 || !owner.admits(index, value) {
		return 0
	}
	rank, ok := owner.schema.WidenRank(value)
	if !ok {
		return 0
	}
	return rank
}

func bindingOpen(binding *engine.SchemaBinding) bool {
	return binding != nil && !binding.Sealed() && !binding.Poisoned() && binding.Schema() == nil
}
