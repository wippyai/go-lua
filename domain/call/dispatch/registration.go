package dispatch

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// rulePrincipals is this package's own statement of the cold owners its rule
// declares against. It names only peer types this package already speaks, so
// the composition that supplies the principal record satisfies it structurally
// and neither side learns the other's shape.
type rulePrincipals interface {
	ValuePrincipal() *valueowner.SchemaFragment
	CallPrincipal() *callowner.SchemaFragment
}

// ruleAuthorities is the sealed authority set this rule binds against, stated
// the same way.
type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
	HeapSchema() heapdomain.Schema
	PackSchema() *packdomain.Schema
}

// RuleEntry is this package's call-dispatch rule declaration. P and A are the
// composition's own principal and authority records, admitted by the need
// interfaces above.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:    "call-dispatch",
		Writes: "call",
		Owner:  "call",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/call", Requirement: "requirement/unrestricted", Form: "issuance/call-stage", Input: "input/finish", Stage: "stage/call-dispatch"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/call/dispatch",
		Roles:    []schema.Key{"semantic/operand/call/dispatch", "semantic/evidence/call/dispatch"},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	semantics, ok := context.Roles.Rule("call/dispatch")
	if !ok {
		return nil, false
	}
	return DeclareSchema(builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.ValuePrincipal(), context.Principals.CallPrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](binding *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(binding, context.Fragment, context.Authorities.ValueAuthority(), context.Authorities.CallAuthority(), context.Authorities.HeapSchema(), context.Authorities.PackSchema())
}

func FinalizeRule[A ruleAuthorities](context rule.Finalization[A, *HotRule]) bool {
	return context.Rule.SealOccurrenceReceipts()
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the three roles its rule is identified by. A role is
// declared where it is used, so the row and the reference that names it are one
// package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.RuleRoleSpecs("call/dispatch")
}
