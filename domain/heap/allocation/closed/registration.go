package closed

import (
	"github.com/wippyai/go-lua/analysis/engine"
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
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:    "heap-closed",
		Writes: "heap",
		Owner:  "heap",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/allocation-closed", Requirement: "program-requirement/unrestricted", Form: "program-form/local-successor"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/heap/allocation-closed",
		Roles:    []schema.Key{"semantic/operand/heap/allocation-closed", "semantic/transform/heap/allocation-closed"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	semantic, semanticOK := context.Roles.Key("semantic/rule/heap/allocation-closed")
	operand, operandOK := context.Roles.Key("semantic/operand/heap/allocation-closed")
	transform, transformOK := context.Roles.Key("semantic/transform/heap/allocation-closed")
	if !semanticOK || !operandOK || !transformOK {
		return nil, false
	}
	return DeclareSchema(builder, semantic, operand, transform, context.Principals.HeapPrincipal(), context.Principals.ValuePrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(binding, context.Fragment, context.Authorities.HeapAuthority(), context.Authorities.ValueAuthority(), context.Authorities.Allocations())
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the rule, operand, and transform roles its rule is identified by,
// the
// form included because its output is normalized before publication. A role is
// declared where it is used, so the row and the reference that names it are one
// package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/heap/allocation-closed", "operand/heap/allocation-closed", "transform/heap/allocation-closed")
}
