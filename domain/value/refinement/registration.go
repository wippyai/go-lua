package refinement

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
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
}

// RuleEntry is this package's value-presence-refinement rule declaration. P
// and A are the composition's own principal and authority records, admitted by
// the need interfaces above.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:    "value-presence-refinement",
		Writes: "value",
		Owner:  "value",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/binary-presence-refinement", Requirement: "requirement/unrestricted", Form: "issuance/local-predecessor", Input: "input/predecessor", Stage: "stage/local"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/value/presence-refinement",
		Roles:    []schema.Key{"semantic/operand/value/presence-refinement", "semantic/evidence/value/presence-refinement"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	semantics, ok := context.Roles.Rule("value/presence-refinement")
	if !ok {
		return nil, false
	}
	return DeclareSchema(builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.ValuePrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](_ *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(context.Fragment, context.Authorities.ValueAuthority())
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the three roles its rule is identified by. A role is
// declared where it is used, so the row and the reference that names it are one
// package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.RuleRoleSpecs("value/presence-refinement")
}
