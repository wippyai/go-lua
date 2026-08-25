package activation

import (
	"github.com/wippyai/go-lua/analysis/engine"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
)

// ruleAuthorities is the sealed authority set the generated activation family
// needs. The declaration names Call as the axis its rows are indexed by and as
// the one static schema its structural judgment rests on, so the composition
// supplies that owner and nothing else: no activation callback, no route
// catalog and no private rule payload crosses this seam.
type ruleAuthorities interface {
	CallAuthority() *callowner.HotOwner
}

// InstallFamily claims Call activation's one generated family ordinal.
//
// All execution geometry comes from program.Activation and the sealed plan
// row. What this arm supplies is the one immutable owner schema the declared
// judgment is issued by - the algebra whose body table the branch ordinals are
// positions in.
//
// A structural rule names no output Factor, so the family claim is fenced by
// the axis its rows are INDEXED by rather than by a written semantic. That is
// the same seam every fact-writing family installs through; one seam, two
// declared geometries.
func InstallFamily[A ruleAuthorities](binding *engine.SchemaBinding, slot *engine.GeneratedRuleSlot, authorities A) bool {
	if binding == nil || slot == nil || !slot.Available() {
		return false
	}
	calls := authorities.CallAuthority()
	if calls == nil || !calls.MatchesBinding(binding) {
		return false
	}
	algebra := calls.Algebra()
	if algebra == nil || !algebra.Valid() {
		return false
	}
	installer, installerOK := NewFamilyInstaller(algebra)
	if !installerOK {
		return false
	}
	return engine.BindRuleFamily[calldomain.DenseCoordinate](binding, slot, calls.FactorRef(), installer)
}
