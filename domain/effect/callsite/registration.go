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

// rulePrincipals is this package's own statement of the cold owners its rules
// declare against. All three call-site rules write the effect lane against the
// same call operand, so they share one statement of need.
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

// SelectedEntry is this package's effect-selected rule declaration. P and A are
// the composition's own principal and authority records, admitted by the need
// interfaces above.
func SelectedEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:    "effect-selected",
		Writes: "effect",
		Owner:  "effect",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/call", Requirement: "requirement/unrestricted", Form: "issuance/call-stage", Input: "input/finish", Stage: "stage/call-effect"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/effect/callsite-selected",
		Roles:    []schema.Key{"semantic/operand/effect/callsite-selected", "semantic/evidence/effect/callsite-selected"},
	}
}

func DeclareSelected[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SelectedSchemaFragment, bool) {
	semantics, ok := context.Roles.Rule("effect/callsite-selected")
	if !ok {
		return nil, false
	}
	return DeclareSelectedSchema(builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.CallPrincipal(), context.Principals.EffectPrincipal())
}

func RegisterSelected(binding *engine.SchemaBinding, context rule.Registration[*SelectedSchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindSelected[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *SelectedSchemaFragment]) (*HotRule, bool) {
	return BindSelectedHot(binding, context.Fragment, context.Authorities.CallAuthority(), context.Authorities.EffectAuthority())
}

func FinalizeSelected[A ruleAuthorities](context rule.Finalization[A, *HotRule]) bool {
	return context.Rule != nil && !context.Rule.opaque && context.Rule.valid() &&
		context.Rule.calls == context.Authorities.CallAuthority() && context.Rule.effects == context.Authorities.EffectAuthority()
}

// OpaqueEntry is this package's effect-opaque rule declaration.
func OpaqueEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:    "effect-opaque",
		Writes: "effect",
		Owner:  "effect",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/call", Requirement: "requirement/unrestricted", Form: "issuance/call-stage", Input: "input/finish", Stage: "stage/call-effect"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/effect/callsite-opaque",
		Roles:    []schema.Key{"semantic/operand/effect/callsite-opaque", "semantic/evidence/effect/callsite-opaque"},
	}
}

func DeclareOpaque[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*OpaqueSchemaFragment, bool) {
	semantics, ok := context.Roles.Rule("effect/callsite-opaque")
	if !ok {
		return nil, false
	}
	return DeclareOpaqueSchema(builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.CallPrincipal(), context.Principals.EffectPrincipal())
}

func RegisterOpaque(binding *engine.SchemaBinding, context rule.Registration[*OpaqueSchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindOpaque[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *OpaqueSchemaFragment]) (*HotRule, bool) {
	return BindOpaqueHot(binding, context.Fragment, context.Authorities.CallAuthority(), context.Authorities.EffectAuthority())
}

func FinalizeOpaque[A ruleAuthorities](context rule.Finalization[A, *HotRule]) bool {
	return context.Rule != nil && context.Rule.opaque && context.Rule.valid() &&
		context.Rule.calls == context.Authorities.CallAuthority() && context.Rule.effects == context.Authorities.EffectAuthority()
}

// BodyEntry is this package's effect-body rule declaration.
func BodyEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:    "effect-body",
		Writes: "effect",
		Owner:  "effect",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/call", Requirement: "requirement/unrestricted", Form: "issuance/call-stage", Input: "input/finish", Stage: "stage/call-effect"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/effect/callsite-body",
		Roles:    []schema.Key{"semantic/operand/effect/callsite-body", "semantic/evidence/effect/callsite-body"},
	}
}

func DeclareBody[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*BodySchemaFragment, bool) {
	semantics, ok := context.Roles.Rule("effect/callsite-body")
	if !ok {
		return nil, false
	}
	return DeclareBodySchema(builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.CallPrincipal(), context.Principals.EffectPrincipal())
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
// role vocabulary: the three roles each of its three rules is identified by. A
// role is declared where it is used, so the row and the reference that names it
// are one package's statement.
func StructureSpecs() []structure.Spec {
	specs := vocabulary.RuleRoleSpecs("effect/callsite-selected")
	specs = append(specs, vocabulary.RuleRoleSpecs("effect/callsite-opaque")...)
	return append(specs, vocabulary.RuleRoleSpecs("effect/callsite-body")...)
}
