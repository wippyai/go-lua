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
// the three sources visible at function entry in a single product-state write:
// declared annotation, exact caller-provided entry value, and body-demand
// contract. Declared structural annotations are contracts, not precision erasers;
// exact entry evidence may refine their dynamic/soft interior slots.
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
		} else if !entry.IsZero() {
			av = entrySeedContractRefinement(av, contract)
		} else {
			av = entrySeedContractValue(av, contract)
		}
	}
	return av
}

func entrySeedContractRefinement(entry, contract product.AbstractValue) product.AbstractValue {
	entryType := entry.ProjectValue()
	contractType := contract.ProjectValue()
	if entryType == nil || contractType == nil {
		return entry
	}
	if refined, changed := value.RefineStructuralAnnotation(entryType, contractType, typ.JoinPreferNonSoft); changed {
		return product.FromType(refined)
	}
	return entry
}

func entrySeedContractValue(entry, contract product.AbstractValue) product.AbstractValue {
	entryType := entry.ProjectValue()
	contractType := contract.ProjectValue()
	if entryType == nil || contractType == nil {
		return product.Domain.Join(entry, contract)
	}
	merged := paramevidence.BodyEntryContractJoin(entryType, contractType)
	if merged == nil {
		return product.Domain.Join(entry, contract)
	}
	return product.FromType(merged)
}

func entrySeedDeclaredValue(declared typ.Type, entry product.AbstractValue) product.AbstractValue {
	if declared == nil {
		return product.AbstractValue{}
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
