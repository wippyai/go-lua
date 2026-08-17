package equality

import (
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
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
// the same way.
type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
}

// RuleEntry is this package's value-binary-equality rule declaration. P and A
// are the composition's own principal and authority records, admitted by the
// need interfaces above.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec[P, A, *SchemaFragment, *HotRule] {
	return rule.Spec[P, A, *SchemaFragment, *HotRule]{
		Key:    "value-binary-equality",
		Writes: "value",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/binary-equality", Form: "issuance/computation", Input: "input/finish", Stage: "stage/local"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/value/binary-equality",
		Roles:    []schema.Key{"semantic/operand/value/binary-equality", "semantic/evidence/value/binary-equality"},
		Declare: func(context rule.Declaration[P]) (*SchemaFragment, bool) {
			semantics, ok := context.Roles.Rule("value/binary-equality")
			if !ok {
				return nil, false
			}
			return DeclareSchema(context.Builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.ValuePrincipal())
		},
		Register: func(context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
			return rule.RegisterMountedSlot(context.Binding, context.Fragment.RuleSlot())
		},
		Bind: func(context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
			return BindHot(context.Fragment, context.Authorities.ValueAuthority())
		},
		Attach: func(context rule.Attach[*HotRule]) bool {
			_, ok := context.Rule.AttachMountedRule(context.Assembly, context.Mount, context.Point, context.Occurrence)
			return ok
		},
		Member: func(context rule.Member[*HotRule]) bool {
			_, ok := context.Rule.AttachMountedReceiptMember(context.Compilation, context.Graph, context.Mount, context.Point, context.Occurrence)
			return ok
		},
	}
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the three roles its rule is identified by. A role is
// declared where it is used, so the row and the reference that names it are one
// package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.RuleRoleSpecs("value/binary-equality")
}
