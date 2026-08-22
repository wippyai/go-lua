// Package moduleload declares Value's bounded call-result module-load rule.
package moduleload

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

// RuleEntry declares the Value-owned transfer from an exact scoped require
// Call result to the existing result Value coordinate.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:    "value-callresult-moduleload",
		Writes: "value",
		Owner:  "value",
		Issues: []rule.Issuance{{
			Occurrence:  "occurrence/call",
			Requirement: "program-requirement/module-load-call-result",
			Form:        "program-form/call-summary",
		}},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/value/callresult-moduleload",
		Roles: []schema.Key{
			"semantic/operand/value/callresult-moduleload",
		},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	ruleSemantic, ruleOK := context.Roles.Key(vocabulary.RoleKey("rule/value/callresult-moduleload"))
	operandFamily, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/value/callresult-moduleload"))
	if !ruleOK || !operandOK {
		return nil, false
	}
	return DeclareSchema(builder, ruleSemantic, operandFamily, context.Principals.ValuePrincipal(), context.Principals.CallPrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](_ *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(context.Fragment, context.Authorities.ValueAuthority(), context.Authorities.CallAuthority())
}

func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/value/callresult-moduleload", "operand/value/callresult-moduleload")
}
