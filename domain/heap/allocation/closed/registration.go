package closed

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// rulePrincipals is this package's own statement of the cold owners its rule
// declares against. It names only peer types this package already speaks, so
// the composition that supplies the principal record satisfies it structurally
// and neither side learns the other's shape.
type rulePrincipals interface {
	HeapPrincipal() *heapowner.SchemaFragment
	ValuePrincipal() *valueowner.SchemaFragment
}

// ruleAuthorities is the sealed authority set this rule binds against, stated
// the same way.
type ruleAuthorities interface {
	HeapAuthority() *heapowner.HotOwner
	ValueAuthority() *valueowner.HotOwner
	Allocations() *allocationcatalog.Catalog
}

// RuleEntry is this package's heap-closed rule declaration. P and A are the
// composition's own principal and authority records, admitted by the need
// interfaces above.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec[P, A, *SchemaFragment, *HotRule] {
	return rule.Spec[P, A, *SchemaFragment, *HotRule]{
		Key:    "heap-closed",
		Writes: "heap",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/allocation", Form: "issuance/local", Input: "input/finish", Stage: "stage/local", Code: uint64(flow.AllocationFormClosed), HasCode: true},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/heap/allocation-closed",
		Roles:    []schema.Key{"semantic/operand/heap/allocation-closed", "semantic/evidence/heap/allocation-closed", "semantic/transform/heap/allocation-closed"},
		Declare: func(context rule.Declaration[P]) (*SchemaFragment, bool) {
			semantics, ok := context.Roles.Transformed("heap/allocation-closed")
			if !ok {
				return nil, false
			}
			return DeclareSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Transform, semantics.Evidence, context.Principals.HeapPrincipal(), context.Principals.ValuePrincipal())
		},
		Register: func(context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
			return rule.RegisterMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
			return BindHot(context.Binding, context.Fragment, context.Authorities.HeapAuthority(), context.Authorities.ValueAuthority(), context.Authorities.Allocations())
		},
		Attach: func(context rule.Attach[*HotRule]) bool {
			_, ok := context.Rule.AttachMountedOccurrence(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*HotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the four roles its rule is identified by, the transform
// form included because its output is normalized before admission. A role is
// declared where it is used, so the row and the reference that names it are one
// package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.TransformedRuleRoleSpecs("heap/allocation-closed")
}
