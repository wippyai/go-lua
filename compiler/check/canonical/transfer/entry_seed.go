package transfer

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/typ"
)

// EntryReachabilityEffect lifts bottom-only axes to their reachable identity at a
// function entry point. The graph entry has no predecessor, so the solver's bottom
// seed must become the empty numeric environment and no-op cell-effect summary
// before ordinary parameter entry seeds run.
type EntryReachabilityEffect struct{}

func (t *Transfer) applyEntryReachabilityEffect(out *flow.PointState, _ EntryReachabilityEffect) bool {
	if out == nil {
		return false
	}
	changed := false
	if out.Num == nil || out.Num.IsUnsat() {
		out.Num = numeric.NewState()
		changed = true
	}
	if constraint.Domain.Equal(out.Cond, constraint.Domain.Bottom()) {
		out.Cond = constraint.Domain.Top()
		changed = true
	}
	if out.Rel.IsBottom() {
		out.Rel = flow.PointRelationsDomain.Top()
		changed = true
	}
	if out.ReturnRel.IsBottom() {
		out.ReturnRel = flow.ReturnRelationsDomain.Top()
		changed = true
	}
	if out.CellEffects.IsBottom() {
		out.CellEffects = flow.CaptureEffectsIdentity()
		changed = true
	}
	if out.ReceiverEffects.IsBottom() {
		out.ReceiverEffects = flow.ReceiverEffectsIdentity()
		changed = true
	}
	if out.StaticMembers.IsBottom() {
		out.StaticMembers = flow.StaticMemberFactsDomain.Top()
		changed = true
	}
	if out.KeyPresence.IsBottom() {
		out.KeyPresence = flow.KeyPresenceFactsDomain.Top()
		changed = true
	}
	if out.ValueOrigins.IsBottom() {
		out.ValueOrigins = flow.ValueOriginFactsDomain.Top()
		changed = true
	}
	return changed
}

// EntrySeedEffect is the entry-point reducer for one parameter slot. It composes
// the sources visible at function entry in a single product-state write:
// declared annotation, exact caller-provided entry value, and, only for
// unannotated parameters, body-demand contract. Declared annotations are source
// authority; body demand is an obligation, not proof about a declared dynamic
// slot. Exact entry evidence may still refine dynamic/soft annotation interiors.
type EntrySeedEffect struct {
	Symbol   cfg.SymbolID
	Declared typ.Type
	Entry    product.AbstractValue
	Contract product.AbstractValue
}

func (t *Transfer) applyEntrySeedEffect(out *flow.PointState, effect EntrySeedEffect) bool {
	if out == nil || effect.Symbol == 0 {
		return false
	}
	av := entrySeedValue(effect.Declared, effect.Entry, effect.Contract)
	if av.IsZero() {
		return false
	}
	t.setSymbolValue(out, effect.Symbol, av, false)
	return true
}

func entrySeedValue(declared typ.Type, entry, contract product.AbstractValue) product.AbstractValue {
	av := entrySeedDeclaredValue(declared, entry)
	if av.IsZero() && !entry.IsZero() {
		av = entry
	}
	if !contract.IsZero() {
		if !paramevidence.IsInformative(contract.ProjectValue()) {
			return av
		}
		if av.IsZero() {
			av = contract
		}
		// With a declared annotation or exact caller entry value, keep `av` as the
		// declared/entry composition. Refining it by the demand creates a
		// self-fulfilling precondition: the parameter becomes what the body demands,
		// guard complements vanish, and declared `any` is mistaken for concrete proof.
	}
	return av
}

func entrySeedDeclaredValue(declared typ.Type, entry product.AbstractValue) product.AbstractValue {
	if declared == nil {
		return product.AbstractValue{}
	}
	if typ.ContainsTypeParam(declared) && entryHasClosedInformativeValue(entry) {
		// An open generic annotation (`T`, `{T}`, ...) is a binder constraint, not
		// a closed runtime fact. Once the call-entry context supplies a closed value,
		// entry state must carry that value so the body is interpreted under the
		// instantiated call, not under the callee's binder syntax.
		return entry
	}
	if !entry.IsZero() {
		if evidence := entry.ProjectValue(); evidence != nil && !typ.IsAbsentOrUnknown(evidence) {
			if refined, changed := value.RefineStructuralAnnotation(declared, evidence, typ.JoinPreferNonSoft); changed {
				return product.FromType(refined)
			}
		}
	}
	return product.FromType(declared)
}

func entryHasClosedInformativeValue(entry product.AbstractValue) bool {
	if entry.IsZero() {
		return false
	}
	t := entry.ProjectValue()
	return t != nil && !typ.IsAbsentOrUnknown(t) && !typ.ContainsTypeParam(t)
}
