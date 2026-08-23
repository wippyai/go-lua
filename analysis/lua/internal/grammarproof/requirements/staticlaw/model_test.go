package staticlaw

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/provenance"
	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	sourceowner "github.com/wippyai/go-lua/analysis/program/source"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
	"github.com/wippyai/go-lua/analysis/program/target/typeindex"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

const staticLawFile = "static.lua"

// TestExactStaticSourceLaws establishes source span, typed relation, and
// authored child order for every parser-reachable static constructor. The
// StaticTypeTerm enumeration is the one public static-type authority; it is
// not a generic Program-Term scan.
func TestExactStaticSourceLaws(t *testing.T) {
	seen := make(map[Family]bool, len(Requirements()))
	for _, family := range Requirements() {
		if seen[family] {
			t.Fatalf("duplicate static requirement %d", family)
		}
		seen[family] = true
		t.Run(fmt.Sprintf("family-%d", family), func(t *testing.T) {
			source := staticSource(family)
			statements, err := parse.ParseString(source, staticLawFile)
			if err != nil {
				t.Fatal(err)
			}
			if bound := bind.BindChunk(statements, typeindex.Table{}); bound == nil {
				t.Fatal("public binder returned nil result")
			}
			p, err := lualower.Lower(lualower.Source{Name: staticLawFile, Text: []byte(source)})
			if err != nil {
				t.Fatal(err)
			}
			declaration, err := staticDeclaration(statements)
			if err != nil {
				t.Fatal(err)
			}
			if err := verifyStaticFamily(p, declaration, family); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestExactInterfaceSourceLaw keeps declaration-level interface provenance
// separate from type-expression constructors. It proves the name/token and
// ordered member relation without collapsing interface fields into a record.
func TestExactInterfaceSourceLaw(t *testing.T) {
	const source = "interface Subject\n  field?: string\nend"
	statements, err := parse.ParseString(source, staticLawFile)
	if err != nil {
		t.Fatal(err)
	}
	if bound := bind.BindChunk(statements, typeindex.Table{}); bound == nil {
		t.Fatal("public binder returned nil result")
	}
	p, err := lualower.Lower(lualower.Source{Name: staticLawFile, Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	declaration, ok := statements[0].(*ast.InterfaceDefStmt)
	if !ok || len(declaration.Members) != 1 {
		t.Fatalf("interface source = %T", statements[0])
	}
	interfaces := p.Static().Declarations().Interfaces()
	term, err := anchoredTerm(p, declaration, interfaces.Count, interfaces.At)
	if err != nil {
		t.Fatal(err)
	}
	owner, name, _, ok := interfaces.Get(term)
	if !ok || owner == 0 || name == 0 {
		t.Fatalf("Interface = owner %v name %v ok %v", owner, name, ok)
	}
	value, ok := p.Source().Keys().Exact(name)
	if !ok || value.Kind != keyspace.LiteralString || value.String != declaration.Name {
		t.Fatalf("Interface name = %#v/%v", value, ok)
	}
	if span, ok := p.Source().Identity().Render(func() sourceowner.Coordinate {
		_, _, coordinate, _ := interfaces.Get(term)
		return coordinate
	}()); !ok || span != tokenSpan(declaration.NamePosition) {
		t.Fatalf("Interface name span = %#v/%v, want %#v", span, ok, tokenSpan(declaration.NamePosition))
	}
	if count, ok := interfaces.MemberCount(term); !ok || count != 1 {
		t.Fatalf("Interface members = %d/%v", count, ok)
	}
	member, ok := interfaces.MemberAt(term, 0)
	if !ok || member.Kind != staticdecl.InterfaceField || member.Field == 0 {
		t.Fatalf("Interface member = %#v/%v", member, ok)
	}
	key, typ, optional, ok := p.Static().Types().Fields().Get(member.Field)
	_ = key
	if !ok || !optional || typ == 0 {
		t.Fatalf("Interface TypeField = type %v optional %v ok %v", typ, optional, ok)
	}
	if err := exactTokenSpan(p, member.Field, declaration.Members[0].NamePosition); err != nil {
		t.Fatalf("Interface field token: %v", err)
	}
	if err := exactStaticChild(p, typ, declaration.Members[0].Type); err != nil {
		t.Fatalf("Interface field type: %v", err)
	}
	if err := provenance.Exact(p.Source().Identity(), term, declaration, staticLawFile); err != nil {
		t.Fatalf("Interface provenance: %v", err)
	}
}

func staticSource(family Family) string {
	switch family {
	case FamilyPrimitive:
		return "type Subject = number"
	case FamilyLiteral:
		return "type Subject = \"value\""
	case FamilyOptional:
		return "type Subject = string?"
	case FamilyUnion:
		return "type Subject = string | number"
	case FamilyIntersection:
		return "type Subject = Stringish & Numberish"
	case FamilyTypeRef:
		return "type Subject = Namespace.Member"
	case FamilyGeneric:
		return "type Subject = Box<string>"
	case FamilyArray:
		return "type Subject = readonly {string}"
	case FamilyMap:
		return "type Subject = readonly {[string]: number}"
	case FamilyRecord:
		return "type Subject = readonly {field?: string}"
	case FamilySignature:
		return "type Subject = fun(value: string, ... number): (boolean)"
	case FamilyAssertion:
		return "type Subject = asserts value is string"
	case FamilyTypeOf:
		return "local value = 1\ntype Subject = typeof(value)"
	case FamilyKeyOf:
		return "type Subject = keyof(Record)"
	case FamilyIndexAccess:
		return "type Subject = Record[\"field\"]"
	case FamilyConditional:
		return "type Subject = Check extends Constraint ? Then : Else"
	case FamilyAnnotated:
		return "type Subject = string @tag(1)"
	default:
		panic(fmt.Sprintf("invalid static family %d", family))
	}
}

func staticDeclaration(statements []ast.Stmt) (*ast.TypeDefStmt, error) {
	for _, statement := range statements {
		if declaration, ok := statement.(*ast.TypeDefStmt); ok {
			return declaration, nil
		}
	}
	return nil, fmt.Errorf("static source has no type declaration")
}

func verifyStaticFamily(p *program.Program, declaration *ast.TypeDefStmt, family Family) error {
	if p == nil || declaration == nil || declaration.Type == nil {
		return fmt.Errorf("missing Program or static source declaration")
	}
	alias, err := anchoredTypeAlias(p, declaration)
	if err != nil {
		return err
	}
	aliases := p.Static().Declarations().Aliases()
	_, target, _, _, ok := aliases.Get(alias)
	if !ok || target == 0 {
		return fmt.Errorf("TypeAlias has no static target")
	}
	_, _, _, nameCoordinate, ok := aliases.Get(alias)
	nameSpan, rendered := p.Source().Identity().Render(nameCoordinate)
	if !ok || !rendered || nameSpan != tokenSpan(declaration.NamePosition) {
		return fmt.Errorf("TypeAlias name span = %#v/%v, want %#v", nameSpan, ok && rendered, tokenSpan(declaration.NamePosition))
	}
	if family == FamilyAnnotated {
		return verifyAnnotated(p, declaration.Type, target)
	}
	term, err := anchoredStaticTerm(p, declaration.Type)
	if err != nil {
		return err
	}
	if term != target {
		return fmt.Errorf("TypeAlias target %v, want exact source static term %v", target, term)
	}
	switch family {
	case FamilyPrimitive:
		return verifyPrimitive(p, term, declaration.Type)
	case FamilyLiteral:
		return verifyLiteral(p, term, declaration.Type)
	case FamilyOptional:
		return verifyOptional(p, term, declaration.Type)
	case FamilyUnion:
		return verifyTerms(p, term, declaration.Type, true)
	case FamilyIntersection:
		return verifyTerms(p, term, declaration.Type, false)
	case FamilyTypeRef:
		return verifyTypeRef(p, term, declaration.Type)
	case FamilyGeneric:
		return verifyGeneric(p, term, declaration.Type)
	case FamilyArray:
		return verifyArray(p, term, declaration.Type)
	case FamilyMap:
		return verifyMap(p, term, declaration.Type)
	case FamilyRecord:
		return verifyRecord(p, term, declaration.Type)
	case FamilySignature:
		return verifySignature(p, term, declaration.Type)
	case FamilyAssertion:
		return verifyAssertion(p, term, declaration.Type)
	case FamilyTypeOf:
		return verifyTypeOf(p, term, declaration.Type)
	case FamilyKeyOf:
		return verifyKeyOf(p, term, declaration.Type)
	case FamilyIndexAccess:
		return verifyIndexAccess(p, term, declaration.Type)
	case FamilyConditional:
		return verifyConditional(p, term, declaration.Type)
	default:
		return fmt.Errorf("invalid static family %d", family)
	}
}

func anchoredTypeAlias(p *program.Program, declaration *ast.TypeDefStmt) (keyspace.Term, error) {
	aliases := p.Static().Declarations().Aliases()
	return anchoredTerm(p, declaration, aliases.Count, aliases.At)
}

func anchoredStaticTerm(p *program.Program, expr ast.TypeExpr) (keyspace.Term, error) {
	types := p.Static().StaticTypes()
	term, err := anchoredStaticTermAt(p, expr, types)
	if err != nil {
		return 0, err
	}
	if _, ok := types.Ref(term); !ok {
		return 0, fmt.Errorf("source term %v is not public static authority", term)
	}
	return term, nil
}

func anchoredStaticTermAt(p *program.Program, expr ast.TypeExpr, types interface {
	Count() int
	At(int) (staticquery.StaticTypeRef, bool)
}) (keyspace.Term, error) {
	if expr == nil {
		return 0, fmt.Errorf("nil static source anchor")
	}
	want := sourceowner.Span{File: staticLawFile, StartLine: uint32(expr.Line()), StartCol: uint32(expr.Column()), EndLine: uint32(expr.LastLine()), EndCol: uint32(expr.LastColumn())}
	var found keyspace.Term
	for index := 0; index < types.Count(); index++ {
		ref, ok := types.At(index)
		if !ok {
			return 0, fmt.Errorf("public static enumeration missing term %d", index)
		}
		term := ref.Term()
		span, ok := p.Source().Identity().Span(term)
		if !ok || span != want {
			continue
		}
		if found != 0 {
			return 0, fmt.Errorf("source span %d:%d-%d:%d maps to multiple typed static terms", want.StartLine, want.StartCol, want.EndLine, want.EndCol)
		}
		found = term
	}
	if found == 0 {
		return 0, fmt.Errorf("no typed static term at source span %d:%d-%d:%d", want.StartLine, want.StartCol, want.EndLine, want.EndCol)
	}
	return found, nil
}

func anchoredTerm(p *program.Program, node ast.PositionHolder, count func() int, at func(int) (keyspace.Term, bool)) (keyspace.Term, error) {
	if node == nil {
		return 0, fmt.Errorf("nil static source anchor")
	}
	want := sourceowner.Span{File: staticLawFile, StartLine: uint32(node.Line()), StartCol: uint32(node.Column()), EndLine: uint32(node.LastLine()), EndCol: uint32(node.LastColumn())}
	var found keyspace.Term
	for index := 0; index < count(); index++ {
		term, ok := at(index)
		if !ok {
			return 0, fmt.Errorf("public static enumeration missing term %d", index)
		}
		span, ok := p.Source().Identity().Span(term)
		if !ok || span != want {
			continue
		}
		if found != 0 {
			return 0, fmt.Errorf("source span %d:%d-%d:%d maps to multiple typed static terms", want.StartLine, want.StartCol, want.EndLine, want.EndCol)
		}
		found = term
	}
	if found == 0 {
		return 0, fmt.Errorf("no typed static term at source span %d:%d-%d:%d", want.StartLine, want.StartCol, want.EndLine, want.EndCol)
	}
	return found, nil
}

func verifyPrimitive(p *program.Program, term keyspace.Term, source ast.TypeExpr) error {
	node, ok := source.(*ast.PrimitiveTypeExpr)
	if !ok || node.Name != "number" {
		return fmt.Errorf("primitive source = %T/%q", source, node.Name)
	}
	kind, ok := p.Static().Types().Primitives().Get(term)
	if !ok || kind != statictypes.PrimitiveNumber {
		return fmt.Errorf("Primitive = %d/%v", kind, ok)
	}
	return provenance.Exact(p.Source().Identity(), term, node, staticLawFile)
}

func verifyLiteral(p *program.Program, term keyspace.Term, source ast.TypeExpr) error {
	node, ok := source.(*ast.LiteralTypeExpr)
	if !ok || node.Value != "value" {
		return fmt.Errorf("literal source = %T/%#v", source, node.Value)
	}
	literalKind, key, _, ok := p.Static().Types().Literals().Get(term)
	value, valid := p.Source().Keys().Exact(key)
	if !ok || !valid || literalKind != keyspace.LiteralString || value.String != "value" {
		return fmt.Errorf("Literal = %#v/%v", value, ok && valid)
	}
	return provenance.Exact(p.Source().Identity(), term, node, staticLawFile)
}

func verifyOptional(p *program.Program, term keyspace.Term, source ast.TypeExpr) error {
	node, ok := source.(*ast.OptionalTypeExpr)
	if !ok {
		return fmt.Errorf("optional source = %T", source)
	}
	inner, ok := p.Static().Types().Optionals().Get(term)
	if !ok || inner == 0 {
		return fmt.Errorf("Optional = %v/%v", inner, ok)
	}
	if err := exactStaticChild(p, inner, node.Inner); err != nil {
		return fmt.Errorf("optional inner: %w", err)
	}
	return provenance.Exact(p.Source().Identity(), term, node, staticLawFile)
}

func verifyTerms(p *program.Program, term keyspace.Term, source ast.TypeExpr, union bool) error {
	var children []ast.TypeExpr
	if union {
		node, ok := source.(*ast.UnionTypeExpr)
		if !ok {
			return fmt.Errorf("union source = %T", source)
		}
		children = node.Types
	} else {
		node, ok := source.(*ast.IntersectionTypeExpr)
		if !ok {
			return fmt.Errorf("intersection source = %T", source)
		}
		children = node.Types
	}
	var count int
	var ok bool
	if union {
		count, ok = p.Static().Types().Unions().MemberCount(term)
	} else {
		count, ok = p.Static().Types().Intersections().MemberCount(term)
	}
	if !ok || count != len(children) {
		return fmt.Errorf("static member count = %d/%v, want %d", count, ok, len(children))
	}
	for index, child := range children {
		var actual keyspace.Term
		if union {
			actual, ok = p.Static().Types().Unions().MemberAt(term, index)
		} else {
			actual, ok = p.Static().Types().Intersections().MemberAt(term, index)
		}
		if !ok {
			return fmt.Errorf("static member %d missing", index)
		}
		if err := exactStaticChild(p, actual, child); err != nil {
			return fmt.Errorf("static member %d: %w", index, err)
		}
	}
	return provenance.Exact(p.Source().Identity(), term, source, staticLawFile)
}

func verifyTypeRef(p *program.Program, term keyspace.Term, source ast.TypeExpr) error {
	node, ok := source.(*ast.TypeRefExpr)
	if !ok || len(node.Path) != 2 {
		return fmt.Errorf("type-ref source = %T", source)
	}
	resolution, target, root, ok := p.Static().References().Get(term)
	if !ok || resolution != staticrefs.Unresolved || target != 0 || root == 0 {
		return fmt.Errorf("TypeRef = resolution %d target %v root %v ok %v", resolution, target, root, ok)
	}
	if count, ok := p.Static().References().SourceCount(term); !ok || count != len(node.Path) {
		return fmt.Errorf("TypeRef source length = %d/%v", count, ok)
	}
	for index, want := range node.Path {
		key, ok := p.Static().References().SourceAt(term, index)
		value, valid := p.Source().Keys().Exact(key)
		if !ok || !valid || value.Kind != keyspace.LiteralString || value.String != want {
			return fmt.Errorf("TypeRef source[%d] = %#v/%v", index, value, ok && valid)
		}
	}
	span, ok := p.Source().Identity().Span(term)
	if !ok || span.StartLine != uint32(node.RootPosition.Line) || span.StartCol != uint32(node.RootPosition.Column) {
		return fmt.Errorf("TypeRef root span = %#v/%v, want start %d:%d", span, ok, node.RootPosition.Line, node.RootPosition.Column)
	}
	return provenance.Exact(p.Source().Identity(), term, node, staticLawFile)
}

func verifyGeneric(p *program.Program, term keyspace.Term, source ast.TypeExpr) error {
	node, ok := source.(*ast.GenericTypeExpr)
	if !ok || node.Base == nil {
		return fmt.Errorf("generic source = %T", source)
	}
	base, count, ok := p.Static().Types().Generics().Get(term)
	if !ok || base == 0 {
		return fmt.Errorf("Generic base = %v/%v", base, ok)
	}
	if err := exactStaticChild(p, base, node.Base); err != nil {
		return fmt.Errorf("generic base: %w", err)
	}
	if count != len(node.Args) {
		return fmt.Errorf("Generic arg count = %d/%v", count, ok)
	}
	for index, child := range node.Args {
		actual, ok := p.Static().Types().Generics().ArgAt(term, index)
		if !ok {
			return fmt.Errorf("Generic arg %d missing", index)
		}
		if err := exactStaticChild(p, actual, child); err != nil {
			return fmt.Errorf("Generic arg %d: %w", index, err)
		}
	}
	return provenance.Exact(p.Source().Identity(), term, node, staticLawFile)
}

func verifyArray(p *program.Program, term keyspace.Term, source ast.TypeExpr) error {
	node, ok := source.(*ast.ArrayTypeExpr)
	if !ok {
		return fmt.Errorf("array source = %T", source)
	}
	element, readonly, ok := p.Static().Types().Arrays().Get(term)
	if !ok || !readonly || element == 0 {
		return fmt.Errorf("Array = element %v readonly %v ok %v", element, readonly, ok)
	}
	if err := exactStaticChild(p, element, node.Element); err != nil {
		return fmt.Errorf("array element: %w", err)
	}
	return provenance.Exact(p.Source().Identity(), term, node, staticLawFile)
}

func verifyMap(p *program.Program, term keyspace.Term, source ast.TypeExpr) error {
	node, ok := source.(*ast.MapTypeExpr)
	if !ok {
		return fmt.Errorf("map source = %T", source)
	}
	key, value, readonly, ok := p.Static().Types().Maps().Get(term)
	if !ok || !readonly || key == 0 || value == 0 {
		return fmt.Errorf("Map = key %v value %v readonly %v ok %v", key, value, readonly, ok)
	}
	if err := exactStaticChild(p, key, node.Key); err != nil {
		return fmt.Errorf("map key: %w", err)
	}
	if err := exactStaticChild(p, value, node.Value); err != nil {
		return fmt.Errorf("map value: %w", err)
	}
	return provenance.Exact(p.Source().Identity(), term, node, staticLawFile)
}

func verifyRecord(p *program.Program, term keyspace.Term, source ast.TypeExpr) error {
	node, ok := source.(*ast.RecordTypeExpr)
	if !ok || len(node.Fields) != 1 {
		return fmt.Errorf("record source = %T", source)
	}
	readonly, count, ok := p.Static().Types().Records().Get(term)
	if !ok || !readonly || count != len(node.Fields) {
		return fmt.Errorf("Record = readonly %v fields %d ok %v", readonly, count, ok)
	}
	field, ok := p.Static().Types().Records().FieldAt(term, 0)
	if !ok {
		return fmt.Errorf("record field missing")
	}
	owner := term
	key, typ, optional, ok := p.Static().Types().Fields().Get(field)
	if !ok || owner != term || key == 0 || !optional || typ == 0 {
		return fmt.Errorf("TypeField = owner %v key %v type %v optional %v ok %v", owner, key, typ, optional, ok)
	}
	if err := exactStaticChild(p, typ, node.Fields[0].Type); err != nil {
		return fmt.Errorf("record field type: %w", err)
	}
	if err := exactTokenSpan(p, field, node.Fields[0].NamePosition); err != nil {
		return fmt.Errorf("record field name: %w", err)
	}
	return provenance.Exact(p.Source().Identity(), term, node, staticLawFile)
}

func verifySignature(p *program.Program, term keyspace.Term, source ast.TypeExpr) error {
	node, ok := source.(*ast.FunctionTypeExpr)
	if !ok {
		return fmt.Errorf("signature source = %T", source)
	}
	_, variadic, _, known, ok := p.Static().Signatures().TypeFunctions().Get(term)
	if !ok || !known || variadic == 0 {
		return fmt.Errorf("Signature = variadic %v known %v ok %v", variadic, known, ok)
	}
	if err := exactStaticChild(p, variadic, node.Variadic); err != nil {
		return fmt.Errorf("signature variadic: %w", err)
	}
	if count, ok := p.Static().Signatures().TypeFunctions().TypeParamCount(term); !ok || count != len(node.TypeParams) {
		return fmt.Errorf("signature type parameter count = %d/%v, want %d", count, ok, len(node.TypeParams))
	}
	if count, ok := p.Static().Signatures().TypeFunctions().ParameterCount(term); !ok || count != len(node.Params) {
		return fmt.Errorf("signature parameter count = %d/%v", count, ok)
	}
	for index, parameter := range node.Params {
		actual, ok := p.Static().Signatures().TypeFunctions().ParameterAt(term, index)
		if !ok || actual.Type == 0 {
			return fmt.Errorf("signature parameter %d missing", index)
		}
		if err := exactStaticChild(p, actual.Type, parameter.Type); err != nil {
			return fmt.Errorf("signature parameter %d: %w", index, err)
		}
		if parameter.Name == "" {
			if actual.Name != 0 || actual.NameCoordinate != (sourceowner.Coordinate{}) {
				return fmt.Errorf("anonymous signature parameter %d retained name %#v", index, actual)
			}
		} else if actual.Name == 0 {
			return fmt.Errorf("named signature parameter %d has no name", index)
		} else if got, rendered := p.Source().Identity().Render(actual.NameCoordinate); !rendered || got != tokenSpan(parameter.NamePosition) {
			return fmt.Errorf("named signature parameter %d span = %#v/%v, want %#v", index, got, rendered, tokenSpan(parameter.NamePosition))
		}
	}
	if count, ok := p.Static().Signatures().TypeFunctions().ReturnCount(term); !ok || count != len(node.Returns) {
		return fmt.Errorf("signature return count = %d/%v", count, ok)
	}
	for index, child := range node.Returns {
		actual, ok := p.Static().Signatures().TypeFunctions().ReturnAt(term, index)
		if !ok {
			return fmt.Errorf("signature return %d missing", index)
		}
		if err := exactStaticChild(p, actual, child); err != nil {
			return fmt.Errorf("signature return %d: %w", index, err)
		}
	}
	return provenance.Exact(p.Source().Identity(), term, node, staticLawFile)
}

func verifyAssertion(p *program.Program, term keyspace.Term, source ast.TypeExpr) error {
	node, ok := source.(*ast.AssertsTypeExpr)
	if !ok || node.NarrowTo == nil {
		return fmt.Errorf("assertion source = %T", source)
	}
	name, coordinate, bound, param, narrow, ok := p.Static().Signatures().Assertions().Get(term)
	if !ok || name == 0 || bound || param != 0 || narrow == 0 {
		return fmt.Errorf("Assertion = name %v coordinate %#v bound %v param %d narrow %v ok %v", name, coordinate, bound, param, narrow, ok)
	}
	if got, rendered := p.Source().Identity().Render(coordinate); !rendered || got != tokenSpan(node.ParamPosition) {
		return fmt.Errorf("Assertion parameter span = %#v/%v, want %#v", got, rendered, tokenSpan(node.ParamPosition))
	}
	if err := exactStaticChild(p, narrow, node.NarrowTo); err != nil {
		return fmt.Errorf("assertion narrow: %w", err)
	}
	return provenance.Exact(p.Source().Identity(), term, node, staticLawFile)
}

func verifyTypeOf(p *program.Program, term keyspace.Term, source ast.TypeExpr) error {
	node, ok := source.(*ast.TypeOfExpr)
	if !ok {
		return fmt.Errorf("typeof source = %T", source)
	}
	_, operand, ok := p.Static().Operators().TypeOfs().Get(term)
	if !ok || operand == 0 {
		return fmt.Errorf("TypeOf operand = %v/%v", operand, ok)
	}
	if err := provenance.Exact(p.Source().Identity(), operand, node.Expr, staticLawFile); err != nil {
		return fmt.Errorf("typeof operand: %w", err)
	}
	return provenance.Exact(p.Source().Identity(), term, node, staticLawFile)
}

func verifyKeyOf(p *program.Program, term keyspace.Term, source ast.TypeExpr) error {
	node, ok := source.(*ast.KeyOfExpr)
	if !ok {
		return fmt.Errorf("keyof source = %T", source)
	}
	inner, ok := p.Static().Operators().KeyOfs().Get(term)
	if !ok || inner == 0 {
		return fmt.Errorf("KeyOf inner = %v/%v", inner, ok)
	}
	if err := exactStaticChild(p, inner, node.Inner); err != nil {
		return fmt.Errorf("keyof inner: %w", err)
	}
	return provenance.Exact(p.Source().Identity(), term, node, staticLawFile)
}

func verifyIndexAccess(p *program.Program, term keyspace.Term, source ast.TypeExpr) error {
	node, ok := source.(*ast.IndexAccessExpr)
	if !ok {
		return fmt.Errorf("index source = %T", source)
	}
	object, index, ok := p.Static().Operators().IndexAccesses().Get(term)
	if !ok || object == 0 || index == 0 {
		return fmt.Errorf("IndexAccess = object %v index %v ok %v", object, index, ok)
	}
	if err := exactStaticChild(p, object, node.Object); err != nil {
		return fmt.Errorf("index object: %w", err)
	}
	if err := exactStaticChild(p, index, node.Index); err != nil {
		return fmt.Errorf("index key: %w", err)
	}
	return provenance.Exact(p.Source().Identity(), term, node, staticLawFile)
}

func verifyConditional(p *program.Program, term keyspace.Term, source ast.TypeExpr) error {
	node, ok := source.(*ast.ConditionalTypeExpr)
	if !ok {
		return fmt.Errorf("conditional source = %T", source)
	}
	check, extends, then, otherwise, ok := p.Static().Operators().Conditionals().Get(term)
	if !ok || check == 0 || extends == 0 || then == 0 || otherwise == 0 {
		return fmt.Errorf("Conditional has incomplete operands")
	}
	for _, pair := range []struct {
		term   keyspace.Term
		source ast.TypeExpr
	}{{check, node.Check}, {extends, node.Extends}, {then, node.Then}, {otherwise, node.Else}} {
		if err := exactStaticChild(p, pair.term, pair.source); err != nil {
			return fmt.Errorf("conditional child: %w", err)
		}
	}
	return provenance.Exact(p.Source().Identity(), term, node, staticLawFile)
}

func verifyAnnotated(p *program.Program, source ast.TypeExpr, target keyspace.Term) error {
	node, ok := source.(*ast.AnnotatedTypeExpr)
	if !ok || node.Inner == nil || len(node.Annotations) != 1 {
		return fmt.Errorf("annotated source = %T", source)
	}
	if err := exactStaticChild(p, target, node.Inner); err != nil {
		return fmt.Errorf("annotated inner: %w", err)
	}
	annotations := p.Static().Operands().Annotations()
	annotation, err := anchoredTerm(p, &node.Annotations[0], annotations.Count, annotations.At)
	if err != nil {
		return err
	}
	annotationRow, ok := annotations.Get(annotation)
	gotTarget, name, values := annotationRow.Target, annotationRow.Name, annotationRow.Values
	if !ok || gotTarget != target || name == 0 || values == 0 {
		return fmt.Errorf("Annotation = target %v name %v values %v ok %v", gotTarget, name, values, ok)
	}
	if count, ok := p.Flow().Authored().Values().Len(values); !ok || count != len(node.Annotations[0].Args) {
		return fmt.Errorf("annotation Values count = %d/%v", count, ok)
	}
	return provenance.Exact(p.Source().Identity(), annotation, &node.Annotations[0], staticLawFile)
}

func exactStaticChild(p *program.Program, term keyspace.Term, source ast.TypeExpr) error {
	if _, ok := p.Static().StaticTypes().Ref(term); !ok {
		return fmt.Errorf("child %v is not a static term", term)
	}
	return provenance.Exact(p.Source().Identity(), term, source, staticLawFile)
}

func exactTokenSpan(p *program.Program, term keyspace.Term, position ast.Position) error {
	span, ok := p.Source().Identity().Span(term)
	if !ok || span != tokenSpan(position) {
		return fmt.Errorf("Program span = %#v/%v, want token %#v", span, ok, tokenSpan(position))
	}
	return nil
}

func tokenSpan(position ast.Position) sourceowner.Span {
	return sourceowner.Span{
		File:      staticLawFile,
		StartLine: uint32(position.Line),
		StartCol:  uint32(position.Column),
		EndLine:   uint32(position.EndLine),
		EndCol:    uint32(position.EndColumn),
	}
}
