package engine

// generated_occurrence_catalog.go derives the occurrence inventory of a
// generated Rule from the sealed plan alone. A hand-wired Link rule states its
// inventory through an owner-issued callback; a generated one has no callback
// to state it with, and needs none: the plan already names the candidate
// relation, and a globally addressed relation publishes its own occurrence
// directory. The inventory below is that directory read through the plan.

import (
	"github.com/wippyai/go-lua/analysis/identity"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
)

// GeneratedOccurrenceInventory is one generated Rule's sealed occurrence
// inventory. It retains the axis directory and the relation the plan named,
// and no copied identities: the directory is immutable once the axis is
// sealed, so copying it here would only add a second authority for the same
// order.
type GeneratedOccurrenceInventory struct {
	directory memberrelation.OccurrenceDirectory
	relation  uint32
	count     int
}

// Count is the sealed census of the candidate relation this rule reads.
func (inventory GeneratedOccurrenceInventory) Count() int {
	if inventory.directory == nil {
		return 0
	}
	return inventory.count
}

// IDAt is the occurrence identity of one dense candidate, in the axis owner's
// canonical order. The census is re-proved on every read so a directory that
// disagrees with the census it published cannot admit a row.
func (inventory GeneratedOccurrenceInventory) IDAt(index int) (identity.ContentID, bool) {
	if inventory.directory == nil || index < 0 || index >= inventory.count {
		return identity.ContentID{}, false
	}
	count, countOK := inventory.directory.OccurrenceCount(inventory.relation)
	if !countOK || count != inventory.count {
		return identity.ContentID{}, false
	}
	id, ok := inventory.directory.OccurrenceIDAt(inventory.relation, index)
	return id, ok && id.Available()
}

// GeneratedOccurrenceCatalog derives one generated Rule's occurrence inventory
// from its sealed plan. It refuses a slot whose candidate relation is not a
// globally addressed directory: a mounted generated rule's occurrences are the
// artifact's rows, and inventing an inventory for it would let a rule admit
// occurrences no artifact declared.
func GeneratedOccurrenceCatalog(binding *SchemaBinding, slot *GeneratedRuleSlot) (GeneratedOccurrenceInventory, bool) {
	state := bindingState(binding)
	if state == nil || slot == nil || slot.cell == nil || slot.cell.generated == nil {
		return GeneratedOccurrenceInventory{}, false
	}
	ordinal, ordinalOK := slot.Ordinal()
	if !ordinalOK {
		return GeneratedOccurrenceInventory{}, false
	}
	state.mu.Lock()
	sealed := state.phase == schemaBindingSealed && state.schema != nil && state.schema == slot.cell.schema &&
		ordinal < uint64(len(state.rules))
	var bound *generatedRuleBindingCell
	if sealed {
		cell, generated := state.rules[ordinal].(*generatedRuleBindingCell)
		if generated && cell != nil && cell.generated == slot.cell.generated {
			bound = cell
		}
	}
	state.mu.Unlock()
	if bound == nil || !bound.schemaRuleComplete() {
		return GeneratedOccurrenceInventory{}, false
	}
	candidate := bound.generated.program.CandidateRelation()
	directory, directoryOK := relationOwnerForGeneratedAxis(state, candidate.Axis)
	if !directoryOK {
		return GeneratedOccurrenceInventory{}, false
	}
	occurrences, occurrencesOK := directory.(memberrelation.OccurrenceDirectory)
	if !occurrencesOK || occurrences == nil {
		return GeneratedOccurrenceInventory{}, false
	}
	count, countOK := occurrences.OccurrenceCount(candidate.Member)
	if !countOK || count < 0 {
		return GeneratedOccurrenceInventory{}, false
	}
	return GeneratedOccurrenceInventory{directory: occurrences, relation: candidate.Member, count: count}, true
}
