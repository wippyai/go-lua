package dispatch

import (
	"github.com/wippyai/go-lua/analysis/engine"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// ruleAuthorities is the sealed authority set needed by the generated
// dispatch family.  The declaration names Call as its output and Value/Heap
// as static relation inputs; the composition supplies those exact owner
// schemas and no dispatch callback or private rule payload crosses this seam.
type ruleAuthorities interface {
	CallAuthority() *callowner.HotOwner
	ValueSchema() *valuedomain.Schema
	HeapSchema() heapdomain.Schema
}

// InstallFamily claims Call dispatch's one generated family ordinal.  All
// execution geometry comes from program.Dispatch and the sealed plan row;
// this bind arm supplies only the immutable owner schemas the declared route
// derivation consumes.
func InstallFamily[A ruleAuthorities](binding *engine.SchemaBinding, slot *engine.GeneratedRuleSlot, authorities A) bool {
	if binding == nil || slot == nil || !slot.Available() {
		return false
	}
	calls := authorities.CallAuthority()
	values := authorities.ValueSchema()
	heaps := authorities.HeapSchema()
	if calls == nil || values == nil || !heaps.Valid() || !calls.MatchesBinding(binding) || !values.Valid() {
		return false
	}
	algebra := calls.Algebra()
	if algebra == nil || !algebra.Valid() {
		return false
	}
	installer, installerOK := NewFamilyInstaller(algebra, values, heaps)
	if !installerOK {
		return false
	}
	return engine.BindRuleFamily[calldomain.DenseCoordinate](binding, slot, calls.FactorRef(), installer)
}
