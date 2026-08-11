package collector

import (
	"errors"
	"fmt"
	"sync"

	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

// Prepared is the opaque, copyable handoff from lowering to Flow assembly.
// It deliberately exposes neither individual owner finalizers nor the entry:
// callers can only consume the complete quartet through Assemble. Copies
// share one state, so exactly one copy can transfer the bundle.
type Prepared struct{ state *preparedState }

// preparedState is the opaque one-shot root assembly capability. It carries
// owner-issued capabilities only; it does not derive semantics, rewrite
// inputs, or mint a second authority.
//
// Assemble moves and clears the complete bundle while holding mu, before it
// calls flow.Assemble. This makes copies/concurrent callers harmless: the
// winner owns the one transfer attempt and every loser observes terminal
// state without touching any owner.
type preparedState struct {
	mu       sync.Mutex
	source   source.Finalizer
	flow     *flow.Draft
	static   static.Finalizer
	module   module.Finalizer
	entry    Term
	terminal bool
}

// Assemble consumes this Prepared capability exactly once. Flow remains the
// only owner of cross-owner assembly semantics; Prepared supplies fallback
// owner cleanup when Flow cannot claim or complete the assembly.
func (prepared Prepared) Assemble() (*flow.Assembly, error) {
	if prepared.state == nil {
		return nil, errors.New("program/lower/collector: invalid Prepared")
	}
	state := prepared.state
	state.mu.Lock()
	if state.terminal {
		state.mu.Unlock()
		return nil, errors.New("program/lower/collector: Prepared is terminal")
	}
	state.terminal = true
	sourceFinalizer, flowDraft := state.source, state.flow
	staticFinalizer, moduleFinalizer, entry := state.static, state.module, state.entry
	state.source = source.Finalizer{}
	state.flow = nil
	state.static = static.Finalizer{}
	state.module = module.Finalizer{}
	state.entry = 0
	state.mu.Unlock()

	assembly, err := flow.Assemble(sourceFinalizer, staticFinalizer, moduleFinalizer, flowDraft, entry)
	if err == nil {
		return assembly, nil
	}
	// Prepared uniquely moved these sibling capabilities out of its opaque
	// state. If Flow could not claim its Draft, no other invocation can reach
	// them; if Flow failed after claiming, its cleanup has already made these
	// calls terminal. Owner Abort operations are one-shot, so repeating them
	// here is a safe best-effort close and can never reopen or recommit an owner.
	_ = moduleFinalizer.Abort()
	_ = staticFinalizer.Abort()
	_ = sourceFinalizer.Abort()
	return nil, err
}

type prepareStage uint8

const (
	prepareSourceClaimed prepareStage = iota + 1
	prepareStaticClaimed
	prepareModuleClaimed
)

// Prepare is Collector's sole terminal publication boundary. It privately
// builds Source first, uses only the claimed Source preimage to freeze the
// dependent owners, claims Static and Module, and builds Flow last. The
// collector is terminal and its construction scratch is cleared on every
// return path, including validation failure.
func (c *Collector) Prepare() (Prepared, error) {
	if c == nil {
		return Prepared{}, errors.New("program/lower/collector: nil collector")
	}
	if c.terminal {
		if c.err != nil {
			return Prepared{}, c.err
		}
		return Prepared{}, errCollectorTerminal
	}

	// Claim before materialization. Existing root/leaf values all route back
	// through the lifecycle bit or mint/addExact, so this closes every mutator
	// while the private transaction is in flight as well as after it returns.
	c.terminal = true

	sourceInput, entry, err := materializeSourceInput(c)
	if err != nil {
		return Prepared{}, terminalPrepareFailure(c, err)
	}
	sourceDraft, err := source.Build(sourceInput)
	if err != nil {
		return Prepared{}, terminalPrepareFailure(c, err)
	}
	sourceFinalizer, err := sourceDraft.Finalizer()
	if err != nil {
		return Prepared{}, terminalPrepareFailure(c, err)
	}
	stage := prepareSourceClaimed

	// Keep the Source preimage lexical to this transaction. There is no helper
	// accepting a caller-supplied Preimage and therefore no foreign/repeatable
	// key-freeze injection surface inside Collector.
	preimage := sourceFinalizer.Preimage()
	identity := preimage.Identity()
	if identity.Name() != c.name || identity.Name() == "" {
		return failPrepare(c, stage, sourceFinalizer, static.Finalizer{}, module.Finalizer{}, "Source preflight", errors.New("Source Preimage is absent or foreign"))
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			if identity.FamilyCount(family) != 0 {
				return failPrepare(c, stage, sourceFinalizer, static.Finalizer{}, module.Finalizer{}, "Source preflight", errors.New("Source Preimage contains authored Outcome"))
			}
			continue
		}
		if identity.FamilyCount(family) != int(c.counts[family]) {
			return failPrepare(c, stage, sourceFinalizer, static.Finalizer{}, module.Finalizer{}, "Source preflight", errors.New("Source Preimage cardinality disagrees with collector"))
		}
	}
	if identity.TermCount() == 0 {
		return failPrepare(c, stage, sourceFinalizer, static.Finalizer{}, module.Finalizer{}, "Source preflight", errors.New("empty Source Preimage"))
	}
	flowInput, err := c.flow.freeze(preimage, c.counts)
	if err != nil {
		return failPrepare(c, stage, sourceFinalizer, static.Finalizer{}, module.Finalizer{}, "Flow freeze", err)
	}
	staticInput, err := c.static.freeze(preimage, c.counts)
	if err != nil {
		return failPrepare(c, stage, sourceFinalizer, static.Finalizer{}, module.Finalizer{}, "Static freeze", err)
	}
	moduleInput, err := c.module.freeze(c.counts)
	if err != nil {
		return failPrepare(c, stage, sourceFinalizer, static.Finalizer{}, module.Finalizer{}, "Module freeze", err)
	}

	staticDraft, err := static.Build(staticInput)
	if err != nil {
		return failPrepare(c, stage, sourceFinalizer, static.Finalizer{}, module.Finalizer{}, "Static build", err)
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		return failPrepare(c, stage, sourceFinalizer, static.Finalizer{}, module.Finalizer{}, "Static claim", err)
	}
	stage = prepareStaticClaimed

	moduleDraft, err := module.Build(moduleInput)
	if err != nil {
		return failPrepare(c, stage, sourceFinalizer, staticFinalizer, module.Finalizer{}, "Module build", err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		return failPrepare(c, stage, sourceFinalizer, staticFinalizer, module.Finalizer{}, "Module claim", err)
	}
	stage = prepareModuleClaimed

	flowDraft, err := flow.Build(flowInput)
	if err != nil {
		return failPrepare(c, stage, sourceFinalizer, staticFinalizer, moduleFinalizer, "Flow build", err)
	}

	terminalize(c)
	return Prepared{state: &preparedState{
		source: sourceFinalizer,
		flow:   flowDraft,
		static: staticFinalizer,
		module: moduleFinalizer,
		entry:  entry,
	}}, nil
}

// terminalPrepareFailure records the exact first private transaction cause
// before clearing the Collector construction plane. Prepare has already
// claimed terminal, so fail cannot be used here (it intentionally ignores
// already-terminal cursors).
func terminalPrepareFailure(c *Collector, cause error) error {
	if cause == nil {
		cause = errors.New("program/lower/collector: unspecified Prepare failure")
	}
	if c != nil && c.err == nil {
		c.err = cause
	}
	terminalize(c)
	if c != nil && c.err != nil {
		return c.err
	}
	return cause
}

// materializeSourceInput is deliberately private to the terminal transaction.
// It validates the complete Source denominator and then returns a borrowed
// view over Collector-owned rows for the immediate synchronous source.Build
// call. Prepare has already closed the serial Collector construction cursor;
// this view is neither published nor safe for concurrent Prepare/mutation.
func materializeSourceInput(c *Collector) (source.Input, Term, error) {
	if c == nil {
		return source.Input{}, 0, errors.New("program/lower/collector: nil collector")
	}
	if !c.terminal || c.err != nil {
		return source.Input{}, 0, errors.New("program/lower/collector: Source materialization outside terminal Prepare")
	}
	if c.name == "" || c.source.entry == 0 || len(c.source.bodies) == 0 {
		return source.Input{}, 0, errors.New("program/lower/collector: incomplete Source construction")
	}
	if err := validateSourceRows(c); err != nil {
		return source.Input{}, 0, err
	}
	if len(c.source.filled) != len(c.source.bodies) {
		return source.Input{}, 0, errors.New("program/lower/collector: incomplete Body fill denominator")
	}
	for _, filled := range c.source.filled {
		if !filled {
			return source.Input{}, 0, errors.New("program/lower/collector: unfilled Body")
		}
	}
	for _, filled := range c.source.importFilled {
		if !filled {
			return source.Input{}, 0, errors.New("program/lower/collector: unfilled reserved Import")
		}
	}
	if !c.module.complete() {
		return source.Input{}, 0, errors.New("program/lower/collector: incomplete Module census")
	}
	if !validBodyTerm(c.source.entry) || keyspace.TermOrdinal(c.source.entry) > c.counts[keyspace.FamilyBody] {
		return source.Input{}, 0, errors.New("program/lower/collector: Entry is not a known Body")
	}

	input := source.Input{
		Name:       string([]byte(c.name)),
		Families:   make([]source.FamilySpans, int(keyspace.FamilyCount-1)),
		Nil:        c.source.nil,
		Bool:       c.source.bool,
		Integer:    c.source.integer,
		Float:      c.source.float,
		String:     c.source.string,
		Bodies:     c.source.bodies,
		Binds:      c.source.binds,
		Functions:  c.source.functions,
		Keys:       c.source.keys,
		Faults:     c.source.faults,
		ExactAtoms: c.source.exact,
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		input.Families[family-1] = source.FamilySpans{Family: family, Spans: c.spans[family]}
	}
	return input, c.source.entry, nil
}

// validateSourceRows closes the Source denominator before the borrowed
// synchronous view can be formed. All authored family rows must agree with the
// single collector census; a partial or hand-mutated row plane is rejected rather
// than delegated to a later owner.
func validateSourceRows(c *Collector) error {
	if c == nil {
		return errors.New("program/lower/collector: nil Source rows")
	}
	if c.counts[keyspace.FamilyInvalid] != 0 {
		return errors.New("program/lower/collector: invalid Source family denominator")
	}
	pairs := []struct {
		family keyspace.Family
		got    int
		name   string
	}{
		{keyspace.FamilyNil, len(c.source.nil), "Nil"},
		{keyspace.FamilyBool, len(c.source.bool), "Bool"},
		{keyspace.FamilyInteger, len(c.source.integer), "Integer"},
		{keyspace.FamilyFloat, len(c.source.float), "Float"},
		{keyspace.FamilyString, len(c.source.string), "String"},
		{keyspace.FamilyBody, len(c.source.bodies), "Body"},
		{keyspace.FamilyBind, len(c.source.binds), "Bind"},
		{keyspace.FamilyFunction, len(c.source.functions), "Function"},
		{keyspace.FamilyKey, len(c.source.keys), "Key"},
		{keyspace.FamilyControlFault, len(c.source.faults), "ControlFault"},
	}
	for _, pair := range pairs {
		if pair.got != int(c.counts[pair.family]) {
			return fmt.Errorf("program/lower/collector: Source %s row count %d disagrees with census %d", pair.name, pair.got, c.counts[pair.family])
		}
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if len(c.spans[family]) != int(c.counts[family]) {
			return fmt.Errorf("program/lower/collector: Source %v span count %d disagrees with census %d", family, len(c.spans[family]), c.counts[family])
		}
	}
	if len(c.source.importFilled) != int(c.counts[keyspace.FamilyImport]) {
		return errors.New("program/lower/collector: Import fill denominator disagrees with census")
	}
	return nil
}

// failPrepare aborts exactly the owners already claimed by this transaction,
// in reverse order, then clears all Collector construction scratch. Abort
// failures are joined to the primary cause so a lifecycle defect cannot be
// silently mistaken for ordinary validation failure.
func failPrepare(
	c *Collector,
	stage prepareStage,
	sourceFinalizer source.Finalizer,
	staticFinalizer static.Finalizer,
	moduleFinalizer module.Finalizer,
	label string,
	cause error,
) (Prepared, error) {
	var cleanup error
	if stage >= prepareModuleClaimed && !moduleFinalizer.Abort() {
		cleanup = errors.Join(cleanup, errors.New("program/lower/collector: Module abort failed"))
	}
	if stage >= prepareStaticClaimed {
		cleanup = errors.Join(cleanup, staticFinalizer.Abort())
	}
	if stage >= prepareSourceClaimed {
		cleanup = errors.Join(cleanup, sourceFinalizer.Abort())
	}
	// Prepare has already claimed the Collector lifecycle bit. Preserve the
	// first private transaction cause as the Collector's exact error; cleanup
	// diagnostics are returned alongside it but can never replace it.
	if c != nil && c.err == nil {
		c.err = cause
	}
	terminalize(c)
	primary := cause
	if primary == nil {
		primary = fmt.Errorf("program/lower/collector: prepare %s failed", label)
	}
	if cleanup != nil {
		return Prepared{}, errors.Join(primary, cleanup)
	}
	return Prepared{}, primary
}
