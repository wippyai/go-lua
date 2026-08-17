package bootstrap

import (
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
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
	ValuePrincipal() *valueowner.SchemaFragment
}

// ruleAuthorities is the sealed authority set this rule binds against, stated
// the same way. The value schema is the fence the sealed occurrence catalog is
// proved against.
type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	ValueSchema() *valuedomain.Schema
}

// RuleEntry is this package's value-bootstrap rule declaration. P and A are
// the composition's own principal and authority records, admitted by the need
// interfaces above.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec[P, A, *SchemaFragment, *HotRule] {
	return rule.Spec[P, A, *SchemaFragment, *HotRule]{
		Key:      "value-bootstrap",
		Role:     programartifact.RuleRoleValueBootstrap,
		Lane:     rule.LaneLink,
		Semantic: func(bundle vocabulary.Bundle) identity.SemanticKey { return bundle.ValueBootstrapRule.Rule },
		Declare: func(context rule.Declaration[P]) (*SchemaFragment, bool) {
			semantics := context.Bundle.ValueBootstrapRule
			return DeclareSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.ValuePrincipal())
		},
		Register: func(context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
			return rule.RegisterLinkSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
			return BindHot(context.Fragment, context.Authorities.ValueAuthority())
		},
		Finalize: func(context rule.Finalization[A, *HotRule]) bool {
			catalog := context.Rule.Catalog()
			return catalog != nil && catalog.FencedTo(context.Authorities.ValueSchema())
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
