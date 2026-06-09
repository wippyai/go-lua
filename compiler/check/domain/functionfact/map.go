package functionfact

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// Evidence is the per-symbol evidence admitted into FunctionFacts projection.
type Evidence struct {
	Params      []typ.Type
	BodyParams  []typ.Type
	EntryParams []typ.Type
	Summary     []typ.Type
	Narrow      []typ.Type
	Signature   *typ.Function
	Refinement  *constraint.FunctionRefinement
	EnvReturns  []contract.EnvReturnSpec
}

// Builder admits per-symbol evidence into FunctionFacts projection products.
type Builder struct {
	evidence map[cfg.SymbolID]Evidence
}

// NewBuilder creates an empty FunctionFact evidence builder.
func NewBuilder() *Builder {
	return &Builder{}
}

func (b *Builder) update(sym cfg.SymbolID, update func(*Evidence)) {
	if b == nil || sym == 0 || update == nil {
		return
	}
	if b.evidence == nil {
		b.evidence = make(map[cfg.SymbolID]Evidence)
	}
	part := b.evidence[sym]
	update(&part)
	b.evidence[sym] = part
}

// AddPublicParams admits public caller-obligation parameter evidence.
func (b *Builder) AddPublicParams(sym cfg.SymbolID, params []typ.Type) {
	if len(params) == 0 {
		return
	}
	b.update(sym, func(part *Evidence) {
		part.Params = paramevidence.JoinCallVectors(part.Params, params)
	})
}

// AddBodyParams admits body-contract parameter evidence.
func (b *Builder) AddBodyParams(sym cfg.SymbolID, params []typ.Type) {
	if len(params) == 0 {
		return
	}
	b.update(sym, func(part *Evidence) {
		part.BodyParams = paramevidence.JoinBodyVectors(part.BodyParams, params)
	})
}

// AddEntryParams admits observed call-entry parameter evidence.
func (b *Builder) AddEntryParams(sym cfg.SymbolID, params []typ.Type) {
	if len(params) == 0 {
		return
	}
	b.update(sym, func(part *Evidence) {
		part.EntryParams = paramevidence.JoinEntryVectors(part.EntryParams, params)
	})
}

// EntryParamsFacts converts call-entry evidence vectors into FunctionFacts
// projection.
func EntryParamsFacts(entries map[cfg.SymbolID][]typ.Type) api.FunctionFacts {
	if len(entries) == 0 {
		return nil
	}
	builder := NewBuilder()
	for _, sym := range cfg.SortedSymbolIDs(entries) {
		builder.AddEntryParams(sym, entries[sym])
	}
	return builder.Build()
}

// AddSummary admits declared/pre-flow return summary evidence.
func (b *Builder) AddSummary(sym cfg.SymbolID, summary []typ.Type) {
	if len(summary) == 0 {
		return
	}
	b.update(sym, func(part *Evidence) {
		part.Summary = returnsummary.Merge(part.Summary, summary)
	})
}

// AddNarrow admits post-flow return summary evidence.
func (b *Builder) AddNarrow(sym cfg.SymbolID, narrow []typ.Type) {
	if len(narrow) == 0 {
		return
	}
	b.update(sym, func(part *Evidence) {
		part.Narrow = returnsummary.Merge(part.Narrow, narrow)
	})
}

// AddSignature admits a source signature.
func (b *Builder) AddSignature(sym cfg.SymbolID, sig *typ.Function) {
	if sig == nil {
		return
	}
	b.update(sym, func(part *Evidence) {
		part.Signature = MergeSignature(part.Signature, sig)
	})
}

// AddRefinement admits function refinement evidence.
func (b *Builder) AddRefinement(sym cfg.SymbolID, refinement *constraint.FunctionRefinement) {
	if refinement == nil {
		return
	}
	b.update(sym, func(part *Evidence) {
		part.Refinement = MergeRefinement(part.Refinement, refinement)
	})
}

// AddEnvReturns admits environment-parameterized return evidence.
func (b *Builder) AddEnvReturns(sym cfg.SymbolID, envReturns []contract.EnvReturnSpec) {
	if len(envReturns) == 0 {
		return
	}
	b.update(sym, func(part *Evidence) {
		part.EnvReturns = JoinEnvReturns(part.EnvReturns, envReturns)
	})
}

// Build returns the admitted FunctionFacts projection.
func (b *Builder) Build() api.FunctionFacts {
	if b == nil || len(b.evidence) == 0 {
		return nil
	}
	return Build(b.evidence)
}

func fromFact(sym cfg.SymbolID, fact api.FunctionFact) api.FunctionFacts {
	if sym == 0 {
		return nil
	}
	ff := Join(api.FunctionFact{}, fact)
	if Empty(ff) {
		return nil
	}
	return api.FunctionFacts{sym: ff}
}

// BuildOne builds FunctionFacts projection from one per-symbol evidence value.
func BuildOne(sym cfg.SymbolID, evidence Evidence) api.FunctionFacts {
	return fromFact(sym, factFromEvidence(evidence))
}

// Build builds FunctionFacts projection from per-symbol evidence.
func Build(parts map[cfg.SymbolID]Evidence) api.FunctionFacts {
	if len(parts) == 0 {
		return nil
	}
	out := make(api.FunctionFacts, len(parts))
	for _, sym := range cfg.SortedSymbolIDs(parts) {
		if sym == 0 {
			continue
		}
		ff := Join(api.FunctionFact{}, factFromEvidence(parts[sym]))
		if Empty(ff) {
			continue
		}
		out[sym] = ff
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func factFromEvidence(part Evidence) api.FunctionFact {
	return api.FunctionFact{
		Call:    api.FunctionCallProjection{Params: product.LiftVector(part.Params)},
		Body:    api.FunctionBodyProjection{Params: product.LiftVector(part.BodyParams)},
		Entry:   api.FunctionEntryProjection{Params: product.LiftVector(part.EntryParams)},
		Returns: api.FunctionReturnProjection{Preflow: product.LiftVector(part.Summary), Postflow: product.LiftVector(part.Narrow)},
		Public:  api.FunctionPublicProjection{Signature: part.Signature},
		Effects: api.FunctionEffectProjection{Refinement: part.Refinement},
		Export:  api.FunctionExportProjection{EnvReturns: part.EnvReturns},
	}
}
