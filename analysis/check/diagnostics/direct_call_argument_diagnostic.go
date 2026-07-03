package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func tooFewArgsDiagnostic(point cfg.Point, call *ast.FuncCallExpr, name string, want, got int, declSpan ast.Span) diagnostic.Diagnostic {
	span := ast.SpanOf(call)
	declEvidenceSpan := directCallDeclarationEvidenceSpan(call, declSpan)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:     span,
		Code:     CodeDirectCallTooFewArgs,
		Severity: diagnostic.SeverityError,
		Message:  callArityMismatchMessage(name, want, got),
		Help:     callArityHelp(want, got),
		Labels:   []diagnostic.Label{sourceLabel(span, labelCallExpression)},
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: callArgumentCountEvidence(name, got),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceUserAssertion,
				Trust:   diagnostic.TrustClaimed,
				Span:    declEvidenceSpan,
				Message: callParameterCountEvidence(name, want),
			},
		),
	})
}

func tooManyArgsDiagnostic(point cfg.Point, call *ast.FuncCallExpr, name string, want, got int, declSpan ast.Span, extra ast.Expr) diagnostic.Diagnostic {
	span := ast.SpanOf(call)
	extraSpan := ast.SpanOf(extra)
	declEvidenceSpan := directCallDeclarationEvidenceSpan(call, declSpan)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:     span,
		Code:     CodeDirectCallTooManyArgs,
		Severity: diagnostic.SeverityError,
		Message:  callArityMismatchMessage(name, want, got),
		Help:     callArityHelp(want, got),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: callArgumentCountEvidence(name, got),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceUserAssertion,
				Trust:   diagnostic.TrustClaimed,
				Span:    declEvidenceSpan,
				Message: callParameterCountEvidence(name, want),
			},
		),
		Labels: []diagnostic.Label{sourceLabel(extraSpan, labelExtraArgument)},
	})
}

func argTypeDiagnostic(call *ast.FuncCallExpr, name string, index int, got typ.Type, gotDisplay string, want typ.Type, wantDisplay string, arg ast.Expr, declSpan ast.Span, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	subject := fmt.Sprintf("argument %d", index+1)
	declEvidenceSpan := directCallDeclarationEvidenceSpan(call, declSpan)
	return argTypeDiagnosticEnvelope(call, arg, index, got,
		argumentTypeMismatchMessageDisplay(subject, arg, got, gotDisplay, want, wantDisplay),
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnostic.TrustClaimed,
			Span:    declEvidenceSpan,
			Message: callParameterTypeEvidenceDisplay(name, index+1, "", want, wantDisplay),
		},
		gotDisplay,
		extraEvidence...)
}

func argProofBoundaryDiagnostic(call *ast.FuncCallExpr, name string, index int, got typ.Type, gotDisplay string, want typ.Type, wantDisplay string, arg ast.Expr, declSpan ast.Span, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	subject := fmt.Sprintf("argument %d", index+1)
	declEvidenceSpan := directCallDeclarationEvidenceSpan(call, declSpan)
	return argTypeDiagnosticEnvelope(call, arg, index, got,
		argumentBoundaryProofMessage(subject, arg, want),
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnostic.TrustClaimed,
			Span:    declEvidenceSpan,
			Message: callParameterTypeEvidenceDisplay(name, index+1, "", want, wantDisplay),
		},
		gotDisplay,
		extraEvidence...)
}

func objectLiteralArgTypeDiagnostic(call *ast.FuncCallExpr, name string, index int, arg ast.Expr, mismatch objectLiteralTypeMismatch, declSpan ast.Span, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	subject := fmt.Sprintf("argument %d", index+1)
	if mismatch.suffix != "" {
		subject += mismatch.suffix
	}
	frameExpr := mismatch.expr
	if frameExpr == nil {
		frameExpr = arg
	}
	declEvidenceSpan := directCallDeclarationEvidenceSpan(call, declSpan)
	evidence := []diagnostic.Evidence{{
		Kind:    diagnostic.EvidenceUserAssertion,
		Trust:   diagnostic.TrustClaimed,
		Span:    declEvidenceSpan,
		Message: callParameterTypeEvidence(name, index+1, mismatch.suffix, mismatch.want),
	}}
	evidence = append(evidence, extraEvidence...)
	evidence = append(evidence, mismatch.missingMemberEvidence()...)
	evidence = append(evidence, mismatch.unionArmEvidence...)
	message := fmt.Sprintf("%s is %s, not %s", subject, formatType(mismatch.got), formatType(mismatch.want))
	if mismatch.missingMethod.Name != "" {
		message = fmt.Sprintf("%s does not implement %s: missing method %q", subject, formatType(mismatch.want), mismatch.missingMethod.Name)
	}
	return argTypeDiagnosticEnvelopeWithSubject(call, frameExpr, index, mismatch.got, "", subject,
		message,
		evidence[0], evidence[1:]...)
}

// argTypeDiagnosticEnvelope builds the shared argument-type diagnostic shell: the
// call/argument spans and labels, the "argument N is <got>" abstract fact, the
// caller's message, and a second evidence item describing what was expected.
func argTypeDiagnosticEnvelope(call *ast.FuncCallExpr, arg ast.Expr, index int, got typ.Type, message string, expected diagnostic.Evidence, gotDisplay string, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	return argTypeDiagnosticEnvelopeWithSubject(call, arg, index, got, gotDisplay, fmt.Sprintf("argument %d", index+1), message, expected, extraEvidence...)
}

func argTypeDiagnosticEnvelopeWithSubject(call *ast.FuncCallExpr, arg ast.Expr, index int, got typ.Type, gotDisplay string, subject string, message string, expected diagnostic.Evidence, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	callSpan := ast.SpanOf(call)
	argName := exprEvidenceName(arg)
	argSpan := directCallArgumentSpan(call, arg, index, argName)
	primarySpan := argSpan
	if !primarySpan.Valid() {
		primarySpan = callSpan
	}
	evidenceSubject := subject
	if argName != "" && argName != unknownSourceName {
		evidenceSubject = fmt.Sprintf("%s (%s)", subject, argName)
	}
	extraEvidence = clarifyTypeMismatchEvidence(extraEvidence, argName, got, argSpan, "parameter type")
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    argSpan,
			Message: assignmentSourceTypeEvidenceDisplay(evidenceSubject, got, gotDisplay),
		},
		expected,
	}
	evidence = append(evidence, extraEvidence...)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        primarySpan,
		Code:        CodeDirectCallArgType,
		Severity:    diagnostic.SeverityError,
		Message:     message,
		Explanation: diagnostic.NewExplanation(evidence...),
		Help:        argumentTypeMismatchHelpForEvidence(subject, argName, got, evidence),
		Labels:      []diagnostic.Label{sourceLabel(argSpan, labelArgumentValue)},
	})
}

func directCallArgumentSpan(call *ast.FuncCallExpr, arg ast.Expr, index int, argName string) diagnostic.Span {
	span := ast.SpanOf(arg)
	if _, ok := arg.(*ast.TableExpr); ok && call != nil && index > 0 && index <= len(call.Args)-1 && span.Valid() {
		prev := ast.SpanOf(call.Args[index-1])
		prevLine := prev.StartLine
		prevEndCol := prev.EndCol
		if prevEndCol <= 0 {
			prevEndCol = prev.StartCol
		}
		if prev.Valid() && span.StartLine != prevLine && span.StartCol > prevEndCol {
			span.StartLine = prevLine
			span.EndLine = span.StartLine
			span.EndCol = span.StartCol + 1
			return span
		}
	}
	return spanWithEvidenceName(span, argName)
}

func directCallDeclarationEvidenceSpan(call *ast.FuncCallExpr, declSpan ast.Span) diagnostic.Span {
	if !declSpan.Valid() {
		return diagnostic.Span{}
	}
	callSpan := ast.SpanOf(call)
	if callSpan.Valid() &&
		declSpan.StartLine == callSpan.StartLine &&
		declSpan.StartCol == callSpan.StartCol {
		return diagnostic.Span{}
	}
	return declSpan
}
