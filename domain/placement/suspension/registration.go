package suspension

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type rulePrincipals interface {
	ValuePrincipal() *valueowner.SchemaFragment
	CallPrincipal() *callowner.SchemaFragment
	PlacementPrincipal() *placementowner.SchemaFragment
	EvidencePrincipal() *EvidenceFactorFragment
}

type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
	PlacementAuthority() *placementowner.HotOwner
	EvidenceAuthority() *EvidenceOwner
	ValueSchema() *valuedomain.Schema
	PlacementSchema() placementdomain.Schema
}

// RuleEntry declares the mounted consumer of Program subject-liveness
// evidence. It subscribes to the occurrence Program issues for each liveness
// span, because the judgment is only decidable at the mounted point: whether a
// span's boundary is a yield boundary is the solved Call fact at that call,
// and Call is written by mounted rules at mounted points. A rule reading it
// from the Link bootstrap point would read the Factor's default forever.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:      "placement-suspension",
		Writes:   "placement",
		Owner:    "placement",
		Issues:   RuleIssues(),
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/placement/suspension",
		Roles:    []schema.Key{"semantic/operand/placement/suspension"},
	}
}

// RuleIssues is the one subject-liveness subscription both suspension
// producers consume. They answer over the same spans and differ only in the
// Factor they write.
func RuleIssues() []rule.Issuance {
	return []rule.Issuance{{
		Occurrence:  "occurrence/subject-liveness",
		Requirement: programissuance.RequirementUnrestricted,
		Form:        programissuance.FormCallSummary,
	}}
}

// EvidenceRuleEntry declares the independent producer whose output is
// Placement's Heap-aligned suspension-evidence Factor. It deliberately shares
// the Program/Value/Call bridge and the subject-liveness subscription with
// RuleEntry but has its own semantic rule identity and output receipt.
func EvidenceRuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:      "placement-suspension-evidence",
		Writes:   "placement-suspension-evidence",
		Owner:    "placement-suspension-evidence",
		Issues:   RuleIssues(),
		Lane:     rule.LaneMounted,
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
	return DeclareSchema(builder, ruleSemantic, operandFamily, context.Principals.ValuePrincipal(), context.Principals.CallPrincipal(), context.Principals.PlacementPrincipal())
}

func DeclareEvidenceRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*EvidenceSchemaFragment, bool) {
	ruleSemantic, ruleOK := context.Roles.Key(vocabulary.RoleKey("rule/placement/suspension-evidence"))
	operandFamily, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/placement/suspension-evidence"))
	if !ruleOK || !operandOK {
		return nil, false
	}
	return DeclareEvidenceSchema(builder, ruleSemantic, operandFamily, context.Principals.ValuePrincipal(), context.Principals.CallPrincipal(), context.Principals.EvidencePrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func RegisterEvidenceRule(binding *engine.SchemaBinding, context rule.Registration[*EvidenceSchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(binding, context.Fragment, context.Authorities.PlacementAuthority(), context.Authorities.ValueAuthority(), context.Authorities.CallAuthority(), context.Authorities.ValueSchema(), context.Authorities.PlacementSchema())
}

func BindEvidenceRule[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *EvidenceSchemaFragment]) (*EvidenceHotRule, bool) {
	return BindEvidenceHot(binding, context.Fragment, context.Authorities.EvidenceAuthority(), context.Authorities.ValueAuthority(), context.Authorities.CallAuthority(), context.Authorities.ValueSchema(), context.Authorities.PlacementSchema())
}

func FinalizeRule[A ruleAuthorities](context rule.Finalization[A, *HotRule]) bool {
	return context.Rule != nil && context.Rule.Catalog() != nil && context.Rule.Catalog().FencedTo(context.Authorities.PlacementSchema(), context.Authorities.ValueSchema())
}

func FinalizeEvidenceRule[A ruleAuthorities](context rule.Finalization[A, *EvidenceHotRule]) bool {
	return context.Rule != nil && context.Rule.Catalog() != nil && context.Rule.Catalog().FencedTo(context.Authorities.PlacementSchema(), context.Authorities.ValueSchema())
}

func StructureSpecs() []structure.Spec {
	specs := FactorStructureSpecs()
	specs = append(specs, vocabulary.RoleSpecs("rule/placement/suspension", "operand/placement/suspension")...)
	return append(specs, vocabulary.RoleSpecs("rule/placement/suspension-evidence", "operand/placement/suspension-evidence")...)
}
