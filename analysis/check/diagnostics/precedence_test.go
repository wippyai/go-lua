package diagnostics

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/compiler/source"
)

func TestApplyDiagnosticPrecedenceSuppressesCoveredDependent(t *testing.T) {
	assignment := precedenceDiagnostic("main.lua", CodeAssignmentType, source.Span{StartLine: 7, StartCol: 10, EndLine: 7, EndCol: 30})
	callArg := precedenceDiagnostic("main.lua", CodeDirectCallArgType, source.Span{StartLine: 7, StartCol: 18, EndLine: 7, EndCol: 24})

	got := applyDiagnosticPrecedence([]diagnostic.Diagnostic{assignment, callArg}, []diagnosticPrecedenceRule{{
		cause:      CodeDirectCallArgType,
		suppressed: CodeAssignmentType,
		relation:   diagnosticPrecedenceCoveredSpan,
	}})
	if len(got) != 1 || got[0].Code != CodeDirectCallArgType {
		t.Fatalf("precedence result = %#v, want only call argument cause", got)
	}
}

func TestApplyDiagnosticPrecedenceSuppressesCauseInsideMultiLineDependent(t *testing.T) {
	assignment := precedenceDiagnostic("main.lua", CodeAssignmentType, source.Span{StartLine: 7, StartCol: 10, EndLine: 9, EndCol: 5})
	callArg := precedenceDiagnostic("main.lua", CodeDirectCallArgType, source.Span{StartLine: 8, StartCol: 18, EndLine: 8, EndCol: 24})

	got := applyDiagnosticPrecedence([]diagnostic.Diagnostic{assignment, callArg}, []diagnosticPrecedenceRule{{
		cause:      CodeDirectCallArgType,
		suppressed: CodeAssignmentType,
		relation:   diagnosticPrecedenceCoveredSpan,
	}})
	if len(got) != 1 || got[0].Code != CodeDirectCallArgType {
		t.Fatalf("precedence result = %#v, want only call argument cause", got)
	}
}

func TestApplyDiagnosticPrecedenceKeepsUncoveredDependent(t *testing.T) {
	assignment := precedenceDiagnostic("main.lua", CodeAssignmentType, source.Span{StartLine: 7, StartCol: 10, EndLine: 7, EndCol: 16})
	callArg := precedenceDiagnostic("main.lua", CodeDirectCallArgType, source.Span{StartLine: 7, StartCol: 18, EndLine: 7, EndCol: 24})

	got := applyDiagnosticPrecedence([]diagnostic.Diagnostic{assignment, callArg}, []diagnosticPrecedenceRule{{
		cause:      CodeDirectCallArgType,
		suppressed: CodeAssignmentType,
		relation:   diagnosticPrecedenceCoveredSpan,
	}})
	if len(got) != 2 {
		t.Fatalf("precedence result = %#v, want both diagnostics", got)
	}
}

func TestDefaultDiagnosticPrecedenceRulesDeclareCascadeOwnership(t *testing.T) {
	rules := defaultDiagnosticPrecedenceRules()
	want := map[[2]diagnostic.Code]struct{}{
		{CodeUnresolvedValueReference, CodeAssignmentType}:          {},
		{CodeMissingMember, CodeAssignmentType}:                     {},
		{CodeDirectCallNotCallable, CodeAssignmentType}:             {},
		{CodeDirectCallNotCallable, CodeDirectCallResultAssignment}: {},
		{CodeDirectCallTooFewArgs, CodeAssignmentType}:              {},
		{CodeDirectCallTooFewArgs, CodeDirectCallResultAssignment}:  {},
		{CodeDirectCallTooManyArgs, CodeAssignmentType}:             {},
		{CodeDirectCallTooManyArgs, CodeDirectCallResultAssignment}: {},
		{CodeDirectCallArgType, CodeAssignmentType}:                 {},
		{CodeDirectCallArgType, CodeDirectCallResultAssignment}:     {},
		{CodeDirectCallResultAssignment, CodeAssignmentType}:        {},
	}
	for _, rule := range rules {
		delete(want, [2]diagnostic.Code{rule.cause, rule.suppressed})
		if rule.relation != diagnosticPrecedenceCoveredSpan {
			t.Fatalf("rule %#v uses relation %v, want covered span", rule, rule.relation)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing default precedence rules: %#v", want)
	}
}

func TestApplyDiagnosticPrecedenceSuppressesAssignmentForMemberMissingCause(t *testing.T) {
	assignment := precedenceDiagnostic("main.lua", CodeAssignmentType, source.Span{StartLine: 7, StartCol: 18, EndLine: 7, EndCol: 30})
	memberMissing := precedenceDiagnostic("main.lua", CodeMissingMember, source.Span{StartLine: 7, StartCol: 18, EndLine: 7, EndCol: 30})

	got := applyDiagnosticPrecedence([]diagnostic.Diagnostic{assignment, memberMissing}, defaultDiagnosticPrecedenceRules())
	if len(got) != 1 || got[0].Code != CodeMissingMember {
		t.Fatalf("precedence result = %#v, want only member-missing cause", got)
	}
}

func TestApplyDiagnosticPrecedenceSuppressesAssignmentForDirectCallContractCauses(t *testing.T) {
	causes := []diagnostic.Code{
		CodeDirectCallNotCallable,
		CodeDirectCallTooFewArgs,
		CodeDirectCallTooManyArgs,
		CodeDirectCallArgType,
		CodeDirectCallResultAssignment,
	}
	for _, causeCode := range causes {
		t.Run(string(causeCode), func(t *testing.T) {
			assignment := precedenceDiagnostic("main.lua", CodeAssignmentType, source.Span{StartLine: 7, StartCol: 10, EndLine: 7, EndCol: 30})
			cause := precedenceDiagnostic("main.lua", causeCode, source.Span{StartLine: 7, StartCol: 18, EndLine: 7, EndCol: 24})

			got := applyDiagnosticPrecedence([]diagnostic.Diagnostic{assignment, cause}, defaultDiagnosticPrecedenceRules())
			if len(got) != 1 || got[0].Code != causeCode {
				t.Fatalf("precedence result = %#v, want only %s", got, causeCode)
			}
		})
	}
}

func TestApplyDiagnosticPrecedenceSuppressesResultAssignmentForDirectCallContractCauses(t *testing.T) {
	causes := []diagnostic.Code{
		CodeDirectCallNotCallable,
		CodeDirectCallTooFewArgs,
		CodeDirectCallTooManyArgs,
		CodeDirectCallArgType,
	}
	for _, causeCode := range causes {
		t.Run(string(causeCode), func(t *testing.T) {
			resultAssignment := precedenceDiagnostic("main.lua", CodeDirectCallResultAssignment, source.Span{StartLine: 7, StartCol: 18, EndLine: 7, EndCol: 30})
			cause := precedenceDiagnostic("main.lua", causeCode, source.Span{StartLine: 7, StartCol: 20, EndLine: 7, EndCol: 24})

			got := applyDiagnosticPrecedence([]diagnostic.Diagnostic{resultAssignment, cause}, defaultDiagnosticPrecedenceRules())
			if len(got) != 1 || got[0].Code != causeCode {
				t.Fatalf("precedence result = %#v, want only %s", got, causeCode)
			}
		})
	}
}

func precedenceDiagnostic(file string, code diagnostic.Code, span source.Span) diagnostic.Diagnostic {
	return diagnostic.New(diagnostic.DiagnosticSpec{
		File:     file,
		Span:     span,
		Code:     code,
		Severity: diagnostic.SeverityError,
		Message:  string(code),
	})
}
