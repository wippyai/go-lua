package ingress

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
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
// the same way.
type ruleAuthorities interface {
	HeapAuthority() *heapowner.HotOwner
	Allocations() *allocationcatalog.Catalog
}

// RuleEntry is this package's heap-ingress rule declaration. P and A are the
// composition's own principal and authority records, admitted by the need
// interfaces above.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec[P, A, *SchemaFragment, *HotRule] {
	return rule.Spec[P, A, *SchemaFragment, *HotRule]{
		Key:    "heap-ingress",
		Writes: "heap",
		Owner:  "heap",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/allocation", Form: "issuance/base", Input: "input/none", Stage: "stage/base"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/heap/allocation-ingress",
		Roles:    []schema.Key{"semantic/operand/heap/allocation-ingress", "semantic/evidence/heap/allocation-ingress"},
		Declare: func(context rule.Declaration[P]) (*SchemaFragment, bool) {
			semantics, ok := context.Roles.Rule("heap/allocation-ingress")
			if !ok {
				return nil, false
			}
			return DeclareSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.HeapPrincipal())
		},
		Register: func(context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
			return rule.RegisterMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
			return BindHot(context.Fragment, context.Authorities.HeapAuthority())
		},
		Finalize: func(context rule.Finalization[A, *HotRule]) bool {
			return context.Rule.AttachCatalog(context.Authorities.Allocations())
		},

	}
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the three roles its rule is identified by. A role is
// declared where it is used, so the row and the reference that names it are one
// package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.RuleRoleSpecs("heap/allocation-ingress")
}
