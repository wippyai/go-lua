// Package module owns the mutable Module census rows used by the Lua
// lowerer. It stores only already-proven canonical terms; request extraction,
// Source exact-key admission, and reserved Import spans stay in assembly core.
package module

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

var (
	errRowsInvalid   = errors.New("program/lower/collector: invalid module rows")
	errSlotRange     = errors.New("program/lower/collector: module census slot out of range")
	errSlotDuplicate = errors.New("program/lower/collector: duplicate module census slot")
	errSlotMissing   = errors.New("program/lower/collector: missing module census slot")
	errAliasInvalid  = errors.New("program/lower/collector: invalid module alias")
	errAliasSet      = errors.New("program/lower/collector: duplicate module alias")
	errCounts        = errors.New("program/lower/collector: module count mismatch")
)

// Rows is the fixed census-indexed Module construction plane. Its mutable
// slices are private to this owner; core can submit a declared row or alias
// through the narrow methods below and can consume only frozen authored rows.
type Rows struct {
	imports  []authored.Import
	declared []bool
	aliases  []bool
	invalid  bool
}

func New(count int) Rows {
	var rows Rows
	rows.init(count)
	return rows
}

func (r *Rows) init(count int) {
	if r == nil {
		return
	}
	*r = Rows{}
	if count < 0 || uint64(count) > uint64(keyspace.MaxTermOrdinal) {
		r.invalid = true
		return
	}
	r.imports = make([]authored.Import, count)
	r.declared = make([]bool, count)
	r.aliases = make([]bool, count)
}

func (r *Rows) Reset() {
	if r != nil {
		*r = Rows{}
	}
}

func (r *Rows) valid() bool {
	return r != nil && !r.invalid && len(r.imports) == len(r.declared) && len(r.imports) == len(r.aliases)
}

func (r *Rows) Complete() bool {
	if !r.valid() {
		return false
	}
	for index, row := range r.imports {
		if !r.declared[index] || row.Term != keyspace.MakeTerm(keyspace.FamilyImport, uint32(index+1)) || row.Call == 0 {
			return false
		}
		if row.Request == 0 || keyspace.TermFamily(row.Request) != keyspace.FamilyString || keyspace.TermOrdinal(row.Request) == 0 {
			return false
		}
	}
	return true
}

func (r *Rows) slot(slot int) bool {
	return r.valid() && slot >= 0 && slot < len(r.imports)
}

// Set stores the already-reserved canonical Import identity and its proven
// Call/request terms exactly once. Slots may be observed out of traversal
// order because census ordinals are fixed before lowering.
func (r *Rows) Set(slot int, term, call, request keyspace.Term) error {
	if !r.slot(slot) {
		return errSlotRange
	}
	if r.declared[slot] {
		return errSlotDuplicate
	}
	want := keyspace.MakeTerm(keyspace.FamilyImport, uint32(slot+1))
	if term == 0 || term != want || keyspace.TermFamily(term) != keyspace.FamilyImport {
		return fmt.Errorf("%w: term %v for slot %d", errRowsInvalid, term, slot)
	}
	if call == 0 || keyspace.TermFamily(call) != keyspace.FamilyCall || keyspace.TermOrdinal(call) == 0 {
		return fmt.Errorf("%w: call %v for slot %d", errRowsInvalid, call, slot)
	}
	if request == 0 || keyspace.TermFamily(request) != keyspace.FamilyString || keyspace.TermOrdinal(request) == 0 {
		return fmt.Errorf("%w: request %v for slot %d", errRowsInvalid, request, slot)
	}
	r.imports[slot] = authored.Import{Term: term, Call: call, Request: request}
	r.declared[slot] = true
	return nil
}

// SetAlias consumes the one optional alias observation. Zero is an authored
// no-alias value and is still a one-shot write.
func (r *Rows) SetAlias(slot int, alias keyspace.Term) error {
	if !r.slot(slot) {
		return errSlotRange
	}
	if !r.declared[slot] {
		return errSlotMissing
	}
	if r.aliases[slot] {
		return errAliasSet
	}
	if alias != 0 && (keyspace.TermFamily(alias) != keyspace.FamilyCell || keyspace.TermOrdinal(alias) == 0) {
		return errAliasInvalid
	}
	r.imports[slot].Alias = alias
	r.aliases[slot] = true
	return nil
}

func (r *Rows) Freeze(counts [keyspace.FamilyCount]uint32) ([]authored.Import, error) {
	if !r.valid() {
		return nil, errRowsInvalid
	}
	if uint64(len(r.imports)) != uint64(counts[keyspace.FamilyImport]) {
		return nil, errCounts
	}
	if counts[keyspace.FamilyInvalid] != 0 || counts[keyspace.FamilyOutcome] != 0 {
		return nil, errCounts
	}
	for family, count := range counts {
		if count > keyspace.MaxTermOrdinal || (keyspace.Family(family) == keyspace.FamilyInvalid && count != 0) {
			return nil, errCounts
		}
	}
	for slot, row := range r.imports {
		if !r.declared[slot] {
			return nil, errSlotMissing
		}
		want := keyspace.MakeTerm(keyspace.FamilyImport, uint32(slot+1))
		if row.Term != want || keyspace.TermFamily(row.Term) != keyspace.FamilyImport || keyspace.TermOrdinal(row.Term) > counts[keyspace.FamilyImport] ||
			row.Call == 0 || keyspace.TermFamily(row.Call) != keyspace.FamilyCall || keyspace.TermOrdinal(row.Call) == 0 || keyspace.TermOrdinal(row.Call) > counts[keyspace.FamilyCall] ||
			(row.Alias != 0 && (keyspace.TermFamily(row.Alias) != keyspace.FamilyCell || keyspace.TermOrdinal(row.Alias) == 0 || keyspace.TermOrdinal(row.Alias) > counts[keyspace.FamilyCell])) ||
			row.Request == 0 || keyspace.TermFamily(row.Request) != keyspace.FamilyString || keyspace.TermOrdinal(row.Request) > counts[keyspace.FamilyString] {
			return nil, fmt.Errorf("%w: row %d", errRowsInvalid, slot)
		}
	}
	return append([]authored.Import(nil), r.imports...), nil
}
