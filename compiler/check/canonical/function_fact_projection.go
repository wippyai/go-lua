package canonical

import (
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/typ"
)

func (d *Driver) installFunctionFactProjection(sess api.AnalysisSession, prog *program, artifact canonicalSolveArtifact) {
	if d == nil || sess == nil || prog == nil {
		return
	}
	store := sess.CanonicalStoreHandle()
	if store == nil {
		return
	}
	projection := d.canonicalFunctionFactProjection(prog, store, artifact)
	store.SetCanonicalFunctionFactsProjection(projection)
}

func (d *Driver) canonicalFunctionFactProjection(prog *program, store api.CanonicalStore, artifact canonicalSolveArtifact) map[api.GraphKey]api.FunctionFacts {
	if d == nil || prog == nil || store == nil {
		return nil
	}
	out := make(map[api.GraphKey]api.FunctionFacts)
	for _, ref := range prog.refs {
		symbols := prog.symbolsForRef(ref)
		if len(symbols) == 0 {
			continue
		}
		sum, ok := artifact.Summaries[ref]
		if !ok {
			continue
		}
		returns := summary.ReturnTypes(sum)
		params := contractTypeVector(sum.Params, prog.NumParams(ref))
		publicParams := prog.publicPredicateParamVector(ref, params)
		sig := d.signatureForRef(prog, ref)
		refinement := sum.Postconditions.FunctionRefinement(prog.facts.HasNoReturn(ref))
		for _, sym := range symbols {
			key, ok := store.ParentGraphKeyForSymbol(sym)
			if !ok {
				continue
			}
			builder := functionfact.NewBuilder()
			builder.AddSignature(sym, sig)
			builder.AddSummary(sym, returns)
			builder.AddNarrow(sym, returns)
			builder.AddBodyParams(sym, params)
			builder.AddPublicParams(sym, publicParams)
			builder.AddRefinement(sym, refinement)
			if facts := builder.Build(); len(facts) > 0 {
				out[key] = mergeFunctionFacts(out[key], facts)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeFunctionFacts(out api.FunctionFacts, facts api.FunctionFacts) api.FunctionFacts {
	if len(facts) == 0 {
		return out
	}
	if out == nil {
		out = make(api.FunctionFacts, len(facts))
	}
	for sym, fact := range facts {
		out[sym] = fact
	}
	return out
}

func contractTypeVector(contracts paramevidence.Contracts, minLen int) []typ.Type {
	typesBySlot := paramevidence.ContractTypes(contracts)
	if len(typesBySlot) == 0 {
		return nil
	}
	n := minLen
	for slot := range typesBySlot {
		if slot >= 0 && slot+1 > n {
			n = slot + 1
		}
	}
	if n <= 0 {
		return nil
	}
	out := make([]typ.Type, n)
	any := false
	for slot, t := range typesBySlot {
		if slot < 0 || slot >= n || t == nil || typ.IsAbsentOrUnknown(t) {
			continue
		}
		out[slot] = t
		any = true
	}
	if !any {
		return nil
	}
	return out
}
