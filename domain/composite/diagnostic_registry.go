package composite

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
)

// The declared-not-composed register.
//
// A diagnostic code is a published external identity. A fixture, a lint
// configuration, or an editor may name one before the analyzer composes a
// producer for it, and until now naming such a code produced nothing at all:
// the policy simply did not select it and the run reported a clean fixture.
// Silence is not an answer a declaration may receive.
//
// This register is the second half of the diagnostic vocabulary. A code is
// either composed - the sealed declaration table holds a row whose lane
// installs a producer - or it is declared here, by name, with the surface that
// owes the judgment. Nothing may be in both, and nothing a fixture references
// may be in neither; the laws below and the corpus law in oracle hold both
// halves. Adding a producer is therefore a two-line change: the row joins the
// sealed table and its entry leaves this register.
//
// An owner is a package path because a diagnostic is owed by the surface that
// holds the facts it is decided from, not by whoever last edited a table.

// DiagnosticDeclaredCode is one code the analyzer publishes as a named
// identity without composing a collectable producer for it. Owner names the
// surface that owes the judgment; Reason states what is missing, in the terms
// of the analyzer rather than of a schedule.
type DiagnosticDeclaredCode struct {
	Code   diagnostic.Code
	Owner  string
	Reason string
}

// The surfaces that owe an uncomposed judgment.
const (
	diagnosticOwnerType      = "domain/type"
	diagnosticOwnerOwnership = "domain/effect/ownership"
	diagnosticOwnerLifecycle = "domain/effect/lifecycle"
	diagnosticOwnerTypestate = "domain/typestate"
	diagnosticOwnerComposite = "domain/composite"
)

// diagnosticDeclaredCodes is the authored register, in code order. Every row
// is a judgment the analyzer names and does not yet decide.
var diagnosticDeclaredCodes = []DiagnosticDeclaredCode{
	{DiagnosticCodeInvariantLoopRead, diagnosticOwnerComposite, "no loop-invariant read observation is composed"},
	{DiagnosticCodeRedundantClaim, diagnosticOwnerComposite, "declared-lane row; no declared-lane collector is installed"},
	{DiagnosticCodeShapePolymorphic, diagnosticOwnerType, "no shape-polymorphism observation is composed"},
	{DiagnosticCodeSplitBirthDiscriminant, diagnosticOwnerType, "no split-birth discriminant observation is composed"},
	{DiagnosticCodeChannelCloseClosed, diagnosticOwnerType, "no channel-state observation is composed"},
	{DiagnosticCodeChannelSendClosed, diagnosticOwnerType, "no channel-state observation is composed"},
	{DiagnosticCodeFreezeMutation, diagnosticOwnerOwnership, "no freeze-mutation observation is composed"},
	{DiagnosticCodeLifecycleUnreleased, diagnosticOwnerLifecycle, "no lifecycle-release observation is composed"},
	{DiagnosticCodeClaimUnproven, diagnosticOwnerType, "no claim-proof observation is composed"},
	{DiagnosticCodeConditionRedundant, diagnosticOwnerComposite, "no redundant-condition observation is composed"},
	{DiagnosticCodeDeadAssignment, diagnosticOwnerComposite, "no dead-assignment observation is composed"},
	{DiagnosticCodeUnionExhaustiveness, diagnosticOwnerType, "no union-exhaustiveness observation is composed"},
	{DiagnosticCodeUnusedLocal, diagnosticOwnerComposite, "declared-lane row; no declared-lane collector is installed"},
	{DiagnosticCodeAssignmentOptionalTarget, diagnosticOwnerType, "no optional-target conformance verdict is composed"},
	{DiagnosticCodeCallNotCallable, diagnosticOwnerType, "no callability verdict is composed"},
	{DiagnosticCodeCallResultAssignment, diagnosticOwnerType, "no call-result conformance verdict is composed"},
	{DiagnosticCodeCallTooFewArgs, diagnosticOwnerType, "no arity verdict is composed"},
	{DiagnosticCodeCallOptionalReceiver, diagnosticOwnerType, "no optional-receiver verdict is composed"},
	{DiagnosticCodeForNumericOperand, diagnosticOwnerType, "no numeric-for operand verdict is composed"},
	{DiagnosticCodeMemberMissing, diagnosticOwnerType, "no member-presence verdict is composed"},
	{DiagnosticCodeNilUnsafeUse, diagnosticOwnerType, "no nilability verdict is composed"},
	{DiagnosticCodeConcatOperand, diagnosticOwnerType, "no concat-operand verdict is composed"},
	{DiagnosticCodeReturnContract, diagnosticOwnerType, "no return-conformance verdict is composed"},
	{DiagnosticCodeTypestateInvalidRequirement, diagnosticOwnerTypestate, "no typestate requirement verdict is composed"},
	{DiagnosticCodeTypestateInvalidTransition, diagnosticOwnerTypestate, "no typestate transition verdict is composed"},
	{DiagnosticCodeTypestateUnprovenRequirement, diagnosticOwnerTypestate, "no typestate proof verdict is composed"},
}

// The declared-not-composed codes. They are spelled here for the same reason
// the composed ones are spelled beside their rows: a code has exactly one
// spelling in the analyzer, whether or not a producer decides it.
const (
	DiagnosticCodeInvariantLoopRead            diagnostic.Code = "advice.invariant_loop_read"
	DiagnosticCodeShapePolymorphic             diagnostic.Code = "advice.shape.polymorphic"
	DiagnosticCodeSplitBirthDiscriminant       diagnostic.Code = "advice.split_birth_discriminant"
	DiagnosticCodeChannelCloseClosed           diagnostic.Code = "channel.close.closed"
	DiagnosticCodeChannelSendClosed            diagnostic.Code = "channel.send.closed"
	DiagnosticCodeFreezeMutation               diagnostic.Code = "effect.freeze.mutation"
	DiagnosticCodeLifecycleUnreleased          diagnostic.Code = "effect.lifecycle.unreleased"
	DiagnosticCodeClaimUnproven                diagnostic.Code = "lint.claim.unproven"
	DiagnosticCodeConditionRedundant           diagnostic.Code = "lint.condition.redundant"
	DiagnosticCodeDeadAssignment               diagnostic.Code = "lint.dead.assignment"
	DiagnosticCodeUnionExhaustiveness          diagnostic.Code = "lint.union.exhaustiveness"
	DiagnosticCodeAssignmentOptionalTarget     diagnostic.Code = "type.assignment.optional_target"
	DiagnosticCodeCallNotCallable              diagnostic.Code = "type.call.direct.not_callable"
	DiagnosticCodeCallResultAssignment         diagnostic.Code = "type.call.direct.result_assignment"
	DiagnosticCodeCallTooFewArgs               diagnostic.Code = "type.call.direct.too_few_args"
	DiagnosticCodeCallOptionalReceiver         diagnostic.Code = "type.call.optional_receiver"
	DiagnosticCodeForNumericOperand            diagnostic.Code = "type.for.numeric_operand"
	DiagnosticCodeMemberMissing                diagnostic.Code = "type.member.missing"
	DiagnosticCodeNilUnsafeUse                 diagnostic.Code = "type.nil.unsafe_use"
	DiagnosticCodeConcatOperand                diagnostic.Code = "type.operator.concat_operand"
	DiagnosticCodeReturnContract               diagnostic.Code = "type.return.contract"
	DiagnosticCodeTypestateInvalidRequirement  diagnostic.Code = "typestate.invalid_requirement"
	DiagnosticCodeTypestateInvalidTransition   diagnostic.Code = "typestate.invalid_transition"
	DiagnosticCodeTypestateUnprovenRequirement diagnostic.Code = "typestate.unproven_requirement"
)

// DiagnosticsDeclaredNotComposed is the register's read model, in code order.
func DiagnosticsDeclaredNotComposed() []DiagnosticDeclaredCode {
	rows := make([]DiagnosticDeclaredCode, len(diagnosticDeclaredCodes))
	copy(rows, diagnosticDeclaredCodes)
	sort.Slice(rows, func(left, right int) bool { return rows[left].Code < rows[right].Code })
	return rows
}

// DiagnosticDeclaredNotComposed answers one code's register entry.
func DiagnosticDeclaredNotComposed(code diagnostic.Code) (DiagnosticDeclaredCode, bool) {
	for _, row := range diagnosticDeclaredCodes {
		if row.Code == code {
			return row, true
		}
	}
	return DiagnosticDeclaredCode{}, false
}

// DiagnosticCodeStatus is one code's answer to "does the analyzer decide this".
type DiagnosticCodeStatus uint8

const (
	// DiagnosticCodeUnknown is a code the analyzer neither composes nor
	// declares. Naming one is a defect in the naming configuration, not a
	// pending judgment.
	DiagnosticCodeUnknown DiagnosticCodeStatus = iota
	// DiagnosticCodeComposed is a code whose sealed row installs a producer.
	DiagnosticCodeComposed
	// DiagnosticCodeDeclared is a code the register names with an owner.
	DiagnosticCodeDeclared
)

// DiagnosticCodeAnswer classifies one named code against the sealed table and
// the register together. It is the single reading a consumer needs: a policy
// that selected an unknown code is misconfigured, and one that selected a
// declared code is waiting on the owner the register names.
func DiagnosticCodeAnswer(compilation Compilation, code diagnostic.Code) (DiagnosticCodeStatus, DiagnosticDeclaredCode) {
	if table, tableOK := Diagnostics(compilation); tableOK {
		if entry, entryOK := table.ForCode(code); entryOK && entry.Collectable() {
			return DiagnosticCodeComposed, DiagnosticDeclaredCode{}
		}
	}
	if row, declared := DiagnosticDeclaredNotComposed(code); declared {
		return DiagnosticCodeDeclared, row
	}
	return DiagnosticCodeUnknown, DiagnosticDeclaredCode{}
}
