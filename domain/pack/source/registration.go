package source

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
)

// rulePrincipals is this package's own statement of the cold owners its rule
// declares against. It names only peer types this package already speaks, so
// the composition that supplies the principal record satisfies it structurally
// and neither side learns the other's shape.
type rulePrincipals interface {
	PackPrincipal() *packowner.SchemaFragment
}

// ruleAuthorities is the sealed authority set this rule binds against, stated
// the same way.
type ruleAuthorities interface {
	PackAuthority() *packowner.HotOwner
	PackSchema() *packdomain.Schema
}

// RuleEntry is this package's pack-source rule declaration. P and A are the
// composition's own principal and authority records, admitted by the need
// interfaces above.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:    "pack-source",
		Writes: "pack",
		Owner:  "pack",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/values", Requirement: "program-requirement/unrestricted", Form: "program-form/base-none-allow-empty"},
			{Occurrence: "occurrence/call", Requirement: "program-requirement/unrestricted", Form: "program-form/base-none"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/pack/source",
		Roles:    []schema.Key{"semantic/operand/pack/source"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	semantic, semanticOK := context.Roles.Key("semantic/rule/pack/source")
	operand, operandOK := context.Roles.Key("semantic/operand/pack/source")
	if !semanticOK || !operandOK {
		return nil, false
	}
	return DeclareSchema(builder, semantic, operand, context.Principals.PackPrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](_ *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(context.Fragment, context.Authorities.PackAuthority(), context.Authorities.PackSchema())
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the rule and operand roles its rule is identified by. A role is
// declared where it is used, so the row and the reference that names it are one
// package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/pack/source", "operand/pack/source")
}
