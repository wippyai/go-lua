package allocation

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// rulePrincipals is this package's own statement of the cold owners its rule
// declares against. It names only peer types this package already speaks, so
// the composition that supplies the principal record satisfies it structurally
// and neither side learns the other's shape.
type rulePrincipals interface {
	ValuePrincipal() *valueowner.SchemaFragment
}

// ruleAuthorities is the sealed authority set this rule binds against, stated
// the same way.
type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	HeapSchema() heapdomain.Schema
	Allocations() *allocationcatalog.Catalog
}

// RuleEntry is this package's value-allocation rule declaration. P and A are
// the composition's own principal and authority records, admitted by the need
// interfaces above.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:    "value-allocation",
		Writes: "value",
		Owner:  "value",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/allocation", Requirement: "requirement/unrestricted", Form: "issuance/local", Input: "input/entry", Stage: "stage/local"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/value/allocation",
		Roles:    []schema.Key{"semantic/operand/value/allocation", "semantic/transform/value/allocation"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	ruleSemantic, ruleOK := context.Roles.Key(vocabulary.RoleKey("rule/value/allocation"))
	operandFamily, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/value/allocation"))
	transform, transformOK := context.Roles.Key(vocabulary.RoleKey("transform/value/allocation"))
	if !ruleOK || !operandOK || !transformOK {
		return nil, false
	}
	return DeclareSchema(builder, ruleSemantic, operandFamily, transform, context.Principals.ValuePrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](_ *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(context.Fragment, context.Authorities.ValueAuthority(), context.Authorities.HeapSchema(), context.Authorities.Allocations())
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the rule, operand, and transform roles its rule is identified by,
// the
// form included because its output is normalized before carry publication. A role
// declared where it is used, so the row and the reference that names it are one
// package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/value/allocation", "operand/value/allocation", "transform/value/allocation")
}
