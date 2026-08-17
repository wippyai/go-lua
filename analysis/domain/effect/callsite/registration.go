package callsite

import (
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	effectowner "github.com/wippyai/go-lua/analysis/domain/effect/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/rule"
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
		Key:      "effect-selected",
		Role:     programartifact.RuleRoleEffectSelected,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) identity.SemanticKey { return bundle.EffectSelectedRule.Rule },
		Declare: func(context rule.Declaration[P]) (*SelectedSchemaFragment, bool) {
			semantics := context.Bundle.EffectSelectedRule
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
		Key:      "effect-opaque",
		Role:     programartifact.RuleRoleEffectOpaque,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) identity.SemanticKey { return bundle.EffectOpaqueRule.Rule },
		Declare: func(context rule.Declaration[P]) (*OpaqueSchemaFragment, bool) {
			semantics := context.Bundle.EffectOpaqueRule
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
		Key:      "effect-body",
		Role:     programartifact.RuleRoleEffectBody,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) identity.SemanticKey { return bundle.EffectBodyRule.Rule },
		Declare: func(context rule.Declaration[P]) (*BodySchemaFragment, bool) {
			semantics := context.Bundle.EffectBodyRule
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
