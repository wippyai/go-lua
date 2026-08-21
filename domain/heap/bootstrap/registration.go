package bootstrap

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
)

// rulePrincipals is this package's own statement of the cold owners its rule
// declares against. It names only peer types this package already speaks, so
// the composition that supplies the principal record satisfies it structurally
// and neither side learns the other's shape.
type rulePrincipals interface {
	HeapPrincipal() *heapowner.SchemaFragment
}

// ruleAuthorities is the sealed authority set this rule binds against, stated
// the same way. The heap schema is the fence the sealed occurrence catalog is
// proved against.
type ruleAuthorities interface {
	HeapAuthority() *heapowner.HotOwner
	HeapSchema() heapdomain.Schema
}

// RuleEntry is this package's heap-bootstrap rule declaration. P and A are the
// composition's own principal and authority records, admitted by the need
// interfaces above.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:      "heap-bootstrap",
		Writes:   "heap",
		Owner:    "heap",
		Lane:     rule.LaneLink,
		Semantic: "semantic/rule/heap/host-bootstrap",
		Roles:    []schema.Key{"semantic/operand/heap/host-bootstrap"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	semantic, semanticOK := context.Roles.Key("semantic/rule/heap/host-bootstrap")
	operand, operandOK := context.Roles.Key("semantic/operand/heap/host-bootstrap")
	if !semanticOK || !operandOK {
		return nil, false
	}
	return DeclareSchema(builder, semantic, operand, context.Principals.HeapPrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterLinkSlot(binding, context.Fragment.RuleSlot())
}

func PairRule(binding *engine.SchemaBinding, _ rule.Pairing[*SchemaFragment], resolve func(schema.Key) (engine.RuleSlotCapability, bool)) bool {
	value, valueOK := resolve("value-bootstrap")
	heap, heapOK := resolve("heap-bootstrap")
	return valueOK && heapOK && engine.RegisterLinkBootstrapTransportPair(binding, value, heap)
}

func BindRule[A ruleAuthorities](_ *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(context.Fragment, context.Authorities.HeapAuthority())
}

func FinalizeRule[A ruleAuthorities](context rule.Finalization[A, *HotRule]) bool {
	catalog := context.Rule.Catalog()
	return catalog != nil && catalog.FencedTo(context.Authorities.HeapSchema())
}

func LinkCatalog(hot *HotRule) (rule.LinkCatalog, bool) {
	catalog := hot.Catalog()
	return catalog, catalog != nil
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the two roles its rule is identified by. A role is
// declared where it is used, so the row and the reference that names it are one
// package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/heap/host-bootstrap", "operand/heap/host-bootstrap")
}
