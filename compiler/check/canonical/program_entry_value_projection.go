package canonical

import (
	"github.com/wippyai/go-lua/compiler/check/canonical/facts"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
)

// programEntryValueProjection owns the summary-level entry values for one
// callee. It composes caller-provided argument facts, prototype receiver facts,
// and explicit source seeds before transfer installs the result at function
// entry.
type programEntryValueProjection struct {
	program            *program
	ref                summary.FuncRef
	deps               summary.EntryValueDependencies
	prototypeReceivers []summary.EntryValuePrototypeReceiver
	hasInferredSlots   bool
}

func (p *program) entryValueProjection(ref summary.FuncRef, deps summary.EntryValueDependencies) (programEntryValueProjection, bool) {
	if p == nil || deps == nil {
		return programEntryValueProjection{}, false
	}
	receivers := p.facts.MethodReceivers(ref)
	return programEntryValueProjection{
		program:            p,
		ref:                ref,
		deps:               deps,
		prototypeReceivers: entryValuePrototypeReceivers(receivers),
		hasInferredSlots:   p.hasInferredEntrySlot(ref),
	}, true
}

func (p programEntryValueProjection) project() summary.EntryValues {
	out := p.aggregate()
	out = p.withSourceEntrySeeds(out)
	out = p.program.withPrototypeReceiverBaselines(p.ref, out, p.prototypeReceivers, p.deps)
	out = p.program.withPrototypeMethodSurfacesForRef(p.ref, out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func (p programEntryValueProjection) aggregate() summary.EntryValues {
	return summary.AggregateEntryValues(summary.EntryValueAggregation{
		Callee:                p.ref,
		HasInferredSlots:      p.hasInferredSlots,
		EachCallerEntryValues: p.eachCallerEntryValues,
		PrototypeReceivers:    p.prototypeReceivers,
		EachPrototypeSource:   p.eachPrototypeSource,
		SlotDeclared: func(slot int) bool {
			return p.program.paramSlotFixed(p.ref, slot)
		},
	})
}

func (p programEntryValueProjection) eachCallerEntryValues(yield func(summary.EntryValues)) {
	if !p.hasInferredSlots {
		return
	}
	for _, dep := range p.program.callerRefs(p.ref) {
		values := p.deps.CallEntryValues(dep, p.ref)
		if len(values) != 0 {
			yield(values)
		}
	}
}

func (p programEntryValueProjection) eachPrototypeSource(yield func(summary.EntryValuePrototypeSource)) {
	for _, dep := range p.program.prototypePublisherRefs(p.prototypeReceivers) {
		if protos := p.program.publishedPrototypes(dep); len(protos) > 0 {
			yield(summary.EntryValuePrototypeSource{
				Prototypes: protos,
				Self:       p.deps.PrototypeSelf(dep),
			})
		}
	}
}

func (p programEntryValueProjection) withSourceEntrySeeds(values summary.EntryValues) summary.EntryValues {
	out := values
	if seeds := entryValueSeeds(p.program.facts.FunctionEntrySeeds(p.ref)); len(seeds) != 0 {
		out = summary.EntryValuesWithSeeds(out, seeds)
	}
	return out
}

func entryValueSeeds(seeds []facts.FunctionEntrySeed) []summary.EntryValueSeed {
	if len(seeds) == 0 {
		return nil
	}
	out := make([]summary.EntryValueSeed, 0, len(seeds))
	for _, seed := range seeds {
		out = append(out, summary.EntryValueSeed{
			Slot: seed.Slot,
			Type: seed.Type,
		})
	}
	return out
}
