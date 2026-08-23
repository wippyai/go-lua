package targetresult

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	staticowner "github.com/wippyai/go-lua/domain/static/owner"
)

type rulePrincipals interface {
	StaticTypePrincipal() *staticowner.SchemaFragment
	CallPrincipal() *callowner.SchemaFragment
}

type ruleAuthorities interface {
	StaticTypeAuthority() *staticowner.HotOwner
	CallAuthority() *callowner.HotOwner
	TargetContract() *contract.Contract
}

// RuleEntry declares Static's mounted ordinary Target-call result projection.
// ResultAlias, fresh-result, and executable-body result rules own their
// separate semantics; this rule admits only the ordinary operation result
// type at the existing ordinal-zero CallResultSlot coordinate.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key: "static-callresult-target", Writes: "static-type", Owner: "static-type",
		Issues: []rule.Issuance{{Occurrence: "occurrence/call", Requirement: "program-requirement/call-result-slot", Form: "program-form/call-summary"}},
		Lane:   rule.LaneMounted, Semantic: "semantic/rule/static/callresult-target",
		Roles: []schema.Key{vocabulary.RoleKey("operand/static/callresult-target")},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	semantic, semanticOK := context.Roles.Key(vocabulary.RoleKey("rule/static/callresult-target"))
	operand, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/static/callresult-target"))
	if !semanticOK || !operandOK {
		return nil, false
	}
	return DeclareSchema(builder, semantic, operand, context.Principals.StaticTypePrincipal(), context.Principals.CallPrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(binding, context.Fragment, context.Authorities.StaticTypeAuthority(), context.Authorities.CallAuthority(), context.Authorities.TargetContract())
}

func FinalizeRule[A ruleAuthorities](context rule.Finalization[A, *HotRule]) bool {
	return context.Rule != nil && context.Rule.statics == context.Authorities.StaticTypeAuthority() &&
		context.Rule.calls == context.Authorities.CallAuthority() && context.Rule.contract == context.Authorities.TargetContract() && context.Rule.valid()
}

func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/static/callresult-target", "operand/static/callresult-target")
}
