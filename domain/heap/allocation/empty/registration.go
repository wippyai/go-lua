package empty

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
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

// RuleEntry is this package's heap-empty rule declaration. P and A are the
// composition's own principal and authority records, admitted by the need
// interfaces above.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:    "heap-empty",
		Writes: "heap",
		Owner:  "heap",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/allocation", Requirement: "requirement/unrestricted", Form: "issuance/local", Input: "input/finish", Stage: "stage/local", Code: uint64(heapdomain.AllocationFormEmpty), HasCode: true},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/heap/allocation-empty",
		Roles:    []schema.Key{"semantic/operand/heap/allocation-empty", "semantic/evidence/heap/allocation-empty", "semantic/transform/heap/allocation-empty"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	semantics, ok := context.Roles.Transformed("heap/allocation-empty")
	if !ok {
		return nil, false
	}
	return DeclareSchema(builder, semantics.Rule, semantics.Operand, semantics.Transform, semantics.Evidence, context.Principals.HeapPrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](_ *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(context.Fragment, context.Authorities.HeapAuthority(), context.Authorities.Allocations())
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the four roles its rule is identified by, the transform
// form included because its output is normalized before admission. A role is
// declared where it is used, so the row and the reference that names it are one
// package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.TransformedRuleRoleSpecs("heap/allocation-empty")
}
