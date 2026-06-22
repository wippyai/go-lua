package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type redundantConditions producerContext

type redundantConditionProof struct {
	always    bool
	check     string
	proven    string
	stable    string
	proofSpan diagnostic.Span
}

func (redundantConditions) Produce(result *body.Result) []diagnostic.Diagnostic {
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	envs := cachedGuardEnvironments(result)
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		fact, ok := result.BranchCondition(point)
		if !ok || !userVisibleBranchKind(fact.Kind) {
			continue
		}
		env := envs[point]
		if env.unreachable {
			continue
		}
		proof, ok := redundantConditionProofFor(env, fact.Check)
		if !ok {
			continue
		}
		out = append(out, redundantConditionDiagnostic(fact, proof))
	}
	diagnostic.Sort(out)
	return out
}

func userVisibleBranchKind(kind semantics.BranchKind) bool {
	return kind == semantics.BranchIf || kind == semantics.BranchWhile || kind == semantics.BranchRepeat
}

func redundantConditionProofFor(env guardEnv, check branchcond.Check) (redundantConditionProof, bool) {
	switch check.Kind {
	case branchcond.CheckTruthy:
		if env.hasTruthy(check.Path) {
			return redundantConditionProof{
				always:    true,
				check:     truthyConditionCheck(check.Path.String()),
				proven:    conditionPathProofEvidence(check.Path.String(), "truthy"),
				stable:    stableConditionEvidence(check.Path.String()),
				proofSpan: env.truthyOrigin(check.Path),
			}, true
		}
		if env.hasFalsy(check.Path) {
			return redundantConditionProof{
				check:     truthyConditionCheck(check.Path.String()),
				proven:    conditionPathProofEvidence(check.Path.String(), "falsy"),
				stable:    stableConditionEvidence(check.Path.String()),
				proofSpan: env.falsyOrigin(check.Path),
			}, true
		}
	case branchcond.CheckFalsy:
		if env.hasFalsy(check.Path) {
			return redundantConditionProof{
				always:    true,
				check:     falsyConditionCheck(check.Path.String()),
				proven:    conditionPathProofEvidence(check.Path.String(), "falsy"),
				stable:    stableConditionEvidence(check.Path.String()),
				proofSpan: env.falsyOrigin(check.Path),
			}, true
		}
		if env.hasTruthy(check.Path) {
			return redundantConditionProof{
				check:     falsyConditionCheck(check.Path.String()),
				proven:    conditionPathProofEvidence(check.Path.String(), "truthy"),
				stable:    stableConditionEvidence(check.Path.String()),
				proofSpan: env.truthyOrigin(check.Path),
			}, true
		}
	case branchcond.CheckNil:
		if env.hasNil(check.Path) {
			return redundantConditionProof{
				always:    true,
				check:     nilConditionCheck(check.Path.String()),
				proven:    conditionPathProofEvidence(check.Path.String(), "nil"),
				stable:    stableConditionEvidence(check.Path.String()),
				proofSpan: env.nilOrigin(check.Path),
			}, true
		}
		if env.hasPresent(check.Path) || env.hasTruthy(check.Path) {
			return redundantConditionProof{
				check:     nilConditionCheck(check.Path.String()),
				proven:    conditionPathProofEvidence(check.Path.String(), "not nil"),
				stable:    stableConditionEvidence(check.Path.String()),
				proofSpan: env.presentOrTruthyOrigin(check.Path),
			}, true
		}
	case branchcond.CheckNotNil:
		if env.hasPresent(check.Path) || env.hasTruthy(check.Path) {
			return redundantConditionProof{
				always:    true,
				check:     nonNilConditionCheck(check.Path.String()),
				proven:    conditionPathProofEvidence(check.Path.String(), "not nil"),
				stable:    stableConditionEvidence(check.Path.String()),
				proofSpan: env.presentOrTruthyOrigin(check.Path),
			}, true
		}
		if env.hasNil(check.Path) {
			return redundantConditionProof{
				check:     nonNilConditionCheck(check.Path.String()),
				proven:    conditionPathProofEvidence(check.Path.String(), "nil"),
				stable:    stableConditionEvidence(check.Path.String()),
				proofSpan: env.nilOrigin(check.Path),
			}, true
		}
	case branchcond.CheckLiteralEqual:
		return literalConditionProof(env, check, false)
	case branchcond.CheckLiteralNot:
		return literalConditionProof(env, check, true)
	case branchcond.CheckTypeEqual:
		return typeConditionProof(env, check, false)
	case branchcond.CheckTypeNot:
		return typeConditionProof(env, check, true)
	}
	return redundantConditionProof{}, false
}

func literalConditionProof(env guardEnv, check branchcond.Check, negated bool) (redundantConditionProof, bool) {
	lit, ok := check.LiteralValue()
	if !ok {
		return redundantConditionProof{}, false
	}
	operator := "equals"
	if negated {
		operator = "does not equal"
	}
	var out redundantConditionProof
	found := false
	for _, c := range env.constraints {
		if !c.target.Equal(check.Path) {
			continue
		}
		proof, ok := literalConditionProofFromConstraint(c, check, lit, operator, negated)
		if !ok {
			continue
		}
		if found && out.always != proof.always {
			return redundantConditionProof{}, false
		}
		out = proof
		found = true
	}
	return out, found
}

func literalConditionProofFromConstraint(c literalConstraint, check branchcond.Check, lit typ.Type, operator string, negated bool) (redundantConditionProof, bool) {
	match := typ.TypeEquals(c.value, lit)
	if c.negated {
		if !match {
			return redundantConditionProof{}, false
		}
		return redundantConditionProof{
			always:    negated,
			check:     fmt.Sprintf("%s %s %s", check.Path.String(), operator, lit.String()),
			proven:    literalProofMessage(c),
			stable:    stableConditionEvidence(check.Path.String()),
			proofSpan: c.span,
		}, true
	}
	return redundantConditionProof{
		always:    match != negated,
		check:     fmt.Sprintf("%s %s %s", check.Path.String(), operator, lit.String()),
		proven:    literalProofMessage(c),
		stable:    stableConditionEvidence(check.Path.String()),
		proofSpan: c.span,
	}, true
}

func literalProofMessage(c literalConstraint) string {
	if c.negated {
		return conditionPathProofEvidence(c.target.String(), "not "+c.value.String())
	}
	return conditionPathProofEvidence(c.target.String(), c.value.String())
}

func typeConditionProof(env guardEnv, check branchcond.Check, negated bool) (redundantConditionProof, bool) {
	if check.TypeName == "nil" {
		if env.hasNil(check.Path) {
			return runtimeTypeConditionProof(check, negated, "nil", !negated, env.nilOrigin(check.Path)), true
		}
		if env.hasPresent(check.Path) || env.hasTruthy(check.Path) {
			return runtimeTypeConditionProof(check, negated, "not nil", negated, env.presentOrTruthyOrigin(check.Path)), true
		}
	}
	for _, c := range env.typeChecks {
		if !c.target.Equal(check.Path) {
			continue
		}
		always := (c.name == check.TypeName) != negated
		return redundantConditionProof{
			always:    always,
			check:     fmt.Sprintf("type(%s) %s %q", check.Path.String(), typeCheckOperator(negated), check.TypeName),
			proven:    conditionTypeProofEvidence(check.Path.String(), c.name),
			stable:    stableConditionEvidence(check.Path.String()),
			proofSpan: c.span,
		}, true
	}
	return redundantConditionProof{}, false
}

func runtimeTypeConditionProof(check branchcond.Check, negated bool, proven string, always bool, proofSpan diagnostic.Span) redundantConditionProof {
	return redundantConditionProof{
		always:    always,
		check:     fmt.Sprintf("type(%s) %s %q", check.Path.String(), typeCheckOperator(negated), check.TypeName),
		proven:    conditionPathProofEvidence(check.Path.String(), proven),
		stable:    stableConditionEvidence(check.Path.String()),
		proofSpan: proofSpan,
	}
}

func stableConditionEvidence(path string) string {
	return conditionStabilityEvidence(path)
}

func typeCheckOperator(negated bool) string {
	if negated {
		return "is not"
	}
	return "is"
}

func redundantConditionDiagnostic(fact semantics.BranchConditionFact, proof redundantConditionProof) diagnostic.Diagnostic {
	span := ast.SpanOf(fact.Condition)
	if !span.Valid() {
		span = ast.SpanOf(fact.Stmt)
	}
	message := redundantConditionMessage(proof.always)
	labels := []diagnostic.Label{sourceLabel(span, labelConditionCheck)}
	proofSpan := proof.proofSpan
	if proofSpan.Valid() && !spanEqual(proofSpan, span) {
		labels = append(labels, sourceLabel(proofSpan, labelProvingGuard))
	}
	proofEvidence := diagnostic.Evidence{
		Kind:    diagnostic.EvidenceAbstractFact,
		Trust:   diagnostic.TrustProven,
		Span:    proofSpan,
		Message: proof.proven,
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:     span,
		Code:     CodeRedundantCondition,
		Severity: diagnostic.SeverityWarning,
		Message:  message,
		Labels:   labels,
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: conditionCheckEvidence(proof.check),
			},
			proofEvidence,
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Message: proof.stable,
			},
		),
		Help: redundantConditionHelp(proof.always),
	})
}
