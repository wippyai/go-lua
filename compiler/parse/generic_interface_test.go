package parse

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func requireParsedSpan(t *testing.T, node ast.PositionHolder, line, column, lastLine, lastColumn int) {
	t.Helper()
	if node == nil {
		t.Fatal("span node is nil")
	}
	if node.Line() != line || node.Column() != column || node.LastLine() != lastLine || node.LastColumn() != lastColumn {
		t.Fatalf("span = %d:%d-%d:%d, want %d:%d-%d:%d", node.Line(), node.Column(), node.LastLine(), node.LastColumn(), line, column, lastLine, lastColumn)
	}
}

func requireTypeRefRoot(t *testing.T, ref *ast.TypeRefExpr, file string, line, column, endColumn int) {
	t.Helper()
	if ref == nil {
		t.Fatal("type reference is nil")
	}
	got := ref.RootPosition
	if got.File != file || got.Line != line || got.Column != column ||
		got.EndLine != line || got.EndColumn != endColumn {
		t.Fatalf("type reference root = %#v, want %s:%d:%d-%d", got, file, line, column, endColumn)
	}
}

func TestParseCallsRetainCompleteArgumentSyntaxSpans(t *testing.T) {
	stmts, err := ParseString(`local empty = f()
local values = f(x, y)
local method = receiver:apply()
local generic = receiver:apply::<Box<Result<number>>>()
local tableArg = f { value = 1 }
local stringArg = f "payload"
local grouped = (f())`, "call_spans.lua")
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) != 7 {
		t.Fatalf("statements = %d, want 7", len(stmts))
	}

	want := [][4]int{
		{1, 15, 1, 17},
		{2, 16, 2, 22},
		{3, 16, 3, 31},
		{4, 17, 4, 55},
		{5, 18, 5, 32},
		{6, 19, 6, 29},
		{7, 17, 7, 21},
	}
	for index, span := range want {
		call := parsedInitializerCall(t, stmts[index])
		requireParsedSpan(t, call, span[0], span[1], span[2], span[3])
	}

	generic := parsedInitializerCall(t, stmts[3])
	if len(generic.TypeArgs) != 1 {
		t.Fatalf("generic TypeArgs = %#v", generic.TypeArgs)
	}
	box, ok := generic.TypeArgs[0].(*ast.GenericTypeExpr)
	if !ok || box.Base == nil || len(box.Args) != 1 {
		t.Fatalf("generic TypeArg = %#v, want Box<Result<number>>", generic.TypeArgs[0])
	}
	requireParsedSpan(t, box, 4, 34, 4, 52)
	requireParsedSpan(t, box.Base, 4, 34, 4, 36)
	result, ok := box.Args[0].(*ast.GenericTypeExpr)
	if !ok || result.Base == nil {
		t.Fatalf("nested TypeArg = %#v, want Result<number>", box.Args[0])
	}
	requireParsedSpan(t, result, 4, 38, 4, 51)
	requireParsedSpan(t, result.Base, 4, 38, 4, 43)
}

func TestParseStaticTypeProvenanceIsExact(t *testing.T) {
	stmts, err := ParseString(`type Alias = pkg.deep.Value
type Boxed = Box<Result<number>>
interface Derived: pkg.deep.Base
  function map<T>(value: T): pkg.deep.Result
  function ready(): ()
end`, "type_spans.lua")
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) != 3 {
		t.Fatalf("statements = %d, want 3", len(stmts))
	}

	alias, ok := stmts[0].(*ast.TypeDefStmt).Type.(*ast.TypeRefExpr)
	if !ok {
		t.Fatalf("Alias type = %T, want TypeRefExpr", stmts[0].(*ast.TypeDefStmt).Type)
	}
	if got, want := alias.Path, []string{"pkg", "deep", "Value"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("Alias path = %v, want %v", got, want)
	}
	requireParsedSpan(t, alias, 1, 14, 1, 27)

	boxed, ok := stmts[1].(*ast.TypeDefStmt).Type.(*ast.GenericTypeExpr)
	if !ok || boxed.Base == nil || len(boxed.Args) != 1 {
		t.Fatalf("Boxed type = %#v, want Box<Result<number>>", stmts[1].(*ast.TypeDefStmt).Type)
	}
	requireParsedSpan(t, boxed, 2, 14, 2, 32)
	requireParsedSpan(t, boxed.Base, 2, 14, 2, 16)
	nested, ok := boxed.Args[0].(*ast.GenericTypeExpr)
	if !ok || nested.Base == nil {
		t.Fatalf("Boxed argument = %#v, want Result<number>", boxed.Args[0])
	}
	requireParsedSpan(t, nested, 2, 18, 2, 31)
	requireParsedSpan(t, nested.Base, 2, 18, 2, 23)

	iface := stmts[2].(*ast.InterfaceDefStmt)
	if len(iface.Extends) != 1 || len(iface.Members) != 2 {
		t.Fatalf("interface extends/members = %d/%d, want 1/2", len(iface.Extends), len(iface.Members))
	}
	requireParsedSpan(t, iface.Extends[0], 3, 20, 3, 32)
	mapMethod := iface.Members[0]
	mapSignature, ok := mapMethod.Type.(*ast.FunctionTypeExpr)
	if mapMethod.Kind != ast.InterfaceMethodMember || !ok || len(mapSignature.Returns) != 1 {
		t.Fatalf("map method = %#v", mapMethod)
	}
	requireParsedSpan(t, mapSignature, 4, 3, 4, 44)
	requireParsedSpan(t, mapSignature.Returns[0], 4, 30, 4, 44)
	second, ok := iface.Members[1].Type.(*ast.FunctionTypeExpr)
	if !ok {
		t.Fatalf("second interface member = %#v", iface.Members[1])
	}
	requireParsedSpan(t, second, 5, 3, 5, 22)
}

func TestParseStaticDeclarationsRetainExactNameTokensAndFullExtents(t *testing.T) {
	const source = `type Alias = pkg.deep.Value
type Generic<T> = pkg.deep.Box<Result<T>>
interface Shape: pkg.deep.Base
  value: Generic<number>
end`
	stmts, err := ParseString(source, "declaration_spans.lua")
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) != 3 {
		t.Fatalf("statements = %d, want 3", len(stmts))
	}

	alias := stmts[0].(*ast.TypeDefStmt)
	if alias.Name != "Alias" || alias.NamePosition.File != "declaration_spans.lua" ||
		alias.NamePosition.Line != 1 || alias.NamePosition.Column != 6 ||
		alias.NamePosition.EndLine != 1 || alias.NamePosition.EndColumn != 10 {
		t.Fatalf("Alias name = %q at %#v", alias.Name, alias.NamePosition)
	}
	requireParsedSpan(t, alias, 1, 1, 1, 27)

	generic := stmts[1].(*ast.TypeDefStmt)
	if generic.Name != "Generic" || generic.NamePosition.File != "declaration_spans.lua" ||
		generic.NamePosition.Line != 2 || generic.NamePosition.Column != 6 ||
		generic.NamePosition.EndLine != 2 || generic.NamePosition.EndColumn != 12 {
		t.Fatalf("Generic name = %q at %#v", generic.Name, generic.NamePosition)
	}
	requireParsedSpan(t, generic, 2, 1, 2, 41)

	iface := stmts[2].(*ast.InterfaceDefStmt)
	if iface.Name != "Shape" || iface.NamePosition.File != "declaration_spans.lua" ||
		iface.NamePosition.Line != 3 || iface.NamePosition.Column != 11 ||
		iface.NamePosition.EndLine != 3 || iface.NamePosition.EndColumn != 15 {
		t.Fatalf("Shape name = %q at %#v", iface.Name, iface.NamePosition)
	}
	requireParsedSpan(t, iface, 3, 1, 5, 3)
}

func TestParseQualifiedGenericBaseUsesCanonicalTypeReferencePath(t *testing.T) {
	const source = `type Subject = pkg.deep.Box<Result<number>>
type QualifiedArgument = Box<pkg.deep.Result<number>>`
	stmts, err := ParseString(source, "qualified_generic.lua")
	if err != nil {
		t.Fatal(err)
	}
	subject := stmts[0].(*ast.TypeDefStmt)
	generic, ok := subject.Type.(*ast.GenericTypeExpr)
	if !ok || generic.Base == nil || len(generic.Args) != 1 {
		t.Fatalf("Subject type = %#v, want qualified GenericTypeExpr", subject.Type)
	}
	wantPath := []string{"pkg", "deep", "Box"}
	if len(generic.Base.Path) != len(wantPath) {
		t.Fatalf("generic base path = %v, want %v", generic.Base.Path, wantPath)
	}
	for index, want := range wantPath {
		if generic.Base.Path[index] != want {
			t.Fatalf("generic base path = %v, want %v", generic.Base.Path, wantPath)
		}
	}
	requireParsedSpan(t, subject, 1, 1, 1, 43)
	requireParsedSpan(t, generic, 1, 16, 1, 43)
	requireParsedSpan(t, generic.Base, 1, 16, 1, 27)

	qualifiedArgument := stmts[1].(*ast.TypeDefStmt)
	outer, ok := qualifiedArgument.Type.(*ast.GenericTypeExpr)
	if !ok || outer.Base == nil || len(outer.Base.Path) != 1 || outer.Base.Path[0] != "Box" || len(outer.Args) != 1 {
		t.Fatalf("QualifiedArgument type = %#v, want Box<qualified generic>", qualifiedArgument.Type)
	}
	inner, ok := outer.Args[0].(*ast.GenericTypeExpr)
	if !ok || inner.Base == nil || len(inner.Base.Path) != 3 ||
		inner.Base.Path[0] != "pkg" || inner.Base.Path[1] != "deep" || inner.Base.Path[2] != "Result" {
		t.Fatalf("QualifiedArgument inner type = %#v, want pkg.deep.Result<number>", outer.Args[0])
	}
	requireParsedSpan(t, qualifiedArgument, 2, 1, 2, 53)
	requireParsedSpan(t, outer, 2, 26, 2, 53)
	requireParsedSpan(t, inner, 2, 30, 2, 52)
}

func TestParseTypeReferencesRetainExactRootToken(t *testing.T) {
	const source = `interface Child: Root, protocol.Parent
end
type Qualified = protocol.User
type Nested = protocol.Box<domain.Result<number>>`
	stmts, err := ParseString(source, "type_reference_roots.lua")
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) != 3 {
		t.Fatalf("statements = %d, want 3", len(stmts))
	}

	iface := stmts[0].(*ast.InterfaceDefStmt)
	if len(iface.Extends) != 2 {
		t.Fatalf("interface extends = %d, want 2", len(iface.Extends))
	}
	requireTypeRefRoot(t, iface.Extends[0], "type_reference_roots.lua", 1, 18, 21)
	requireTypeRefRoot(t, iface.Extends[1], "type_reference_roots.lua", 1, 24, 31)

	qualified, ok := stmts[1].(*ast.TypeDefStmt).Type.(*ast.TypeRefExpr)
	if !ok {
		t.Fatalf("Qualified type = %T, want TypeRefExpr", stmts[1].(*ast.TypeDefStmt).Type)
	}
	requireTypeRefRoot(t, qualified, "type_reference_roots.lua", 3, 18, 25)

	nested, ok := stmts[2].(*ast.TypeDefStmt).Type.(*ast.GenericTypeExpr)
	if !ok || nested.Base == nil || len(nested.Args) != 1 {
		t.Fatalf("Nested type = %#v, want qualified GenericTypeExpr", stmts[2].(*ast.TypeDefStmt).Type)
	}
	requireTypeRefRoot(t, nested.Base, "type_reference_roots.lua", 4, 15, 22)
	argument, ok := nested.Args[0].(*ast.GenericTypeExpr)
	if !ok || argument.Base == nil {
		t.Fatalf("Nested argument = %#v, want qualified GenericTypeExpr", nested.Args[0])
	}
	requireTypeRefRoot(t, argument.Base, "type_reference_roots.lua", 4, 28, 33)
	requireParsedSpan(t, nested, 4, 15, 4, 49)
	requireParsedSpan(t, argument, 4, 28, 4, 48)
}

func TestParseEveryStaticTypeConstructorRetainsItsFullExtent(t *testing.T) {
	tests := []struct {
		syntax   string
		wantType string
	}{
		{`number`, `*ast.PrimitiveTypeExpr`},
		{`true`, `*ast.LiteralTypeExpr`},
		{`"literal"`, `*ast.LiteralTypeExpr`},
		{`42`, `*ast.LiteralTypeExpr`},
		{`pkg.deep.Value`, `*ast.TypeRefExpr`},
		{`Box<Result<number>>`, `*ast.GenericTypeExpr`},
		{`{number}`, `*ast.ArrayTypeExpr`},
		{`{[string]: number}`, `*ast.MapTypeExpr`},
		{`{value: number, optional?: string}`, `*ast.RecordTypeExpr`},
		{`(value: number) -> (string, boolean)`, `*ast.FunctionTypeExpr`},
		{`() -> ()`, `*ast.FunctionTypeExpr`},
		{`fun(value: number): (string, boolean)`, `*ast.FunctionTypeExpr`},
		{`fun()`, `*ast.FunctionTypeExpr`},
		{`interface {value: number}`, `*ast.RecordTypeExpr`},
		{`interface {}`, `*ast.RecordTypeExpr`},
		{`number[]`, `*ast.ArrayTypeExpr`},
		{`number @element[] @array`, `*ast.AnnotatedTypeExpr`},
		{`readonly {number}`, `*ast.ArrayTypeExpr`},
		{`readonly {[string]: number}`, `*ast.MapTypeExpr`},
		{`readonly {value: number}`, `*ast.RecordTypeExpr`},
		{`{}`, `*ast.RecordTypeExpr`},
		{`asserts candidate`, `*ast.AssertsTypeExpr`},
		{`asserts candidate is pkg.deep.Value`, `*ast.AssertsTypeExpr`},
		{`typeof(make())`, `*ast.TypeOfExpr`},
		{`keyof(pkg.deep.Value)`, `*ast.KeyOfExpr`},
		{`pkg.deep.Value["field"]`, `*ast.IndexAccessExpr`},
		{`T extends U ? A : B`, `*ast.ConditionalTypeExpr`},
		{`number?`, `*ast.OptionalTypeExpr`},
		{`number | string | boolean`, `*ast.UnionTypeExpr`},
		{`A & B & C`, `*ast.IntersectionTypeExpr`},
		{`(number)`, `*ast.PrimitiveTypeExpr`},
		{`number @range(1, 2)`, `*ast.AnnotatedTypeExpr`},
	}
	for _, test := range tests {
		t.Run(test.syntax, func(t *testing.T) {
			source := `type Subject = ` + test.syntax
			stmts, err := ParseString(source, "static_extent.lua")
			if err != nil {
				t.Fatal(err)
			}
			typ := stmts[0].(*ast.TypeDefStmt).Type
			if got := fmtType(typ); got != test.wantType {
				t.Fatalf("type = %s, want %s", got, test.wantType)
			}
			start := strings.Index(source, test.syntax) + 1
			requireParsedSpan(t, typ, 1, start, 1, start+len(test.syntax)-1)
		})
	}
}

func TestParseStaticParameterIdentitiesRetainExactTokens(t *testing.T) {
	source := `type Callable = (named: number, string, ... boolean) -> number
type Guard = asserts candidate is string
interface Service
  function apply<T>(input: T): T
end`
	stmts, err := ParseString(source, "static_names.lua")
	if err != nil {
		t.Fatal(err)
	}

	callable := stmts[0].(*ast.TypeDefStmt).Type.(*ast.FunctionTypeExpr)
	if len(callable.Params) != 2 {
		t.Fatalf("Callable fixed params = %#v, want two", callable.Params)
	}
	if got := callable.Params[0].NamePosition; got.File != "static_names.lua" || got.Line != 1 || got.Column != 18 || got.EndColumn != 22 {
		t.Fatalf("named parameter position = %#v, want line 1 columns 18-22", got)
	}
	if got := callable.Params[1].NamePosition; got != (ast.Position{}) {
		t.Fatalf("anonymous parameter position = %#v, want zero", got)
	}
	if got := callable.VariadicPosition; got.File != "static_names.lua" || got.Line != 1 || got.Column != 41 || got.EndColumn != 43 {
		t.Fatalf("variadic parameter position = %#v, want line 1 columns 41-43", got)
	}
	if typ, ok := callable.Variadic.(*ast.PrimitiveTypeExpr); !ok || typ.Name != "boolean" {
		t.Fatalf("variadic type = %#v, want boolean", callable.Variadic)
	}

	assertion := stmts[1].(*ast.TypeDefStmt).Type.(*ast.AssertsTypeExpr)
	if assertion.ParamName != "candidate" || assertion.ParamPosition.File != "static_names.lua" || assertion.ParamPosition.Line != 2 || assertion.ParamPosition.Column != 22 || assertion.ParamPosition.EndColumn != 30 {
		t.Fatalf("asserted parameter identity = %q at %#v", assertion.ParamName, assertion.ParamPosition)
	}
	requireParsedSpan(t, assertion, 2, 14, 2, 40)

	method := stmts[2].(*ast.InterfaceDefStmt).Members[0]
	signature, ok := method.Type.(*ast.FunctionTypeExpr)
	if method.Kind != ast.InterfaceMethodMember || !ok || len(signature.Params) != 1 {
		t.Fatalf("interface method = %#v", method)
	}
	if got := signature.Params[0].NamePosition; got.File != "static_names.lua" || got.Line != 4 || got.Column != 21 || got.EndColumn != 25 {
		t.Fatalf("interface parameter position = %#v, want line 4 columns 21-25", got)
	}
}

func fmtType(value any) string {
	return fmt.Sprintf("%T", value)
}

func TestParseFunctionTypeRetainsThreeReturnsOnceEach(t *testing.T) {
	stmts, err := ParseString(`type Returns = () -> (number, string, boolean)`, "returns.lua")
	if err != nil {
		t.Fatal(err)
	}
	fn, ok := stmts[0].(*ast.TypeDefStmt).Type.(*ast.FunctionTypeExpr)
	if !ok {
		t.Fatalf("Returns type = %T, want FunctionTypeExpr", stmts[0].(*ast.TypeDefStmt).Type)
	}
	if len(fn.Returns) != 3 {
		t.Fatalf("returns = %#v, want exactly three entries", fn.Returns)
	}
	for index, want := range []string{"number", "string", "boolean"} {
		got, ok := fn.Returns[index].(*ast.PrimitiveTypeExpr)
		if !ok || got.Name != want {
			t.Fatalf("return[%d] = %#v, want %q", index, fn.Returns[index], want)
		}
	}
}

func TestParseGenericCallsPreserveTypeArgsAndCallShapes(t *testing.T) {
	stmts, err := ParseString(`
local direct = f::<T, U>(x, y)
local member = obj.method::<Box<Result<number>>>(x)
local colon = obj:method::<T>(x)
`, "generic_calls.lua")
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) != 3 {
		t.Fatalf("statements = %d, want 3", len(stmts))
	}

	direct := parsedInitializerCall(t, stmts[0])
	if direct.Func == nil || direct.Receiver != nil || direct.Method != "" || len(direct.TypeArgs) != 2 || len(direct.Args) != 2 {
		t.Fatalf("direct generic call = %#v", direct)
	}
	for index, want := range []string{"T", "U"} {
		arg, ok := direct.TypeArgs[index].(*ast.PrimitiveTypeExpr)
		if !ok || arg.Name != want {
			t.Fatalf("direct TypeArg[%d] = %#v, want %s", index, direct.TypeArgs[index], want)
		}
	}

	member := parsedInitializerCall(t, stmts[1])
	attr, ok := member.Func.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyDot || len(member.TypeArgs) != 1 || len(member.Args) != 1 {
		t.Fatalf("member generic call = %#v", member)
	}
	box, ok := member.TypeArgs[0].(*ast.GenericTypeExpr)
	if !ok || box.Base == nil || len(box.Base.Path) != 1 || box.Base.Path[0] != "Box" || len(box.Args) != 1 {
		t.Fatalf("member nested TypeArg = %#v", member.TypeArgs[0])
	}
	result, ok := box.Args[0].(*ast.GenericTypeExpr)
	if !ok || result.Base == nil || len(result.Base.Path) != 1 || result.Base.Path[0] != "Result" || len(result.Args) != 1 {
		t.Fatalf("member nested closegt TypeArg = %#v", box.Args[0])
	}

	colon := parsedInitializerCall(t, stmts[2])
	if colon.Func != nil || colon.Receiver == nil || colon.Method != "method" || len(colon.TypeArgs) != 1 || len(colon.Args) != 1 {
		t.Fatalf("colon generic call = %#v", colon)
	}
	if got := colon.MethodPosition; got.Line != 4 || got.Column != 19 {
		t.Fatalf("colon method position = %#v, want 4:19", got)
	}
}

func TestParseGenericCallAmbiguityPreservesComparisons(t *testing.T) {
	for _, source := range []string{
		`return left < right`,
		`return left > right`,
		`return left <= right`,
		`return left >= right`,
		`return left == right`,
		`return left ~= right`,
		`return left < middle > right`,
	} {
		stmts, err := ParseString(source, "comparison.lua")
		if err != nil {
			t.Fatalf("ParseString(%q): %v", source, err)
		}
		ret, ok := stmts[0].(*ast.ReturnStmt)
		if !ok || len(ret.Exprs) != 1 {
			t.Fatalf("ParseString(%q) = %#v, want one Return expression", source, stmts)
		}
		if _, ok := ret.Exprs[0].(*ast.RelationalOpExpr); !ok {
			t.Fatalf("ParseString(%q) expression = %T, want relational expression", source, ret.Exprs[0])
		}
	}

	stmts, err := ParseString(`return left < middle and middle < right`, "comparison.lua")
	if err != nil {
		t.Fatal(err)
	}
	ret, ok := stmts[0].(*ast.ReturnStmt)
	if !ok || len(ret.Exprs) != 1 {
		t.Fatalf("logical comparison = %#v, want one Return expression", stmts)
	}
	if _, ok := ret.Exprs[0].(*ast.LogicalOpExpr); !ok {
		t.Fatalf("logical comparison expression = %T, want LogicalOpExpr", ret.Exprs[0])
	}
}

func TestParseGenericCallsRejectMalformedTypeArgs(t *testing.T) {
	for _, source := range []string{
		`local value = f::<T>(`,
		`local value = f::<T,>(value)`,
		`local value = obj:method::<T>(`,
		`local value = f::<T(value)`,
	} {
		if _, err := ParseString(source, "malformed_generic_call.lua"); err == nil {
			t.Fatalf("ParseString(%q) succeeded", source)
		}
	}
}

func TestParseTurbofishRequiresAdjacentAngleAndPreservesCasts(t *testing.T) {
	if _, err := ParseString(`local value = f:: <T>(value)`, "turbofish_spacing.lua"); err == nil {
		t.Fatal("spaced :: < generic call unexpectedly parsed")
	}

	stmts, err := ParseString(`local value = f :: T`, "cast.lua")
	if err != nil {
		t.Fatal(err)
	}
	local, ok := stmts[0].(*ast.LocalAssignStmt)
	if !ok || len(local.Exprs) != 1 {
		t.Fatalf("cast statement = %#v", stmts)
	}
	cast, ok := local.Exprs[0].(*ast.CastExpr)
	if !ok || cast.Syntax != ast.CastSyntaxColonColon {
		t.Fatalf("cast expression = %#v, want colon-colon CastExpr", local.Exprs[0])
	}
}

func TestParseTurbofishReservedMethodNamesPreserveSourceSpelling(t *testing.T) {
	for _, name := range []string{"type", "interface", "readonly", "as", "asserts", "is"} {
		stmts, err := ParseString(`local value = receiver:`+name+`::<T>()`, "reserved_method.lua")
		if err != nil {
			t.Fatalf("ParseString(%q): %v", name, err)
		}
		call := parsedInitializerCall(t, stmts[0])
		if call.Method != name || call.MethodPosition.Line != 1 || call.MethodPosition.Column != 24 {
			t.Fatalf("method %q = %q at %#v", name, call.Method, call.MethodPosition)
		}
		if len(call.TypeArgs) != 1 {
			t.Fatalf("method %q TypeArgs = %#v", name, call.TypeArgs)
		}
	}
}

func TestParseInterfaceReachesFieldsQualifiedExtendsAndGenericMethods(t *testing.T) {
	stmts, err := ParseString(`interface Derived: pkg.Parent, third.Deep.Base
  id: number @min(1)
  function map<T: Constraint>(value: T): T
  enabled?: boolean @flag
  function ready(): ()
end`, "interface.lua")
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) != 1 {
		t.Fatalf("statements = %d, want 1", len(stmts))
	}
	iface, ok := stmts[0].(*ast.InterfaceDefStmt)
	if !ok {
		t.Fatalf("statement = %T, want InterfaceDefStmt", stmts[0])
	}
	if iface.Name != "Derived" || len(iface.Extends) != 2 {
		t.Fatalf("interface name/extends = %q/%#v", iface.Name, iface.Extends)
	}
	for index, want := range [][]string{{"pkg", "Parent"}, {"third", "Deep", "Base"}} {
		got := iface.Extends[index]
		if got == nil || len(got.Path) != len(want) {
			t.Fatalf("extends[%d] = %#v, want %v", index, got, want)
		}
		for part, name := range want {
			if got.Path[part] != name {
				t.Fatalf("extends[%d].Path = %v, want %v", index, got.Path, want)
			}
		}
	}
	if got := iface.Extends[0]; got.Line() != 1 || got.Column() != 20 || got.LastLine() != 1 || got.LastColumn() != 29 {
		t.Fatalf("first qualified extends position = %d:%d-%d:%d, want 1:20-1:29", got.Line(), got.Column(), got.LastLine(), got.LastColumn())
	}
	if got := iface.Extends[1]; got.Line() != 1 || got.Column() != 32 || got.LastLine() != 1 || got.LastColumn() != 46 {
		t.Fatalf("second qualified extends position = %d:%d-%d:%d, want 1:32-1:46", got.Line(), got.Column(), got.LastLine(), got.LastColumn())
	}
	if len(iface.Members) != 4 {
		t.Fatalf("interface members = %d, want 4", len(iface.Members))
	}
	first, method, second, ready := iface.Members[0], iface.Members[1], iface.Members[2], iface.Members[3]
	firstType, ok := first.Type.(*ast.AnnotatedTypeExpr)
	if first.Kind != ast.InterfaceFieldMember || !ok || first.Name != "id" || first.Optional || len(firstType.Annotations) != 1 || firstType.Annotations[0].Name != "min" || first.NamePosition.Line != 2 || first.NamePosition.Column != 3 {
		t.Fatalf("first interface field = %#v", first)
	}
	if _, ok := firstType.Inner.(*ast.PrimitiveTypeExpr); !ok {
		t.Fatalf("first interface field inner type = %T, want *ast.PrimitiveTypeExpr", firstType.Inner)
	}
	secondType, ok := second.Type.(*ast.AnnotatedTypeExpr)
	if second.Kind != ast.InterfaceFieldMember || !ok || second.Name != "enabled" || !second.Optional || len(secondType.Annotations) != 1 || secondType.Annotations[0].Name != "flag" || second.NamePosition.Line != 4 || second.NamePosition.Column != 3 {
		t.Fatalf("second interface field = %#v", second)
	}
	if _, ok := secondType.Inner.(*ast.PrimitiveTypeExpr); !ok {
		t.Fatalf("second interface field inner type = %T, want *ast.PrimitiveTypeExpr", secondType.Inner)
	}
	methodType, ok := method.Type.(*ast.FunctionTypeExpr)
	if method.Kind != ast.InterfaceMethodMember || !ok || method.Name != "map" || method.NamePosition.Line != 3 || method.NamePosition.Column != 12 || len(methodType.TypeParams) != 1 || len(methodType.Params) != 1 || len(methodType.Returns) != 1 {
		t.Fatalf("generic interface method = %#v", method)
	}
	if methodType.TypeParams[0].Name != "T" || methodType.TypeParams[0].NamePosition.Line != 3 || methodType.TypeParams[0].NamePosition.Column != 16 || methodType.TypeParams[0].Constraint == nil {
		t.Fatalf("generic interface method TypeParams = %#v", methodType.TypeParams)
	}
	readyType, ok := ready.Type.(*ast.FunctionTypeExpr)
	if ready.Kind != ast.InterfaceMethodMember || !ok || ready.Name != "ready" || ready.NamePosition.Line != 5 || ready.NamePosition.Column != 12 || readyType.Returns == nil || len(readyType.Returns) != 0 {
		t.Fatalf("empty-return interface method = %#v", ready)
	}
	if iface.Line() != 1 || iface.LastLine() != 6 || iface.LastColumn() != 3 {
		t.Fatalf("interface span = %d:%d-%d:%d, want 1:1-6:3", iface.Line(), iface.Column(), iface.LastLine(), iface.LastColumn())
	}
}

func TestParseSharedStaticFieldAndMethodNamePolicies(t *testing.T) {
	stmts, err := ParseString(`type RecordNames = { type: number, typeof?: string }
interface InterfaceNames
  type: number
  typeof?: string
  function readonly(): ()
end`, "contextual_names.lua")
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) != 2 {
		t.Fatalf("statements = %d, want 2", len(stmts))
	}
	record := stmts[0].(*ast.TypeDefStmt).Type.(*ast.RecordTypeExpr)
	if len(record.Fields) != 2 || record.Fields[0].Name != "type" || record.Fields[1].Name != "typeof" || !record.Fields[1].Optional {
		t.Fatalf("record fields = %#v", record.Fields)
	}
	iface := stmts[1].(*ast.InterfaceDefStmt)
	if len(iface.Members) != 3 || iface.Members[0].Kind != ast.InterfaceFieldMember || iface.Members[0].Name != "type" || iface.Members[1].Kind != ast.InterfaceFieldMember || iface.Members[1].Name != "typeof" || !iface.Members[1].Optional {
		t.Fatalf("interface members = %#v", iface.Members)
	}
	if iface.Members[2].Kind != ast.InterfaceMethodMember || iface.Members[2].Name != "readonly" {
		t.Fatalf("interface members = %#v", iface.Members)
	}
}

func TestParseInterfaceRejectsUnsupportedTypeParamsAndMalformedMethods(t *testing.T) {
	for _, source := range []string{
		`interface Generic<T> end`,
		`interface Broken function method<T>( end`,
	} {
		if _, err := ParseString(source, "malformed_interface.lua"); err == nil {
			t.Fatalf("ParseString(%q) succeeded", source)
		}
	}
}

func parsedInitializerCall(t *testing.T, stmt ast.Stmt) *ast.FuncCallExpr {
	t.Helper()
	local, ok := stmt.(*ast.LocalAssignStmt)
	if !ok || len(local.Exprs) != 1 {
		t.Fatalf("statement = %#v, want one-initializer local", stmt)
	}
	call, ok := local.Exprs[0].(*ast.FuncCallExpr)
	if !ok {
		t.Fatalf("initializer = %T, want FuncCallExpr", local.Exprs[0])
	}
	return call
}
