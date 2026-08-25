package containment

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/placement"
)

// operand is the singleton owner-issued closure proof consumed by the
// mounted-point rule. The Placement schema identity fences both ordered
// summary reads; its dense allocation coordinates are retained as the
// source-owned summary range rather than rebuilt for each mounted issuance.
type Operand struct {
	id          identity.ContentID
	summaryKeys []uint64
}

// summaryKeysForSchema issues the one dense coordinate plane Placement
// projects from Heap. It is created at the owner bind boundary; runtime
// admissions borrow the retained range through scalar access instead of
// manufacturing a denominator or coordinate directory of their own.
func summaryKeysForSchema(schema placement.Schema) ([]uint64, bool) {
	if !schema.Valid() || !schema.ContentID().Available() || schema.KeyCount() < 0 {
		return nil, false
	}
	count := schema.KeyCount()
	keys := make([]uint64, count)
	for index := range keys {
		keys[index] = uint64(index)
	}
	return keys, true
}

func operandForSchema(schema placement.Schema) (Operand, bool) {
	keys, keysOK := summaryKeysForSchema(schema)
	if !keysOK {
		return Operand{}, false
	}
	return Operand{id: schema.ContentID(), summaryKeys: keys}, true
}

func completeSummaryKeys(schema placement.Schema, keys []uint64) bool {
	if !schema.Valid() || len(keys) != schema.KeyCount() {
		return false
	}
	for index, key := range keys {
		if key != uint64(index) {
			return false
		}
	}
	return true
}

func operandContentForSchema(schema placement.Schema, candidate Operand) (Operand, [32]byte, bool) {
	if !schema.Valid() || !schema.ContentID().Available() || candidate.id != schema.ContentID() || candidate.summaryKeys == nil || !completeSummaryKeys(schema, candidate.summaryKeys) {
		return Operand{}, [32]byte{}, false
	}
	return candidate, [32]byte(candidate.id), true
}

// SummaryKeyCount and SummaryKeyAt implement the engine's read-only
// source-owned summary seam. Only scalar reads cross the package boundary;
// the owner-retained slice is never exposed to selector admission.
func (candidate Operand) SummaryKeyCount() int {
	if !candidate.id.Available() || candidate.summaryKeys == nil {
		return -1
	}
	return len(candidate.summaryKeys)
}

func (candidate Operand) SummaryKeyAt(index int) (uint64, bool) {
	if candidate.SummaryKeyCount() < 0 || index < 0 || index >= len(candidate.summaryKeys) {
		return 0, false
	}
	return candidate.summaryKeys[index], true
}
