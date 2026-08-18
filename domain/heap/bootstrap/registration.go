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
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec[P, A, *SchemaFragment, *HotRule] {
	return rule.Spec[P, A, *SchemaFragment, *HotRule]{
		Key:      "heap-bootstrap",
		Writes:   "heap",
		Owner:    "heap",
		Lane:     rule.LaneLink,
		Semantic: "semantic/rule/heap/host-bootstrap",
		Roles:    []schema.Key{"semantic/operand/heap/host-bootstrap", "semantic/evidence/heap/host-bootstrap"},
		Declare: func(context rule.Declaration[P]) (*SchemaFragment, bool) {
			semantics, ok := context.Roles.Rule("heap/host-bootstrap")
			if !ok {
				return nil, false
			}
			return DeclareSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.HeapPrincipal())
		},
		Register: func(context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
			return rule.RegisterLinkSlot(context.Binding, context.Fragment.RuleSlot())
		},
		// The bootstrap plane is one transported pair. The join runs in the
		// pairing pass and resolves its partner by the key that rule is declared
		// under, so neither side depends on the other's position in the table.
		Pair: func(context rule.Pairing[*SchemaFragment]) bool {
			value, valueOK := context.Capability("value-bootstrap")
			heap, heapOK := context.Capability("heap-bootstrap")
			return valueOK && heapOK && engine.RegisterLinkBootstrapTransportPair(context.Binding, value, heap)
		},
		Bind: func(context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
			return BindHot(context.Fragment, context.Authorities.HeapAuthority())
		},
		Finalize: func(context rule.Finalization[A, *HotRule]) bool {
			catalog := context.Rule.Catalog()
			return catalog != nil && catalog.FencedTo(context.Authorities.HeapSchema())
		},
		LinkCatalog: func(hot *HotRule) (rule.LinkCatalog, bool) {
			catalog := hot.Catalog()
			return catalog, catalog != nil
		},
	}
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the three roles its rule is identified by. A role is
// declared where it is used, so the row and the reference that names it are one
// package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.RuleRoleSpecs("heap/host-bootstrap")
}
