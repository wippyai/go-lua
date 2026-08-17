package callsite

import (
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	effectowner "github.com/wippyai/go-lua/analysis/domain/effect/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
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
func SelectedEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec[P, A, *SelectedSchemaFragment, *HotRule] {
	return rule.Spec[P, A, *SelectedSchemaFragment, *HotRule]{
		Key:    "effect-selected",
		Writes: "effect",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/call", Form: "issuance/call-stage", Input: "input/finish", Stage: "stage/call-effect"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/effect/callsite-selected",
		Roles:    []schema.Key{"semantic/operand/effect/callsite-selected", "semantic/evidence/effect/callsite-selected"},
		Declare: func(context rule.Declaration[P]) (*SelectedSchemaFragment, bool) {
			semantics, ok := context.Roles.Rule("effect/callsite-selected")
			if !ok {
				return nil, false
			}
			return DeclareSelectedSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.CallPrincipal(), context.Principals.EffectPrincipal())
		},
		Register: func(context rule.Registration[*SelectedSchemaFragment]) (engine.RuleSlotCapability, bool) {
			return rule.RegisterMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[A, *SelectedSchemaFragment]) (*HotRule, bool) {
			return BindSelectedHot(context.Binding, context.Fragment, context.Authorities.CallAuthority(), context.Authorities.EffectAuthority())
		},
		Finalize: func(context rule.Finalization[A, *HotRule]) bool {
			return context.Rule.SealOccurrenceReceipts()
		},
		Attach: func(context rule.Attach[*HotRule]) bool {
			_, ok := context.Rule.AttachMountedOccurrence(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*HotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}
}

// OpaqueEntry is this package's effect-opaque rule declaration.
func OpaqueEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec[P, A, *OpaqueSchemaFragment, *HotRule] {
	return rule.Spec[P, A, *OpaqueSchemaFragment, *HotRule]{
		Key:    "effect-opaque",
		Writes: "effect",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/call", Form: "issuance/call-stage", Input: "input/finish", Stage: "stage/call-effect"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/effect/callsite-opaque",
		Roles:    []schema.Key{"semantic/operand/effect/callsite-opaque", "semantic/evidence/effect/callsite-opaque"},
		Declare: func(context rule.Declaration[P]) (*OpaqueSchemaFragment, bool) {
			semantics, ok := context.Roles.Rule("effect/callsite-opaque")
			if !ok {
				return nil, false
			}
			return DeclareOpaqueSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.CallPrincipal(), context.Principals.EffectPrincipal())
		},
		Register: func(context rule.Registration[*OpaqueSchemaFragment]) (engine.RuleSlotCapability, bool) {
			return rule.RegisterMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[A, *OpaqueSchemaFragment]) (*HotRule, bool) {
			return BindOpaqueHot(context.Binding, context.Fragment, context.Authorities.CallAuthority(), context.Authorities.EffectAuthority())
		},
		Finalize: func(context rule.Finalization[A, *HotRule]) bool {
			return context.Rule.SealOccurrenceReceipts()
		},
		Attach: func(context rule.Attach[*HotRule]) bool {
			_, ok := context.Rule.AttachMountedOccurrence(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*HotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}
}

// BodyEntry is this package's effect-body rule declaration.
func BodyEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec[P, A, *BodySchemaFragment, *BodyHotRule] {
	return rule.Spec[P, A, *BodySchemaFragment, *BodyHotRule]{
		Key:    "effect-body",
		Writes: "effect",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/call", Form: "issuance/call-stage", Input: "input/finish", Stage: "stage/call-effect"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/effect/callsite-body",
		Roles:    []schema.Key{"semantic/operand/effect/callsite-body", "semantic/evidence/effect/callsite-body"},
		Declare: func(context rule.Declaration[P]) (*BodySchemaFragment, bool) {
			semantics, ok := context.Roles.Rule("effect/callsite-body")
			if !ok {
				return nil, false
			}
			return DeclareBodySchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.CallPrincipal(), context.Principals.EffectPrincipal())
		},
		Register: func(context rule.Registration[*BodySchemaFragment]) (engine.RuleSlotCapability, bool) {
			return rule.RegisterMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[A, *BodySchemaFragment]) (*BodyHotRule, bool) {
			return BindBodyHot(context.Binding, context.Fragment, context.Authorities.CallAuthority(), context.Authorities.EffectAuthority())
		},
		Finalize: func(context rule.Finalization[A, *BodyHotRule]) bool {
			return context.Rule.SealOccurrenceReceipts()
		},
		Attach: func(context rule.Attach[*BodyHotRule]) bool {
			_, ok := context.Rule.AttachMountedOccurrence(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*BodyHotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}
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
