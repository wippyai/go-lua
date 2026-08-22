// Package resultalias declares Value's Target ResultAlias consumer. It is
// intentionally not added to the composite roster until the environment-owned
// rule roster is available.
package resultalias

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type rulePrincipals interface {
	ValuePrincipal() *valueowner.SchemaFragment
	CallPrincipal() *callowner.SchemaFragment
}

// resultAliasAuthorityInputs is the small cold authority join required by
// this consumer. Target is retained once as a bind-time alias plan; Pack
// supplies direct sealed mounted-actual projections on the hot path.
type resultAliasAuthorityInputs interface {
	TargetContract() *contract.Contract
	PackSchema() *packdomain.Schema
}

type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
	resultAliasAuthorityInputs
}

// RuleEntry declares one Value operand per mounted ordinal-0 CallResultSlot. The
// selected Target operation is a hot plan, not a structural operand axis.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:    "value-callresult-resultalias",
		Writes: "value",
		Owner:  "value",
		Issues: []rule.Issuance{{
			Occurrence:  "occurrence/call",
			Requirement: "program-requirement/unrestricted",
			Form:        "program-form/call-summary",
		}},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/value/callresult-resultalias",
		Roles:    []schema.Key{"semantic/operand/value/callresult-resultalias"},
	}
}

// DeclareRule contributes only neutral engine shape. The environment chooses
// whether and where to register this isolated consumer.
func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	ruleSemantic, ruleOK := context.Roles.Key(vocabulary.RoleKey("rule/value/callresult-resultalias"))
	operandFamily, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/value/callresult-resultalias"))
	if !ruleOK || !operandOK {
		return nil, false
	}
	return DeclareSchema(builder, ruleSemantic, operandFamily, context.Principals.ValuePrincipal(), context.Principals.CallPrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(binding, context.Fragment, context.Authorities.ValueAuthority(), context.Authorities.CallAuthority(), context.Authorities.TargetContract(), context.Authorities.PackSchema())
}

// FinalizeRule keeps the exact owner/authority fence after the shared binding
// seals. It does not create a second result or alias receipt.
func FinalizeRule[A ruleAuthorities](context rule.Finalization[A, *HotRule]) bool {
	if context.Rule == nil || context.Rule.values != context.Authorities.ValueAuthority() || context.Rule.calls != context.Authorities.CallAuthority() ||
		context.Rule.contract != context.Authorities.TargetContract() ||
		context.Rule.pack != context.Authorities.PackSchema() {
		return false
	}
	return context.Rule.values != nil && context.Rule.values.Schema() != nil && context.Rule.calls != nil && context.Rule.calls.Algebra() != nil &&
		context.Rule.contract != nil && context.Rule.pack != nil && context.Rule.values.Schema().LinkOwner().Matches(context.Rule.calls.Algebra().LinkOwner()) &&
		context.Rule.pack.LinkOwner().Matches(context.Rule.calls.Algebra().LinkOwner()) && context.Rule.calls.OwnsTargetContract(context.Rule.contract)
}

func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/value/callresult-resultalias", "operand/value/callresult-resultalias")
}
