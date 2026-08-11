// Package collector owns the construction-only census rows produced by the
// lowerer.  This file contains the Module vertical; it deliberately has no
// AST walk and no request/key resolution authority.
package collector

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/source"
)

var (
	errModuleRowsInvalid   = errors.New("program/lower/collector: invalid module rows")
	errModuleSlotRange     = errors.New("program/lower/collector: module census slot out of range")
	errModuleSlotDuplicate = errors.New("program/lower/collector: duplicate module census slot")
	errModuleSlotMissing   = errors.New("program/lower/collector: missing module census slot")
	errModuleAliasInvalid  = errors.New("program/lower/collector: invalid module alias")
	errModuleAliasSet      = errors.New("program/lower/collector: duplicate module alias")
	errModuleCounts        = errors.New("program/lower/collector: module count mismatch")
	errModuleRequestEmpty  = errors.New("program/lower/collector: empty Module Import request")
)

// moduleRows is a fixed census-indexed Module construction plane.
//
// The lowerer receives the complete direct-require census before traversal.
// Therefore a row is selected by its final census ordinal, never appended in
// traversal order.  This matters for callers which visit nested bodies or
// otherwise observe the same set in a different order: the Import term and
// its source span remain tied to the census slot.
//
// The term itself is supplied by the collector's root allocator.  In
// particular, observe/declare does not mint a new identity and cannot create
// a second ordinal plane.  The zero Alias in a row is the authored absence of
// a direct local binding; Request and Key are never written here.
type moduleRows struct {
	imports  []module.Import
	declared []bool
	aliases  []bool
	invalid  bool
}

// init allocates exactly one row for every census slot.  It intentionally has
// no append path.  An invalid count leaves a permanently invalid, inert
// value, so a hostile census cannot trigger a negative conversion or a
// partially usable row plane.
func (r *moduleRows) init(count int) {
	if r == nil {
		return
	}
	*r = moduleRows{}
	if count < 0 || uint64(count) > uint64(keyspace.MaxTermOrdinal) {
		r.invalid = true
		return
	}
	r.imports = make([]module.Import, count)
	r.declared = make([]bool, count)
	r.aliases = make([]bool, count)
}

func (r *moduleRows) valid() bool {
	return r != nil && !r.invalid && len(r.imports) == len(r.declared) && len(r.imports) == len(r.aliases)
}

// complete reports whether every census slot has exactly one authored Import
// row.  An absent optional Alias is complete authored state: zero is the
// explicit no-alias value and does not require a synthetic row or a second
// traversal.
func (r *moduleRows) complete() bool {
	if !r.valid() {
		return false
	}
	for index, row := range r.imports {
		if !r.declared[index] || row.Term != keyspace.MakeTerm(keyspace.FamilyImport, uint32(index+1)) || row.Call == 0 {
			return false
		}
		if row.Request == 0 || keyspace.TermFamily(row.Request) != keyspace.FamilyString || keyspace.TermOrdinal(row.Request) == 0 || row.Key != 0 {
			return false
		}
	}
	return true
}

func (r *moduleRows) slot(slot int) bool {
	return r.valid() && slot >= 0 && slot < len(r.imports)
}

// declare reserves one already-minted Import term for a census slot and
// installs its authored Call.  It is the preferred row operation for the
// collector because the canonical term is derived from the slot rather than
// from discovery order.
func (r *moduleRows) declare(slot int, call, request keyspace.Term) (keyspace.Term, error) {
	if !r.slot(slot) {
		return 0, errModuleSlotRange
	}
	term := keyspace.MakeTerm(keyspace.FamilyImport, uint32(slot+1))
	if term == 0 {
		return 0, errModuleSlotRange
	}
	if err := r.set(slot, term, call, request); err != nil {
		return 0, err
	}
	return term, nil
}

// set writes the authored Term/Call pair exactly once.  It accepts an
// explicit term so a root allocator can prove that the reserved identity is
// the same canonical identity it handed to the collector.  Out-of-order
// slots are valid; duplicate slots and noncanonical terms are not.
func (r *moduleRows) set(slot int, term, call, request keyspace.Term) error {
	if !r.slot(slot) {
		return errModuleSlotRange
	}
	if r.declared[slot] {
		return errModuleSlotDuplicate
	}
	want := keyspace.MakeTerm(keyspace.FamilyImport, uint32(slot+1))
	if term == 0 || term != want || keyspace.TermFamily(term) != keyspace.FamilyImport {
		return fmt.Errorf("%w: term %v for slot %d", errModuleRowsInvalid, term, slot)
	}
	if call == 0 || keyspace.TermFamily(call) != keyspace.FamilyCall || keyspace.TermOrdinal(call) == 0 {
		return fmt.Errorf("%w: call %v for slot %d", errModuleRowsInvalid, call, slot)
	}
	if request == 0 || keyspace.TermFamily(request) != keyspace.FamilyString || keyspace.TermOrdinal(request) == 0 {
		return fmt.Errorf("%w: request %v for slot %d", errModuleRowsInvalid, request, slot)
	}
	r.imports[slot] = module.Import{Term: term, Call: call, Request: request}
	// Alias is intentionally zero until the one optional alias observation.
	// freeze treats an unset alias as the explicit authored-zero form.
	r.declared[slot] = true
	return nil
}

// freeze returns a fresh authored Module input. It validates the complete
// census denominator and the cross-family bounds available at this stage,
// while intentionally leaving only derived Key and Entry absent for
// flow.Assemble. Repeated calls are deterministic and each result owns its
// slice, so caller mutation cannot affect a later freeze.
func (r *moduleRows) freeze(counts [keyspace.FamilyCount]uint32) (module.Input, error) {
	if !r.valid() {
		return module.Input{}, errModuleRowsInvalid
	}
	if uint64(len(r.imports)) != uint64(counts[keyspace.FamilyImport]) {
		return module.Input{}, errModuleCounts
	}
	// Module's input is authored-only. Outcome identity and every resolution
	// or Entry projection are installed by Flow.Assemble, so an authored
	// collector freeze must not accept either Invalid-family or derived Outcome
	// cardinality as part of its denominator.
	if counts[keyspace.FamilyInvalid] != 0 || counts[keyspace.FamilyOutcome] != 0 {
		return module.Input{}, errModuleCounts
	}
	for family, count := range counts {
		if count > keyspace.MaxTermOrdinal {
			return module.Input{}, errModuleCounts
		}
		if keyspace.Family(family) == keyspace.FamilyInvalid {
			if count != 0 {
				return module.Input{}, errModuleCounts
			}
		}
	}
	for slot, row := range r.imports {
		if !r.declared[slot] {
			return module.Input{}, errModuleSlotMissing
		}
		want := keyspace.MakeTerm(keyspace.FamilyImport, uint32(slot+1))
		if row.Term != want || keyspace.TermFamily(row.Term) != keyspace.FamilyImport ||
			keyspace.TermOrdinal(row.Term) > counts[keyspace.FamilyImport] ||
			row.Call == 0 || keyspace.TermFamily(row.Call) != keyspace.FamilyCall ||
			keyspace.TermOrdinal(row.Call) == 0 || keyspace.TermOrdinal(row.Call) > counts[keyspace.FamilyCall] ||
			(row.Alias != 0 && (keyspace.TermFamily(row.Alias) != keyspace.FamilyCell ||
				keyspace.TermOrdinal(row.Alias) == 0 || keyspace.TermOrdinal(row.Alias) > counts[keyspace.FamilyCell])) ||
			row.Request == 0 || keyspace.TermFamily(row.Request) != keyspace.FamilyString ||
			keyspace.TermOrdinal(row.Request) > counts[keyspace.FamilyString] || row.Key != 0 {
			return module.Input{}, fmt.Errorf("%w: row %d", errModuleRowsInvalid, slot)
		}
	}
	imports := append([]module.Import(nil), r.imports...)
	return module.Input{Imports: imports}, nil
}

// ModuleRoot.Import records one census-declared direct require occurrence. The binder
// census is zero-based, while canonical Term ordinals are one-based.  This
// method accepts the former and performs the sole +1 translation at the
// reserved-span boundary.  It never calls mint and cannot create a
// discovery-order identity.
func (w ModuleRoot) Import(ordinal int, span source.Span, call Term) Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if ordinal < 0 || uint64(ordinal) >= uint64(c.counts[keyspace.FamilyImport]) ||
		!validTermInCounts(c, call) || keyspace.TermFamily(call) != keyspace.FamilyCall {
		rejectMutation(c, errors.New("program/lower/collector: invalid Module Import observation"))
		return 0
	}
	request, ok := c.Flow().Calls().moduleRequestTerm(call)
	if !ok {
		c.fail(errors.New("program/lower/collector: Module Import Call has no exact first String request"))
		return 0
	}
	raw, ok := c.Source().Literals().exactLiteral(request)
	if !ok || raw.Kind != keyspace.LiteralString {
		c.fail(errors.New("program/lower/collector: Module Import request is not a Source String"))
		return 0
	}
	if raw.String == "" {
		c.fail(errModuleRequestEmpty)
		return 0
	}
	if !c.addExact(raw) {
		return 0
	}
	term := w.fillReservedImport(uint32(ordinal+1), span)
	if term == 0 {
		return 0
	}
	if err := c.module.set(ordinal, term, call, request); err != nil {
		c.fail(err)
		return 0
	}
	return term
}

// SetImportAlias records the one optional authored local Cell alias for an
// already observed Import. A zero alias is legal and consumes the one write
// as an explicit authored absence. Key resolution remains outside this
// collector and is therefore impossible to smuggle through this path.
func (w ModuleRoot) SetImportAlias(importTerm, alias Term) bool {
	c := w.collector
	if !mutationReady(c) {
		return false
	}
	if keyspace.TermFamily(importTerm) != keyspace.FamilyImport ||
		keyspace.TermOrdinal(importTerm) == 0 || keyspace.TermOrdinal(importTerm) > c.counts[keyspace.FamilyImport] ||
		(alias != 0 && !validFamilyTerm(c, alias, keyspace.FamilyCell)) {
		return rejectMutation(c, errModuleAliasInvalid)
	}
	slot := int(keyspace.TermOrdinal(importTerm) - 1)
	// The semantic Cell bound is proved above at the owner proxy. Keep the row
	// mutation here rather than behind a moduleRows setter that another
	// production path could call without the Collector's live denominator.
	if !c.module.slot(slot) {
		c.fail(errModuleSlotRange)
		return false
	}
	if !c.module.declared[slot] {
		c.fail(errModuleSlotMissing)
		return false
	}
	if c.module.aliases[slot] {
		c.fail(errModuleAliasSet)
		return false
	}
	c.module.imports[slot].Alias = alias
	c.module.aliases[slot] = true
	return true
}
