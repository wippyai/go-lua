package index

import (
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// rawGetPrincipals is this package's own statement of the cold owners the
// raw-get rule declares against. It names only peer types this package already
// speaks, so the composition that supplies the principal record satisfies it
// structurally and neither side learns the other's shape.
type rawGetPrincipals interface {
	ValuePrincipal() *valueowner.SchemaFragment
	CallPrincipal() *callowner.SchemaFragment
	HeapPrincipal() *heapowner.SchemaFragment
	PackPrincipal() *packowner.SchemaFragment
}

// rawGetAuthorities is the sealed authority set the raw-get rule binds
// against, stated the same way.
type rawGetAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
	HeapAuthority() *heapowner.HotOwner
	PackAuthority() *packowner.HotOwner
	Topology() *Topology
}

// RawGetEntry is this package's raw-get rule declaration. P and A are the
// composition's own principal and authority records, admitted by the need
// interfaces above.
func RawGetEntry[P rawGetPrincipals, A rawGetAuthorities]() rule.Spec[P, A, *RawGetSchemaFragment, *RawGetHotRule] {
	return rule.Spec[P, A, *RawGetSchemaFragment, *RawGetHotRule]{
		Key:      "raw-get",
		Role:     programartifact.RuleRoleRawGet,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) identity.SemanticKey { return bundle.RawGetRule.Rule },
		Declare: func(context rule.Declaration[P]) (*RawGetSchemaFragment, bool) {
			semantics := context.Bundle.RawGetRule
			return DeclareRawGetSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.ValuePrincipal(), context.Principals.CallPrincipal(), context.Principals.HeapPrincipal(), context.Principals.PackPrincipal())
		},
		Register: func(context rule.Registration[*RawGetSchemaFragment]) (engine.RuleSlotCapability, bool) {
			return rule.RegisterMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[A, *RawGetSchemaFragment]) (*RawGetHotRule, bool) {
			return BindRawGetHot(context.Binding, context.Fragment, context.Authorities.Topology(), context.Authorities.ValueAuthority(), context.Authorities.CallAuthority(), context.Authorities.HeapAuthority(), context.Authorities.PackAuthority())
		},
		Attach: func(context rule.Attach[*RawGetHotRule]) bool {
			_, ok := context.Rule.AttachMountedOccurrence(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*RawGetHotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}
}

// rawSetPrincipals is the cold owner set the raw-set rule declares against. It
// writes the heap rather than reading a call target, so it names one principal
// fewer than raw-get.
type rawSetPrincipals interface {
	ValuePrincipal() *valueowner.SchemaFragment
	HeapPrincipal() *heapowner.SchemaFragment
	PackPrincipal() *packowner.SchemaFragment
}

// rawSetAuthorities is the sealed authority set the raw-set rule binds
// against.
type rawSetAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	HeapAuthority() *heapowner.HotOwner
	PackAuthority() *packowner.HotOwner
	Topology() *Topology
}

// RawSetEntry is this package's raw-set rule declaration.
func RawSetEntry[P rawSetPrincipals, A rawSetAuthorities]() rule.Spec[P, A, *RawSetSchemaFragment, *RawSetHotRule] {
	return rule.Spec[P, A, *RawSetSchemaFragment, *RawSetHotRule]{
		Key:      "raw-set",
		Role:     programartifact.RuleRoleRawSet,
		Lane:     rule.LaneMounted,
		Semantic: func(bundle vocabulary.Bundle) identity.SemanticKey { return bundle.RawSetRule.Rule },
		Declare: func(context rule.Declaration[P]) (*RawSetSchemaFragment, bool) {
			semantics := context.Bundle.RawSetRule
			return DeclareRawSetSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.ValuePrincipal(), context.Principals.HeapPrincipal(), context.Principals.PackPrincipal())
		},
		Register: func(context rule.Registration[*RawSetSchemaFragment]) (engine.RuleSlotCapability, bool) {
			return rule.RegisterMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[A, *RawSetSchemaFragment]) (*RawSetHotRule, bool) {
			return BindRawSetHot(context.Binding, context.Fragment, context.Authorities.Topology(), context.Authorities.ValueAuthority(), context.Authorities.HeapAuthority(), context.Authorities.PackAuthority())
		},
		Attach: func(context rule.Attach[*RawSetHotRule]) bool {
			_, ok := context.Rule.AttachMountedOccurrence(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*RawSetHotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}
}
