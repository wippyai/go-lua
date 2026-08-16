// Package returnprojection seals the one Body-owned Return projection used by
// transformers. It transfers already-validated ReturnExit/Propagation facts
// once while authored, Body, Outcome, and Executable owners are live.
package returnprojection

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

type row struct {
	outcome keyspace.Term
	start   uint32
	end     uint32
}

// Result is an immutable dense Body projection. Rows are indexed by canonical
// Body ordinal; values is the single ordered alternative plane. It retains no
// authored, Body, Outcome, or Executable owner and exposes no public lookup
// map or copied causal topology.
type Result struct {
	sourceID identity.ContentID
	flowID   identity.ContentID
	staticID identity.ContentID
	moduleID identity.ContentID
	sealed   bool
	rows     []row
	values   []keyspace.Term
}

func Matches(result *Result, sourceID, flowID, staticID, moduleID identity.ContentID) bool {
	return result != nil && sourceID.Available() && flowID.Available() && staticID.Available() && moduleID.Available() &&
		result.sourceID == sourceID && result.flowID == flowID && result.staticID == staticID && result.moduleID == moduleID &&
		result.valid()
}

// valid is the constant-time publication fence. Seal performs the complete
// row/range proof once before publishing this immutable projection.
func (result *Result) valid() bool {
	return result != nil && result.sealed && result.sourceID.Available() && result.flowID.Available() &&
		result.staticID.Available() && result.moduleID.Available()
}

func (result *Result) validateResult() bool {
	if result == nil || result.sealed || !result.sourceID.Available() || !result.flowID.Available() || !result.staticID.Available() || !result.moduleID.Available() || len(result.rows) < 2 || result.rows[0] != (row{}) {
		return false
	}
	var cursor uint32
	for ordinal := 1; ordinal < len(result.rows); ordinal++ {
		row := result.rows[ordinal]
		if row.outcome == 0 {
			if row.start != 0 || row.end != 0 {
				return false
			}
			continue
		}
		if keyspace.TermFamily(row.outcome) != keyspace.FamilyOutcome || keyspace.TermOrdinal(row.outcome) == 0 || row.start != cursor || row.end < row.start || uint64(row.end) > uint64(len(result.values)) {
			return false
		}
		for _, value := range result.values[row.start:row.end] {
			if keyspace.TermFamily(value) != keyspace.FamilyValues || keyspace.TermOrdinal(value) == 0 {
				return false
			}
		}
		cursor = row.end
	}
	return cursor == uint32(len(result.values))
}

func (result *Result) bodyOrdinal(body keyspace.Term) (uint32, bool) {
	if result == nil || keyspace.TermFamily(body) != keyspace.FamilyBody {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(body)
	return ordinal, ordinal != 0 && uint64(ordinal) < uint64(len(result.rows))
}

// ForBody returns the sole targetless Return Outcome and its ordered Values
// alternative count. Bodies without executable Returns return false.
func (result *Result) ForBody(body keyspace.Term) (keyspace.Term, int, bool) {
	if !result.valid() {
		return 0, 0, false
	}
	ordinal, ok := result.bodyOrdinal(body)
	if !ok {
		return 0, 0, false
	}
	row := result.rows[ordinal]
	if row.outcome == 0 || row.end <= row.start || uint64(row.end) > uint64(len(result.values)) {
		return 0, 0, false
	}
	return row.outcome, int(row.end - row.start), true
}

// ValueAt returns one ordered executable Values alternative for this exact
// Body Return row.
func (result *Result) ValueAt(body keyspace.Term, index int) (keyspace.Term, bool) {
	if !result.valid() || index < 0 {
		return 0, false
	}
	ordinal, ok := result.bodyOrdinal(body)
	if !ok {
		return 0, false
	}
	row := result.rows[ordinal]
	if row.outcome == 0 || row.end < row.start || uint64(index) >= uint64(row.end-row.start) {
		return 0, false
	}
	valueIndex := row.start + uint32(index)
	if uint64(valueIndex) >= uint64(len(result.values)) {
		return 0, false
	}
	value := result.values[valueIndex]
	return value, keyspace.TermFamily(value) == keyspace.FamilyValues && keyspace.TermOrdinal(value) != 0
}
