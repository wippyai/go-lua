package callsite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
)

// rulePrincipals is this package's own statement of the cold owners its rule
// declares against.
type rulePrincipals interface {
	CallPrincipal() *callowner.SchemaFragment
	EffectPrincipal() *effectowner.SchemaFragment
}

// ruleAuthorities is the sealed authority set these rules bind against, stated
// the same way.
type ruleAuthorities interface {
	CallAuthority() *callowner.HotOwner
	EffectAuthority() *effectowner.HotOwner
}

// BodyEntry is this package's effect-body rule declaration.
func BodyEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:    "effect-body",
		Writes: "effect",
		Owner:  "effect",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/call", Requirement: "program-requirement/unrestricted", Form: "program-form/call-effect"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/effect/callsite-body",
		Roles:    []schema.Key{"semantic/operand/effect/callsite-body"},
	}
}

func DeclareBody[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*BodySchemaFragment, bool) {
	semantic, semanticOK := context.Roles.Key("semantic/rule/effect/callsite-body")
	operand, operandOK := context.Roles.Key("semantic/operand/effect/callsite-body")
	if !semanticOK || !operandOK {
		return nil, false
	}
	return DeclareBodySchema(builder, semantic, operand, context.Principals.CallPrincipal(), context.Principals.EffectPrincipal())
}

func RegisterBody(binding *engine.SchemaBinding, context rule.Registration[*BodySchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindBody[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *BodySchemaFragment]) (*BodyHotRule, bool) {
	return BindBodyHot(binding, context.Fragment, context.Authorities.CallAuthority(), context.Authorities.EffectAuthority())
}

func FinalizeBody[A ruleAuthorities](context rule.Finalization[A, *BodyHotRule]) bool {
	return context.Rule != nil && context.Rule.valid() &&
		context.Rule.calls == context.Authorities.CallAuthority() && context.Rule.effects == context.Authorities.EffectAuthority()
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the rule and operand roles identifying its one remaining
// schema-bound rule. A role is declared where it is used, so the row and the
// reference that names it are one package's statement; the two exact call-site
// rules declare theirs in their own declaration packages.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/effect/callsite-body", "operand/effect/callsite-body")
}
