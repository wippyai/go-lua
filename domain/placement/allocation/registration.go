package allocation

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
)

type rulePrincipals interface {
	PlacementPrincipal() *placementowner.SchemaFragment
}

type ruleAuthorities interface {
	PlacementAuthority() *placementowner.HotOwner
	HeapSchema() heapdomain.Schema
}

// RuleEntry is Placement/allocation's mounted zero-input Stack seed Rule.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:      "placement-allocation",
		Writes:   "placement",
		Owner:    "placement",
		Issues:   []rule.Issuance{{Occurrence: "occurrence/allocation", Requirement: "requirement/unrestricted", Form: "issuance/base", Input: "input/none", Stage: "stage/base"}},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/placement/allocation",
		Roles:    []schema.Key{"semantic/operand/placement/allocation"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	semantic, semanticOK := context.Roles.Key(vocabulary.RoleKey("rule/placement/allocation"))
	operand, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/placement/allocation"))
	if !semanticOK || !operandOK {
		return nil, false
	}
	return DeclareSchema(builder, semantic, operand, context.Principals.PlacementPrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(binding, context.Fragment, context.Authorities.PlacementAuthority(), context.Authorities.HeapSchema())
}

// StructureSpecs contributes Placement/allocation's rule and operand role
// identities to the semantic vocabulary.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/placement/allocation", "operand/placement/allocation")
}
