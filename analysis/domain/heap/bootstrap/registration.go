package bootstrap

import (
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
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
		Role:     programartifact.RuleRoleHeapBootstrap,
		Lane:     rule.LaneLink,
		Semantic: func(bundle vocabulary.Bundle) identity.SemanticKey { return bundle.HeapBootstrapRule.Rule },
		Declare: func(context rule.Declaration[P]) (*SchemaFragment, bool) {
			semantics := context.Bundle.HeapBootstrapRule
			return DeclareSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.HeapPrincipal())
		},
		Register: func(context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
			return rule.RegisterLinkSlot(context.Binding, context.Fragment.RuleSlot())
		},
		// The bootstrap plane is one transported pair. The join runs in the
		// pairing pass and resolves its partner by role identity, so neither
		// side depends on the other's position in the table.
		Pair: func(context rule.Pairing[*SchemaFragment]) bool {
			value, valueOK := context.Capability(programartifact.RuleRoleValueBootstrap)
			heap, heapOK := context.Capability(programartifact.RuleRoleHeapBootstrap)
			return valueOK && heapOK && engine.RegisterLinkBootstrapTransportPair(context.Binding, value, heap)
		},
		Bind: func(context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
			return BindHot(context.Fragment, context.Authorities.HeapAuthority())
		},
		Finalize: func(context rule.Finalization[A, *HotRule]) bool {
			catalog := context.Rule.Catalog()
			return catalog != nil && catalog.FencedTo(context.Authorities.HeapSchema())
		},
		LinkAttach: func(context rule.LinkAttach[*HotRule]) bool {
			_, ok := context.Rule.AttachLinkOccurrence(context.Assembly, context.Occurrence)
			return ok
		},
		LinkMember: func(context rule.LinkMember[*HotRule]) bool {
			_, ok := context.Rule.AttachLinkReceiptMember(context.Compilation, context.Graph, context.Occurrence)
			return ok
		},
		LinkCatalog: func(hot *HotRule) (rule.LinkCatalog, bool) {
			catalog := hot.Catalog()
			return catalog, catalog != nil
		},
	}
}
