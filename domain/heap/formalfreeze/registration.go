package formalfreeze

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type rulePrincipals interface {
	ValuePrincipal() *valueowner.SchemaFragment
	CallPrincipal() *callowner.SchemaFragment
	HeapPrincipal() *heapowner.SchemaFragment
}

type formalAuthorityInputs interface {
	TargetContract() *contract.Contract
	PackSchema() *packdomain.Schema
}

type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
	HeapAuthority() *heapowner.HotOwner
	formalAuthorityInputs
}

// RuleEntry is the mounted Heap consumer for exact operation-owned Freeze
// rows. It runs at the call-effect stage after Call has selected targets and
// Value has published the mounted actual facts.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:      "heap-formal-freeze",
		Writes:   "heap",
		Owner:    "heap",
		Issues:   []rule.Issuance{{Occurrence: "occurrence/call", Requirement: "program-requirement/unrestricted", Form: "program-form/call-effect"}},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/heap/formal-freeze",
		Roles:    []schema.Key{"semantic/operand/heap/formal-freeze"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	semantic, semanticOK := context.Roles.Key(vocabulary.RoleKey("rule/heap/formal-freeze"))
	operand, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/heap/formal-freeze"))
	if !semanticOK || !operandOK {
		return nil, false
	}
	return DeclareSchema(builder, semantic, operand,
		context.Principals.ValuePrincipal(), context.Principals.CallPrincipal(), context.Principals.HeapPrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(binding, context.Fragment, context.Authorities.HeapAuthority(), context.Authorities.ValueAuthority(), context.Authorities.CallAuthority(), context.Authorities.TargetContract(), context.Authorities.PackSchema())
}

// FinalizeRule closes the formal-freeze consumer's typed authority join after
// the shared SchemaBinding seals. The exact Heap/Value/Call/Pack fences are
// restated here so a foreign equal-content authority cannot be published.
func FinalizeRule[A ruleAuthorities](context rule.Finalization[A, *HotRule]) bool {
	if context.Rule == nil || context.Rule.owner != context.Authorities.HeapAuthority() ||
		context.Rule.values != context.Authorities.ValueAuthority() || context.Rule.calls != context.Authorities.CallAuthority() ||
		context.Rule.contract == nil || context.Rule.contract != context.Authorities.TargetContract() ||
		context.Rule.packs == nil || context.Rule.packs != context.Authorities.PackSchema() {
		return false
	}
	return context.Rule.values != nil && context.Rule.values.Schema() != nil && context.Rule.calls.Algebra() != nil && context.Rule.calls.Algebra().Valid() &&
		context.Rule.packs.LinkOwner().Available() && context.Rule.packs.LinkOwner().Matches(context.Rule.calls.Algebra().LinkOwner()) &&
		context.Rule.calls.OwnsTargetContract(context.Rule.contract) && context.Rule.owner.Schema() == context.Rule.values.Schema().Heap()
}

// StructureSpecs contributes the formal-freeze consumer's rule and operand
// identities to the semantic vocabulary.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/heap/formal-freeze", "operand/heap/formal-freeze")
}
