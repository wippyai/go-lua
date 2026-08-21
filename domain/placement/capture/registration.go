package capture

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type rulePrincipals interface {
	ValuePrincipal() *valueowner.SchemaFragment
	PlacementPrincipal() *placementowner.SchemaFragment
}

type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	PlacementAuthority() *placementowner.HotOwner
	ValueSchema() *valuedomain.Schema
	PlacementSchema() placementdomain.Schema
}

// RuleEntry is the mounted closure-capture transport. Its issuance is limited
// to closure allocation occurrences with a positive capture boundary.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:      "placement-closure-capture",
		Writes:   "placement",
		Owner:    "placement",
		Issues:   []rule.Issuance{{Occurrence: "occurrence/allocation", Requirement: "requirement/closure-capture", Form: "issuance/local-successor", Input: "input/finish", Stage: "stage/local"}},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/placement/closure-capture",
		Roles:    []schema.Key{"semantic/operand/placement/closure-capture"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	semantic, semanticOK := context.Roles.Key(vocabulary.RoleKey("rule/placement/closure-capture"))
	operand, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/placement/closure-capture"))
	if !semanticOK || !operandOK {
		return nil, false
	}
	return DeclareSchema(builder, semantic, operand, context.Principals.ValuePrincipal(), context.Principals.PlacementPrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(binding, context.Fragment, context.Authorities.PlacementAuthority(), context.Authorities.ValueAuthority(), context.Authorities.ValueSchema(), context.Authorities.PlacementSchema())
}

func FinalizeRule[A ruleAuthorities](context rule.Finalization[A, *HotRule]) bool {
	return context.Rule != nil && context.Rule.owner == context.Authorities.PlacementAuthority() &&
		context.Rule.values == context.Authorities.ValueAuthority() && context.Rule.schema == context.Authorities.PlacementSchema() &&
		context.Rule.valueSchema == context.Authorities.ValueSchema()
}

// StructureSpecs contributes the capture rule's rule and operand identities to
// the semantic role vocabulary.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/placement/closure-capture", "operand/placement/closure-capture")
}
