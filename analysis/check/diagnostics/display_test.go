package diagnostics

import (
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/lua/exprdisplay"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestDiagnosticLabelsUseCentralVocabulary(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var violations []string
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		findRawDiagnosticLabelMessages(fset, parsed, &violations)
	}
	if len(violations) > 0 {
		t.Fatalf("diagnostic labels must use display.go label constants or sourceLabel, not raw strings:\n%s", strings.Join(violations, "\n"))
	}
}

func TestDiagnosticMessagesUseCentralVocabulary(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var violations []string
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		findRawDiagnosticMessageLiterals(fset, parsed, &violations)
	}
	if len(violations) > 0 {
		t.Fatalf("diagnostic messages/help/evidence must use display.go helpers, not raw strings:\n%s", strings.Join(violations, "\n"))
	}
}

func TestDeadAssignmentMessagesUseCentralDisplay(t *testing.T) {
	if got := display.DeadAssignmentMessage("value", false); got != `assignment to "value" is overwritten before it is read` {
		t.Fatalf("display.DeadAssignmentMessage overwrite = %q", got)
	}
	if got := display.DeadAssignmentMessage("exit_value", true); got != `assignment to "exit_value" is discarded before it is read` {
		t.Fatalf("display.DeadAssignmentMessage exit = %q", got)
	}
	if got := display.DeadAssignmentOverwriteEvidence("value"); got != `later assignment replaces "value" before the earlier value is read` {
		t.Fatalf("display.DeadAssignmentOverwriteEvidence = %q", got)
	}
	if got := display.DeadAssignmentExitEvidence("exit_value"); got != `control can leave before "exit_value" is read` {
		t.Fatalf("display.DeadAssignmentExitEvidence = %q", got)
	}
	if got := display.DeadAssignmentHelp("value", false); got != "Remove this assignment, or read `value` before the later overwrite." {
		t.Fatalf("display.DeadAssignmentHelp overwrite = %q", got)
	}
	if got := display.DeadAssignmentHelp("exit_value", true); got != "Remove this assignment, or read `exit_value` before every later overwrite or exit." {
		t.Fatalf("display.DeadAssignmentHelp exit = %q", got)
	}
}

func TestRedundantConditionMessagesUseCentralDisplay(t *testing.T) {
	if got := display.TruthyConditionCheck("flag"); got != "flag is checked as truthy" {
		t.Fatalf("display.TruthyConditionCheck = %q", got)
	}
	if got := display.FalsyConditionCheck("flag"); got != "flag is checked as falsy" {
		t.Fatalf("display.FalsyConditionCheck = %q", got)
	}
	if got := display.NilConditionCheck("cache.value"); got != "cache.value == nil" {
		t.Fatalf("display.NilConditionCheck = %q", got)
	}
	if got := display.NonNilConditionCheck("cache.value"); got != "cache.value ~= nil" {
		t.Fatalf("display.NonNilConditionCheck = %q", got)
	}
	if got := display.ConditionStabilityEvidence("flag"); got != "flag is unchanged between the prior guard and this check" {
		t.Fatalf("display.ConditionStabilityEvidence = %q", got)
	}
	if got := display.ConditionCheckEvidence("flag is checked as truthy"); got != "current check: flag is checked as truthy" {
		t.Fatalf("display.ConditionCheckEvidence = %q", got)
	}
	if got := display.ConditionPathProofEvidence("flag", "truthy"); got != "prior guard established flag is truthy" {
		t.Fatalf("display.ConditionPathProofEvidence = %q", got)
	}
	if got := display.RedundantConditionMessage(true); got != "condition is always true here" {
		t.Fatalf("display.RedundantConditionMessage(true) = %q", got)
	}
	if got := display.RedundantConditionMessage(false); got != "condition is always false here" {
		t.Fatalf("display.RedundantConditionMessage(false) = %q", got)
	}
	if got := display.RedundantConditionHelp(true); got != "Remove this repeated check, or move any needed work into the branch already guarded above." {
		t.Fatalf("display.RedundantConditionHelp(true) = %q", got)
	}
	if got := display.RedundantConditionHelp(false); got != "Remove this unreachable branch, or change the prior guard if this path should still run." {
		t.Fatalf("display.RedundantConditionHelp(false) = %q", got)
	}
}

func TestDiagnosticProducerMessagesUseCentralDisplay(t *testing.T) {
	if got := display.UnresolvedTypeMessage("protocol.Policy"); got != "unknown type protocol.Policy" {
		t.Fatalf("display.UnresolvedTypeMessage = %q", got)
	}
	if got := display.UnresolvedTypeEvidence("protocol.Policy"); got != "no type named protocol.Policy is declared in this scope, a parent scope, or an imported module" {
		t.Fatalf("display.UnresolvedTypeEvidence = %q", got)
	}
	if got := display.UnresolvedTypeHelp(); got != "Declare the type in scope, import the module that exports it, or use the fully qualified exported type name." {
		t.Fatalf("display.UnresolvedTypeHelp = %q", got)
	}

	if got := display.UnresolvedValueMessage("provider"); got != "unknown value provider" {
		t.Fatalf("display.UnresolvedValueMessage = %q", got)
	}
	if got := display.UnresolvedValueEvidence("provider"); got != "no value named provider is declared, predeclared, imported, or configured global in this scope" {
		t.Fatalf("display.UnresolvedValueEvidence = %q", got)
	}
	if got := display.UnresolvedValueHelp(); got != "Declare the value, import it through require, or add it to the configured globals when it is intentionally ambient." {
		t.Fatalf("display.UnresolvedValueHelp = %q", got)
	}

	if got := display.ChannelSelectExhaustivenessMessage("cases", "ready, failed"); got != "channel select is not exhaustive; missing cases: ready, failed" {
		t.Fatalf("display.ChannelSelectExhaustivenessMessage = %q", got)
	}
	if got := display.ChannelSelectExhaustivenessHelp(); got != "Add an elseif branch for each missing case, or add a default branch when a fallback is valid." {
		t.Fatalf("display.ChannelSelectExhaustivenessHelp = %q", got)
	}
	if got := display.DiscriminatedUnionExhaustivenessMessage("case", "`action.kind == \"cancel\"`"); got != "discriminated union handling is not exhaustive; missing case: `action.kind == \"cancel\"`" {
		t.Fatalf("display.DiscriminatedUnionExhaustivenessMessage = %q", got)
	}
	if got := display.DiscriminatedUnionExhaustivenessHelp(); got != "Handle each missing case, or add an else branch when a fallback is valid." {
		t.Fatalf("display.DiscriminatedUnionExhaustivenessHelp = %q", got)
	}
	if got := display.DispatchTableExhaustivenessMessage("key", "`handlers.cancel`"); got != "dispatch table is not exhaustive; missing key: `handlers.cancel`" {
		t.Fatalf("display.DispatchTableExhaustivenessMessage = %q", got)
	}
	if got := display.DispatchTableExhaustivenessHelp(); got != "Add each missing dispatch key, or route through an explicit fallback when missing keys are intentional." {
		t.Fatalf("display.DispatchTableExhaustivenessHelp = %q", got)
	}
	if got := display.RegistrationExhaustivenessMessage("registration", "`router.cancel`"); got != "registered callbacks are not exhaustive; missing registration: `router.cancel`" {
		t.Fatalf("display.RegistrationExhaustivenessMessage = %q", got)
	}
	if got := display.RegistrationExhaustivenessHelp(); got != "Register each missing case, or dispatch through an explicit fallback when missing registrations are intentional." {
		t.Fatalf("display.RegistrationExhaustivenessHelp = %q", got)
	}
	if got := display.ResultShapeExhaustivenessMessage("result.value", "result.ok == true"); got != "case-specific field read is not exhaustive; `result.value` requires `result.ok == true`" {
		t.Fatalf("display.ResultShapeExhaustivenessMessage = %q", got)
	}
	if got := display.ResultShapeExhaustivenessHelp(); got != "Check the union case before reading this field, or return from the opposite case before continuing." {
		t.Fatalf("display.ResultShapeExhaustivenessHelp = %q", got)
	}
	if got := display.OptionalExhaustivenessMessage("case", "`maybe == nil`"); got != "optional handling is not exhaustive; missing case: `maybe == nil`" {
		t.Fatalf("display.OptionalExhaustivenessMessage = %q", got)
	}
	if got := display.OptionalExhaustivenessHelp(); got != "Handle the nil case with an else branch, or return before continuing when nil is intentionally ignored." {
		t.Fatalf("display.OptionalExhaustivenessHelp = %q", got)
	}

	if got := display.FrozenTableMutationMessage("session"); got != `cannot mutate frozen table "session"` {
		t.Fatalf("display.FrozenTableMutationMessage = %q", got)
	}
	if got := display.FrozenTableCallMutationMessage("session"); got != `cannot call mutator on frozen table "session"` {
		t.Fatalf("display.FrozenTableCallMutationMessage = %q", got)
	}
	if got := display.FrozenTableAssignmentHelp(); got != "Create a mutable copy before writing, or move this assignment before the table is frozen." {
		t.Fatalf("display.FrozenTableAssignmentHelp = %q", got)
	}
	if got := display.FrozenTableCallHelp(); got != "Create a mutable copy before calling the mutator, or call it before the table is frozen." {
		t.Fatalf("display.FrozenTableCallHelp = %q", got)
	}

	if got := display.UnusedLocalMessage("tmp"); got != `local "tmp" is never read` {
		t.Fatalf("display.UnusedLocalMessage = %q", got)
	}
	if got := display.UnusedLocalEvidence("tmp"); got != `no read of local "tmp" was found in this scope` {
		t.Fatalf("display.UnusedLocalEvidence = %q", got)
	}
	if got := display.UnusedLocalHelp(); got != "Remove it, use it, or rename it with a leading _ when intentionally unused." {
		t.Fatalf("display.UnusedLocalHelp = %q", got)
	}
}

func TestSpanWithEvidenceNameOnlyExtendsSimpleIdentifiers(t *testing.T) {
	base := diagnostic.Span{StartLine: 4, StartCol: 12}

	got := spanWithEvidenceName(base, "value")
	if got.EndLine != 4 || got.EndCol != 17 {
		t.Fatalf("simple identifier span = %#v, want end column from identifier width", got)
	}

	for _, name := range []string{`maybe.tags["source"]`, `provider.get(...)`, `row[key]`, `assigned value`} {
		got := spanWithEvidenceName(base, name)
		if got != base {
			t.Fatalf("complex evidence name %q widened span to %#v, want original exact caret span %#v", name, got, base)
		}
	}

	realSpan := diagnostic.Span{StartLine: 4, StartCol: 12, EndLine: 4, EndCol: 20}
	if got := spanWithEvidenceName(realSpan, `maybe.tags["source"]`); got != realSpan {
		t.Fatalf("real parser span changed to %#v, want %#v", got, realSpan)
	}
}

func TestExprEvidenceNamePreservesReadablePaths(t *testing.T) {
	expr := &ast.FuncCallExpr{
		Receiver: &ast.AttrGetExpr{
			Object:    &ast.IdentExpr{Value: "client"},
			Key:       &ast.IdentExpr{Value: "session"},
			KeySyntax: ast.AttrKeyDot,
		},
		Method: "refresh",
	}
	if got := exprdisplay.NameOK(expr); got != "client.session:refresh(...)" {
		t.Fatalf("exprdisplay.NameOK = %q, want readable receiver call path", got)
	}
}

func TestExprEvidenceNameFallsBackAtAdversarialDepth(t *testing.T) {
	var expr ast.Expr = &ast.IdentExpr{Value: "value"}
	for i := 0; i < typ.DefaultRecursionDepth+8; i++ {
		expr = &ast.NonNilAssertExpr{Expr: expr}
	}
	if got := exprdisplay.NameOK(expr); got != "" {
		t.Fatalf("exprdisplay.NameOK = %q, want bounded empty result", got)
	}
	if got := exprdisplay.Name(expr, unknownSourceName); got != unknownSourceName {
		t.Fatalf("exprdisplay.Name = %q, want safe fallback %q", got, unknownSourceName)
	}
}

func TestRequiredFieldPathUsesLuaSyntax(t *testing.T) {
	tests := []struct {
		target string
		field  string
		want   string
	}{
		{target: "p", field: "id", want: "p.id"},
		{target: "p", field: "_id2", want: "p._id2"},
		{target: "p", field: "display-name", want: `p["display-name"]`},
		{target: "p", field: "1st", want: `p["1st"]`},
		{target: unknownSourceName, field: "display-name", want: `["display-name"]`},
		{target: "p", field: "", want: "p"},
	}
	for _, tc := range tests {
		if got := requiredFieldPath(tc.target, tc.field); got != tc.want {
			t.Fatalf("requiredFieldPath(%q, %q) = %q, want %q", tc.target, tc.field, got, tc.want)
		}
	}
}

func findRawDiagnosticMessageLiterals(fset *token.FileSet, file *goast.File, violations *[]string) {
	goast.Inspect(file, func(n goast.Node) bool {
		lit, ok := n.(*goast.CompositeLit)
		if !ok || !isDiagnosticMessageCarrierType(lit.Type) {
			return true
		}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*goast.KeyValueExpr)
			if !ok || (!isIdent(kv.Key, "Message") && !isIdent(kv.Key, "Help")) {
				continue
			}
			if raw, ok := kv.Value.(*goast.BasicLit); ok && raw.Kind == token.STRING {
				*violations = append(*violations, fmt.Sprintf("%s: raw diagnostic text %s", fset.Position(raw.Pos()), raw.Value))
			}
		}
		return true
	})
}

func isDiagnosticMessageCarrierType(expr goast.Expr) bool {
	switch t := expr.(type) {
	case *goast.SelectorExpr:
		return isIdent(t.X, "diagnostic") && (t.Sel.Name == "Diagnostic" || t.Sel.Name == "Evidence")
	case *goast.ArrayType:
		return isDiagnosticMessageCarrierType(t.Elt)
	}
	return false
}

func findRawDiagnosticLabelMessages(fset *token.FileSet, file *goast.File, violations *[]string) {
	var walkExpr func(goast.Expr, bool)
	walkExpr = func(expr goast.Expr, inLabelLiteral bool) {
		switch n := expr.(type) {
		case *goast.CompositeLit:
			localInLabel := inLabelLiteral || isDiagnosticLabelType(n.Type)
			for _, elt := range n.Elts {
				if kv, ok := elt.(*goast.KeyValueExpr); ok {
					if localInLabel && isIdent(kv.Key, "Message") {
						if lit, ok := kv.Value.(*goast.BasicLit); ok && lit.Kind == token.STRING {
							*violations = append(*violations, fmt.Sprintf("%s: raw label message %s", fset.Position(lit.Pos()), lit.Value))
						}
					}
					walkExpr(kv.Value, false)
					continue
				}
				walkExpr(elt, localInLabel)
			}
		case *goast.CallExpr:
			walkExpr(n.Fun, false)
			for _, arg := range n.Args {
				walkExpr(arg, false)
			}
		case *goast.UnaryExpr:
			walkExpr(n.X, false)
		case *goast.BinaryExpr:
			walkExpr(n.X, false)
			walkExpr(n.Y, false)
		case *goast.IndexExpr:
			walkExpr(n.X, false)
			walkExpr(n.Index, false)
		case *goast.IndexListExpr:
			walkExpr(n.X, false)
			for _, index := range n.Indices {
				walkExpr(index, false)
			}
		case *goast.ParenExpr:
			walkExpr(n.X, inLabelLiteral)
		case *goast.SelectorExpr:
			walkExpr(n.X, false)
		case *goast.SliceExpr:
			walkExpr(n.X, false)
			if n.Low != nil {
				walkExpr(n.Low, false)
			}
			if n.High != nil {
				walkExpr(n.High, false)
			}
			if n.Max != nil {
				walkExpr(n.Max, false)
			}
		}
	}
	goast.Inspect(file, func(n goast.Node) bool {
		switch n := n.(type) {
		case goast.Expr:
			walkExpr(n, false)
			return false
		default:
			return true
		}
	})
}

func isDiagnosticLabelType(expr goast.Expr) bool {
	switch t := expr.(type) {
	case *goast.SelectorExpr:
		return isIdent(t.X, "diagnostic") && t.Sel.Name == "Label"
	case *goast.ArrayType:
		return isDiagnosticLabelType(t.Elt)
	}
	return false
}

func isIdent(expr goast.Expr, name string) bool {
	ident, ok := expr.(*goast.Ident)
	return ok && ident.Name == name
}

func TestDeclaredTypeEvidenceUsesTopLevelAnnotationAlias(t *testing.T) {
	annotation := &ast.TypeRefExpr{Path: []string{"protocol", "PolicyEvaluator"}}
	got := declaredTypeEvidence("evaluator", annotation, typ.String)
	if got != "evaluator is declared as protocol.PolicyEvaluator" {
		t.Fatalf("declaredTypeEvidence = %q", got)
	}
}

func TestDeclaredTypeEvidenceUsesProjectedTypeForNestedField(t *testing.T) {
	annotation := &ast.TypeRefExpr{Path: []string{"TreeNode"}}
	got := declaredTypeEvidence("node.label", annotation, typ.String)
	if got != "node.label is declared as string" {
		t.Fatalf("declaredTypeEvidence = %q", got)
	}
}

func TestDeclaredTypeEvidenceFormatsFunctionAnnotationReturns(t *testing.T) {
	annotation := &ast.FunctionTypeExpr{
		Returns: []ast.TypeExpr{&ast.TypeRefExpr{Path: []string{"Res"}}},
	}
	got := declaredTypeEvidence("f", annotation, typ.String)
	if got != "f is declared as fun() -> Res" {
		t.Fatalf("declaredTypeEvidence = %q", got)
	}
}

func TestDeclaredTypeEvidenceFormatsTypeLevelQueryAnnotations(t *testing.T) {
	tests := []struct {
		name       string
		annotation ast.TypeExpr
		want       string
	}{
		{
			name: "typeof",
			annotation: &ast.TypeOfExpr{
				Expr: &ast.IdentExpr{Value: "defaults"},
			},
			want: "value is declared as typeof(defaults)",
		},
		{
			name: "keyof",
			annotation: &ast.KeyOfExpr{
				Inner: &ast.TypeRefExpr{Path: []string{"User"}},
			},
			want: "value is declared as keyof User",
		},
		{
			name: "index access",
			annotation: &ast.IndexAccessExpr{
				Object: &ast.TypeRefExpr{Path: []string{"User"}},
				Index:  &ast.LiteralTypeExpr{Value: "id"},
			},
			want: "value is declared as User[\"id\"]",
		},
		{
			name: "conditional optional",
			annotation: &ast.OptionalTypeExpr{Inner: &ast.ConditionalTypeExpr{
				Check:   &ast.TypeRefExpr{Path: []string{"T"}},
				Extends: &ast.PrimitiveTypeExpr{Name: "string"},
				Then:    &ast.LiteralTypeExpr{Value: true},
				Else:    &ast.LiteralTypeExpr{Value: false},
			}},
			want: "value is declared as (T extends string ? true : false)?",
		},
		{
			name: "generic function params",
			annotation: &ast.FunctionTypeExpr{
				TypeParams: []ast.TypeParamExpr{{Name: "T", Constraint: &ast.TypeRefExpr{Path: []string{"User"}}}},
				Params:     []ast.FunctionParamExpr{{Name: "input", Type: &ast.TypeRefExpr{Path: []string{"T"}}}},
				Returns:    []ast.TypeExpr{&ast.TypeRefExpr{Path: []string{"T"}}},
			},
			want: "value is declared as fun<T: User>(input: T) -> T",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := declaredTypeEvidence("value", tc.annotation, typ.String); got != tc.want {
				t.Fatalf("declaredTypeEvidence = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatTypeAnnotationTruncatesDeepAnnotations(t *testing.T) {
	var annotation ast.TypeExpr = &ast.PrimitiveTypeExpr{Name: "string"}
	for i := 0; i < typ.DefaultRecursionDepth+8; i++ {
		annotation = &ast.OptionalTypeExpr{Inner: annotation}
	}
	got, ok := formatTypeAnnotation(annotation)
	if !ok || !strings.Contains(got, "...") {
		t.Fatalf("formatTypeAnnotation = %q, %v; want bounded rendering with ellipsis", got, ok)
	}
	if fallback := declaredTypeEvidence("value", annotation, typ.String); !strings.Contains(fallback, "...") {
		t.Fatalf("declaredTypeEvidence = %q, want bounded annotation instead of fallback type", fallback)
	}
}

func TestBoundaryProofMessagesUseCentralTypeDisplay(t *testing.T) {
	if got := explicitBoundaryProofMessage(typ.String); got != "assigned value comes from any/unknown" {
		t.Fatalf("explicitBoundaryProofMessage = %q", got)
	}
	if got := missingBoundaryProofMessage(typ.String); got != "no proof on this path shows assigned value is string" {
		t.Fatalf("missingBoundaryProofMessage = %q", got)
	}
	if got := explicitBoundaryProofMessageForSubject("raw.id", typ.String); got != "raw.id comes from any/unknown" {
		t.Fatalf("explicitBoundaryProofMessageForSubject = %q", got)
	}
	if got := missingBoundaryProofMessageForSubject("raw.id", typ.String); got != "no proof on this path shows raw.id is string" {
		t.Fatalf("missingBoundaryProofMessageForSubject = %q", got)
	}
	if got := missingIndexReadProofMessage(typ.String); !strings.Contains(got, "indexed read can miss or read nil") ||
		!strings.Contains(got, "no proof shows the selected slot satisfies string here") {
		t.Fatalf("missingIndexReadProofMessage = %q", got)
	}
}

func TestMemberDisplayMessagesUsePathWhenAvailable(t *testing.T) {
	if got := display.MissingMemberMessage(typ.String, "send"); got != `string has no member "send"` {
		t.Fatalf("missingMemberMessage = %q", got)
	}
	if got := display.MemberNotCallableMessage("client.send", typ.String, typ.Number, "send"); got != "client.send is number, not callable" {
		t.Fatalf("memberNotCallableMessage = %q", got)
	}
	if got := display.MemberNotCallableMessage("client.send", typ.String, typ.Any, "send"); got != "client.send comes from any/unknown; no proof shows it is callable" {
		t.Fatalf("memberNotCallableMessage any = %q", got)
	}
	if got := display.MemberReadReceiverEvidence(`client["send"]`, "send", typ.String); got != `client["send"] reads member "send" from receiver type string` {
		t.Fatalf("memberReadReceiverEvidence = %q", got)
	}
	if got := display.ReceiverForMemberEvidence("client.send", typ.String); got != "client.send has receiver type string" {
		t.Fatalf("receiverForMemberEvidence = %q", got)
	}
	if got := display.MissingMemberHelp("send"); got != "Narrow the receiver before reading `send`, or add `send` to every reachable receiver shape." {
		t.Fatalf("missingMemberHelp = %q", got)
	}
	if got := display.MemberTypeAtCallEvidence("client.send", typ.Number); got != "client.send has type number at call" {
		t.Fatalf("memberTypeAtCallEvidence = %q", got)
	}
	if got := display.MemberTypeAtCallEvidence("M.f", typ.LiteralInt(42)); got != "M.f has literal value 42 at call" {
		t.Fatalf("memberTypeAtCallEvidence literal = %q", got)
	}
	if got := display.MemberNotCallableHelp("client.send"); got != "Narrow `client.send` to a function-valued member before calling it, or call a different member." {
		t.Fatalf("memberNotCallableHelp = %q", got)
	}
}

func TestDirectCallDisplayMessagesUseCentralTypeDisplay(t *testing.T) {
	if got := display.DirectNotCallableMessage("target", typ.Number); got != "target is number, not callable" {
		t.Fatalf("directNotCallableMessage = %q", got)
	}
	if got := display.DirectNotCallableMessage("target", typ.Any); got != "target comes from any/unknown; no proof shows it is callable" {
		t.Fatalf("directNotCallableMessage any = %q", got)
	}
	if got := display.DirectNotCallableHelp("target"); got != "Call a function value, or replace `target` with a callable expression before this call." {
		t.Fatalf("directNotCallableHelp = %q", got)
	}
	if got := annotatedTypeEvidence("target", typ.Number); got != "target is annotated number" {
		t.Fatalf("annotatedTypeEvidence = %q", got)
	}
	if got := argumentTypeMismatchHelpForEvidence("argument 1", "payload", typ.String, []diagnostic.Evidence{{
		Kind:    diagnostic.EvidenceUserAssertion,
		Reason:  diagnostic.EvidenceReasonUserAssertedAny,
		Message: "wording changed upstream",
	}}); got != "Validate or narrow `payload` before passing it; any/unknown values do not prove parameter contracts." {
		t.Fatalf("argumentTypeMismatchHelpForEvidence explicit-any = %q", got)
	}
	if got := argumentTypeMismatchHelpForEvidence("argument 1", "payload", typ.String, []diagnostic.Evidence{{
		Kind:    diagnostic.EvidencePrecisionBoundary,
		Reason:  diagnostic.EvidenceReasonExplicitBoundaryValidation,
		Message: "wording changed upstream",
	}}); got != "Validate or narrow `payload` before passing it; any/unknown values do not prove parameter contracts." {
		t.Fatalf("argumentTypeMismatchHelpForEvidence precision-boundary = %q", got)
	}
	if got := argumentTypeMismatchHelpForEvidence("argument 1", "payload", typ.String, []diagnostic.Evidence{{
		Kind:    diagnostic.EvidenceUserAssertion,
		Reason:  diagnostic.EvidenceReasonUserTypeAssertion,
		Message: "user asserted any; stale wording should not control behavior",
	}}); got != "Pass `payload` as a value compatible with the parameter type, or change the callee signature if that argument is valid." {
		t.Fatalf("argumentTypeMismatchHelpForEvidence non-any assertion = %q", got)
	}
	if got := argumentTypeMismatchHelpForEvidence("argument 2", unknownSourceName, typ.String, nil); got != "Pass a value for argument 2 that satisfies the parameter type, or change the callee signature if that argument is valid." {
		t.Fatalf("argumentTypeMismatchHelpForEvidence subject fallback = %q", got)
	}
	if got := callParamObligationEvidence("forward", "argument 1 (payload)", typ.String); got != "inside forward, argument 1 (payload) must satisfy string" {
		t.Fatalf("callParamObligationEvidence = %q", got)
	}
	if got := callParamObligationEvidence("forward", "", typ.String); got != "inside forward, the argument must satisfy string" {
		t.Fatalf("callParamObligationEvidence fallback = %q", got)
	}
	if got := memberCallParamObligationEvidence("invoke", "argument 2 (payload)", "argument 1.client.send", 1, typ.Number); got != "inside invoke, argument 2 (payload) is passed to argument 1.client.send parameter 1, which requires number" {
		t.Fatalf("memberCallParamObligationEvidence = %q", got)
	}
}

func TestOptionalTypeDisplayLeavesNilabilityToProofContext(t *testing.T) {
	if got := formatType(typ.MaterializeOptional(typ.Unknown)); got != "unknown" {
		t.Fatalf("formatType optional unknown = %q", got)
	}
	if got := formatType(typ.MaterializeOptional(typ.Any)); got != "any" {
		t.Fatalf("formatType optional any = %q", got)
	}
	if got := formatType(typ.MaterializeOptional(typ.String)); got != "string?" {
		t.Fatalf("formatType optional string = %q", got)
	}
	if got := assignmentMessage("cache.value", typ.MaterializeOptional(typ.String), typ.String); got != "cannot assign cache.value because it is string?, not string" {
		t.Fatalf("assignmentMessage optional = %q", got)
	}
	if got := assignmentMessage("cache.value", typ.Number, typ.String); got != "cannot assign cache.value because it is number, not string" {
		t.Fatalf("assignmentMessage path mismatch = %q", got)
	}
	if got := assignmentMessageDisplay("M.run", typ.Func().Returns(typ.Nil).Build(), typ.Func().Returns(typ.String).Build(), "fun() -> Res"); got != "cannot assign M.run because it is fun() -> nil, not fun() -> Res" {
		t.Fatalf("assignmentMessageDisplay alias = %q", got)
	}
	if got := assignmentMessage("", typ.Number, typ.String); got != "cannot assign number to string" {
		t.Fatalf("assignmentMessage mismatch = %q", got)
	}
	if got := memberAssignmentMessage("p.id", "raw", typ.Any, typ.String); got != "cannot assign raw to p.id because raw is any, not string" {
		t.Fatalf("memberAssignmentMessage mismatch = %q", got)
	}
	if got := memberAssignmentMessageDisplay("M.run", "f", typ.Func().Returns(typ.Nil).Build(), typ.Func().Returns(typ.String).Build(), "fun() -> Res"); got != "cannot assign f to M.run because f is fun() -> nil, not fun() -> Res" {
		t.Fatalf("memberAssignmentMessageDisplay alias = %q", got)
	}
	if got := memberAssignmentMessage("impl.read", unknownSourceName, typ.Number, typ.String); got != "cannot assign impl.read because assigned value is number, not string" {
		t.Fatalf("memberAssignmentMessage fallback = %q", got)
	}
	if got := assignmentHelp("cache.value", true); got != "Guard `cache.value` with a nil check, provide a default value, or change the target type to accept nil." {
		t.Fatalf("assignmentHelp optional = %q", got)
	}
	if got := assignmentHelp("cache.value", false); got != "Use a value compatible with the expected type, or change the target type if `cache.value` is valid." {
		t.Fatalf("assignmentHelp mismatch = %q", got)
	}
	if got := assignmentTargetTypeEvidence("row[key]", typ.String); got != "assignment target row[key] requires string" {
		t.Fatalf("assignmentTargetTypeEvidence = %q", got)
	}
	if got := assignmentTargetTypeEvidence("", typ.Number); got != "assignment target requires number" {
		t.Fatalf("assignmentTargetTypeEvidence fallback = %q", got)
	}
	if got := underSuppliedTargetEvidence("b", "one(...)", 1); got != "b receives result 2 from `one(...)`, but no value was produced for that result slot" {
		t.Fatalf("underSuppliedTargetEvidence call = %q", got)
	}
	if got := underSuppliedTargetEvidence("b", "", -1); got != "b has no supplied value in this assignment, so Lua fills it with nil" {
		t.Fatalf("underSuppliedTargetEvidence nil-fill = %q", got)
	}
	if got := underSuppliedTargetHelp("b"); got != "Provide a value for `b`, remove the extra target, or change the target type to accept nil." {
		t.Fatalf("underSuppliedTargetHelp = %q", got)
	}
	if got := assignmentSourceTypeEvidence("argument 1", typ.LiteralInt(42)); got != "argument 1 has literal value 42" {
		t.Fatalf("assignmentSourceTypeEvidence literal = %q", got)
	}
	if got := optionalAssignmentTargetMessage("bag"); got != "cannot assign through optional bag without nil check" {
		t.Fatalf("optionalAssignmentTargetMessage = %q", got)
	}
	if got := optionalAssignmentTargetMessage(""); got != "cannot assign through an optional value without a nil check" {
		t.Fatalf("optionalAssignmentTargetMessage fallback = %q", got)
	}
	if got := optionalAssignmentTargetContainerEvidence("bag", typ.MaterializeOptional(typ.String)); got != "bag can be string or nil here" {
		t.Fatalf("optionalAssignmentTargetContainerEvidence = %q", got)
	}
	if got := optionalAssignmentTargetWriteEvidence("bag.name"); got != "writing bag.name requires its container to be non-nil" {
		t.Fatalf("optionalAssignmentTargetWriteEvidence = %q", got)
	}
	if got := optionalAssignmentTargetHelp("bag"); got != "Guard `bag` with a nil check before assigning through it, or write to a non-optional container." {
		t.Fatalf("optionalAssignmentTargetHelp = %q", got)
	}
	if got := missingRequiredFieldMessage("id"); got != `object literal is missing required field "id"` {
		t.Fatalf("missingRequiredFieldMessage = %q", got)
	}
	if got := missingRequiredFieldEvidence("id"); got != `object literal does not provide field "id"` {
		t.Fatalf("missingRequiredFieldEvidence = %q", got)
	}
	if got := missingRequiredFieldPathEvidence("p.id", typ.String); got != `required field p.id has type string, but the object literal does not provide it` {
		t.Fatalf("missingRequiredFieldPathEvidence = %q", got)
	}
	if got := objectLiteralShapeEvidence(typetable.NewRecord().Field("x", typ.LiteralInt(10)).Build()); got != `object literal has type {x: 10}` {
		t.Fatalf("objectLiteralShapeEvidence = %q", got)
	}
	if got := missingRequiredFieldHelp("id"); got != "Add field `id`, or make it optional in the declared type if it may be absent." {
		t.Fatalf("missingRequiredFieldHelp = %q", got)
	}
	if got := display.MissingNonNilGuardHereMessage("cache.value"); got != "no guard on this path proves cache.value is non-nil" {
		t.Fatalf("missingNonNilGuardHereMessage = %q", got)
	}
	if got := display.OptionalReceiverReadEvidence("store:lookup_policy(...)", ".tags"); got != "store:lookup_policy(...) may be nil before reading .tags" {
		t.Fatalf("optionalReceiverReadEvidence = %q", got)
	}
	if got := display.OptionalReceiverReadEvidence("store:lookup_policy(...).tags", `["source"]`); got != `store:lookup_policy(...).tags may be nil before indexing ["source"]` {
		t.Fatalf("optionalReceiverReadEvidence index = %q", got)
	}
	if got := display.IndexedReadExpectedProofMessage("items[i]", "declared type"); !strings.Contains(got, "items[i] is an indexed read") ||
		!strings.Contains(got, "satisfies the declared type here") {
		t.Fatalf("indexedReadExpectedProofMessage = %q", got)
	}
	if got := display.MissingExpectedProofMessage("raw", "parameter type"); got != "no proof on this path shows raw satisfies the parameter type" {
		t.Fatalf("missingExpectedProofMessage = %q", got)
	}
}

func TestAssignmentMessageLeavesNilabilityToProofContext(t *testing.T) {
	msg := assignmentMessage("cache.value", typ.MaterializeOptional(typ.String), typ.MaterializeOptional(typ.Number))
	if strings.Contains(msg, "may be nil") || !strings.Contains(msg, "not") {
		t.Fatalf("optional-to-optional assignment message = %q, want plain type mismatch", msg)
	}
}

func TestReturnProofMessagesUseCentralDisplay(t *testing.T) {
	if got := returnDeclaredTypeEvidence("returned value 1", typ.String); got != "returned value 1 must satisfy declared return type string" {
		t.Fatalf("returnDeclaredTypeEvidence = %q", got)
	}
	if got := returnIndexedReadProofMessage("returned value 1 (xs[i])"); !strings.Contains(got, "returned value 1 (xs[i]) is an indexed read") ||
		!strings.Contains(got, "declared return type here") {
		t.Fatalf("returnIndexedReadProofMessage = %q", got)
	}
	if got := returnExplicitBoundaryProofMessage("returned value 1"); got != "returned value 1 comes from any/unknown" {
		t.Fatalf("returnExplicitBoundaryProofMessage = %q", got)
	}
	if got := returnMissingProofMessage("returned value 1 (raw)"); got != "no proof on this path shows returned value 1 (raw) satisfies the declared return type" {
		t.Fatalf("returnMissingProofMessage = %q", got)
	}
	if got := callResultAssignmentHelp(false); got != "Assign the call result to a compatible target type, or change the callee return type if this result is valid." {
		t.Fatalf("callResultAssignmentHelp mismatch = %q", got)
	}
	if got := callResultAssignmentHelp(true); got != "Guard the call result before assigning it, provide a default value, or change the target type to accept nil." {
		t.Fatalf("callResultAssignmentHelp optional = %q", got)
	}
	if got := callResultDeclaredReturnEvidence("get", "call result 1", typ.MaterializeOptional(typ.String)); got != "get declares call result 1 as string?" {
		t.Fatalf("callResultDeclaredReturnEvidence = %q", got)
	}
}

func TestCallNilabilityMessagesUseCentralDisplay(t *testing.T) {
	if got := display.PossiblyNilCallTargetMessage("maybe_send"); got != "cannot call maybe_send because it may be nil" {
		t.Fatalf("possiblyNilCallTargetMessage = %q", got)
	}
	if got := display.PossiblyNilCalleeTypeEvidence("maybe_send", typ.MaterializeOptional(typ.String), false); got != "maybe_send can be string or nil at the call" {
		t.Fatalf("possiblyNilCalleeTypeEvidence noncallable = %q", got)
	}
	if got := display.PossiblyNilCalleeTypeEvidence("maybe_send", nil, false); got != "maybe_send may be nil at the call" {
		t.Fatalf("possiblyNilCalleeTypeEvidence nil = %q", got)
	}
	if got := display.PossiblyNilCalleeTypeEvidence("maybe_send", typ.MaterializeOptional(typ.String), true); got != "maybe_send has a callable type, but may also be nil" {
		t.Fatalf("possiblyNilCalleeTypeEvidence callable = %q", got)
	}
	if got := display.MissingNonNilBeforeCallMessage("maybe_send"); got != "no guard on this path proves maybe_send is non-nil before this call" {
		t.Fatalf("missingNonNilBeforeCallMessage = %q", got)
	}
	if got := display.PossiblyNilCallTargetHelp("maybe_send"); got != "Guard `maybe_send` with a nil check before calling it." {
		t.Fatalf("possiblyNilCallTargetHelp = %q", got)
	}
}

func TestOptionalMethodMessagesUseCentralDisplay(t *testing.T) {
	if got := display.OptionalMethodCallMessage(); got != "cannot call method on an optional value without a nil check" {
		t.Fatalf("optionalMethodCallMessage = %q", got)
	}
	if got := display.OptionalMethodReceiverEvidence("receiver client", " at call to client.send"); got != "receiver client is optional at call to client.send" {
		t.Fatalf("optionalMethodReceiverEvidence = %q", got)
	}
	if got := display.OptionalMethodReceiverEvidence("receiver", ""); got != "receiver is optional" {
		t.Fatalf("optionalMethodReceiverEvidence bare receiver = %q", got)
	}
	if got := display.OptionalMethodMissingNilCheckEvidence("receiver client", "calling client.send"); got != "no nil check proves receiver client is present before calling client.send" {
		t.Fatalf("optionalMethodMissingNilCheckEvidence = %q", got)
	}
	if got := display.OptionalMethodCallHelp("client", "client.send"); got != "check client ~= nil before calling client.send." {
		t.Fatalf("optionalMethodCallHelp named = %q", got)
	}
	if got := display.OptionalMethodCallHelp("", ""); got != "check the receiver for nil before calling a method on it." {
		t.Fatalf("optionalMethodCallHelp receiver = %q", got)
	}
}

func TestCallArityMessagesUseCentralDisplay(t *testing.T) {
	if got := callArityMismatchMessage("encode", 2, 1); got != "encode expects 2 arguments, got 1" {
		t.Fatalf("callArityMismatchMessage = %q", got)
	}
	if got := callArityMismatchMessage("encode", 1, 0); got != "encode expects 1 argument, got 0" {
		t.Fatalf("callArityMismatchMessage singular = %q", got)
	}
	if got := callArgumentCountEvidence("encode", 1); got != "call to encode passes 1 argument" {
		t.Fatalf("callArgumentCountEvidence = %q", got)
	}
	if got := callArgumentCountEvidence("encode", 2); got != "call to encode passes 2 arguments" {
		t.Fatalf("callArgumentCountEvidence plural = %q", got)
	}
	if got := callParameterCountEvidence("encode", 2); got != "encode declares 2 parameters" {
		t.Fatalf("callParameterCountEvidence = %q", got)
	}
	if got := callParameterCountEvidence("encode", 1); got != "encode declares 1 parameter" {
		t.Fatalf("callParameterCountEvidence singular = %q", got)
	}
	if got := callParameterTypeEvidence("encode", 2, ".payload", typ.String); got != "encode parameter 2.payload expects string" {
		t.Fatalf("callParameterTypeEvidence = %q", got)
	}
	if got := callArityHelp(2, 1); got != "Pass the missing required arguments, or change the callee signature if fewer arguments are valid." {
		t.Fatalf("callArityHelp too few = %q", got)
	}
	if got := callArityHelp(1, 2); got != "Remove the extra argument, or change the callee signature if the extra argument is valid." {
		t.Fatalf("callArityHelp too many = %q", got)
	}
}

func TestNumericForMessagesUseCentralDisplay(t *testing.T) {
	if got := display.NumericForOperandMessage("initial value", typ.String); got != "numeric for initial value must be number, got string" {
		t.Fatalf("display.NumericForOperandMessage = %q", got)
	}
	if got := display.NumericForOperandTypeEvidence("limit", typ.String); got != "limit has type string" {
		t.Fatalf("display.NumericForOperandTypeEvidence = %q", got)
	}
	if got := display.NumericForOperandHelp("step"); got != "Use a number for the numeric for step, or convert it before the loop." {
		t.Fatalf("display.NumericForOperandHelp = %q", got)
	}
}

func TestConcatOperandMessagesUseCentralDisplay(t *testing.T) {
	if got := display.ConcatOperandMessage("right"); got != "right operand of `..` may be nil" {
		t.Fatalf("display.ConcatOperandMessage = %q", got)
	}
	if got := display.ConcatOperandTypeEvidence("right", "maybe", typ.MaterializeOptional(typ.String)); got != "right operand `maybe` can be string or nil here" {
		t.Fatalf("display.ConcatOperandTypeEvidence named = %q", got)
	}
	if got := display.ConcatOperandTypeEvidence("left", "", typ.MaterializeOptional(typ.Number)); got != "left operand can be number or nil here" {
		t.Fatalf("display.ConcatOperandTypeEvidence anonymous = %q", got)
	}
	if got := display.ConcatOperandHelp("maybe"); got != "Guard `maybe` or provide a default string before using `..`." {
		t.Fatalf("display.ConcatOperandHelp named = %q", got)
	}
	if got := display.ConcatOperandHelp(""); got != "Guard the value or provide a default string before using `..`." {
		t.Fatalf("display.ConcatOperandHelp anonymous = %q", got)
	}
}
