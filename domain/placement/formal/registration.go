package formal

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type rulePrincipals interface {
	ValuePrincipal() *valueowner.SchemaFragment
	CallPrincipal() *callowner.SchemaFragment
	PlacementPrincipal() *placementowner.SchemaFragment
}

// formalAuthorityInputs is the sealed authority surface needed in addition to
// the three factor owners. The composition supplies the exact Target
// contract and the Pack authority once. Keeping them as accessors prevents
// the hot callback from reopening Link or inventing a formal invocation
// table.
type formalAuthorityInputs interface {
	TargetContract() *contract.Contract
	PackSchema() *packdomain.Schema
}

type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
	PlacementAuthority() *placementowner.HotOwner
	formalAuthorityInputs
}

// RuleEntry is the mounted Placement consumer for operation-owned formal
// ownership rows. The occurrence runs at the call-effect cut, after Call has
// selected its target alternatives and Value has published actual facts.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:    "placement-formal",
		Writes: "placement",
		Owner:  "placement",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/call", Requirement: "requirement/unrestricted", Form: "issuance/call-stage", Input: "input/finish", Stage: "stage/call-effect"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/placement/formal",
		Roles:    []schema.Key{"semantic/operand/placement/formal"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	semantic, semanticOK := context.Roles.Key(vocabulary.RoleKey("rule/placement/formal"))
	operand, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/placement/formal"))
	if !semanticOK || !operandOK {
		return nil, false
	}
	return DeclareSchema(builder, semantic, operand,
		context.Principals.ValuePrincipal(), context.Principals.CallPrincipal(), context.Principals.PlacementPrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(binding, context.Fragment, context.Authorities.PlacementAuthority(), context.Authorities.ValueAuthority(), context.Authorities.CallAuthority(), context.Authorities.TargetContract(), context.Authorities.PackSchema())
}

// FinalizeRule closes the formal consumer's typed authority join after the
// shared SchemaBinding seals. This pass restates the exact Call/Pack Link
// fence and Target authority; no runtime receipt or generic Result plane is
// introduced.
func FinalizeRule[A ruleAuthorities](context rule.Finalization[A, *HotRule]) bool {
	if context.Rule == nil || context.Rule.owner != context.Authorities.PlacementAuthority() ||
		context.Rule.values != context.Authorities.ValueAuthority() || context.Rule.calls != context.Authorities.CallAuthority() ||
		context.Rule.packs == nil || context.Rule.contract == nil {
		return false
	}
	return context.Rule.packs == context.Authorities.PackSchema() &&
		context.Rule.calls.Algebra() != nil && context.Rule.calls.Algebra().Valid() &&
		context.Rule.packs.LinkOwner().Available() && context.Rule.packs.LinkOwner().Matches(context.Rule.calls.LinkOwner()) &&
		context.Rule.calls.OwnsTargetContract(context.Rule.contract) &&
		context.Rule.contract == context.Authorities.TargetContract()
}

// StructureSpecs contributes the formal consumer's rule and operand role
// identities to the semantic vocabulary owned by the composite declaration.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/placement/formal", "operand/placement/formal")
}
