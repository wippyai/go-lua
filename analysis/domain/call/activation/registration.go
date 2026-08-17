package activation

import (
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	effectowner "github.com/wippyai/go-lua/analysis/domain/effect/owner"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// rulePrincipals is this package's own statement of the cold owners its rule
// declares against. The activation shape is declared against the call lane
// alone.
type rulePrincipals interface {
	CallPrincipal() *callowner.SchemaFragment
}

// ruleAuthorities is the sealed authority set this rule binds against. The
// activation plane transports every factor lane across a call boundary, so it
// names all five factor authorities and its own target batch catalog.
type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
	HeapAuthority() *heapowner.HotOwner
	PackAuthority() *packowner.HotOwner
	EffectAuthority() *effectowner.HotOwner
	ActivationCatalog() *TargetBatchCatalog
}

// RuleEntry is this package's call-activation rule declaration. P and A are
// the composition's own principal and authority records, admitted by the need
// interfaces above.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec[P, A, *SchemaFragment, *HotRule] {
	return rule.Spec[P, A, *SchemaFragment, *HotRule]{
		Key:      "call-activation",
		Role:     programartifact.RuleRoleCallActivation,
		Lane:     rule.LaneActivation,
		Semantic: func(bundle vocabulary.Bundle) identity.SemanticKey { return bundle.CallActivation },
		Declare: func(context rule.Declaration[P]) (*SchemaFragment, bool) {
			return DeclareSchema(context.Builder, context.Bundle.CallActivation, context.Bundle.CallActivationFamily, context.Bundle.CallActivationAdmission, context.Principals.CallPrincipal())
		},
		Register: func(context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
			slot := context.Fragment.ActivationSlot()
			capability, ok := engine.IssueActivationRuleCapability(context.Binding, slot)
			if !ok || !engine.RegisterActivationRuleSlot(context.Binding, slot, capability) {
				return engine.RuleSlotCapability{}, false
			}
			return capability, true
		},
		Bind: func(context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
			hot, ok := BindHot(context.Fragment, context.Authorities.CallAuthority(), context.Authorities.ActivationCatalog())
			if !ok {
				return nil, false
			}
			if !BindMountedTransport(hot, context.Authorities.ValueAuthority().FactorRef(), context.Authorities.CallAuthority().FactorRef(), context.Authorities.HeapAuthority().FactorRef(), context.Authorities.PackAuthority().FactorRef(), context.Authorities.EffectAuthority().FactorRef()) {
				return nil, false
			}
			return hot, true
		},
		Finalize: func(context rule.Finalization[A, *HotRule]) bool {
			return context.Rule.SealOccurrenceReceipts()
		},
		Attach: func(context rule.Attach[*HotRule]) bool {
			return context.Rule.AttachMountedOccurrence(context.Assembly, context.Mount, context.Point, context.Occurrence)
		},
		Member: func(context rule.Member[*HotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}
}
