// Package freshresult declares Value's Target fresh-result Call transfer.
// The package is intentionally isolated from the composite roster while the
// environment wiring for this Link-lane rule is completed.
package freshresult

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type rulePrincipals interface {
	ValuePrincipal() *valueowner.SchemaFragment
	CallPrincipal() *callowner.SchemaFragment
}

type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
	ValueSchema() *valuedomain.Schema
}

// RuleEntry declares the Link-lane rule that transfers an authenticated Target
// fresh result into the existing fixed CallResultValue coordinate.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:      "value-callresult-freshresult",
		Writes:   "value",
		Owner:    "value",
		Lane:     rule.LaneLink,
		Semantic: "semantic/rule/value/callresult-freshresult",
		Roles: []schema.Key{
			"semantic/operand/value/callresult-freshresult",
			"semantic/transform/value/callresult-freshresult",
		},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	ruleSemantic, ruleOK := context.Roles.Key(vocabulary.RoleKey("rule/value/callresult-freshresult"))
	operandFamily, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/value/callresult-freshresult"))
	transform, transformOK := context.Roles.Key(vocabulary.RoleKey("transform/value/callresult-freshresult"))
	if !ruleOK || !operandOK || !transformOK {
		return nil, false
	}
	return DeclareSchema(builder, ruleSemantic, operandFamily, transform, context.Principals.ValuePrincipal(), context.Principals.CallPrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterLinkSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](_ *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(context.Fragment, context.Authorities.ValueAuthority(), context.Authorities.CallAuthority())
}

func FinalizeRule[A ruleAuthorities](context rule.Finalization[A, *HotRule]) bool {
	if context.Rule == nil || context.Authorities.ValueSchema() == nil || context.Rule.values == nil {
		return false
	}
	schema := context.Authorities.ValueSchema()
	return context.Rule.values.Schema() == schema && context.Rule.Count() == schema.FreshResultCallCount()
}

func OccurrenceCatalog(hot *HotRule) (rule.OccurrenceCatalog, bool) {
	return hot, hot != nil
}

// StructureSpecs contributes the isolated rule's semantic roles. Composite
// roster/environment wiring intentionally remains outside this package.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(
		"rule/value/callresult-freshresult",
		"operand/value/callresult-freshresult",
		"transform/value/callresult-freshresult",
	)
}
