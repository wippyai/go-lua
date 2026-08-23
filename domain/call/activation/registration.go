package activation

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// rulePrincipals is this package's own statement of the cold owners its rule
// declares against. The activation shape is declared against the call lane
// alone.
type rulePrincipals interface {
	CallPrincipal() *callowner.SchemaFragment
}

// ruleAuthorities is the sealed authority set this rule binds against. The
// activation plane transports the exact factor lanes required across a call
// boundary. Its route keys are private bind-time
// output derived from Call itself.
type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
	HeapAuthority() *heapowner.HotOwner
	PackAuthority() *packowner.HotOwner
	EffectAuthority() *effectowner.HotOwner
	PlacementAuthority() *placementowner.HotOwner
}

// RuleEntry is this package's call-activation rule declaration. P and A are
// the composition's own principal and authority records, admitted by the need
// interfaces above.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:    "call-activation",
		Writes: "call",
		Owner:  "call",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/call-activation", Requirement: "program-requirement/unrestricted", Form: "program-form/call-summary"},
		},
		Lane:     rule.LaneActivation,
		Semantic: "semantic/activation/call-body",
		Roles:    []schema.Key{"semantic/activation-family/call-body"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	activation, activationOK := context.Roles.Key("semantic/activation/call-body")
	family, familyOK := context.Roles.Key("semantic/activation-family/call-body")
	if !activationOK || !familyOK {
		return nil, false
	}
	return DeclareSchema(builder, activation, family, context.Principals.CallPrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	slot := context.Fragment.ActivationSlot()
	capability, ok := engine.IssueActivationRuleCapability(binding, slot)
	if !ok || !engine.RegisterActivationRuleSlot(binding, slot, capability) {
		return engine.RuleSlotCapability{}, false
	}
	return capability, true
}

func BindRule[A ruleAuthorities](_ *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(context.Fragment, context.Authorities.CallAuthority(),
		context.Authorities.ValueAuthority().FactorRef(), context.Authorities.CallAuthority().FactorRef(),
		context.Authorities.HeapAuthority().FactorRef(), context.Authorities.PackAuthority().FactorRef(),
		context.Authorities.EffectAuthority().FactorRef(), context.Authorities.PlacementAuthority().FactorRef())
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the call-body activation and the family its variants are
// grouped under. A role is declared where it is used, so the row and the
// reference that names it are one package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(
		"activation/call-body",
		"activation-family/call-body",
	)
}
