package bodyresult

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type rulePrincipals interface {
	ValuePrincipal() *valueowner.SchemaFragment
	CallPrincipal() *callowner.SchemaFragment
}

type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
}

func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key: "value-callresult-body", Writes: "value", Owner: "value",
		Issues: []rule.Issuance{{Occurrence: "occurrence/call", Requirement: "program-requirement/call-result-slot", Form: "program-form/call-summary"}},
		Lane:   rule.LaneMounted, Semantic: "semantic/rule/value/callresult-body",
		Roles: []schema.Key{"semantic/operand/value/callresult-body"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	semantic, semanticOK := context.Roles.Key(vocabulary.RoleKey("rule/value/callresult-body"))
	operand, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/value/callresult-body"))
	if !semanticOK || !operandOK {
		return nil, false
	}
	return DeclareSchema(builder, semantic, operand, context.Principals.ValuePrincipal(), context.Principals.CallPrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(binding, context.Fragment, context.Authorities.ValueAuthority(), context.Authorities.CallAuthority())
}

func FinalizeRule[A ruleAuthorities](context rule.Finalization[A, *HotRule]) bool {
	return context.Rule != nil && context.Rule.values == context.Authorities.ValueAuthority() && context.Rule.calls == context.Authorities.CallAuthority() && context.Rule.valid()
}

func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/value/callresult-body", "operand/value/callresult-body")
}
