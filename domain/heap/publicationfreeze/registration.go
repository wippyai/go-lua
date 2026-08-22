package publicationfreeze

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type rulePrincipals interface {
	ValuePrincipal() *valueowner.SchemaFragment
	CallPrincipal() *callowner.SchemaFragment
	HeapPrincipal() *heapowner.SchemaFragment
}

type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
	HeapAuthority() *heapowner.HotOwner
	EffectAuthority() *effectowner.HotOwner
}

// RuleEntry is the mounted Heap consumer for Effect-authored FreezeSeal rows.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:      "heap-publication-freeze",
		Writes:   "heap",
		Owner:    "heap",
		Issues:   []rule.Issuance{{Occurrence: "occurrence/call", Requirement: "program-requirement/unrestricted", Form: "program-form/call-effect"}},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/heap/publication-freeze",
		Roles:    []schema.Key{"semantic/operand/heap/publication-freeze"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	semantic, semanticOK := context.Roles.Key(vocabulary.RoleKey("rule/heap/publication-freeze"))
	operand, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/heap/publication-freeze"))
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
	return BindHot(binding, context.Fragment, context.Authorities.HeapAuthority(), context.Authorities.ValueAuthority(), context.Authorities.CallAuthority(), context.Authorities.EffectAuthority())
}

func FinalizeRule[A ruleAuthorities](context rule.Finalization[A, *HotRule]) bool {
	if context.Rule == nil || context.Rule.owner != context.Authorities.HeapAuthority() ||
		context.Rule.values != context.Authorities.ValueAuthority() || context.Rule.calls != context.Authorities.CallAuthority() ||
		context.Rule.effects != context.Authorities.EffectAuthority() {
		return false
	}
	return context.Rule.values != nil && context.Rule.values.Schema() != nil && context.Rule.calls.Algebra() != nil && context.Rule.calls.Algebra().Valid() &&
		context.Rule.effects.Algebra() != nil && context.Rule.effects.Algebra().Valid() &&
		context.Rule.owner.Schema() == context.Rule.values.Schema().Heap() &&
		context.Rule.effects.Algebra().LinkOwner().Matches(context.Rule.calls.Algebra().LinkOwner())
}

// StructureSpecs contributes the publication-freeze rule and operand roles.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/heap/publication-freeze", "operand/heap/publication-freeze")
}
