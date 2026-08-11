// Package collector owns the lowerer's single construction cursor.
//
// The collector is deliberately not a Program authority.  It only gathers
// caller-owned construction rows until the typed Source/Flow/Static/Module
// owners can be built.  Source is the first owner: it allocates canonical
// Term identities and is the only capability allowed to turn raw exact-key
// payloads into Key handles.
package collector

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

var errCollectorTerminal = errors.New("program/lower/collector: collector is terminal")

// Term is the lower construction spelling of a canonical Program identity.
// It is an alias, not a second identity type or encoding.
type Term = keyspace.Term

// Collector is one unfinished, single-owner lower construction.  The fields
// are intentionally flat and typed by owner.  In particular, no core Builder,
// generic node table, provisional Key map, or semantic adapter is retained.
//
// Sibling files add the Flow/Static/Module row implementations. Their rows
// remain construction-only and are consumed by the one terminal Prepare
// transaction; no owner input or Source preimage escapes through Collector.
type Collector struct {
	name   string
	counts [keyspace.FamilyCount]uint32
	spans  [keyspace.FamilyCount][]source.Span
	source sourceRows
	flow   flowRows
	static staticRows
	module moduleRows
	// err is the first construction failure, if any. terminal is the
	// lifecycle bit: a successful Prepare is terminal with a nil err, while a
	// rejected mutation is terminal with its exact first err. Keeping these
	// independent prevents successful terminalization from manufacturing a
	// failure cause and prevents later failures from replacing the first one.
	err      error
	terminal bool
}

// sourceRows is the complete authored Source input before Source Build.  The
// slices are ordered by their canonical family ordinal; no map or interning
// table is part of this construction state.
type sourceRows struct {
	nil          []source.NilLiteral
	bool         []source.BoolLiteral
	integer      []source.IntegerLiteral
	float        []source.FloatLiteral
	string       []source.StringLiteral
	bodies       []source.BodySource
	binds        []source.BindCells
	functions    []source.FunctionFormals
	keys         []source.KeyInput
	faults       []source.ControlFault
	exact        []keyspace.LiteralValue
	entry        keyspace.Term
	filled       []bool
	importFilled []bool
}

// New creates one fresh lower collector. moduleImports and globals are
// pre-censused before lowering; their dense identities are reserved before
// any source traversal so visitation order cannot change Program terms.
func New(name string, moduleImports int, globals bind.GlobalCensus) *Collector {
	c := &Collector{name: name}
	if name == "" {
		c.fail(errors.New("program/lower/collector: empty source name"))
		return c
	}
	if moduleImports < 0 || uint64(moduleImports) > uint64(keyspace.MaxTermOrdinal) {
		c.fail(fmt.Errorf("program/lower/collector: invalid Import census %d", moduleImports))
		return c
	}
	// Imports are census identities, not visitation identities. Their
	// ordinals are fixed before lowering starts and their spans are filled
	// by the module lane as each census row is encountered.
	c.counts[keyspace.FamilyImport] = uint32(moduleImports)
	c.spans[keyspace.FamilyImport] = make([]source.Span, moduleImports)
	c.source.importFilled = make([]bool, moduleImports)
	if err := c.reserveGlobalCells(globals); err != nil {
		c.fail(err)
		return c
	}
	c.module.init(moduleImports)
	return c
}

func (c *Collector) fail(err error) {
	if c == nil || c.terminal || c.err != nil {
		return
	}
	if err == nil {
		err = errors.New("program/lower/collector: unspecified construction failure")
	}
	c.err = err
	terminalize(c)
}

func failure(c *Collector) error {
	if c == nil {
		return errors.New("program/lower/collector: nil collector")
	}
	if c.err != nil {
		return c.err
	}
	if c.terminal {
		return errCollectorTerminal
	}
	return nil
}

// terminalize irreversibly drops every construction row and poisons every
// writer capability already handed out by the Collector roots. Owner drafts
// returned by a successful Prepare own their compacted copies and do not
// retain these slice headers, maps, or census arrays.
func terminalize(c *Collector) {
	if c == nil {
		return
	}
	c.name = ""
	c.counts = [keyspace.FamilyCount]uint32{}
	c.spans = [keyspace.FamilyCount][]source.Span{}
	c.source = sourceRows{}
	c.flow = flowRows{}
	c.static = staticRows{}
	c.module = moduleRows{}
	c.terminal = true
}

// mutationReady is the common gate for public mutators. A nil capability is
// deliberately inert; an already terminal capability is also inert and must
// not replace the first cause. Callers must report their operation-specific
// rejection through rejectMutation after this gate.
func mutationReady(c *Collector) bool {
	return c != nil && !c.terminal && c.err == nil
}

// rejectMutation records the first exact rejection cause and terminalizes the
// whole construction cursor. It intentionally does nothing for nil or already
// terminal collectors so lawful nil/after-terminal probes cannot manufacture a
// new cause.
func rejectMutation(c *Collector, err error) bool {
	if !mutationReady(c) {
		return false
	}
	c.fail(err)
	return false
}

func rejectMutationf(c *Collector, format string, args ...any) bool {
	return rejectMutation(c, fmt.Errorf(format, args...))
}

func rejectTermMutationf(c *Collector, format string, args ...any) Term {
	rejectMutationf(c, format, args...)
	return 0
}

// validSpan performs only source-coordinate validation; it never publishes or
// retains a Source authority. Keeping it package-scoped prevents validation
// detail from becoming another Collector semantic method.
func validSpan(c *Collector, span source.Span) bool {
	return c != nil && c.err == nil && !c.terminal && validSourceSpan(span) &&
		(span.File == "" || span.File == c.name)
}

func validSourceSpan(span source.Span) bool {
	allZero := span.StartLine == 0 && span.StartCol == 0 && span.EndLine == 0 && span.EndCol == 0
	if allZero {
		return true
	}
	if span.StartLine == 0 || span.StartCol == 0 {
		return false
	}
	if span.EndLine == 0 || span.EndCol == 0 {
		return span.EndLine == 0 && span.EndCol == 0
	}
	return span.EndLine > span.StartLine ||
		(span.EndLine == span.StartLine && span.EndCol >= span.StartCol)
}

func validBodyTerm(term keyspace.Term) bool {
	return keyspace.TermFamily(term) == keyspace.FamilyBody && keyspace.TermOrdinal(term) != 0
}

func validTerm(term keyspace.Term) bool {
	return keyspace.TermFamily(term) != keyspace.FamilyInvalid && keyspace.TermOrdinal(term) != 0
}

func validTermInCounts(c *Collector, term keyspace.Term) bool {
	if c == nil || c.terminal || !validTerm(term) {
		return false
	}
	family := keyspace.TermFamily(term)
	return family < keyspace.FamilyCount && family != keyspace.FamilyImport &&
		keyspace.TermOrdinal(term) <= c.counts[family]
}
