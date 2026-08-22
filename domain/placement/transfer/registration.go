package transfer

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

// transferAuthorityInputs is the exact Link-side Target/Pack surface.  The
// consumer receives the already sealed Target Contract and Pack Schema; it
// does not reopen Boundary, Project, Program, or Effect.
type transferAuthorityInputs interface {
	TargetContract() *contract.Contract
	PackSchema() *packdomain.Schema
}

type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
	PlacementAuthority() *placementowner.HotOwner
	transferAuthorityInputs
}

// RuleEntry declares the mounted Target-transfer Placement consumer.  The
// call-effect cut is the invocation boundary at which Call alternatives,
// Pack actuals, and Target transfer declarations are all authenticated.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:      "placement-transfer",
		Writes:   "placement",
		Owner:    "placement",
		Issues:   []rule.Issuance{{Occurrence: "occurrence/call", Requirement: "program-requirement/unrestricted", Form: "program-form/call-effect"}},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/placement/transfer",
		Roles:    []schema.Key{"semantic/operand/placement/transfer"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	semantic, semanticOK := context.Roles.Key(vocabulary.RoleKey("rule/placement/transfer"))
	operand, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/placement/transfer"))
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
	return BindHot(binding, context.Fragment,
		context.Authorities.PlacementAuthority(), context.Authorities.ValueAuthority(),
		context.Authorities.CallAuthority(), context.Authorities.TargetContract(), context.Authorities.PackSchema())
}

// FinalizeRule closes the exact Link/runtime authority join after the shared
// binding seals.  The transfer consumer has no Effect or contextual Heap
// adapter dependency; Heap roots are projected through Placement's owner.
func FinalizeRule[A ruleAuthorities](context rule.Finalization[A, *HotRule]) bool {
	if context.Rule == nil || context.Rule.owner != context.Authorities.PlacementAuthority() ||
		context.Rule.values != context.Authorities.ValueAuthority() || context.Rule.calls != context.Authorities.CallAuthority() ||
		context.Rule.contract == nil || context.Rule.contract != context.Authorities.TargetContract() ||
		context.Rule.packs == nil || context.Rule.packs != context.Authorities.PackSchema() {
		return false
	}
	if context.Rule.calls.Algebra() == nil || !context.Rule.calls.Algebra().Valid() ||
		!context.Rule.calls.OwnsTargetContract(context.Rule.contract) {
		return false
	}
	owner := context.Rule.calls.Algebra().LinkOwner()
	return context.Rule.packs.LinkOwner().Available() && context.Rule.packs.LinkOwner().Matches(owner) &&
		context.Rule.values.Schema() != nil && context.Rule.values.Schema().LinkOwner().Matches(owner)
}

// StructureSpecs contributes the transfer consumer's rule and operand roles
// to the semantic vocabulary owned by the composite declaration.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/placement/transfer", "operand/placement/transfer")
}
