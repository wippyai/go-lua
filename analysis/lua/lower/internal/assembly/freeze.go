package assembly

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	programimports "github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
)

// Publish is Collector's terminal publication transaction. It privately
// builds and claims every owner, seals Flow from those lexical capabilities,
// and immediately publishes the canonical immutable Program. No construction
// owner or finalizer escapes this call.
func (c *Collector) Publish() (*program.Program, error) {
	if c == nil {
		return nil, errors.New("program/lower/collector: nil collector")
	}
	if c.terminal {
		if c.err != nil {
			return nil, c.err
		}
		return nil, errCollectorTerminal
	}

	// Claim before materialization. Existing root/leaf values all route back
	// through the lifecycle bit or mint/addExact, so this closes every mutator
	// while the private transaction is in flight as well as after it returns.
	c.terminal = true

	var (
		sourceFinalizer source.Finalizer
		staticComponent *static.Component
		staticView      staticquery.View
		moduleFinalizer programimports.Finalizer
		sourceClaimed   bool
		moduleClaimed   bool
	)
	abortClaimed := func() error {
		var cleanup error
		if moduleClaimed && !moduleFinalizer.Abort() {
			cleanup = errors.Join(cleanup, errors.New("program/lower/collector: Module abort failed"))
		}
		if sourceClaimed {
			cleanup = errors.Join(cleanup, sourceFinalizer.Abort())
		}
		return cleanup
	}
	fail := func(label string, cause error) (*program.Program, error) {
		cleanup := abortClaimed()
		if cause == nil {
			cause = fmt.Errorf("program/lower/collector: publication %s failed", label)
		}
		if c.err == nil {
			c.err = cause
		}
		terminalize(c)
		primary := cause
		if cleanup != nil {
			return nil, errors.Join(primary, cleanup)
		}
		return nil, primary
	}

	if err := validateSourceInput(c); err != nil {
		return fail("Source validation", err)
	}
	sourceDraft, err := source.Build(c.source)
	if err != nil {
		return fail("Source build", err)
	}
	sourceFinalizer, err = sourceDraft.Finalizer()
	if err != nil {
		return fail("Source claim", err)
	}
	sourceClaimed = true

	// Keep the Source preimage lexical to this transaction. There is no helper
	// accepting a caller-supplied Preimage and therefore no foreign/repeatable
	// key-freeze injection surface inside Collector.
	preimage := sourceFinalizer.Preimage()
	identity := preimage.Identity()
	if identity.Name() != c.name || identity.Name() == "" {
		return fail("Source preflight", errors.New("source preimage is absent or foreign"))
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			if identity.FamilyCount(family) != 0 {
				return fail("Source preflight", errors.New("source preimage contains authored outcome"))
			}
			continue
		}
		if identity.FamilyCount(family) != int(c.counts[family]) {
			return fail("Source preflight", errors.New("source preimage cardinality disagrees with collector"))
		}
	}
	if identity.TermCount() == 0 {
		return fail("Source preflight", errors.New("empty Source Preimage"))
	}
	flowInput, err := c.flow.Freeze(preimage, c.counts)
	if err != nil {
		return fail("Flow freeze", err)
	}
	staticInput, err := c.static.Freeze(preimage, c.counts)
	if err != nil {
		return fail("Static freeze", err)
	}
	moduleInput, err := c.module.Freeze(c.counts)
	if err != nil {
		return fail("Module freeze", err)
	}

	staticComponent, staticView, err = static.Build(staticInput)
	if err != nil {
		return fail("Static build", err)
	}

	moduleDraft, err := programimports.Build(moduleInput)
	if err != nil {
		return fail("Module build", err)
	}
	moduleFinalizer, err = moduleDraft.Finalizer()
	if err != nil {
		return fail("Module claim", err)
	}
	moduleClaimed = true

	flowDraft, err := flow.Build(flowInput)
	if err != nil {
		return fail("Flow build", err)
	}
	entry := c.entry

	// The Collector scratch is no longer needed once all owner inputs and
	// drafts are local. Flow owns cleanup for every claimed owner from here.
	terminalize(c)
	sealed, err := flow.Assemble(sourceFinalizer, staticComponent, staticView, moduleFinalizer, flowDraft, entry)
	if err != nil {
		return nil, fmt.Errorf("program/lower: Flow publication: %w", err)
	}
	published, err := program.Publish(sealed)
	if err != nil {
		return nil, fmt.Errorf("program/lower: Program publication: %w", err)
	}
	return published, nil
}

// validateSourceInput closes the Lua lowering construction boundary before
// Source.Build consumes the canonical Input directly. It checks only the
// census/transaction obligations that are not represented by Input itself;
// Source remains the authority for row ownership and semantic validation.
func validateSourceInput(c *Collector) error {
	if c == nil {
		return errors.New("program/lower/collector: nil collector")
	}
	if !c.terminal || c.err != nil {
		return errors.New("program/lower/collector: Source validation outside terminal publication")
	}
	if c.source.Name == "" || c.entry == 0 || len(c.source.Bodies) == 0 {
		return errors.New("program/lower/collector: incomplete Source construction")
	}
	if keyspace.TermFamily(c.entry) != keyspace.FamilyBody || keyspace.TermOrdinal(c.entry) == 0 ||
		keyspace.TermOrdinal(c.entry) > c.counts[keyspace.FamilyBody] {
		return errors.New("program/lower/collector: Entry is not a known Body")
	}
	if !c.module.Complete() {
		return errors.New("program/lower/collector: incomplete Module census")
	}
	if c.counts[keyspace.FamilyInvalid] != 0 {
		return errors.New("program/lower/collector: invalid Source family denominator")
	}
	if len(c.imports) != int(c.counts[keyspace.FamilyImport]) {
		return errors.New("program/lower/collector: incomplete reserved Import")
	}
	for _, filled := range c.imports {
		if !filled {
			return errors.New("program/lower/collector: incomplete reserved Import")
		}
	}
	rows := []struct {
		family keyspace.Family
		got    int
		name   string
	}{
		{keyspace.FamilyNil, len(c.source.Nil), "Nil"},
		{keyspace.FamilyBool, len(c.source.Bool), "Bool"},
		{keyspace.FamilyInteger, len(c.source.Integer), "Integer"},
		{keyspace.FamilyFloat, len(c.source.Float), "Float"},
		{keyspace.FamilyString, len(c.source.String), "String"},
		{keyspace.FamilyBody, len(c.source.Bodies), "Body"},
		{keyspace.FamilyBind, len(c.source.Binds), "Bind"},
		{keyspace.FamilyFunction, len(c.source.Functions), "Function"},
		{keyspace.FamilyKey, len(c.source.Keys), "Key"},
		{keyspace.FamilyControlFault, len(c.source.Faults), "ControlFault"},
	}
	for _, row := range rows {
		if row.got != int(c.counts[row.family]) {
			return fmt.Errorf("program/lower/collector: Source %s row count %d disagrees with census %d", row.name, row.got, c.counts[row.family])
		}
	}
	if c.source.CellSpellings != nil && len(c.source.CellSpellings) != int(c.counts[keyspace.FamilyCell]) {
		return fmt.Errorf("program/lower/collector: Source Cell spelling rows %d disagree with census %d", len(c.source.CellSpellings), c.counts[keyspace.FamilyCell])
	}
	var previousCall keyspace.Term
	for _, row := range c.source.CallSpellings {
		if keyspace.TermFamily(row.Call) != keyspace.FamilyCall || keyspace.TermOrdinal(row.Call) == 0 ||
			keyspace.TermOrdinal(row.Call) > c.counts[keyspace.FamilyCall] || row.Name == "" || previousCall >= row.Call {
			return errors.New("program/lower/collector: invalid Source Call spelling row")
		}
		previousCall = row.Call
	}
	if len(c.source.Families) != int(keyspace.FamilyCount-1) {
		return errors.New("program/lower/collector: incomplete Source family spans")
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		row := c.source.Families[family-1]
		if row.Family != family || len(row.Spans) != int(c.counts[family]) {
			return fmt.Errorf("program/lower/collector: Source %v span count %d disagrees with census %d", family, len(row.Spans), c.counts[family])
		}
	}
	if len(c.bodies) != len(c.source.Bodies) {
		return errors.New("program/lower/collector: incomplete Body construction")
	}
	for _, filled := range c.bodies {
		if !filled {
			return errors.New("program/lower/collector: unfilled Body")
		}
	}
	return nil
}
