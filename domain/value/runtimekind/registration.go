// Package runtimekind declares Value's cross-axis runtime-kind call rule.
//
// The package owns the rule declaration and transfer judgment.  Call and
// Value enter only through their principal/authority surfaces; no Program
// row, target contract, or runtime-kind spelling is reconstructed here.
package runtimekind

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type rulePrincipals interface {
	ValuePrincipal() *valueowner.SchemaFragment
	CallPrincipal() *callowner.SchemaFragment
}

type ruleAuthorities interface {
	ValueAuthority() *valueowner.HotOwner
	CallAuthority() *callowner.HotOwner
}

// RuleEntry declares the Value-owned rule that projects a known call's
// schema-declared runtime-kind behavior onto the call result Value.
func RuleEntry[P rulePrincipals, A ruleAuthorities]() rule.Spec {
	return rule.Spec{
		Key:    "value-runtime-kind-call",
		Writes: "value",
		Owner:  "value",
		// The runtime-kind projection reads the sole actual of a strict unary
		// plain call: Value seals an operand for exactly that geometry, so the
		// subscription requires it and a method, nullary, multi-argument, or
		// tail-expanded call issues nothing here.
		Issues: []rule.Issuance{{
			Occurrence:  "occurrence/call",
			Requirement: "requirement/call-plain-unary",
			Form:        "issuance/call-stage",
			Input:       "input/finish",
			Stage:       "stage/call-summary",
		}},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/value/runtime-kind-call",
		Roles: []schema.Key{
			"semantic/operand/value/runtime-kind-call",
			"semantic/evidence/value/runtime-kind-call",
		},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	semantics, ok := context.Roles.Rule("value/runtime-kind-call")
	if !ok {
		return nil, false
	}
	return DeclareSchema(builder, semantics.Rule, semantics.Operand, semantics.Evidence, context.Principals.ValuePrincipal(), context.Principals.CallPrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](_ *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(context.Fragment, context.Authorities.ValueAuthority(), context.Authorities.CallAuthority())
}

// StructureSpecs contributes the three semantic identities owned by this
// rule.  The runtime-kind result relation itself is declared by the runtimekind
// domain; this rule consumes that opaque identity through Call's projection.
func StructureSpecs() []structure.Spec {
	return vocabulary.RuleRoleSpecs("value/runtime-kind-call")
}
