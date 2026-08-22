package bootstrap

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	valuedomain "github.com/wippyai/go-lua/domain/value"
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
// the same way. The value schema is the fence the sealed occurrence catalog is
// proved against.
type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	ValueSchema() *valuedomain.Schema
}

// RuleEntry is this package's value-bootstrap rule declaration. P and A are
// the composition's own principal and authority records, admitted by the need
// interfaces above.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:      "value-bootstrap",
		Writes:   "value",
		Owner:    "value",
		Lane:     rule.LaneLink,
		Semantic: "semantic/rule/value/host-global-bootstrap",
		Roles:    []schema.Key{"semantic/operand/value/host-global-bootstrap"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	ruleSemantic, ruleOK := context.Roles.Key(vocabulary.RoleKey("rule/value/host-global-bootstrap"))
	operandFamily, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/value/host-global-bootstrap"))
	if !ruleOK || !operandOK {
		return nil, false
	}
	return DeclareSchema(builder, ruleSemantic, operandFamily, context.Principals.ValuePrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterLinkSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](_ *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(context.Fragment, context.Authorities.ValueAuthority())
}

func FinalizeRule[A ruleAuthorities](context rule.Finalization[A, *HotRule]) bool {
	schema := context.Authorities.ValueSchema()
	return context.Rule != nil && context.Rule.owner != nil && schema != nil &&
		context.Rule.owner.Schema() == schema
}

func OccurrenceCatalog(hot *HotRule) (rule.OccurrenceCatalog, bool) {
	if hot == nil || hot.owner == nil || hot.owner.Schema() == nil {
		return nil, false
	}
	return hot, true
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the two roles its rule is identified by. A role is
// declared where it is used, so the row and the reference that names it are one
// package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/value/host-global-bootstrap", "operand/value/host-global-bootstrap")
}
