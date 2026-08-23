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

// IndexReadIssuance is the canonical mounted occurrence consumed by rules
// whose operand is Heap/index's sealed read candidate. Keeping this declaration
// here prevents consumers from authoring a second, subtly divergent inventory
// of the index-read geometry.
func IndexReadIssuance() rule.Issuance {
	return rule.Issuance{
		Occurrence:  "occurrence/index-read",
		Requirement: "program-requirement/unrestricted",
		Form:        "program-form/computation",
	}
}

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
func RawGetEntry[P rawGetPrincipals, A rawGetAuthorities]() rule.Spec {
	return rule.Spec{
		Key:    "raw-get",
		Writes: "value",
		Owner:  "heap",
		Issues: []rule.Issuance{
			// An indexed read may consume a storage-read result issued at the
			// ordinary Local cut of the same point.  Local is immutable for one
			// execution cut, so place raw-get in its occurrence-specific
			// computation successor and read the predecessor finish; this gives
			// the receiver a complete local Value before raw-get writes the
			// indexed result that Call dispatch consumes.
			IndexReadIssuance(),
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/heap/index-get-raw",
		Roles:    []schema.Key{vocabulary.RoleKey("operand/heap/index-get-raw")},
	}
}

func DeclareRawGet[P rawGetPrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*RawGetSchemaFragment, bool) {
	semantic, semanticOK := context.Roles.Key(vocabulary.RoleKey("rule/heap/index-get-raw"))
	operandFamily, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/heap/index-get-raw"))
	if !semanticOK || !operandOK {
		return nil, false
	}
	return DeclareRawGetSchema(builder, semantic, operandFamily, context.Principals.ValuePrincipal(), context.Principals.CallPrincipal(), context.Principals.HeapPrincipal(), context.Principals.PackPrincipal())
}

func RegisterRawGet(binding *engine.SchemaBinding, context rule.Registration[*RawGetSchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRawGet[A rawGetAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *RawGetSchemaFragment]) (*RawGetHotRule, bool) {
	return BindRawGetHot(binding, context.Fragment, context.Authorities.Topology(), context.Authorities.ValueAuthority(), context.Authorities.CallAuthority(), context.Authorities.HeapAuthority(), context.Authorities.PackAuthority())
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
func RawSetEntry[P rawSetPrincipals, A rawSetAuthorities]() rule.Spec {
	return rule.Spec{
		Key:    "raw-set",
		Writes: "heap",
		Owner:  "heap",
		Issues: []rule.Issuance{
			// The receiver/key/RHS Value producers run at the occurrence's Local
			// cut. RawSet must therefore consume the canonical post-Local state;
			// a predecessor placement observes an absent receiver on acyclic
			// branches and only appears to work when recurrence replays the point.
			{Occurrence: "occurrence/index-write", Requirement: "program-requirement/unrestricted", Form: "program-form/local-successor"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/heap/index-set-raw",
		Roles:    []schema.Key{vocabulary.RoleKey("operand/heap/index-set-raw")},
	}
}

func DeclareRawSet[P rawSetPrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*RawSetSchemaFragment, bool) {
	semantic, semanticOK := context.Roles.Key(vocabulary.RoleKey("rule/heap/index-set-raw"))
	operandFamily, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/heap/index-set-raw"))
	if !semanticOK || !operandOK {
		return nil, false
	}
	return DeclareRawSetSchema(builder, semantic, operandFamily, context.Principals.ValuePrincipal(), context.Principals.HeapPrincipal(), context.Principals.PackPrincipal())
}

func RegisterRawSet(binding *engine.SchemaBinding, context rule.Registration[*RawSetSchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRawSet[A rawSetAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *RawSetSchemaFragment]) (*RawSetHotRule, bool) {
	return BindRawSetHot(binding, context.Fragment, context.Authorities.Topology(), context.Authorities.ValueAuthority(), context.Authorities.HeapAuthority(), context.Authorities.PackAuthority())
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the rule and operand roles each of its two rules is
// identified by. A
// role is declared where it is used, so the row and the reference that names it
// are one package's statement.
func StructureSpecs() []structure.Spec {
	return append(
		vocabulary.RoleSpecs("rule/heap/index-get-raw", "operand/heap/index-get-raw"),
		vocabulary.RoleSpecs("rule/heap/index-set-raw", "operand/heap/index-set-raw")...,
	)
}
