// runtime_program.go declares the sealed row-model program: the dense member
// and factor tables the solve loop reads, and the one Seal that decides a
// program's validity. The value holds role-specific concrete tables and no
// pointer into a resizable slice: every cross-table reference is a dense index
// or a canonical key.

package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// memberExec is the one hot per-member entry point. It is minted at bind time
// over the concrete bound rule, so a sealed row retains the executable rule and
// nothing of the binder draft that described it.
type memberExec func(*carrier.Work, carrier.RuleContributionBase, []carrier.State, support.Mask) memberResult

// memberRow is one hot member of the sealed program. It carries exactly what
// the group fold reads per member: the execution closure, the output slot it
// patches, and the member's dense position in its own graph Group.
//
// memberIndex is that position, not a pointer to an identity: one RuleMember
// value is 392 bytes, so a row that carried its identity would retain more of
// the draft than it replaced. Failure reporting recovers the identity from the
// Group node it already holds.
type memberRow struct {
	exec        memberExec
	outputSlot  shape.Slot
	memberIndex int32
	hasSlot     bool
}

func (row memberRow) valid() bool {
	return row.exec != nil && row.memberIndex >= 0 && (!row.hasSlot || row.outputSlot >= 0)
}

// factorRecord is one sealed Factor coordinate: its canonical key, the carrier
// slot it owns, and the dense index of the retained typed owner. The owner
// keeps the route universe by reference; the record never copies it.
type factorRecord struct {
	key   composition.Key
	slot  shape.Slot
	owner int32
}

func (record factorRecord) valid() bool {
	return record.key.Available() && record.slot >= 0 && record.owner >= 0
}

// memberSpan is the half-open member range one graph Group owns in the dense
// member table. Groups address their members by span, never by a retained
// slice header into the table.
type memberSpan struct {
	start int32
	end   int32
}

func (span memberSpan) count() int { return int(span.end - span.start) }

// runtimeProgram is the sealed row-model program. It is produced by exactly one
// constructor, is never written after that constructor returns, and exposes its
// tables only through copying accessors.
type runtimeProgram struct {
	memberTable      []memberRow
	groupSpans       []memberSpan
	factorTable      []factorRecord
	factorOwners     []runtimeFactor
	queryTable       []runtimeQuery
	observationTable []runtimeObservation
	programSealed    bool
}

// sealRuntimeProgram is the sole Seal and the sole writer of a runtimeProgram.
// It takes the one program-level validity decision: either every table is
// mutually consistent and the program is sealed, or no program exists.
func sealRuntimeProgram(rows []memberRow, spans []memberSpan, factors []factorRecord, owners []runtimeFactor, queries []runtimeQuery, observations []runtimeObservation) (*runtimeProgram, bool) {
	if len(factors) != len(owners) {
		return nil, false
	}
	next := int32(0)
	for _, span := range spans {
		if span.start != next || span.end < span.start || int(span.end) > len(rows) {
			return nil, false
		}
		// A Group's rows are a permutation of its graph members: each row carries
		// a distinct position, so recovering an identity from the Group is
		// injective and no member is represented twice.
		taken := make([]bool, span.count())
		for position := span.start; position < span.end; position++ {
			row := rows[position]
			if !row.valid() || int(row.memberIndex) >= len(taken) || taken[row.memberIndex] {
				return nil, false
			}
			taken[row.memberIndex] = true
		}
		next = span.end
	}
	if int(next) != len(rows) {
		return nil, false
	}
	for index, record := range factors {
		owner := owners[index]
		slot, slotOK := shape.Slot(0), false
		if owner != nil {
			slot, slotOK = owner.runtimeSlot()
		}
		if !record.valid() || int(record.owner) != index || owner == nil || !slotOK || slot != record.slot || compositionKeyOf(owner.semantic()) != record.key {
			return nil, false
		}
	}
	for _, query := range queries {
		if query == nil {
			return nil, false
		}
	}
	for _, observation := range observations {
		if observation == nil {
			return nil, false
		}
	}
	program := &runtimeProgram{
		memberTable:      rows,
		groupSpans:       spans,
		factorTable:      factors,
		factorOwners:     owners,
		queryTable:       queries,
		observationTable: observations,
		programSealed:    true,
	}
	return program, true
}

// valid reports the one decision sealRuntimeProgram already took. No later
// predicate re-derives a program's admissibility.
func (program *runtimeProgram) valid() bool {
	return program != nil && program.programSealed
}

func (program *runtimeProgram) memberCount() int {
	if !program.valid() {
		return 0
	}
	return len(program.memberTable)
}

// memberRowAt returns the row by value.
func (program *runtimeProgram) memberRowAt(index int) (memberRow, bool) {
	if !program.valid() || index < 0 || index >= len(program.memberTable) {
		return memberRow{}, false
	}
	return program.memberTable[index], true
}

// memberRows is the hot fold's view of one Group's rows: a contiguous run of
// the sealed table, bounds-checked once per Group instead of once per member.
// The rows are immutable after the Seal, which the assignment law enforces.
func (program *runtimeProgram) memberRows(span memberSpan) []memberRow {
	if !program.valid() || span.start < 0 || span.end < span.start || int(span.end) > len(program.memberTable) {
		return nil
	}
	return program.memberTable[span.start:span.end]
}

// memberRowIdentity recovers the cold member identity a row was bound from,
// out of the graph Group that owns it. Only diagnostics and failure reporting
// need it; the hot fold does not, and no table retains it.
func memberRowIdentity(group equation.GroupNode, row memberRow) (equation.RuleMember, bool) {
	if row.memberIndex < 0 || int(row.memberIndex) >= group.MemberCount() {
		return equation.RuleMember{}, false
	}
	return group.MemberAt(int(row.memberIndex))
}

func (program *runtimeProgram) groupCount() int {
	if !program.valid() {
		return 0
	}
	return len(program.groupSpans)
}

func (program *runtimeProgram) groupSpanAt(group int) (memberSpan, bool) {
	if !program.valid() || group < 0 || group >= len(program.groupSpans) {
		return memberSpan{}, false
	}
	return program.groupSpans[group], true
}

func (program *runtimeProgram) factorCount() int {
	if !program.valid() {
		return 0
	}
	return len(program.factorTable)
}

func (program *runtimeProgram) factorRecordAt(index int) (factorRecord, bool) {
	if !program.valid() || index < 0 || index >= len(program.factorTable) {
		return factorRecord{}, false
	}
	return program.factorTable[index], true
}

// factorOwnerAt resolves the retained typed owner a record indexes. Route
// universes and typed observation stay owner-side exactly as they are today.
func (program *runtimeProgram) factorOwnerAt(owner int32) (runtimeFactor, bool) {
	if !program.valid() || owner < 0 || int(owner) >= len(program.factorOwners) {
		return nil, false
	}
	return program.factorOwners[owner], true
}

// factorOwnerByKey resolves a canonical Factor key through the sealed table.
// The table is ordered by key at bind time, so the lookup is a search over
// dense records rather than a hash of a 40-byte key into a retained map.
func (program *runtimeProgram) factorOwnerByKey(key composition.Key) (runtimeFactor, bool) {
	record, found := program.factorRecordByKey(key)
	if !found {
		return nil, false
	}
	return program.factorOwnerAt(record.owner)
}

func (program *runtimeProgram) factorRecordByKey(key composition.Key) (factorRecord, bool) {
	if !program.valid() || !key.Available() {
		return factorRecord{}, false
	}
	index := sort.Search(len(program.factorTable), func(position int) bool {
		return !lessRuntimeKey(program.factorTable[position].key, key)
	})
	if index >= len(program.factorTable) || program.factorTable[index].key != key {
		return factorRecord{}, false
	}
	return program.factorTable[index], true
}

func (program *runtimeProgram) queryCount() int {
	if !program.valid() {
		return 0
	}
	return len(program.queryTable)
}

func (program *runtimeProgram) queryAt(index int) (runtimeQuery, bool) {
	if !program.valid() || index < 0 || index >= len(program.queryTable) {
		return nil, false
	}
	return program.queryTable[index], true
}

func (program *runtimeProgram) observationCount() int {
	if !program.valid() {
		return 0
	}
	return len(program.observationTable)
}

func (program *runtimeProgram) observationAt(index int) (runtimeObservation, bool) {
	if !program.valid() || index < 0 || index >= len(program.observationTable) {
		return nil, false
	}
	return program.observationTable[index], true
}
