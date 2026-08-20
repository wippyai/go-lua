package containment

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
)

type rulePrincipals interface {
	PlacementPrincipal() *placementowner.SchemaFragment
	HeapPrincipal() *heapowner.SchemaFragment
}

type ruleAuthorities interface {
	PlacementAuthority() *placementowner.HotOwner
	HeapAuthority() *heapowner.HotOwner
	PlacementSchema() placementdomain.Schema
	HeapSchema() heapdomain.Schema
}

// RuleEntry is the Link-lane Placement containment transport. Its parent
// denominator is the complete Heap allocation-root view, not a mounted
// program occurrence stream.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:      "placement-containment",
		Writes:   "placement",
		Owner:    "placement",
		Lane:     rule.LaneLink,
		Semantic: "semantic/rule/placement/containment",
		Roles:    []schema.Key{"semantic/operand/placement/containment"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	semantic, semanticOK := context.Roles.Key(vocabulary.RoleKey("rule/placement/containment"))
	operand, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/placement/containment"))
	if !semanticOK || !operandOK {
		return nil, false
	}
	return DeclareSchema(builder, semantic, operand, context.Principals.PlacementPrincipal(), context.Principals.HeapPrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterLinkSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(binding, context.Fragment, context.Authorities.PlacementAuthority(), context.Authorities.HeapAuthority(), context.Authorities.PlacementSchema())
}

func FinalizeRule[A ruleAuthorities](context rule.Finalization[A, *HotRule]) bool {
	return context.Rule != nil && context.Rule.owner == context.Authorities.PlacementAuthority() &&
		context.Rule.heap == context.Authorities.HeapAuthority() &&
		context.Authorities.HeapSchema() == context.Authorities.PlacementSchema().Heap()
}

func LinkCatalog(hot *HotRule) (rule.LinkCatalog, bool) {
	if hot == nil {
		return nil, false
	}
	return hot, true
}

// StructureSpecs contributes the containment rule's rule and operand
// identities to the semantic role vocabulary.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/placement/containment", "operand/placement/containment")
}
