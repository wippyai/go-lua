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
		staticFinalizer static.Finalizer
		moduleFinalizer programimports.Finalizer
		sourceClaimed   bool
		staticClaimed   bool
		moduleClaimed   bool
	)
	abortClaimed := func() error {
		var cleanup error
		if moduleClaimed && !moduleFinalizer.Abort() {
			cleanup = errors.Join(cleanup, errors.New("program/lower/collector: Module abort failed"))
		}
		if staticClaimed {
			cleanup = errors.Join(cleanup, staticFinalizer.Abort())
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

	sourceInput, entry, err := materializeSourceInput(c)
	if err != nil {
		return fail("Source materialization", err)
	}
	sourceDraft, err := source.Build(sourceInput)
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

	staticDraft, err := static.Build(staticInput)
	if err != nil {
		return fail("Static build", err)
	}
	staticFinalizer, err = staticDraft.Finalizer()
	if err != nil {
		return fail("Static claim", err)
	}
	staticClaimed = true

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

	// The Collector scratch is no longer needed once all owner inputs and
	// drafts are local. Flow owns cleanup for every claimed owner from here.
	terminalize(c)
	sealed, err := flow.Assemble(sourceFinalizer, staticFinalizer, moduleFinalizer, flowDraft, entry)
	if err != nil {
		return nil, fmt.Errorf("program/lower: Flow publication: %w", err)
	}
	published, err := program.Publish(sealed)
	if err != nil {
		return nil, fmt.Errorf("program/lower: Program publication: %w", err)
	}
	return published, nil
}

// materializeSourceInput is deliberately private to the terminal transaction.
// It validates the complete Source denominator and then returns a borrowed
// view over Collector-owned rows for the immediate synchronous source.Build
// call. Publish has already closed the serial Collector construction cursor;
// this view is neither published nor safe for concurrent mutation.
func materializeSourceInput(c *Collector) (source.Input, keyspace.Term, error) {
	if c == nil {
		return source.Input{}, 0, errors.New("program/lower/collector: nil collector")
	}
	if !c.terminal || c.err != nil {
		return source.Input{}, 0, errors.New("program/lower/collector: Source materialization outside terminal publication")
	}
	return c.source.Materialize(c.name, c.spans, c.counts, c.module.Complete())
}
