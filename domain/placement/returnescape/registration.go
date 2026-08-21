package returnescape

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type rulePrincipals interface {
	ValuePrincipal() *valueowner.SchemaFragment
	PlacementPrincipal() *placementowner.SchemaFragment
}

type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	PlacementAuthority() *placementowner.HotOwner
}

// RuleEntry is the mounted Program-return Placement escape Rule. Its operand
// is issued by the exact occurrence/return-boundary row; no caller-result edge
// is declared because Heap allocation roots are Link-global.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:      "placement-return-escape",
		Writes:   "placement",
		Owner:    "placement",
		Issues:   []rule.Issuance{{Occurrence: "occurrence/return-boundary", Requirement: "requirement/unrestricted", Form: "issuance/local-successor", Input: "input/finish", Stage: "stage/local"}},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/placement/return-escape",
		Roles:    []schema.Key{"semantic/operand/placement/return-escape"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	ruleSemantic, ruleOK := context.Roles.Key(vocabulary.RoleKey("rule/placement/return-escape"))
	operandFamily, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/placement/return-escape"))
	if !ruleOK || !operandOK {
		return nil, false
	}
	return DeclareSchema(builder, ruleSemantic, operandFamily, context.Principals.ValuePrincipal(), context.Principals.PlacementPrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(binding, context.Fragment, context.Authorities.PlacementAuthority(), context.Authorities.ValueAuthority())
}

// StructureSpecs contributes the return escape Rule's rule and operand
// identities to the semantic role vocabulary.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/placement/return-escape", "operand/placement/return-escape")
}
