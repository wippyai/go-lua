package index

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
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
		Key:    "raw-get",
		Writes: "value",
		Owner:  "heap",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/index-read", Form: "issuance/local", Input: "input/entry", Stage: "stage/local"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/heap/index-get-raw",
		Roles:    []schema.Key{"semantic/operand/heap/index-get-raw", "semantic/evidence/heap/index-get-raw"},
		Declare: func(context rule.Declaration[P]) (*RawGetSchemaFragment, bool) {
			semantics, ok := context.Roles.Rule("heap/index-get-raw")
			if !ok {
				return nil, false
			}
			return DeclareRawGetSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.ValuePrincipal(), context.Principals.CallPrincipal(), context.Principals.HeapPrincipal(), context.Principals.PackPrincipal())
		},
		Register: func(context rule.Registration[*RawGetSchemaFragment]) (engine.RuleSlotCapability, bool) {
			return rule.RegisterMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[A, *RawGetSchemaFragment]) (*RawGetHotRule, bool) {
			return BindRawGetHot(context.Binding, context.Fragment, context.Authorities.Topology(), context.Authorities.ValueAuthority(), context.Authorities.CallAuthority(), context.Authorities.HeapAuthority(), context.Authorities.PackAuthority())
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
		Key:    "raw-set",
		Writes: "heap",
		Owner:  "heap",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/index-write", Form: "issuance/local-predecessor", Input: "input/predecessor", Stage: "stage/local"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/heap/index-set-raw",
		Roles:    []schema.Key{"semantic/operand/heap/index-set-raw", "semantic/evidence/heap/index-set-raw"},
		Declare: func(context rule.Declaration[P]) (*RawSetSchemaFragment, bool) {
			semantics, ok := context.Roles.Rule("heap/index-set-raw")
			if !ok {
				return nil, false
			}
			return DeclareRawSetSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.ValuePrincipal(), context.Principals.HeapPrincipal(), context.Principals.PackPrincipal())
		},
		Register: func(context rule.Registration[*RawSetSchemaFragment]) (engine.RuleSlotCapability, bool) {
			return rule.RegisterMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[A, *RawSetSchemaFragment]) (*RawSetHotRule, bool) {
			return BindRawSetHot(context.Binding, context.Fragment, context.Authorities.Topology(), context.Authorities.ValueAuthority(), context.Authorities.HeapAuthority(), context.Authorities.PackAuthority())
		},
	}
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the three roles each of its two rules is identified by. A
// role is declared where it is used, so the row and the reference that names it
// are one package's statement.
func StructureSpecs() []structure.Spec {
	return append(
		vocabulary.RuleRoleSpecs("heap/index-get-raw"),
		vocabulary.RuleRoleSpecs("heap/index-set-raw")...,
	)
}
