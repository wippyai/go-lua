package grammar

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
	"github.com/wippyai/go-lua/analysis/engine"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
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

// ruleTemplates is the authored analyzer rule inventory. Each row is one
// domain's contribution to the rule surface, instantiated with that domain's
// own fragment and hot rule types and handed to the table as a plain value.
//
// The domain imports above are this cut's registration mechanism: a rule is
// declared where the table is composed. The surface record itself is blind to
// every domain, so the same rows move into generated per-domain registration
// without changing the interface, and these imports leave with them.
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

	add(rule.New(rule.Spec[principals, authorities, *valuesource.SchemaFragment, *valuesource.HotRule]{
		Key:      "value-source",
		Role:     programartifact.RuleRoleValueSource,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.ValueSourceRule.Rule },
		Declare: func(context declaration) (*valuesource.SchemaFragment, bool) {
			semantics := context.Bundle.ValueSourceRule
			return valuesource.DeclareSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.value)
		},
		Register: func(context rule.Registration[*valuesource.SchemaFragment]) (engine.RuleSlotCapability, bool) {
			return registerMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[authorities, *valuesource.SchemaFragment]) (*valuesource.HotRule, bool) {
			return valuesource.BindHot(context.Fragment, context.Authorities.value)
		},
		Attach: func(context rule.Attach[*valuesource.HotRule]) bool {
			_, ok := context.Rule.AttachMountedRule(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*valuesource.HotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}))

	add(rule.New(rule.Spec[principals, authorities, *packsource.SchemaFragment, *packsource.HotRule]{
		Key:      "pack-source",
		Role:     programartifact.RuleRolePackSource,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.PackSourceRule.Rule },
		Declare: func(context declaration) (*packsource.SchemaFragment, bool) {
			semantics := context.Bundle.PackSourceRule
			return packsource.DeclareSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.pack)
		},
		Register: func(context rule.Registration[*packsource.SchemaFragment]) (engine.RuleSlotCapability, bool) {
			return registerMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[authorities, *packsource.SchemaFragment]) (*packsource.HotRule, bool) {
			return packsource.BindHot(context.Fragment, context.Authorities.pack, context.Authorities.packSchema)
		},
		Finalize: func(context rule.Finalization[authorities, *packsource.HotRule]) bool {
			return context.Rule.SealOccurrenceReceipts()
		},
		Attach: func(context rule.Attach[*packsource.HotRule]) bool {
			_, ok := context.Rule.AttachMountedOccurrence(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*packsource.HotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}))

	add(rule.New(rule.Spec[principals, authorities, *heapingress.SchemaFragment, *heapingress.HotRule]{
		Key:      "heap-ingress",
		Role:     programartifact.RuleRoleHeapIngress,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.HeapIngressRule.Rule },
		Declare: func(context declaration) (*heapingress.SchemaFragment, bool) {
			semantics := context.Bundle.HeapIngressRule
			return heapingress.DeclareSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.heap)
		},
		Register: func(context rule.Registration[*heapingress.SchemaFragment]) (engine.RuleSlotCapability, bool) {
			return registerMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[authorities, *heapingress.SchemaFragment]) (*heapingress.HotRule, bool) {
			return heapingress.BindHot(context.Fragment, context.Authorities.heap)
		},
		Finalize: func(context rule.Finalization[authorities, *heapingress.HotRule]) bool {
			return context.Rule.AttachCatalog(context.Authorities.allocations)
		},
		Attach: func(context rule.Attach[*heapingress.HotRule]) bool {
			_, ok := context.Rule.AttachMountedOccurrence(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*heapingress.HotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}))

	add(rule.New(rule.Spec[principals, authorities, *valueallocation.SchemaFragment, *valueallocation.HotRule]{
		Key:      "value-allocation",
		Role:     programartifact.RuleRoleValueAllocation,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.ValueAllocationRule.Rule },
		Declare: func(context declaration) (*valueallocation.SchemaFragment, bool) {
			semantics := context.Bundle.ValueAllocationRule
			return valueallocation.DeclareSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Transform, semantics.Evidence, context.Principals.value)
		},
		Register: func(context rule.Registration[*valueallocation.SchemaFragment]) (engine.RuleSlotCapability, bool) {
			return registerMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[authorities, *valueallocation.SchemaFragment]) (*valueallocation.HotRule, bool) {
			return valueallocation.BindHot(context.Fragment, context.Authorities.value, context.Authorities.heapSchema, context.Authorities.allocations)
		},
		Attach: func(context rule.Attach[*valueallocation.HotRule]) bool {
			_, ok := context.Rule.AttachMountedRule(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*valueallocation.HotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}))

	add(rule.New(rule.Spec[principals, authorities, *heapempty.SchemaFragment, *heapempty.HotRule]{
		Key:      "heap-empty",
		Role:     programartifact.RuleRoleHeapEmpty,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.HeapEmptyRule.Rule },
		Declare: func(context declaration) (*heapempty.SchemaFragment, bool) {
			semantics := context.Bundle.HeapEmptyRule
			return heapempty.DeclareSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Transform, semantics.Evidence, context.Principals.heap)
		},
		Register: func(context rule.Registration[*heapempty.SchemaFragment]) (engine.RuleSlotCapability, bool) {
			return registerMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[authorities, *heapempty.SchemaFragment]) (*heapempty.HotRule, bool) {
			return heapempty.BindHot(context.Fragment, context.Authorities.heap, context.Authorities.allocations)
		},
		Attach: func(context rule.Attach[*heapempty.HotRule]) bool {
			_, ok := context.Rule.AttachMountedOccurrence(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*heapempty.HotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}))

	add(rule.New(rule.Spec[principals, authorities, *heapclosed.SchemaFragment, *heapclosed.HotRule]{
		Key:      "heap-closed",
		Role:     programartifact.RuleRoleHeapClosed,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.HeapClosedRule.Rule },
		Declare: func(context declaration) (*heapclosed.SchemaFragment, bool) {
			semantics := context.Bundle.HeapClosedRule
			return heapclosed.DeclareSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Transform, semantics.Evidence, context.Principals.heap, context.Principals.value)
		},
		Register: func(context rule.Registration[*heapclosed.SchemaFragment]) (engine.RuleSlotCapability, bool) {
			return registerMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[authorities, *heapclosed.SchemaFragment]) (*heapclosed.HotRule, bool) {
			return heapclosed.BindHot(context.Binding, context.Fragment, context.Authorities.heap, context.Authorities.value, context.Authorities.allocations)
		},
		Attach: func(context rule.Attach[*heapclosed.HotRule]) bool {
			_, ok := context.Rule.AttachMountedOccurrence(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*heapclosed.HotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}))

	add(rule.New(rule.Spec[principals, authorities, *heapindex.RawGetSchemaFragment, *heapindex.RawGetHotRule]{
		Key:      "raw-get",
		Role:     programartifact.RuleRoleRawGet,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.RawGetRule.Rule },
		Declare: func(context declaration) (*heapindex.RawGetSchemaFragment, bool) {
			semantics := context.Bundle.RawGetRule
			return heapindex.DeclareRawGetSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.value, context.Principals.call, context.Principals.heap, context.Principals.pack)
		},
		Register: func(context rule.Registration[*heapindex.RawGetSchemaFragment]) (engine.RuleSlotCapability, bool) {
			return registerMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[authorities, *heapindex.RawGetSchemaFragment]) (*heapindex.RawGetHotRule, bool) {
			return heapindex.BindRawGetHot(context.Binding, context.Fragment, context.Authorities.topology, context.Authorities.value, context.Authorities.call, context.Authorities.heap, context.Authorities.pack)
		},
		Attach: func(context rule.Attach[*heapindex.RawGetHotRule]) bool {
			_, ok := context.Rule.AttachMountedOccurrence(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*heapindex.RawGetHotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}))

	add(rule.New(rule.Spec[principals, authorities, *heapindex.RawSetSchemaFragment, *heapindex.RawSetHotRule]{
		Key:      "raw-set",
		Role:     programartifact.RuleRoleRawSet,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.RawSetRule.Rule },
		Declare: func(context declaration) (*heapindex.RawSetSchemaFragment, bool) {
			semantics := context.Bundle.RawSetRule
			return heapindex.DeclareRawSetSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.value, context.Principals.heap, context.Principals.pack)
		},
		Register: func(context rule.Registration[*heapindex.RawSetSchemaFragment]) (engine.RuleSlotCapability, bool) {
			return registerMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[authorities, *heapindex.RawSetSchemaFragment]) (*heapindex.RawSetHotRule, bool) {
			return heapindex.BindRawSetHot(context.Binding, context.Fragment, context.Authorities.topology, context.Authorities.value, context.Authorities.heap, context.Authorities.pack)
		},
		Attach: func(context rule.Attach[*heapindex.RawSetHotRule]) bool {
			_, ok := context.Rule.AttachMountedOccurrence(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*heapindex.RawSetHotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}))

	add(rule.New(rule.Spec[principals, authorities, *calldispatch.SchemaFragment, *calldispatch.HotRule]{
		Key:      "call-dispatch",
		Role:     programartifact.RuleRoleCallDispatch,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.CallDispatchRule.Rule },
		Declare: func(context declaration) (*calldispatch.SchemaFragment, bool) {
			semantics := context.Bundle.CallDispatchRule
			return calldispatch.DeclareSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.value, context.Principals.call)
		},
		Register: func(context rule.Registration[*calldispatch.SchemaFragment]) (engine.RuleSlotCapability, bool) {
			return registerMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[authorities, *calldispatch.SchemaFragment]) (*calldispatch.HotRule, bool) {
			return calldispatch.BindHot(context.Binding, context.Fragment, context.Authorities.value, context.Authorities.call, context.Authorities.heapSchema, context.Authorities.packSchema)
		},
		Finalize: func(context rule.Finalization[authorities, *calldispatch.HotRule]) bool {
			return context.Rule.SealOccurrenceReceipts()
		},
		Attach: func(context rule.Attach[*calldispatch.HotRule]) bool {
			_, ok := context.Rule.AttachMountedOccurrence(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*calldispatch.HotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}))

	add(rule.New(rule.Spec[principals, authorities, *callsite.SelectedSchemaFragment, *callsite.HotRule]{
		Key:      "effect-selected",
		Role:     programartifact.RuleRoleEffectSelected,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.EffectSelectedRule.Rule },
		Declare: func(context declaration) (*callsite.SelectedSchemaFragment, bool) {
			semantics := context.Bundle.EffectSelectedRule
			return callsite.DeclareSelectedSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.call, context.Principals.effect)
		},
		Register: func(context rule.Registration[*callsite.SelectedSchemaFragment]) (engine.RuleSlotCapability, bool) {
			return registerMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[authorities, *callsite.SelectedSchemaFragment]) (*callsite.HotRule, bool) {
			return callsite.BindSelectedHot(context.Binding, context.Fragment, context.Authorities.call, context.Authorities.effect)
		},
		Finalize: func(context rule.Finalization[authorities, *callsite.HotRule]) bool {
			return context.Rule.SealOccurrenceReceipts()
		},
		Attach: func(context rule.Attach[*callsite.HotRule]) bool {
			_, ok := context.Rule.AttachMountedOccurrence(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*callsite.HotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}))

	add(rule.New(rule.Spec[principals, authorities, *callsite.OpaqueSchemaFragment, *callsite.HotRule]{
		Key:      "effect-opaque",
		Role:     programartifact.RuleRoleEffectOpaque,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.EffectOpaqueRule.Rule },
		Declare: func(context declaration) (*callsite.OpaqueSchemaFragment, bool) {
			semantics := context.Bundle.EffectOpaqueRule
			return callsite.DeclareOpaqueSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.call, context.Principals.effect)
		},
		Register: func(context rule.Registration[*callsite.OpaqueSchemaFragment]) (engine.RuleSlotCapability, bool) {
			return registerMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[authorities, *callsite.OpaqueSchemaFragment]) (*callsite.HotRule, bool) {
			return callsite.BindOpaqueHot(context.Binding, context.Fragment, context.Authorities.call, context.Authorities.effect)
		},
		Finalize: func(context rule.Finalization[authorities, *callsite.HotRule]) bool {
			return context.Rule.SealOccurrenceReceipts()
		},
		Attach: func(context rule.Attach[*callsite.HotRule]) bool {
			_, ok := context.Rule.AttachMountedOccurrence(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*callsite.HotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}))

	add(rule.New(rule.Spec[principals, authorities, *callsite.BodySchemaFragment, *callsite.BodyHotRule]{
		Key:      "effect-body",
		Role:     programartifact.RuleRoleEffectBody,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.EffectBodyRule.Rule },
		Declare: func(context declaration) (*callsite.BodySchemaFragment, bool) {
			semantics := context.Bundle.EffectBodyRule
			return callsite.DeclareBodySchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.call, context.Principals.effect)
		},
		Register: func(context rule.Registration[*callsite.BodySchemaFragment]) (engine.RuleSlotCapability, bool) {
			return registerMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[authorities, *callsite.BodySchemaFragment]) (*callsite.BodyHotRule, bool) {
			return callsite.BindBodyHot(context.Binding, context.Fragment, context.Authorities.call, context.Authorities.effect)
		},
		Finalize: func(context rule.Finalization[authorities, *callsite.BodyHotRule]) bool {
			return context.Rule.SealOccurrenceReceipts()
		},
		Attach: func(context rule.Attach[*callsite.BodyHotRule]) bool {
			_, ok := context.Rule.AttachMountedOccurrence(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*callsite.BodyHotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}))

	add(rule.New(rule.Spec[principals, authorities, *callactivation.SchemaFragment, *callactivation.HotRule]{
		Key:      "call-activation",
		Role:     programartifact.RuleRoleCallActivation,
		Lane:     rule.LaneActivation,
		Semantic: func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.CallActivation },
		Declare: func(context declaration) (*callactivation.SchemaFragment, bool) {
			return callactivation.DeclareSchema(context.Builder, context.Bundle.CallActivation, context.Bundle.CallActivationFamily, context.Bundle.CallActivationAdmission, context.Principals.call)
		},
		Register: func(context rule.Registration[*callactivation.SchemaFragment]) (engine.RuleSlotCapability, bool) {
			slot := context.Fragment.ActivationSlot()
			capability, ok := engine.IssueActivationRuleCapability(context.Binding, slot)
			if !ok || !engine.RegisterActivationRuleSlot(context.Binding, slot, capability) {
				return engine.RuleSlotCapability{}, false
			}
			return capability, true
		},
		Bind: func(context rule.Binding[authorities, *callactivation.SchemaFragment]) (*callactivation.HotRule, bool) {
			hot, ok := callactivation.BindHot(context.Fragment, context.Authorities.call, context.Authorities.activation)
			if !ok {
				return nil, false
			}
			if !callactivation.BindMountedTransport(hot, context.Authorities.value.FactorRef(), context.Authorities.call.FactorRef(), context.Authorities.heap.FactorRef(), context.Authorities.pack.FactorRef(), context.Authorities.effect.FactorRef()) {
				return nil, false
			}
			return hot, true
		},
		Finalize: func(context rule.Finalization[authorities, *callactivation.HotRule]) bool {
			return context.Rule.SealOccurrenceReceipts()
		},
		Attach: func(context rule.Attach[*callactivation.HotRule]) bool {
			return context.Rule.AttachMountedOccurrence(context.Assembly, context.Mount, context.Point, context.Occurrence)
		},
		Member: func(context rule.Member[*callactivation.HotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}))

	add(rule.New(rule.Spec[principals, authorities, *valuebootstrap.SchemaFragment, *valuebootstrap.HotRule]{
		Key:      "value-bootstrap",
		Role:     programartifact.RuleRoleValueBootstrap,
		Lane:     rule.LaneLink,
		Semantic: func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.ValueBootstrapRule.Rule },
		Declare: func(context declaration) (*valuebootstrap.SchemaFragment, bool) {
			semantics := context.Bundle.ValueBootstrapRule
			return valuebootstrap.DeclareSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.value)
		},
		Register: func(context rule.Registration[*valuebootstrap.SchemaFragment]) (engine.RuleSlotCapability, bool) {
			return registerLinkSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[authorities, *valuebootstrap.SchemaFragment]) (*valuebootstrap.HotRule, bool) {
			return valuebootstrap.BindHot(context.Fragment, context.Authorities.value)
		},
		Finalize: func(context rule.Finalization[authorities, *valuebootstrap.HotRule]) bool {
			catalog := context.Rule.Catalog()
			return catalog != nil && catalog.FencedTo(context.Authorities.valueSchema)
		},
		LinkAttach: func(context rule.LinkAttach[*valuebootstrap.HotRule]) bool {
			_, ok := context.Rule.AttachLinkOccurrence(context.Assembly, context.Occurrence)
			return ok
		},
		LinkMember: func(context rule.LinkMember[*valuebootstrap.HotRule]) bool {
			_, ok := context.Rule.AttachLinkReceiptMember(context.Compilation, context.Graph, context.Occurrence)
			return ok
		},
		LinkCatalog: func(hot *valuebootstrap.HotRule) (rule.LinkCatalog, bool) {
			catalog := hot.Catalog()
			return catalog, catalog != nil
		},
	}))

	add(rule.New(rule.Spec[principals, authorities, *heapbootstrap.SchemaFragment, *heapbootstrap.HotRule]{
		Key:      "heap-bootstrap",
		Role:     programartifact.RuleRoleHeapBootstrap,
		Lane:     rule.LaneLink,
		Semantic: func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.HeapBootstrapRule.Rule },
		Declare: func(context declaration) (*heapbootstrap.SchemaFragment, bool) {
			semantics := context.Bundle.HeapBootstrapRule
			return heapbootstrap.DeclareSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.heap)
		},
		Register: func(context rule.Registration[*heapbootstrap.SchemaFragment]) (engine.RuleSlotCapability, bool) {
			return registerLinkSlot(context.Binding, context.Fragment.RuleSlot())
		},
		// The bootstrap plane is one transported pair. The join runs in the
		// pairing pass and resolves its partner by role identity, so neither
		// side depends on the other's position in the table.
		Pair: func(context rule.Pairing[*heapbootstrap.SchemaFragment]) bool {
			value, valueOK := context.Capability(programartifact.RuleRoleValueBootstrap)
			heap, heapOK := context.Capability(programartifact.RuleRoleHeapBootstrap)
			return valueOK && heapOK && engine.RegisterLinkBootstrapTransportPair(context.Binding, value, heap)
		},
		Bind: func(context rule.Binding[authorities, *heapbootstrap.SchemaFragment]) (*heapbootstrap.HotRule, bool) {
			return heapbootstrap.BindHot(context.Fragment, context.Authorities.heap)
		},
		Finalize: func(context rule.Finalization[authorities, *heapbootstrap.HotRule]) bool {
			catalog := context.Rule.Catalog()
			return catalog != nil && catalog.FencedTo(context.Authorities.heapSchema)
		},
		LinkAttach: func(context rule.LinkAttach[*heapbootstrap.HotRule]) bool {
			_, ok := context.Rule.AttachLinkOccurrence(context.Assembly, context.Occurrence)
			return ok
		},
		LinkMember: func(context rule.LinkMember[*heapbootstrap.HotRule]) bool {
			_, ok := context.Rule.AttachLinkReceiptMember(context.Compilation, context.Graph, context.Occurrence)
			return ok
		},
		LinkCatalog: func(hot *heapbootstrap.HotRule) (rule.LinkCatalog, bool) {
			catalog := hot.Catalog()
			return catalog, catalog != nil
		},
	}))

	add(rule.New(rule.Spec[principals, authorities, *valuetransfer.SchemaFragment, *valuetransfer.HotRule]{
		Key:      "value-transfer",
		Role:     programartifact.RuleRoleValueStorageTransfer,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.ValueTransferRule.Rule },
		Declare: func(context declaration) (*valuetransfer.SchemaFragment, bool) {
			semantics := context.Bundle.ValueTransferRule
			return valuetransfer.DeclareSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.value)
		},
		Register: func(context rule.Registration[*valuetransfer.SchemaFragment]) (engine.RuleSlotCapability, bool) {
			return registerMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[authorities, *valuetransfer.SchemaFragment]) (*valuetransfer.HotRule, bool) {
			return valuetransfer.BindHot(context.Fragment, context.Authorities.value)
		},
		Attach: func(context rule.Attach[*valuetransfer.HotRule]) bool {
			_, ok := context.Rule.AttachMountedRule(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*valuetransfer.HotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}))

	add(rule.New(rule.Spec[principals, authorities, *valuearithmetic.SchemaFragment, *valuearithmetic.HotRule]{
		Key:      "value-binary-arithmetic",
		Role:     programartifact.RuleRoleValueBinaryArithmetic,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.ValueBinaryArithmeticRule.Rule },
		Declare: func(context declaration) (*valuearithmetic.SchemaFragment, bool) {
			semantics := context.Bundle.ValueBinaryArithmeticRule
			return valuearithmetic.DeclareSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.value)
		},
		Register: func(context rule.Registration[*valuearithmetic.SchemaFragment]) (engine.RuleSlotCapability, bool) {
			return registerMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[authorities, *valuearithmetic.SchemaFragment]) (*valuearithmetic.HotRule, bool) {
			return valuearithmetic.BindHot(context.Fragment, context.Authorities.value)
		},
		Attach: func(context rule.Attach[*valuearithmetic.HotRule]) bool {
			_, ok := context.Rule.AttachMountedRule(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*valuearithmetic.HotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}))

	add(rule.New(rule.Spec[principals, authorities, *valueequality.SchemaFragment, *valueequality.HotRule]{
		Key:      "value-binary-equality",
		Role:     programartifact.RuleRoleValueBinaryEquality,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.ValueBinaryEqualityRule.Rule },
		Declare: func(context declaration) (*valueequality.SchemaFragment, bool) {
			semantics := context.Bundle.ValueBinaryEqualityRule
			return valueequality.DeclareSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.value)
		},
		Register: func(context rule.Registration[*valueequality.SchemaFragment]) (engine.RuleSlotCapability, bool) {
			return registerMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[authorities, *valueequality.SchemaFragment]) (*valueequality.HotRule, bool) {
			return valueequality.BindHot(context.Fragment, context.Authorities.value)
		},
		Attach: func(context rule.Attach[*valueequality.HotRule]) bool {
			_, ok := context.Rule.AttachMountedRule(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*valueequality.HotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}))

	add(rule.New(rule.Spec[principals, authorities, *valueorder.SchemaFragment, *valueorder.HotRule]{
		Key:      "value-binary-order",
		Role:     programartifact.RuleRoleValueBinaryOrder,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.ValueBinaryOrderRule.Rule },
		Declare: func(context declaration) (*valueorder.SchemaFragment, bool) {
			semantics := context.Bundle.ValueBinaryOrderRule
			return valueorder.DeclareSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.value)
		},
		Register: func(context rule.Registration[*valueorder.SchemaFragment]) (engine.RuleSlotCapability, bool) {
			return registerMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[authorities, *valueorder.SchemaFragment]) (*valueorder.HotRule, bool) {
			return valueorder.BindHot(context.Fragment, context.Authorities.value)
		},
		Attach: func(context rule.Attach[*valueorder.HotRule]) bool {
			_, ok := context.Rule.AttachMountedRule(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*valueorder.HotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}))

	add(rule.New(rule.Spec[principals, authorities, *valuerefinement.SchemaFragment, *valuerefinement.HotRule]{
		Key:      "value-presence-refinement",
		Role:     programartifact.RuleRoleValuePresenceRefinement,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.ValuePresenceRefinementRule.Rule },
		Declare: func(context declaration) (*valuerefinement.SchemaFragment, bool) {
			semantics := context.Bundle.ValuePresenceRefinementRule
			return valuerefinement.DeclareSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.value)
		},
		Register: func(context rule.Registration[*valuerefinement.SchemaFragment]) (engine.RuleSlotCapability, bool) {
			return registerMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[authorities, *valuerefinement.SchemaFragment]) (*valuerefinement.HotRule, bool) {
			return valuerefinement.BindHot(context.Fragment, context.Authorities.value)
		},
		Attach: func(context rule.Attach[*valuerefinement.HotRule]) bool {
			_, ok := context.Rule.AttachMountedRule(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*valuerefinement.HotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}))

	return admitted, !rejected && len(admitted) > 0
}

// registerMountedSlot performs the short-lived pre-seal owner handoff. The
// capability is not retained past the pairing pass: once SchemaBinding seals,
// callers resolve the exact capability from its semantic directory.
func registerMountedSlot[V, O any](binding *engine.SchemaBinding, slot *engine.RuleSlot[V, O]) (engine.RuleSlotCapability, bool) {
	capability, ok := engine.IssueMountedRuleCapability(binding, slot)
	if !ok || !engine.RegisterRuleSlot(binding, slot, capability) {
		return engine.RuleSlotCapability{}, false
	}
	return capability, true
}

func registerLinkSlot[V, O any](binding *engine.SchemaBinding, slot *engine.RuleSlot[V, O]) (engine.RuleSlotCapability, bool) {
	capability, ok := engine.IssueLinkRuleCapability(binding, slot)
	if !ok || !engine.RegisterRuleSlot(binding, slot, capability) {
		return engine.RuleSlotCapability{}, false
	}
	return capability, true
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
	// BindPrincipals is the caller-owned query surface step. It receives the
	// bound principals for exactly the duration of the call; the binding keeps
	// no query policy and publishes no principal accessor.
	BindPrincipals func(value *valueowner.HotOwner, effect *effectowner.HotOwner, views QueryViews) bool
}

func (inputs LinkInputs) available() bool {
	return inputs.ValueSchema != nil && inputs.CallAlgebra != nil && inputs.CallAlgebra.Valid() &&
		inputs.HeapSchema.Valid() && len(inputs.HeapMounts) > 0 && inputs.PackSchema != nil &&
		inputs.EffectAlgebra != nil && inputs.EffectAlgebra.Valid() && inputs.Topology != nil &&
		inputs.ActivationCatalog != nil && inputs.BindPrincipals != nil
}

// The axis input getters are the record's read surface for the axis pass. An
// axis's owning domain names exactly the one it binds against in its own need
// interface, so this record's shape never reaches the domain.
func (inputs LinkInputs) ValueInput() *valuedomain.Schema { return inputs.ValueSchema }

func (inputs LinkInputs) CallInput() *calldomain.Algebra { return inputs.CallAlgebra }

func (inputs LinkInputs) HeapInput() heapdomain.Schema { return inputs.HeapSchema }

func (inputs LinkInputs) PackInput() *packdomain.Schema { return inputs.PackSchema }

func (inputs LinkInputs) EffectInput() *effectfactor.Algebra { return inputs.EffectAlgebra }
