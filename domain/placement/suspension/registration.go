package suspension

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
	EvidencePrincipal() *EvidenceFactorFragment
}

type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	PlacementAuthority() *placementowner.HotOwner
	EvidenceAuthority() *EvidenceOwner
	ValueSchema() *valuedomain.Schema
	PlacementSchema() placementdomain.Schema
}

// RuleEntry declares the Link-lane consumer of Program subject-liveness
// evidence. It has no artifact occurrence subscription: the mounted Program
// set is joined once into the Link catalog at binding time.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:      "placement-suspension",
		Writes:   "placement",
		Owner:    "placement",
		Lane:     rule.LaneLink,
		Semantic: "semantic/rule/placement/suspension",
		Roles:    []schema.Key{"semantic/operand/placement/suspension"},
	}
}

// EvidenceRuleEntry declares the independent Link-lane producer whose output
// is Placement's Heap-aligned suspension-evidence Factor. It deliberately
// shares the Program/Value bridge with RuleEntry but has its own semantic
// rule identity and output receipt.
func EvidenceRuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:      "placement-suspension-evidence",
		Writes:   "placement-suspension-evidence",
		Owner:    "placement-suspension-evidence",
		Lane:     rule.LaneLink,
		Semantic: "semantic/rule/placement/suspension-evidence",
		Roles:    []schema.Key{"semantic/operand/placement/suspension-evidence"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	ruleSemantic, ruleOK := context.Roles.Key(vocabulary.RoleKey("rule/placement/suspension"))
	operandFamily, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/placement/suspension"))
	if !ruleOK || !operandOK {
		return nil, false
	}
	return DeclareSchema(builder, ruleSemantic, operandFamily, context.Principals.ValuePrincipal(), context.Principals.PlacementPrincipal())
}

func DeclareEvidenceRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*EvidenceSchemaFragment, bool) {
	ruleSemantic, ruleOK := context.Roles.Key(vocabulary.RoleKey("rule/placement/suspension-evidence"))
	operandFamily, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/placement/suspension-evidence"))
	if !ruleOK || !operandOK {
		return nil, false
	}
	return DeclareEvidenceSchema(builder, ruleSemantic, operandFamily, context.Principals.ValuePrincipal(), context.Principals.EvidencePrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterLinkSlot(binding, context.Fragment.RuleSlot())
}

func RegisterEvidenceRule(binding *engine.SchemaBinding, context rule.Registration[*EvidenceSchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterLinkSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(binding, context.Fragment, context.Authorities.PlacementAuthority(), context.Authorities.ValueAuthority(), context.Authorities.ValueSchema(), context.Authorities.PlacementSchema())
}

func BindEvidenceRule[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *EvidenceSchemaFragment]) (*EvidenceHotRule, bool) {
	return BindEvidenceHot(binding, context.Fragment, context.Authorities.EvidenceAuthority(), context.Authorities.ValueAuthority(), context.Authorities.ValueSchema(), context.Authorities.PlacementSchema())
}

func FinalizeRule[A ruleAuthorities](context rule.Finalization[A, *HotRule]) bool {
	return context.Rule != nil && context.Rule.Catalog() != nil && context.Rule.Catalog().FencedTo(context.Authorities.PlacementSchema(), context.Authorities.ValueSchema())
}

func FinalizeEvidenceRule[A ruleAuthorities](context rule.Finalization[A, *EvidenceHotRule]) bool {
	return context.Rule != nil && context.Rule.Catalog() != nil && context.Rule.Catalog().FencedTo(context.Authorities.PlacementSchema(), context.Authorities.ValueSchema())
}

func OccurrenceCatalog(hot *HotRule) (rule.OccurrenceCatalog, bool) {
	if hot == nil || hot.Catalog() == nil {
		return nil, false
	}
	return hot.Catalog(), true
}

func EvidenceOccurrenceCatalog(hot *EvidenceHotRule) (rule.OccurrenceCatalog, bool) {
	if hot == nil || hot.Catalog() == nil {
		return nil, false
	}
	return hot.Catalog(), true
}

func StructureSpecs() []structure.Spec {
	specs := FactorStructureSpecs()
	specs = append(specs, vocabulary.RoleSpecs("rule/placement/suspension", "operand/placement/suspension")...)
	return append(specs, vocabulary.RoleSpecs("rule/placement/suspension-evidence", "operand/placement/suspension-evidence")...)
}
