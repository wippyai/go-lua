package canonical

import (
	"slices"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/facts"
	canonref "github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// programEntryValueProjection owns the summary-level entry values for one
// callee. It composes caller-provided argument facts, prototype receiver facts,
// and explicit source seeds before transfer installs the result at function
// entry.
type programEntryValueProjection struct {
	program            *program
	ref                summary.FuncRef
	deps               summary.EntryPublicationDependencies
	prototypeReceivers []summary.EntryValuePrototypeReceiver
	hasInferredSlots   bool
}

func (p *program) entryValueProjection(ref summary.FuncRef, deps summary.EntryPublicationDependencies) (programEntryValueProjection, bool) {
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

func (p *program) EntryValues(ref summary.FuncRef, deps summary.EntryPublicationDependencies) map[int]product.AbstractValue {
	proj, ok := p.entryValueProjection(ref, deps)
	if !ok {
		return nil
	}
	return proj.project()
}

func (p *program) EntrySymbolValues(ref summary.FuncRef) map[cfg.SymbolID]product.AbstractValue {
	var out map[cfg.SymbolID]product.AbstractValue
	add := func(sym cfg.SymbolID, t typ.Type) {
		if sym == 0 || t == nil || typ.IsAbsentOrUnknown(t) {
			return
		}
		if out == nil {
			out = make(map[cfg.SymbolID]product.AbstractValue)
		}
		seed := product.FromType(t)
		if prev, had := out[sym]; had {
			out[sym] = product.Domain.Join(prev, seed)
		} else {
			out[sym] = seed
		}
	}

	if g := p.Graph(ref); g != nil {
		if obsCtx, ok := p.observationContexts[ref]; ok && len(obsCtx.declared) > 0 {
			bindings := g.Bindings()
			if bindings != nil {
				for _, sym := range bindings.ReferencedGlobals() {
					t, ok := obsCtx.declared[sym]
					if !ok {
						continue
					}
					add(sym, t)
				}
			}
		}
	}

	entries := p.facts.CallbackEnv(ref)
	for _, entry := range entries {
		add(entry.Symbol, entry.Type)
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
		values := p.deps.CallEntryPublication(dep, p.ref).Values
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

// hasInferredEntrySlot is a query-dependency guard: functions whose parameters
// are all fixed declarations do not need aggregate caller entry evidence, so
// EntryValues must not read caller summaries and perturb the interprocedural
// cache/fixpoint. Refinable structural annotations (`{any}`, `any[]`, maps with
// dynamic interiors) are not fixed; EntrySeedEffect can compose caller evidence
// with them.
func (p *program) hasInferredEntrySlot(ref summary.FuncRef) bool {
	if p == nil {
		return false
	}
	g := p.Graph(ref)
	if g == nil {
		return false
	}
	for slot := range g.ParamSymbols() {
		if !p.paramSlotFixed(ref, slot) {
			return true
		}
	}
	return false
}

func (p *program) publishedPrototypes(ref summary.FuncRef) []cfg.SymbolID {
	sites := p.facts.SetMetatableSites(ref)
	if len(sites) == 0 {
		return nil
	}
	out := make([]cfg.SymbolID, 0, len(sites))
	for _, site := range sites {
		if site.PrototypeSym != 0 {
			out = append(out, site.PrototypeSym)
		}
	}
	if len(out) == 0 {
		return nil
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func (p *program) callerRefs(ref summary.FuncRef) []summary.FuncRef {
	if p == nil {
		return nil
	}
	if p.callerRefsByCallee != nil {
		return append([]summary.FuncRef(nil), p.callerRefsByCallee[ref]...)
	}
	var out []summary.FuncRef
	for _, caller := range p.refs {
		for _, callee := range p.Callees(caller) {
			if callee == ref {
				out = append(out, caller)
				break
			}
		}
	}
	return out
}

func (p *program) prototypePublisherRefs(receivers []summary.EntryValuePrototypeReceiver) []summary.FuncRef {
	if p == nil || len(receivers) == 0 || len(p.prototypePublishersBySym) == 0 {
		return nil
	}
	var out []summary.FuncRef
	seen := make(map[summary.FuncRef]bool)
	for _, receiver := range receivers {
		if receiver.Prototype == 0 {
			continue
		}
		for _, dep := range p.prototypePublishersBySym[receiver.Prototype] {
			if seen[dep] {
				continue
			}
			seen[dep] = true
			out = append(out, dep)
		}
	}
	canonref.SortFuncRefs(out)
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
