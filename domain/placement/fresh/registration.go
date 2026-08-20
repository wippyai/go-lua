package fresh

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
)

type rulePrincipals interface {
	PlacementPrincipal() *placementowner.SchemaFragment
}

type ruleAuthorities interface {
	PlacementAuthority() *placementowner.HotOwner
	PlacementSchema() placementdomain.Schema
}

// RuleEntry is Placement/fresh's zero-input Link-scoped Stack seed rule. Its
// occurrence inventory is the canonical Heap fresh-root denominator, not a
// mounted Program allocation stream.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:      "placement-fresh",
		Writes:   "placement",
		Owner:    "placement",
		Lane:     rule.LaneLink,
		Semantic: "semantic/rule/placement/fresh",
		Roles:    []schema.Key{"semantic/operand/placement/fresh"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	semantic, semanticOK := context.Roles.Key(vocabulary.RoleKey("rule/placement/fresh"))
	operand, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/placement/fresh"))
	if !semanticOK || !operandOK {
		return nil, false
	}
	return DeclareSchema(builder, semantic, operand, context.Principals.PlacementPrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterLinkSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(binding, context.Fragment, context.Authorities.PlacementAuthority(), context.Authorities.PlacementSchema())
}

func FinalizeRule[A ruleAuthorities](context rule.Finalization[A, *HotRule]) bool {
	return context.Rule != nil && context.Authorities.PlacementAuthority() != nil &&
		context.Rule.owner == context.Authorities.PlacementAuthority() &&
		context.Authorities.PlacementAuthority().Schema() == context.Authorities.PlacementSchema() &&
		context.Rule.Count() == context.Authorities.PlacementSchema().Heap().FreshCount()
}

func LinkCatalog(hot *HotRule) (rule.LinkCatalog, bool) {
	return hot, hot != nil
}

// StructureSpecs contributes Placement/fresh's rule and operand identities to
// the semantic vocabulary.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/placement/fresh", "operand/placement/fresh")
}
