// Package field declares Static's mounted direct-field type-fact transfer.
// Heap/index owns the candidate geometry; Static owns the TypeFact projection.
package field

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	heapindex "github.com/wippyai/go-lua/domain/heap/index"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	staticowner "github.com/wippyai/go-lua/domain/static/owner"
)

type rulePrincipals interface {
	StaticTypePrincipal() *staticowner.SchemaFragment
}

type ruleAuthorities interface {
	StaticTypeAuthority() *staticowner.HotOwner
	HeapAuthority() *heapowner.HotOwner
	Topology() *heapindex.Topology
}

// RuleEntry declares the one mounted direct-field projection. Its occurrence
// is the existing index-read candidate; no separate field/index occurrence or
// Placement axis is introduced.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:      "static-field",
		Writes:   "static-type",
		Owner:    "heap",
		Issues:   []rule.Issuance{heapindex.IndexReadIssuance()},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/static/field",
		Roles:    []schema.Key{vocabulary.RoleKey("operand/static/field")},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	semantic, semanticOK := context.Roles.Key(vocabulary.RoleKey("rule/static/field"))
	operandFamily, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/static/field"))
	if !semanticOK || !operandOK {
		return nil, false
	}
	return DeclareSchema(builder, semantic, operandFamily, context.Principals.StaticTypePrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(binding, context.Fragment, context.Authorities.StaticTypeAuthority(), context.Authorities.HeapAuthority(), context.Authorities.Topology())
}

// StructureSpecs contributes only this rule's own semantic identity and its
// operand-family identity. The index-read occurrence vocabulary is owned by
// Heap/index and is deliberately not copied here.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/static/field", "operand/static/field")
}
