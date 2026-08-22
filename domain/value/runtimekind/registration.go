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
		// plain call and writes the call result: Value seals an operand for
		// exactly that geometry with a finite result slot, so the subscription
		// requires it. A method, nullary, multi-argument, tail-expanded, or
		// result-discarding call issues nothing here.
		//
		// The guarded arm of the same call is its own occurrence family, and
		// Value seals a refinement operand for every row of it, so the rule
		// subscribes to that family too. The arm is reached along its route
		// predecessor, which is where the narrowed subject Value is carried.
		Issues: []rule.Issuance{
			{
				Occurrence:  "occurrence/call",
				Requirement: "requirement/call-result-slot",
				Form:        "issuance/call-stage",
				Input:       "input/finish",
				Stage:       "stage/call-summary",
			},
			{
				Occurrence:  "occurrence/operation-predicate-refinement",
				Requirement: "requirement/unrestricted",
				Form:        "issuance/local-predecessor",
				Input:       "input/predecessor",
				Stage:       "stage/local",
			},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/value/runtime-kind-call",
		Roles: []schema.Key{
			"semantic/operand/value/runtime-kind-call",
		},
	}
}

func DeclareRule[P rulePrincipals](builder *engine.SchemaBuilder, context rule.Declaration[P]) (*SchemaFragment, bool) {
	ruleSemantic, ruleOK := context.Roles.Key(vocabulary.RoleKey("rule/value/runtime-kind-call"))
	operandFamily, operandOK := context.Roles.Key(vocabulary.RoleKey("operand/value/runtime-kind-call"))
	if !ruleOK || !operandOK {
		return nil, false
	}
	return DeclareSchema(builder, ruleSemantic, operandFamily, context.Principals.ValuePrincipal(), context.Principals.CallPrincipal())
}

func RegisterRule(binding *engine.SchemaBinding, context rule.Registration[*SchemaFragment]) (engine.RuleSlotCapability, bool) {
	return engine.RegisterMountedSlot(binding, context.Fragment.RuleSlot())
}

func BindRule[A ruleAuthorities](_ *engine.SchemaBinding, context rule.Binding[A, *SchemaFragment]) (*HotRule, bool) {
	return BindHot(context.Fragment, context.Authorities.ValueAuthority(), context.Authorities.CallAuthority())
}

// StructureSpecs contributes the rule and operand semantic identities owned by this
// rule.  The runtime-kind result relation itself is declared by the runtimekind
// domain; this rule consumes that opaque identity through Call's projection.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/value/runtime-kind-call", "operand/value/runtime-kind-call")
}
