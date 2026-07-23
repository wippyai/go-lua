package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

// StateEdit is the operation-scoped edit facade for State. It owns the
// per-lane transactions needed by a transfer operation and publishes them
// together, so callers do not have to compose lane editors manually.
//
// It preserves the semantics of repeated State writes: each staged lane has
// read-after-write behavior, and DoneOn publishes only the lanes that changed
// onto the supplied base state.
type StateEdit struct {
	state State
	reg   *axis.Registry

	valuesOpened bool
	values       ValueEdit

	pathEvidenceOpened bool
	pathEvidence       PathEvidenceEdit

	dynamicIndexOpened bool
	dynamicIndex       DynamicIndexEdit
}

// Edit opens an operation-scoped state edit transaction.
func (s State) Edit(reg *axis.Registry) StateEdit {
	return StateEdit{state: s, reg: reg}
}

func (e *StateEdit) valuesEdit() *ValueEdit {
	if !e.valuesOpened {
		e.values = e.state.EditValues(e.reg)
		e.valuesOpened = true
	}
	return &e.values
}

func (e *StateEdit) pathEvidenceEdit() *PathEvidenceEdit {
	if !e.pathEvidenceOpened {
		e.pathEvidence = e.state.EditPathEvidence(e.reg)
		e.pathEvidenceOpened = true
	}
	return &e.pathEvidence
}

func (e *StateEdit) dynamicIndexEdit() *DynamicIndexEdit {
	if !e.dynamicIndexOpened {
		e.dynamicIndex = e.state.EditDynamicIndex(e.reg)
		e.dynamicIndexOpened = true
	}
	return &e.dynamicIndex
}

// ReadValue reads a value slot, including staged value writes.
func (e *StateEdit) ReadValue(slot key.Value) product.Value {
	if e == nil {
		return product.Value{}
	}
	if e.valuesOpened {
		return e.values.Read(slot)
	}
	return e.state.ReadValue(e.reg, slot)
}

// WriteValue stages a value-lane write.
func (e *StateEdit) WriteValue(slot key.Value, value product.Value) {
	if e == nil {
		return
	}
	e.valuesEdit().Write(slot, value)
}

// UpdateValue reads a staged value, applies fn, and stages the transformed
// value.
func (e *StateEdit) UpdateValue(slot key.Value, fn func(product.Value) product.Value) {
	if e == nil {
		return
	}
	e.valuesEdit().Update(slot, fn)
}

// WriteReturnSlot stages a return-slot value write.
func (e *StateEdit) WriteReturnSlot(index int, value product.Value) {
	if e == nil {
		return
	}
	e.valuesEdit().WriteReturnSlot(index, value)
}

// ReadPathKey reads a path-refinement key, including staged path writes.
func (e *StateEdit) ReadPathKey(ks *keyspace.KeySpace, pathKey pathdom.PathKey) product.Value {
	if e == nil {
		return product.Value{}
	}
	if ks == nil {
		return product.Bottom(e.reg)
	}
	if e.pathEvidenceOpened {
		localKey, ok := ks.FromPathKey(pathKey)
		if !ok {
			return product.Bottom(e.reg)
		}
		return e.pathEvidence.edit.ReadPathKey(localKey)
	}
	return e.state.ReadPathKey(e.reg, ks, pathKey)
}

// WritePathKey stages a path-refinement write.
func (e *StateEdit) WritePathKey(ks *keyspace.KeySpace, pathKey pathdom.PathKey, value product.Value) bool {
	if e == nil {
		return false
	}
	return e.pathEvidenceEdit().WritePathKey(ks, pathKey, value)
}

// WriteLocalPathKey stages an already-interned path-refinement write.
func (e *StateEdit) WriteLocalPathKey(pathKey keyspace.Key, value product.Value) bool {
	if e == nil {
		return false
	}
	return e.pathEvidenceEdit().WriteLocalPathKey(pathKey, value)
}

// WritePathStaticMember stages a static-member evidence write.
func (e *StateEdit) WritePathStaticMember(ks *keyspace.KeySpace, pathKey pathdom.PathKey, value product.Value) bool {
	if e == nil {
		return false
	}
	return e.pathEvidenceEdit().WritePathStaticMember(ks, pathKey, value)
}

// WriteLocalPathStaticMember stages an already-interned static-member evidence
// write.
func (e *StateEdit) WriteLocalPathStaticMember(pathKey keyspace.Key, value product.Value) bool {
	if e == nil {
		return false
	}
	return e.pathEvidenceEdit().WriteLocalPathStaticMember(pathKey, value)
}

// ReadDynamicIndexFact reads a dynamic-index fact, including staged dynamic
// index writes.
func (e *StateEdit) ReadDynamicIndexFact(k dynamicindex.Key) dynamicindex.Fact {
	if e == nil {
		return dynamicindex.Fact{}
	}
	if e.dynamicIndexOpened {
		return e.dynamicIndex.Read(k)
	}
	return e.state.ReadDynamicIndexFact(e.reg, k)
}

// WriteDynamicIndexFact stages a dynamic-index fact write.
func (e *StateEdit) WriteDynamicIndexFact(k dynamicindex.Key, fact dynamicindex.Fact) bool {
	if e == nil {
		return false
	}
	return e.dynamicIndexEdit().Write(k, fact)
}

// Done publishes staged edits onto the original state.
func (e *StateEdit) Done() State {
	if e == nil {
		return State{}
	}
	return e.DoneOn(e.state)
}

// DoneOn publishes staged edits onto base. Callers must ensure no independent
// writes were made to the same lanes on base while the edit was open.
func (e *StateEdit) DoneOn(base State) State {
	if e == nil {
		return base
	}
	out := base
	if e.valuesOpened {
		out = e.values.DoneOn(out)
	}
	if e.pathEvidenceOpened {
		out = e.pathEvidence.DoneOn(out)
	}
	if e.dynamicIndexOpened {
		out = e.dynamicIndex.DoneOn(out)
	}
	return out
}
