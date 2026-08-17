package composite

import (
	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callactivation "github.com/wippyai/go-lua/analysis/domain/call/activation"
	calldispatch "github.com/wippyai/go-lua/analysis/domain/call/dispatch"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	callsite "github.com/wippyai/go-lua/analysis/domain/effect/callsite"
	effectfactor "github.com/wippyai/go-lua/analysis/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/analysis/domain/effect/owner"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/analysis/domain/heap/allocation/catalog"
	heapclosed "github.com/wippyai/go-lua/analysis/domain/heap/allocation/closed"
	heapempty "github.com/wippyai/go-lua/analysis/domain/heap/allocation/empty"
	heapingress "github.com/wippyai/go-lua/analysis/domain/heap/allocation/ingress"
	heapbootstrap "github.com/wippyai/go-lua/analysis/domain/heap/bootstrap"
	heapindex "github.com/wippyai/go-lua/analysis/domain/heap/index"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	packsource "github.com/wippyai/go-lua/analysis/domain/pack/source"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueallocation "github.com/wippyai/go-lua/analysis/domain/value/allocation"
	valuearithmetic "github.com/wippyai/go-lua/analysis/domain/value/arithmetic"
	valuebootstrap "github.com/wippyai/go-lua/analysis/domain/value/bootstrap"
	valueequality "github.com/wippyai/go-lua/analysis/domain/value/equality"
	valueorder "github.com/wippyai/go-lua/analysis/domain/value/order"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	valuerefinement "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	valuesource "github.com/wippyai/go-lua/analysis/domain/value/source"
	valuetransfer "github.com/wippyai/go-lua/analysis/domain/value/transfer"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/rule"
)

// principals is the composition's cold factor principal record. It is the
// declaration surface's P parameter, so a rule's Declare hook receives its
// owners already typed and never asserts.
type principals struct {
	value  *valueowner.SchemaFragment
	call   *callowner.SchemaFragment
	heap   *heapowner.SchemaFragment
	pack   *packowner.SchemaFragment
	effect *effectowner.SchemaFragment
}

func (set principals) available() bool {
	return set.value != nil && set.call != nil && set.heap != nil && set.pack != nil && set.effect != nil
}

// The principal getters are the record's read surface. An owning domain names
// exactly the ones its own Declare hook consumes in its own need interface, so
// a rule reaches its cold owners without this record's shape reaching the
// domain.
func (set principals) ValuePrincipal() *valueowner.SchemaFragment { return set.value }

func (set principals) CallPrincipal() *callowner.SchemaFragment { return set.call }

func (set principals) HeapPrincipal() *heapowner.SchemaFragment { return set.heap }

func (set principals) PackPrincipal() *packowner.SchemaFragment { return set.pack }

func (set principals) EffectPrincipal() *effectowner.SchemaFragment { return set.effect }

// has reports whether the factor lane a rule writes has a declared owner.
func (set principals) has(kind programartifact.RuleOutputKind) bool {
	switch kind {
	case programartifact.RuleOutputValue:
		return set.value != nil
	case programartifact.RuleOutputCall:
		return set.call != nil
	case programartifact.RuleOutputHeap:
		return set.heap != nil
	case programartifact.RuleOutputPack:
		return set.pack != nil
	case programartifact.RuleOutputEffect:
		return set.effect != nil
	default:
		return false
	}
}

// authorities is the composition's Link authority record. It is the surface's
// A parameter and carries every already-sealed authority a hot rule binds
// against; no runtime policy or live capability enters through it.
type authorities struct {
	value  *valueowner.HotOwner
	call   *callowner.HotOwner
	heap   *heapowner.HotOwner
	pack   *packowner.HotOwner
	effect *effectowner.HotOwner

	valueSchema *valuedomain.Schema
	heapSchema  heapdomain.Schema
	packSchema  *packdomain.Schema
	topology    *heapindex.Topology
	allocations *allocationcatalog.Catalog
	activation  *callactivation.TargetBatchCatalog
}

func (set authorities) available() bool {
	return set.value != nil && set.call != nil && set.heap != nil && set.pack != nil && set.effect != nil &&
		set.valueSchema != nil && set.heapSchema.Valid() && set.packSchema != nil &&
		set.topology != nil && set.allocations != nil && set.activation != nil
}

func (set authorities) has(kind programartifact.RuleOutputKind) bool {
	switch kind {
	case programartifact.RuleOutputValue:
		return set.value != nil
	case programartifact.RuleOutputCall:
		return set.call != nil
	case programartifact.RuleOutputHeap:
		return set.heap != nil
	case programartifact.RuleOutputPack:
		return set.pack != nil
	case programartifact.RuleOutputEffect:
		return set.effect != nil
	default:
		return false
	}
}

// The authority getters are the record's read surface. An owning domain names
// exactly the ones its own Bind and Finalize hooks consume in its own need
// interface, so a rule reaches its sealed authorities without this record's
// shape reaching the domain.
func (set authorities) ValueAuthority() *valueowner.HotOwner { return set.value }

func (set authorities) CallAuthority() *callowner.HotOwner { return set.call }

func (set authorities) HeapAuthority() *heapowner.HotOwner { return set.heap }

func (set authorities) PackAuthority() *packowner.HotOwner { return set.pack }

func (set authorities) EffectAuthority() *effectowner.HotOwner { return set.effect }

func (set authorities) ValueSchema() *valuedomain.Schema { return set.valueSchema }

func (set authorities) HeapSchema() heapdomain.Schema { return set.heapSchema }

func (set authorities) PackSchema() *packdomain.Schema { return set.packSchema }

func (set authorities) Topology() *heapindex.Topology { return set.topology }

func (set authorities) Allocations() *allocationcatalog.Catalog { return set.allocations }

func (set authorities) ActivationCatalog() *callactivation.TargetBatchCatalog { return set.activation }

type (
	declaration = rule.Declaration[principals]
	template    = rule.Template[principals, authorities]
)

// ruleTemplates is the authored analyzer rule inventory. Each row instantiates
// one owning domain's own declaration with this composition's principal and
// authority records, which that domain admits through its own need interface.
//
// The declaration itself lives with the domain that owns the rule, so the
// inventory here states membership and order alone: it never restates a
// domain's hooks, and the surface record stays blind to every domain.
func ruleTemplates() ([]*template, bool) {
	var admitted []*template
	rejected := false
	add := func(entry *template, ok bool) {
		if !ok {
			rejected = true
			return
		}
		admitted = append(admitted, entry)
	}

	add(rule.New(valuesource.RuleEntry[principals, authorities]()))
	add(rule.New(packsource.RuleEntry[principals, authorities]()))
	add(rule.New(heapingress.RuleEntry[principals, authorities]()))
	add(rule.New(valueallocation.RuleEntry[principals, authorities]()))
	add(rule.New(heapempty.RuleEntry[principals, authorities]()))
	add(rule.New(heapclosed.RuleEntry[principals, authorities]()))
	add(rule.New(heapindex.RawGetEntry[principals, authorities]()))
	add(rule.New(heapindex.RawSetEntry[principals, authorities]()))
	add(rule.New(calldispatch.RuleEntry[principals, authorities]()))
	add(rule.New(callsite.SelectedEntry[principals, authorities]()))
	add(rule.New(callsite.OpaqueEntry[principals, authorities]()))
	add(rule.New(callsite.BodyEntry[principals, authorities]()))
	add(rule.New(callactivation.RuleEntry[principals, authorities]()))
	add(rule.New(valuebootstrap.RuleEntry[principals, authorities]()))
	add(rule.New(heapbootstrap.RuleEntry[principals, authorities]()))
	add(rule.New(valuetransfer.RuleEntry[principals, authorities]()))
	add(rule.New(valuearithmetic.RuleEntry[principals, authorities]()))
	add(rule.New(valueequality.RuleEntry[principals, authorities]()))
	add(rule.New(valueorder.RuleEntry[principals, authorities]()))
	add(rule.New(valuerefinement.RuleEntry[principals, authorities]()))

	return admitted, !rejected && len(admitted) > 0
}

// LinkInputs is the neutral set of already-sealed Link authorities one binding
// transaction consumes. Every field is an input the Link already owns; no
// runtime policy or live capability enters here, and no principal or catalog
// is handed back out.
type LinkInputs struct {
	ValueSchema       *valuedomain.Schema
	CallAlgebra       *calldomain.Algebra
	HeapSchema        heapdomain.Schema
	HeapMounts        []heapdomain.ArtifactMount
	PackSchema        *packdomain.Schema
	EffectAlgebra     *effectfactor.Algebra
	Topology          *heapindex.Topology
	ActivationCatalog *callactivation.TargetBatchCatalog
}

func (inputs LinkInputs) available() bool {
	return inputs.ValueSchema != nil && inputs.CallAlgebra != nil && inputs.CallAlgebra.Valid() &&
		inputs.HeapSchema.Valid() && len(inputs.HeapMounts) > 0 && inputs.PackSchema != nil &&
		inputs.EffectAlgebra != nil && inputs.EffectAlgebra.Valid() && inputs.Topology != nil &&
		inputs.ActivationCatalog != nil
}

// The axis input getters are the record's read surface for the axis pass. An
// axis's owning domain names exactly the one it binds against in its own need
// interface, so this record's shape never reaches the domain.
func (inputs LinkInputs) ValueInput() *valuedomain.Schema { return inputs.ValueSchema }

func (inputs LinkInputs) CallInput() *calldomain.Algebra { return inputs.CallAlgebra }

func (inputs LinkInputs) HeapInput() heapdomain.Schema { return inputs.HeapSchema }

func (inputs LinkInputs) PackInput() *packdomain.Schema { return inputs.PackSchema }

func (inputs LinkInputs) EffectInput() *effectfactor.Algebra { return inputs.EffectAlgebra }
