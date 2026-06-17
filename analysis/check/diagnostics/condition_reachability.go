package diagnostics

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/compiler/ast"
)

type redundantConditions producerContext

type redundantConditionProof struct {
	always  bool
	check   string
	proven  string
	missing string
}

func (redundantConditions) Produce(result *body.Result) []diagnostic.Diagnostic {
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	envs := guardEnvironments(result)
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
	sort.Slice(out, func(i, j int) bool {
		if out[i].Span.StartLine != out[j].Span.StartLine {
			return out[i].Span.StartLine < out[j].Span.StartLine
		}
		if out[i].Span.StartCol != out[j].Span.StartCol {
			return out[i].Span.StartCol < out[j].Span.StartCol
		}
		return out[i].Message < out[j].Message
	})
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
				always:  true,
				check:   fmt.Sprintf("%s is tested for truthiness", check.Path.String()),
				proven:  fmt.Sprintf("incoming guard state proves %s is already truthy", check.Path.String()),
				missing: fmt.Sprintf("no invalidating assignment or call affecting %s reaches this condition", check.Path.String()),
			}, true
		}
		if env.hasFalsy(check.Path) {
			return redundantConditionProof{
				check:   fmt.Sprintf("%s is tested for truthiness", check.Path.String()),
				proven:  fmt.Sprintf("incoming guard state proves %s is already falsy", check.Path.String()),
				missing: fmt.Sprintf("no invalidating assignment or call affecting %s reaches this condition", check.Path.String()),
			}, true
		}
	case branchcond.CheckFalsy:
		if env.hasFalsy(check.Path) {
			return redundantConditionProof{
				always:  true,
				check:   fmt.Sprintf("%s is tested for falsiness", check.Path.String()),
				proven:  fmt.Sprintf("incoming guard state proves %s is already falsy", check.Path.String()),
				missing: fmt.Sprintf("no invalidating assignment or call affecting %s reaches this condition", check.Path.String()),
			}, true
		}
		if env.hasTruthy(check.Path) {
			return redundantConditionProof{
				check:   fmt.Sprintf("%s is tested for falsiness", check.Path.String()),
				proven:  fmt.Sprintf("incoming guard state proves %s is already truthy", check.Path.String()),
				missing: fmt.Sprintf("no invalidating assignment or call affecting %s reaches this condition", check.Path.String()),
			}, true
		}
	case branchcond.CheckNil:
		if env.hasNil(check.Path) {
			return redundantConditionProof{
				always:  true,
				check:   fmt.Sprintf("%s is compared with nil", check.Path.String()),
				proven:  fmt.Sprintf("incoming guard state proves %s is nil", check.Path.String()),
				missing: fmt.Sprintf("no invalidating assignment or call affecting %s reaches this condition", check.Path.String()),
			}, true
		}
		if env.hasPresent(check.Path) || env.hasTruthy(check.Path) {
			return redundantConditionProof{
				check:   fmt.Sprintf("%s is compared with nil", check.Path.String()),
				proven:  fmt.Sprintf("incoming guard state proves %s is not nil", check.Path.String()),
				missing: fmt.Sprintf("no invalidating assignment or call affecting %s reaches this condition", check.Path.String()),
			}, true
		}
	case branchcond.CheckNotNil:
		if env.hasPresent(check.Path) || env.hasTruthy(check.Path) {
			return redundantConditionProof{
				always:  true,
				check:   fmt.Sprintf("%s is compared with nil", check.Path.String()),
				proven:  fmt.Sprintf("incoming guard state proves %s is not nil", check.Path.String()),
				missing: fmt.Sprintf("no invalidating assignment or call affecting %s reaches this condition", check.Path.String()),
			}, true
		}
		if env.hasNil(check.Path) {
			return redundantConditionProof{
				check:   fmt.Sprintf("%s is compared with nil", check.Path.String()),
				proven:  fmt.Sprintf("incoming guard state proves %s is nil", check.Path.String()),
				missing: fmt.Sprintf("no invalidating assignment or call affecting %s reaches this condition", check.Path.String()),
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
	operator := "equals"
	if negated {
		operator = "does not equal"
	}
	for _, c := range env.constraints {
		if !c.target.Equal(check.Path) {
			continue
		}
		if c.negated {
			if c.value != check.LiteralString {
				return redundantConditionProof{}, false
			}
			return redundantConditionProof{
				always:  negated,
				check:   fmt.Sprintf("%s %s %q", check.Path.String(), operator, check.LiteralString),
				proven:  literalProofMessage(c),
				missing: fmt.Sprintf("no invalidating assignment or call affecting %s reaches this condition", check.Path.String()),
			}, true
		}
		match := c.value == check.LiteralString
		always := match != negated
		return redundantConditionProof{
			always:  always,
			check:   fmt.Sprintf("%s %s %q", check.Path.String(), operator, check.LiteralString),
			proven:  literalProofMessage(c),
			missing: fmt.Sprintf("no invalidating assignment or call affecting %s reaches this condition", check.Path.String()),
		}, true
	}
	return redundantConditionProof{}, false
}

func literalProofMessage(c literalConstraint) string {
	if c.negated {
		return fmt.Sprintf("incoming guard state proves %s is not %q", c.target.String(), c.value)
	}
	return fmt.Sprintf("incoming guard state proves %s is %q", c.target.String(), c.value)
}

func typeConditionProof(env guardEnv, check branchcond.Check, negated bool) (redundantConditionProof, bool) {
	if check.TypeName == "nil" {
		if env.hasNil(check.Path) {
			return runtimeTypeConditionProof(check, "nil", !negated), true
		}
		if env.hasPresent(check.Path) || env.hasTruthy(check.Path) {
			return runtimeTypeConditionProof(check, "not nil", negated), true
		}
	}
	for _, c := range env.typeChecks {
		if !c.target.Equal(check.Path) {
			continue
		}
		always := (c.name == check.TypeName) != negated
		return redundantConditionProof{
			always:  always,
			check:   fmt.Sprintf("type(%s) is compared with %q", check.Path.String(), check.TypeName),
			proven:  fmt.Sprintf("incoming guard state proves type(%s) is %q", check.Path.String(), c.name),
			missing: fmt.Sprintf("no invalidating assignment or call affecting %s reaches this condition", check.Path.String()),
		}, true
	}
	return redundantConditionProof{}, false
}

func runtimeTypeConditionProof(check branchcond.Check, proven string, always bool) redundantConditionProof {
	return redundantConditionProof{
		always:  always,
		check:   fmt.Sprintf("type(%s) is compared with %q", check.Path.String(), check.TypeName),
		proven:  fmt.Sprintf("incoming guard state proves %s is %s", check.Path.String(), proven),
		missing: fmt.Sprintf("no invalidating assignment or call affecting %s reaches this condition", check.Path.String()),
	}
}

func redundantConditionDiagnostic(fact semantics.BranchConditionFact, proof redundantConditionProof) diagnostic.Diagnostic {
	span := ast.SpanOf(fact.Condition)
	if !span.Valid() {
		span = ast.SpanOf(fact.Stmt)
	}
	value := "false"
	unreachableEdge := "true"
	if proof.always {
		value = "true"
		unreachableEdge = "false"
	}
	message := fmt.Sprintf("condition is always %s here", value)
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      span.StartLine,
			Column:    span.StartCol,
			EndLine:   span.EndLine,
			EndColumn: span.EndCol,
		},
		Span:     span,
		Code:     CodeRedundantCondition,
		Severity: diagnostic.SeverityWarning,
		Message:  message,
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: "condition check: " + proof.check,
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: proof.proven,
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: fmt.Sprintf("CFG %s edge is unreachable after guard propagation", unreachableEdge),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: proof.missing,
			},
		),
		Labels: []diagnostic.Label{{Span: span, Message: "redundant condition"}},
		Help:   "Remove the redundant condition, or preserve any needed side effects before simplifying the branch.",
	}
}
