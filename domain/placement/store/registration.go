package store

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

// RuleEntry declares the engine-level Placement consumer for Program storage
// transfers. Value supplies the fixed transfer operand; this rule writes only
// Placement and never fabricates a domain result.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:    "placement-storage",
		Writes: "placement",
		Owner:  "placement",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/storage-bind-transfer", Requirement: "requirement/unrestricted", Form: "issuance/local", Input: "input/entry", Stage: "stage/local"},
			{Occurrence: "occurrence/storage-bind-transfer", Requirement: "requirement/call-result-slot", Form: "issuance/call-stage", Input: "input/finish", Stage: "stage/call-effect"},
			{Occurrence: "occurrence/storage-write", Requirement: "requirement/unrestricted", Form: "issuance/local-predecessor", Input: "input/predecessor", Stage: "stage/local"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/placement/storage",
		Roles:    []schema.Key{"semantic/operand/placement/storage"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	ruleSemantic, ruleOK := context.Roles.Key(vocabulary.RoleKey("rule/placement/storage"))
	operandFamily, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/placement/storage"))
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

func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/placement/storage", "operand/placement/storage")
}
