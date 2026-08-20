package composite

import (
	"github.com/wippyai/go-lua/analysis/schema/rule"
	callactivation "github.com/wippyai/go-lua/domain/call/activation"
	calldispatch "github.com/wippyai/go-lua/domain/call/dispatch"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	callsite "github.com/wippyai/go-lua/domain/effect/callsite"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	heapclosed "github.com/wippyai/go-lua/domain/heap/allocation/closed"
	heapempty "github.com/wippyai/go-lua/domain/heap/allocation/empty"
	heapingress "github.com/wippyai/go-lua/domain/heap/allocation/ingress"
	heapbootstrap "github.com/wippyai/go-lua/domain/heap/bootstrap"
	heapindex "github.com/wippyai/go-lua/domain/heap/index"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
	packsource "github.com/wippyai/go-lua/domain/pack/source"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueallocation "github.com/wippyai/go-lua/domain/value/allocation"
	valuearithmetic "github.com/wippyai/go-lua/domain/value/arithmetic"
	valuebootstrap "github.com/wippyai/go-lua/domain/value/bootstrap"
	valueequality "github.com/wippyai/go-lua/domain/value/equality"
	valueorder "github.com/wippyai/go-lua/domain/value/order"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
	valuerefinement "github.com/wippyai/go-lua/domain/value/refinement"
	valueruntimekind "github.com/wippyai/go-lua/domain/value/runtimekind"
	valuesource "github.com/wippyai/go-lua/domain/value/source"
	valuetransfer "github.com/wippyai/go-lua/domain/value/transfer"
)

type Principals interface {
	ValuePrincipal() *valueowner.SchemaFragment
	CallPrincipal() *callowner.SchemaFragment
	HeapPrincipal() *heapowner.SchemaFragment
	PackPrincipal() *packowner.SchemaFragment
	EffectPrincipal() *effectowner.SchemaFragment
}

type Authorities interface {
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
	HeapAuthority() *heapowner.HotOwner
	PackAuthority() *packowner.HotOwner
	EffectAuthority() *effectowner.HotOwner
	ValueSchema() *valuedomain.Schema
	HeapSchema() heapdomain.Schema
	PackSchema() *packdomain.Schema
	Topology() *heapindex.Topology
	Allocations() *allocationcatalog.Catalog
	ActivationCatalog() *callactivation.TargetBatchCatalog
}

func activationRule(hot *callactivation.HotRule) ActivationRule { return hot }

// RuleTemplates is the single schema composition registration for executable
// rules. It returns data-only catalog entries and the typed compose passes that
// bind each entry exactly once to its domain implementation.
func RuleTemplates[P Principals, A Authorities]() ([]*rule.Template, []RuleContributor[P, A], bool) {
	var admitted []*rule.Template
	var contributors []RuleContributor[P, A]
	rejected := false
	add := func(entry *rule.Template, contributor RuleContributor[P, A], ok bool) {
		if !ok || !contributor.complete(entry.Lane()) {
			rejected = true
			return
		}
		admitted = append(admitted, entry)
		contributors = append(contributors, contributor)
	}

	add(WireRule(valuesource.RuleEntry[P, A](), valuesource.DeclareRule[P], valuesource.RegisterRule, nil, valuesource.BindRule[A], nil, nil, valuesource.SealProgramRule, nil))
	add(WireRule(packsource.RuleEntry[P, A](), packsource.DeclareRule[P], packsource.RegisterRule, nil, packsource.BindRule[A], nil, nil, packsource.SealProgramRule, nil))
	add(WireRule(heapingress.RuleEntry[P, A](), heapingress.DeclareRule[P], heapingress.RegisterRule, nil, heapingress.BindRule[A], heapingress.FinalizeRule[A], nil, heapingress.SealProgramRule, nil))
	add(WireRule(valueallocation.RuleEntry[P, A](), valueallocation.DeclareRule[P], valueallocation.RegisterRule, nil, valueallocation.BindRule[A], nil, nil, valueallocation.SealProgramRule, nil))
	add(WireRule(heapempty.RuleEntry[P, A](), heapempty.DeclareRule[P], heapempty.RegisterRule, nil, heapempty.BindRule[A], nil, nil, heapempty.SealProgramRule, nil))
	add(WireRule(heapclosed.RuleEntry[P, A](), heapclosed.DeclareRule[P], heapclosed.RegisterRule, nil, heapclosed.BindRule[A], nil, nil, heapclosed.SealProgramRule, nil))
	add(WireRule(heapindex.RawGetEntry[P, A](), heapindex.DeclareRawGet[P], heapindex.RegisterRawGet, nil, heapindex.BindRawGet[A], nil, nil, heapindex.SealRawGetProgramRule, nil))
	add(WireRule(heapindex.RawSetEntry[P, A](), heapindex.DeclareRawSet[P], heapindex.RegisterRawSet, nil, heapindex.BindRawSet[A], nil, nil, heapindex.SealRawSetProgramRule, nil))
	add(WireRule(calldispatch.RuleEntry[P, A](), calldispatch.DeclareRule[P], calldispatch.RegisterRule, nil, calldispatch.BindRule[A], calldispatch.FinalizeRule[A], nil, calldispatch.SealProgramRule, nil))
	add(WireRule(callsite.SelectedEntry[P, A](), callsite.DeclareSelected[P], callsite.RegisterSelected, nil, callsite.BindSelected[A], callsite.FinalizeSelected[A], nil, callsite.SealProgramRule, nil))
	add(WireRule(callsite.OpaqueEntry[P, A](), callsite.DeclareOpaque[P], callsite.RegisterOpaque, nil, callsite.BindOpaque[A], callsite.FinalizeOpaque[A], nil, callsite.SealProgramRule, nil))
	add(WireRule(callsite.BodyEntry[P, A](), callsite.DeclareBody[P], callsite.RegisterBody, nil, callsite.BindBody[A], callsite.FinalizeBody[A], nil, callsite.SealBodyProgramRule, nil))
	add(WireRule(callactivation.RuleEntry[P, A](), callactivation.DeclareRule[P], callactivation.RegisterRule, nil, callactivation.BindRule[A], nil, nil, nil, activationRule))
	add(WireRule(valueruntimekind.RuleEntry[P, A](), valueruntimekind.DeclareRule[P], valueruntimekind.RegisterRule, nil, valueruntimekind.BindRule[A], nil, nil, valueruntimekind.SealProgramRule, nil))
	add(WireRule(valuebootstrap.RuleEntry[P, A](), valuebootstrap.DeclareRule[P], valuebootstrap.RegisterRule, nil, valuebootstrap.BindRule[A], valuebootstrap.FinalizeRule[A], valuebootstrap.LinkCatalog, valuebootstrap.SealProgramRule, nil))
	add(WireRule(heapbootstrap.RuleEntry[P, A](), heapbootstrap.DeclareRule[P], heapbootstrap.RegisterRule, heapbootstrap.PairRule, heapbootstrap.BindRule[A], heapbootstrap.FinalizeRule[A], heapbootstrap.LinkCatalog, heapbootstrap.SealProgramRule, nil))
	add(WireRule(valuetransfer.RuleEntry[P, A](), valuetransfer.DeclareRule[P], valuetransfer.RegisterRule, nil, valuetransfer.BindRule[A], nil, nil, valuetransfer.SealProgramRule, nil))
	add(WireRule(valuearithmetic.RuleEntry[P, A](), valuearithmetic.DeclareRule[P], valuearithmetic.RegisterRule, nil, valuearithmetic.BindRule[A], nil, nil, valuearithmetic.SealProgramRule, nil))
	add(WireRule(valueequality.RuleEntry[P, A](), valueequality.DeclareRule[P], valueequality.RegisterRule, nil, valueequality.BindRule[A], nil, nil, valueequality.SealProgramRule, nil))
	add(WireRule(valueorder.RuleEntry[P, A](), valueorder.DeclareRule[P], valueorder.RegisterRule, nil, valueorder.BindRule[A], nil, nil, valueorder.SealProgramRule, nil))
	add(WireRule(valuerefinement.RuleEntry[P, A](), valuerefinement.DeclareRule[P], valuerefinement.RegisterRule, nil, valuerefinement.BindRule[A], nil, nil, valuerefinement.SealProgramRule, nil))

	return admitted, contributors, !rejected && len(admitted) > 0
}
