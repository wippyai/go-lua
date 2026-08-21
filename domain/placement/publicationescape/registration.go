package publicationescape

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type rulePrincipals interface {
	ValuePrincipal() *valueowner.SchemaFragment
	CallPrincipal() *callowner.SchemaFragment
	PlacementPrincipal() *placementowner.SchemaFragment
}

type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
	PlacementAuthority() *placementowner.HotOwner
	EffectAuthority() *effectowner.HotOwner
}

// RuleEntry contributes the mounted call-level publication escape consumer.
func RuleEntry[P rulePrincipals, A any]() rule.Spec {
	return rule.Spec{
		Key:      "placement-publication-escape",
		Writes:   "placement",
		Owner:    "placement",
		Issues:   []rule.Issuance{{Occurrence: "occurrence/call", Requirement: "requirement/unrestricted", Form: "issuance/call-stage", Input: "input/finish", Stage: "stage/call-effect"}},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/placement/publication-escape",
		Roles:    []schema.Key{"semantic/operand/placement/publication-escape"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	ruleSemantic, ruleOK := context.Roles.Key(vocabulary.RoleKey("rule/placement/publication-escape"))
	operandFamily, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/placement/publication-escape"))
	if !ruleOK || !operandOK {
		return nil, false
	}
	return DeclareSchema(builder, ruleSemantic, operandFamily, context.Principals.ValuePrincipal(), context.Principals.CallPrincipal(), context.Principals.PlacementPrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(binding, context.Fragment, context.Authorities.PlacementAuthority(), context.Authorities.ValueAuthority(), context.Authorities.CallAuthority(), context.Authorities.EffectAuthority())
}

// StructureSpecs contributes the publication escape rule and operand roles.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/placement/publication-escape", "operand/placement/publication-escape")
}
