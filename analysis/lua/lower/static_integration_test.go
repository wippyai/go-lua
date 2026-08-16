package lower_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

func TestLowerParsedInterfacePreservesOneAuthoredMemberSequence(t *testing.T) {
	p := parseBindLower(t, `
interface Base end
interface Shape: Base
    id: number @min(1)
    function map<T: string>(value: T): T
    enabled?: boolean
    function empty(): ()
end
return Shape()
`)
	staticView := p.Static()
	interfaces := staticView.Declarations().Interfaces()
	if interfaces.Count() != 2 {
		t.Fatalf("InterfaceCount = %d, want 2", interfaces.Count())
	}
	base, _ := interfaces.At(0)
	shape, ok := interfaces.At(1)
	if !ok {
		t.Fatal("missing Shape interface")
	}
	if count, ok := interfaces.ExtendCount(shape); !ok || count != 1 {
		t.Fatalf("extends = %d/%v, want one", count, ok)
	}
	extends, _ := interfaces.ExtendAt(shape, 0)
	if state, target, _, ok := staticView.References().Get(extends); !ok || state != static.TypeRefDeclaration || target != base {
		t.Fatalf("extends = %v/%v/%v", state, target, ok)
	}
	if count, ok := interfaces.MemberCount(shape); !ok || count != 4 {
		t.Fatalf("members = %d/%v, want 4", count, ok)
	}
	first, _ := interfaces.MemberAt(shape, 0)
	second, _ := interfaces.MemberAt(shape, 1)
	third, _ := interfaces.MemberAt(shape, 2)
	fourth, _ := interfaces.MemberAt(shape, 3)
	if first.Kind != static.InterfaceField || second.Kind != static.InterfaceMethod ||
		third.Kind != static.InterfaceField || fourth.Kind != static.InterfaceMethod {
		t.Fatalf("member kinds = %#v %#v %#v %#v, want field/method/field/method", first, second, third, fourth)
	}
	if _, _, optional, ok := staticView.Types().Fields().Get(first.Field); !ok || optional {
		t.Fatalf("first field = optional %v ok %v", optional, ok)
	}
	if _, _, optional, ok := staticView.Types().Fields().Get(third.Field); !ok || !optional {
		t.Fatalf("third field = optional %v ok %v", optional, ok)
	}
	secondName, secondNameOK := p.Source().Identity().Render(second.NameCoordinate)
	if second.Name == 0 || !secondNameOK || secondName.StartLine == 0 || second.Signature == 0 {
		t.Fatalf("second method identity = %#v", second)
	}
	if scope, _, _, returnsKnown, ok := staticView.Signatures().TypeFunctions().Get(second.Signature); !ok || scope != shape || !returnsKnown {
		t.Fatalf("map signature = scope %v known %v ok %v", scope, returnsKnown, ok)
	}
	if generics, ok := staticView.Signatures().TypeFunctions().TypeParamCount(second.Signature); !ok || generics != 1 {
		t.Fatalf("map generics = %d/%v", generics, ok)
	}
	if _, _, _, returnsKnown, ok := staticView.Signatures().TypeFunctions().Get(fourth.Signature); !ok || !returnsKnown {
		t.Fatalf("empty signature = known %v ok %v", returnsKnown, ok)
	}
	if returns, ok := staticView.Signatures().TypeFunctions().ReturnCount(fourth.Signature); !ok || returns != 0 {
		t.Fatalf("empty returns = %d/%v", returns, ok)
	}
}

func TestLowerParsedInterfaceDuplicateMembersStayDistinctAndDeterministic(t *testing.T) {
	const source = `
interface Repeated
    id: number
    id: number
    function map(): string
    function map(): string
end
`
	left := parseBindLower(t, source)
	right := parseBindLower(t, source)
	for _, p := range []*program.Program{left, right} {
		interfaces := p.Static().Declarations().Interfaces()
		iface, ok := interfaces.At(0)
		if !ok {
			t.Fatal("missing Repeated interface")
		}
		if count, ok := interfaces.MemberCount(iface); !ok || count != 4 {
			t.Fatalf("members = %d/%v, want 4", count, ok)
		}
		first, _ := interfaces.MemberAt(iface, 0)
		second, _ := interfaces.MemberAt(iface, 1)
		third, _ := interfaces.MemberAt(iface, 2)
		fourth, _ := interfaces.MemberAt(iface, 3)
		if first.Kind != static.InterfaceField || second.Kind != static.InterfaceField ||
			third.Kind != static.InterfaceMethod || fourth.Kind != static.InterfaceMethod ||
			first.Field == second.Field || third.Name != fourth.Name || third.Signature == fourth.Signature {
			t.Fatalf("duplicate member identities = %#v %#v %#v %#v", first, second, third, fourth)
		}
	}
	leftInterfaces := left.Static().Declarations().Interfaces()
	rightInterfaces := right.Static().Declarations().Interfaces()
	leftIface, _ := leftInterfaces.At(0)
	rightIface, _ := rightInterfaces.At(0)
	for index := 0; index < 4; index++ {
		leftMember, _ := leftInterfaces.MemberAt(leftIface, index)
		rightMember, _ := rightInterfaces.MemberAt(rightIface, index)
		if leftMember != rightMember {
			t.Fatalf("non-deterministic member[%d] = %#v vs %#v", index, leftMember, rightMember)
		}
	}
}

func TestSourceStaticOperatorsKeepExactStaticRows(t *testing.T) {
	p := parseBindLower(t, "type Check = number\ntype Constraint = string\ntype Then = boolean\ntype Otherwise = never\ntype Record = {}\ntype Key = \"field\"\ntype Keys = keyof(Record)\ntype Field = Record[Key]\ntype Choice = Check extends Constraint ? Then : Otherwise")
	aliases := p.Static().Declarations().Aliases()
	if aliases.Count() != 9 {
		t.Fatalf("Static Alias count = %d, want 9", aliases.Count())
	}
	keyAlias, _ := aliases.At(6)
	indexAlias, _ := aliases.At(7)
	conditionalAlias, _ := aliases.At(8)
	_, keyOf, _, _, keyAliasOK := aliases.Get(keyAlias)
	_, indexAccess, _, _, indexAliasOK := aliases.Get(indexAlias)
	_, conditional, _, _, conditionalAliasOK := aliases.Get(conditionalAlias)
	if !keyAliasOK || !indexAliasOK || !conditionalAliasOK {
		t.Fatal("missing Static Alias targets")
	}
	operators := p.Static().Operators()
	if operators.KeyOfs().Count() != 1 || operators.IndexAccesses().Count() != 1 || operators.Conditionals().Count() != 1 {
		t.Fatalf("operator counts = keyof %d indexed %d conditional %d", operators.KeyOfs().Count(), operators.IndexAccesses().Count(), operators.Conditionals().Count())
	}
	inner, keyOK := operators.KeyOfs().Get(keyOf)
	object, index, indexedOK := operators.IndexAccesses().Get(indexAccess)
	check, extends, then, otherwise, conditionalOK := operators.Conditionals().Get(conditional)
	if !keyOK || !indexedOK || !conditionalOK || inner == 0 || object == 0 || index == 0 || check == 0 || extends == 0 || then == 0 || otherwise == 0 {
		t.Fatalf("static operator rows key=%v indexed=%v conditional=%v", keyOK, indexedOK, conditionalOK)
	}
	for _, term := range []keyspace.Term{keyOf, inner, indexAccess, object, index, conditional, check, extends, then, otherwise} {
		if span, ok := p.Source().Identity().Span(term); !ok || span.StartLine == 0 {
			t.Fatalf("static term %v has no Source span", term)
		}
	}
}

func TestSourceStaticOperatorsReachDeclarationParameterAndCallHosts(t *testing.T) {
	for _, sample := range []struct {
		name  string
		input string
		want  func() int
	}{
		{"keyof", "type Subject = keyof(User)\nlocal function f<T: keyof(User)>(value: keyof(User)): keyof(User) return value end\nreturn f::<keyof(User)>(nil)", func() int { return 5 }},
		{"indexed", "type Subject = User[\"field\"]\nlocal function f<T: User[\"field\"]>(value: User[\"field\"]): User[\"field\"] return value end\nreturn f::<User[\"field\"]>(nil)", func() int { return 5 }},
		{"conditional", "type Subject = T extends U ? Then : Else\nlocal function f<T: T extends U ? Then : Else>(value: T extends U ? Then : Else): T extends U ? Then : Else return value end\nreturn f::<T extends U ? Then : Else>(nil)", func() int { return 5 }},
	} {
		t.Run(sample.name, func(t *testing.T) {
			p := parseBindLower(t, sample.input)
			operators := p.Static().Operators()
			var count int
			switch sample.name {
			case "keyof":
				count = operators.KeyOfs().Count()
			case "indexed":
				count = operators.IndexAccesses().Count()
			case "conditional":
				count = operators.Conditionals().Count()
			}
			if count != sample.want() {
				t.Fatalf("%s static operator count = %d, want %d", sample.name, count, sample.want())
			}
			call, ok := p.Flow().Authored().Calls().At(0)
			if !ok {
				t.Fatal("missing authored generic Call")
			}
			if count, ok := p.Static().Contracts().Calls().TypeArgumentCount(call); !ok || count != 1 {
				t.Fatalf("Call type arguments = %d/%v, want one", count, ok)
			}
		})
	}
}

func TestSourceStaticAnnotationRowsAreOwnedByStaticOperands(t *testing.T) {
	p := parseBindLower(t, "type Item = number\nlocal value: Item @note(1) = 0")
	annotations := p.Static().Operands().Annotations()
	annotation, ok := annotations.At(0)
	if !ok || annotations.Count() != 1 {
		t.Fatalf("Static Annotation = %v/%v count=%d", annotation, ok, annotations.Count())
	}
	row, rowOK := annotations.Get(annotation)
	if !rowOK || row.Target == 0 || row.Values == 0 {
		t.Fatalf("Static Annotation row = %#v/%v", row, rowOK)
	}
	if count, ok := annotations.ForCount(row.Target); !ok || count != 1 {
		t.Fatalf("annotation target count = %d/%v, want one", count, ok)
	}
	if indexed, ok := annotations.ForAt(row.Target, 0); !ok || indexed != annotation {
		t.Fatalf("annotation target row = %v/%v, want %v", indexed, ok, annotation)
	}
}

func TestSourceStaticOperatorContentIDIsDeterministic(t *testing.T) {
	input := "type Result = keyof(User) | User[\"field\"] | (T extends U ? Then : Else)"
	first := parseBindLower(t, input)
	second := parseBindLower(t, input)
	if first.ContentID() != second.ContentID() {
		t.Fatal("same static operators produced different Program ContentID")
	}
}

// These laws enter through authored source, then read token provenance from
// Source and authored static structure from Static.  They intentionally do
// not recreate the retired root forwarding surface.
func TestSourceStaticLawTokenProvenanceAndSignatureShape(t *testing.T) {
	p := parseBindLower(t, `type Alias<T: number> = fun(named: T, string, ... boolean): (asserts named is string)
interface Service
  function apply<U>(input: U): U
end`)
	identity := p.Source().Identity()
	aliases := p.Static().Declarations().Aliases()
	params := p.Static().Declarations().TypeParams()
	signatures := p.Static().Signatures().TypeFunctions()
	assertions := p.Static().Signatures().Assertions()
	primitives := p.Static().Types().Primitives()
	interfaces := p.Static().Declarations().Interfaces()

	alias, ok := aliases.At(0)
	if !ok {
		t.Fatal("missing Static Alias")
	}
	_, _, _, aliasCoordinate, aliasOK := aliases.Get(alias)
	span, spanOK := identity.Render(aliasCoordinate)
	if !aliasOK || !spanOK || span != (source.Span{
		File: "fixture.lua", StartLine: 1, StartCol: 6, EndLine: 1, EndCol: 10,
	}) {
		t.Fatalf("Alias name span = %#v/%v", span, spanOK)
	}
	if count, ok := aliases.ParamCount(alias); !ok || count != 1 {
		t.Fatalf("Alias generic count = %d/%v", count, ok)
	}
	aliasParam, ok := aliases.ParamAt(alias, 0)
	if !ok {
		t.Fatal("missing Alias generic")
	}
	if _, _, _, ok := params.Get(aliasParam); !ok {
		t.Fatal("missing Static TypeParam row")
	}
	if span, ok := identity.Span(aliasParam); !ok || span != (source.Span{
		File: "fixture.lua", StartLine: 1, StartCol: 12, EndLine: 1, EndCol: 12,
	}) {
		t.Fatalf("Alias generic name span = %#v/%v", span, ok)
	}

	_, signature, _, _, ok := aliases.Get(alias)
	if !ok {
		t.Fatal("missing Alias signature")
	}
	if count, ok := signatures.ParameterCount(signature); !ok || count != 2 {
		t.Fatalf("fixed parameter count = %d/%v", count, ok)
	}
	named, ok := signatures.ParameterAt(signature, 0)
	namedSpan, namedSpanOK := identity.Render(named.NameCoordinate)
	if !ok || named.Name == 0 || !namedSpanOK || namedSpan != (source.Span{
		File: "fixture.lua", StartLine: 1, StartCol: 29, EndLine: 1, EndCol: 33,
	}) {
		t.Fatalf("named parameter = %#v/%v", named, ok)
	}
	anonymous, ok := signatures.ParameterAt(signature, 1)
	if !ok || anonymous.Name != 0 {
		t.Fatalf("anonymous parameter = %#v/%v", anonymous, ok)
	}
	if _, present := identity.Render(anonymous.NameCoordinate); present {
		t.Fatalf("anonymous parameter retained a name coordinate: %#v", anonymous)
	}
	_, variadic, variadicCoordinate, returnsKnown, ok := signatures.Get(signature)
	variadicSpan, variadicSpanOK := identity.Render(variadicCoordinate)
	if !ok || !returnsKnown || variadic == 0 || !variadicSpanOK || variadicSpan != (source.Span{
		File: "fixture.lua", StartLine: 1, StartCol: 47, EndLine: 1, EndCol: 49,
	}) {
		t.Fatalf("signature variadic/returns = %#v/%#v/%v/%v", variadic, variadicSpan, returnsKnown, ok)
	}
	if kind, ok := primitives.Get(variadic); !ok || kind != static.PrimitiveBoolean {
		t.Fatalf("signature variadic type = %v/%v", kind, ok)
	}
	if count, ok := signatures.ReturnCount(signature); !ok || count != 1 {
		t.Fatalf("signature returns = %d/%v", count, ok)
	}
	assertion, ok := signatures.ReturnAt(signature, 0)
	if !ok {
		t.Fatal("missing assertion return")
	}
	_, assertionCoordinate, bound, parameter, _, ok := assertions.Get(assertion)
	assertionSpan, assertionSpanOK := identity.Render(assertionCoordinate)
	if !ok || !bound || parameter != 0 || !assertionSpanOK || assertionSpan != (source.Span{
		File: "fixture.lua", StartLine: 1, StartCol: 70, EndLine: 1, EndCol: 74,
	}) {
		t.Fatalf("assertion subject = bound %v param %d span %#v ok %v", bound, parameter, assertionSpan, ok)
	}

	service, ok := interfaces.At(0)
	if !ok {
		t.Fatal("missing Service")
	}
	_, _, serviceCoordinate, serviceOK := interfaces.Get(service)
	serviceSpan, serviceSpanOK := identity.Render(serviceCoordinate)
	if !serviceOK || !serviceSpanOK || serviceSpan != (source.Span{
		File: "fixture.lua", StartLine: 2, StartCol: 11, EndLine: 2, EndCol: 17,
	}) {
		t.Fatalf("Service name span = %#v/%v", serviceSpan, serviceSpanOK)
	}
}

func TestSourceStaticLawQualifiedTypeRefUsesLexicalRoot(t *testing.T) {
	p := parseBindLower(t, `local module = {}
type Outer = module.User
do
  local module = {}
  type Inner = module.User
end
type Bare = Outer`)
	binds := p.Flow().Authored().Storage().Binds()
	outerBind, ok := binds.At(0)
	if !ok {
		t.Fatal("missing outer module binding")
	}
	outerCell := boundCell(t, p, outerBind, 0)
	innerBind, ok := binds.At(1)
	if !ok {
		t.Fatal("missing inner module binding")
	}
	innerCell := boundCell(t, p, innerBind, 0)

	outerAlias, outer := sourceAliasTargetAtLine(t, p, 2)
	_, inner := sourceAliasTargetAtLine(t, p, 5)
	_, bare := sourceAliasTargetAtLine(t, p, 7)
	outerSource := sourceTypeRefPath(t, p, outer, 2)
	innerSource := sourceTypeRefPath(t, p, inner, 2)
	if outerSource[0] == 0 || outerSource[1] == 0 ||
		outerSource[0] != innerSource[0] || outerSource[1] != innerSource[1] {
		t.Fatalf("qualified TypeRef source paths differ: outer=%v inner=%v", outerSource, innerSource)
	}

	references := p.Static().References()
	for _, want := range []struct {
		name string
		ref  keyspace.Term
		root keyspace.Term
	}{
		{name: "outer", ref: outer, root: outerCell},
		{name: "inner", ref: inner, root: innerCell},
	} {
		resolution, target, root, ok := references.Get(want.ref)
		if !ok || resolution != static.TypeRefUnresolved || target != 0 || root != want.root {
			t.Fatalf("%s TypeRef = resolution %v target %v root %v ok %v; want unresolved/0/%v", want.name, resolution, target, root, ok, want.root)
		}
	}

	if path := sourceTypeRefPath(t, p, bare, 1); path[0] == 0 {
		t.Fatal("bare TypeRef lost its authored source component")
	}
	resolution, target, root, ok := references.Get(bare)
	if !ok || resolution != static.TypeRefDeclaration || target != outerAlias || root != 0 {
		t.Fatalf("bare TypeRef = resolution %v target %v root %v ok %v", resolution, target, root, ok)
	}
}

func TestSourceStaticLawTurbofishArgumentsRemainStatic(t *testing.T) {
	p := parseBindLower(t, `local function identity<T>(value: T): T
  return value
end
local receiver = { identity = identity }
return identity::<string>(1), receiver:identity::<integer>(2)`)
	calls := p.Flow().Authored().Calls()
	values := p.Flow().Authored().Values()
	contracts := p.Static().Contracts().Calls()
	primitives := p.Static().Types().Primitives()
	if calls.Count() != 2 {
		t.Fatalf("Flow Call count = %d, want 2", calls.Count())
	}
	for index, want := range []struct {
		kind       static.PrimitiveKind
		methodCall bool
	}{
		{kind: static.PrimitiveString},
		{kind: static.PrimitiveInteger, methodCall: true},
	} {
		call, ok := calls.At(index)
		if !ok {
			t.Fatalf("missing Flow Call %d", index)
		}
		_, _, receiver, actuals, ok := calls.Get(call)
		if !ok || actuals == 0 || (receiver != 0) != want.methodCall {
			t.Fatalf("Call %d = receiver %v actuals %v ok %v", index, receiver, actuals, ok)
		}
		if fixed, ok := values.Len(actuals); !ok || fixed != 1 {
			t.Fatalf("Call %d runtime fixed actuals = %d/%v", index, fixed, ok)
		}
		if _, tail, ok := values.Get(actuals); !ok || tail != 0 {
			t.Fatalf("Call %d runtime actual tail = %v/%v", index, tail, ok)
		}
		if count, ok := contracts.TypeArgumentCount(call); !ok || count != 1 {
			t.Fatalf("Call %d Static TypeArgs = %d/%v", index, count, ok)
		}
		arg, ok := contracts.TypeArgumentAt(call, 0)
		if !ok || !p.Flow().Containment().Static(arg) {
			t.Fatalf("Call %d Static TypeArg = %v/%v static=%v", index, arg, ok, p.Flow().Containment().Static(arg))
		}
		if kind, ok := primitives.Get(arg); !ok || kind != want.kind {
			t.Fatalf("Call %d Static TypeArg primitive = %v/%v", index, kind, ok)
		}
	}
}

func TestSourceStaticLawOmittedAndAuthoredEmptyReturnsRemainDistinct(t *testing.T) {
	p := parseBindLower(t, "type Omitted = fun()\ntype Empty = fun(): ()")
	signatures := p.Static().Signatures().TypeFunctions()
	for index, wantKnown := range []bool{false, true} {
		signature := sourceAliasTarget(t, p, index)
		_, _, _, gotKnown, ok := signatures.Get(signature)
		if !ok || gotKnown != wantKnown {
			t.Fatalf("signature[%d] returns-known = %v/%v", index, gotKnown, ok)
		}
		if count, ok := signatures.ReturnCount(signature); !ok || count != 0 {
			t.Fatalf("signature[%d] returns = %d/%v", index, count, ok)
		}
	}
}

// Parser-admitted assertion types remain exact Static terms even where a
// later rule may reject their semantic placement.  Static preserves source;
// it does not turn a context diagnosis into a lowering failure.
func TestSourceStaticAssertionsRetainGeneralAndReturnContexts(t *testing.T) {
	p := parseBindLower(t, `type General = asserts candidate is string
type Callable = fun(candidate: any): asserts candidate is number`)
	identity := p.Source().Identity()
	assertions := p.Static().Signatures().Assertions()
	primitives := p.Static().Types().Primitives()

	generalAlias, general := sourceAliasTargetAtLine(t, p, 1)
	if generalAlias == 0 || general == 0 {
		t.Fatalf("general assertion alias/target = %v/%v", generalAlias, general)
	}
	name, coordinate, bound, parameter, narrow, ok := assertions.Get(general)
	span, spanOK := identity.Render(coordinate)
	if !ok || bound || parameter != 0 || !spanOK || span != (source.Span{
		File: "fixture.lua", StartLine: 1, StartCol: 24, EndLine: 1, EndCol: 32,
	}) {
		t.Fatalf("general Assertion = name %v bound %v param %d span %#v narrow %v ok %v", name, bound, parameter, span, narrow, ok)
	}
	value, keyOK := p.Source().Keys().Exact(name)
	if !keyOK || value.Kind != keyspace.LiteralString || value.String != "candidate" {
		t.Fatalf("general Assertion name = %#v/%v", value, keyOK)
	}
	if kind, ok := primitives.Get(narrow); !ok || kind != static.PrimitiveString {
		t.Fatalf("general Assertion narrow = %v/%v", kind, ok)
	}

	callable := sourceAliasTarget(t, p, 1)
	returned, ok := p.Static().Signatures().TypeFunctions().ReturnAt(callable, 0)
	if !ok {
		t.Fatalf("return assertion = %v/%v", returned, ok)
	}
	name, coordinate, bound, parameter, narrow, ok = assertions.Get(returned)
	span, spanOK = identity.Render(coordinate)
	if !ok || !bound || parameter != 0 || !spanOK || span != (source.Span{
		File: "fixture.lua", StartLine: 2, StartCol: 46, EndLine: 2, EndCol: 54,
	}) {
		t.Fatalf("return Assertion = name %v bound %v param %d span %#v narrow %v ok %v", name, bound, parameter, span, narrow, ok)
	}
	value, keyOK = p.Source().Keys().Exact(name)
	if !keyOK || value.Kind != keyspace.LiteralString || value.String != "candidate" {
		t.Fatalf("return Assertion name = %#v/%v", value, keyOK)
	}
	if kind, ok := primitives.Get(narrow); !ok || kind != static.PrimitiveNumber {
		t.Fatalf("return Assertion narrow = %v/%v", kind, ok)
	}
}

func TestSourceStaticLawInterleavedInterfaceMemberOrder(t *testing.T) {
	p := parseBindLower(t, `interface Ordered
  first: number
  function middle(): ()
  second?: string
  function finish(): number
end`)
	interfaces := p.Static().Declarations().Interfaces()
	if count := interfaces.Count(); count != 1 {
		t.Fatalf("Static Interface count = %d, want 1", count)
	}
	iface, _ := interfaces.At(0)
	if count, ok := interfaces.MemberCount(iface); !ok || count != 4 {
		t.Fatalf("Static Interface member count = %d/%v, want 4", count, ok)
	}
	for index, want := range []static.InterfaceMemberKind{
		static.InterfaceField,
		static.InterfaceMethod,
		static.InterfaceField,
		static.InterfaceMethod,
	} {
		member, ok := interfaces.MemberAt(iface, index)
		if !ok || member.Kind != want {
			t.Fatalf("member[%d] = %#v/%v, want kind %v", index, member, ok, want)
		}
		if want == static.InterfaceField {
			if member.Field == 0 || member.Name != 0 || member.Signature != 0 {
				t.Fatalf("field member[%d] = %#v", index, member)
			}
			span, ok := p.Source().Identity().Span(member.Field)
			if !ok || span.StartLine != uint32([]int{2, 0, 4, 0}[index]) || span.StartCol != 3 {
				t.Fatalf("field member[%d] span = %#v/%v", index, span, ok)
			}
		} else if member.Field != 0 || member.Name == 0 || member.Signature == 0 {
			t.Fatalf("method member[%d] = %#v", index, member)
		} else {
			span, spanOK := p.Source().Identity().Render(member.NameCoordinate)
			if !spanOK || span.StartLine != uint32([]int{0, 3, 0, 5}[index]) || span.StartCol != 12 {
				t.Fatalf("method member[%d] name span = %#v/%v", index, span, spanOK)
			}
		}
	}
}

func sourceAliasTarget(t *testing.T, p *program.Program, index int) keyspace.Term {
	t.Helper()
	aliases := p.Static().Declarations().Aliases()
	alias, ok := aliases.At(index)
	if !ok {
		t.Fatalf("missing Static Alias %d", index)
	}
	_, target, _, _, ok := aliases.Get(alias)
	if !ok || target == 0 {
		t.Fatalf("Static Alias %d target = %v/%v", index, target, ok)
	}
	return target
}

func sourceAliasTargetAtLine(t *testing.T, p *program.Program, line int) (keyspace.Term, keyspace.Term) {
	t.Helper()
	aliases := p.Static().Declarations().Aliases()
	identity := p.Source().Identity()
	for index := 0; index < aliases.Count(); index++ {
		alias, ok := aliases.At(index)
		if !ok {
			t.Fatalf("Static Alias At(%d) failed", index)
		}
		_, target, _, coordinate, ok := aliases.Get(alias)
		span, spanOK := identity.Render(coordinate)
		if !ok || !spanOK || int(span.StartLine) != line {
			continue
		}
		if target == 0 {
			t.Fatalf("Static Alias at line %d has no target", line)
		}
		return alias, target
	}
	t.Fatalf("missing Static Alias at line %d", line)
	return 0, 0
}

func sourceTypeRefPath(t *testing.T, p *program.Program, ref keyspace.Term, want int) []keyspace.Key {
	t.Helper()
	references := p.Static().References()
	count, ok := references.SourceCount(ref)
	if !ok || count != want {
		t.Fatalf("Static TypeRef %v source length = %d/%v, want %d", ref, count, ok, want)
	}
	path := make([]keyspace.Key, count)
	for index := range path {
		path[index], ok = references.SourceAt(ref, index)
		if !ok {
			t.Fatalf("Static TypeRef %v source component %d missing", ref, index)
		}
	}
	return path
}

// This vertical witness follows the final static and Flow owners only.  Static
// syntax is not mirrored through a root Program forwarding vocabulary.
func TestSourceTypesVerticalWitnesses(t *testing.T) {
	t.Run("compound type rows", func(t *testing.T) {
		p := parseBindLower(t, "type Item = { readonly name: string, optional count: number }[] | {[string]: number}")
		alias, ok := p.Static().Declarations().Aliases().At(0)
		if !ok {
			t.Fatal("missing Alias")
		}
		_, target, _, _, aliasOK := p.Static().Declarations().Aliases().Get(alias)
		if !aliasOK {
			t.Fatal("missing Alias target")
		}
		if count, ok := p.Static().Types().Unions().MemberCount(target); !ok || count != 2 {
			t.Fatalf("union members = %d/%v, want two", count, ok)
		}
		array, _ := p.Static().Types().Unions().MemberAt(target, 0)
		element, readonly, arrayOK := p.Static().Types().Arrays().Get(array)
		if !arrayOK || readonly || element == 0 {
			t.Fatalf("array member = element %v readonly %v ok %v", element, readonly, arrayOK)
		}
		recordReadonly, fields, recordOK := p.Static().Types().Records().Get(element)
		if !recordOK || !recordReadonly || fields != 2 {
			t.Fatalf("record member = readonly %v fields %d ok %v", recordReadonly, fields, recordOK)
		}
	})

	t.Run("operators and references", func(t *testing.T) {
		p := parseBindLower(t, "type A = number\ntype Result = keyof(A) | A[\"field\"] | (A extends A ? A : A)")
		operators := p.Static().Operators()
		if operators.KeyOfs().Count() != 1 || operators.IndexAccesses().Count() != 1 || operators.Conditionals().Count() != 1 {
			t.Fatalf("static operator counts = %d/%d/%d", operators.KeyOfs().Count(), operators.IndexAccesses().Count(), operators.Conditionals().Count())
		}
		for _, family := range []keyspace.Term{
			func() keyspace.Term { term, _ := operators.KeyOfs().At(0); return term }(),
			func() keyspace.Term { term, _ := operators.IndexAccesses().At(0); return term }(),
			func() keyspace.Term { term, _ := operators.Conditionals().At(0); return term }(),
		} {
			if span, ok := p.Source().Identity().Span(family); !ok || span.StartLine == 0 {
				t.Fatalf("static operator %v has no Source span", family)
			}
		}
	})

	t.Run("runtime type value and typed call", func(t *testing.T) {
		p := parseBindLower(t, "type User = number\nlocal function id<T>(value: T): T return value end\nreturn id::<User>(User)")
		typeValue, typeValueOK := p.Flow().Authored().TypeValues().At(0)
		call, callOK := p.Flow().Authored().Calls().At(0)
		if !typeValueOK || !callOK {
			t.Fatalf("TypeValue/Call = %v/%v %v/%v", typeValue, typeValueOK, call, callOK)
		}
		target, targetOK := p.Static().Operands().TypeValues().Target(typeValue)
		if !targetOK || target == 0 {
			t.Fatalf("TypeValue target = %v/%v", target, targetOK)
		}
		if count, ok := p.Static().Contracts().Calls().TypeArgumentCount(call); !ok || count != 1 {
			t.Fatalf("Call type arguments = %d/%v, want one", count, ok)
		}
		if _, _, _, _, ok := p.Flow().Authored().Calls().Get(call); !ok {
			t.Fatal("Call row is absent")
		}
	})

	t.Run("cast claim has static target", func(t *testing.T) {
		p := parseBindLower(t, "type User = number\nlocal value = 1 as User")
		claim, claimOK := p.Flow().Authored().Claims().At(0)
		if !claimOK {
			t.Fatal("missing ValueClaim")
		}
		_, _, claimKind, rowOK := p.Flow().Authored().Claims().Get(claim)
		if !rowOK || claimKind != kind.ValueClaimTypeAs {
			t.Fatalf("ValueClaim kind = %v/%v", claimKind, rowOK)
		}
		target, targetOK := p.Static().Operands().Claims().Target(claim)
		if !targetOK || target == 0 {
			t.Fatalf("ValueClaim target = %v/%v", target, targetOK)
		}
		resolution, declaration, root, refOK := p.Static().References().Get(target)
		if !refOK || resolution != static.TypeRefDeclaration || declaration == 0 || root != 0 {
			t.Fatalf("cast target reference = %v/%v/%v/%v", resolution, declaration, root, refOK)
		}
	})
}

// TestFunctionStaticScopesSealAtTheirDistinctHeaderPhases keeps the two
// Function-hosted static frontiers separate. A Function TypeParam is authored
// before that function's formals and retains the enclosing body; a return
// type and its annotation are authored after the formals and use the
// function's own body. Nesting makes both ownership identities observable.
func TestFunctionStaticScopesSealAtTheirDistinctHeaderPhases(t *testing.T) {
	p := parseBindLower(t, `
local outer = 1
local f = function<T: typeof(outer)>(value: T): typeof(value) | number @returns(value)
  local nested = function<U: typeof(value)>(inner: U): typeof(inner) | number @returns(inner)
    return inner
  end
  return value
end
return f
`)

	outerBind, ok := p.Flow().Authored().Storage().Binds().At(0)
	if !ok {
		t.Fatal("missing outer Bind")
	}
	outer := boundCell(t, p, outerBind, 0)
	outerFunction, ok := p.Flow().Authored().Functions().At(0)
	if !ok {
		t.Fatal("missing outer Function")
	}
	nestedFunction, ok := p.Flow().Authored().Functions().At(1)
	if !ok {
		t.Fatal("missing nested Function")
	}

	assertFunctionStaticScope := func(function, preFormalsSource keyspace.Term) {
		t.Helper()
		formal, ok := p.Source().Formals().At(function, 0)
		if !ok {
			t.Fatalf("Function(%v) missing formal", function)
		}
		generic, ok := p.Static().Contracts().Functions().TypeParamAt(function, 0)
		if !ok {
			t.Fatalf("Function(%v) missing generic", function)
		}
		_, _, constraint, ok := p.Static().Declarations().TypeParams().Get(generic)
		if !ok {
			t.Fatalf("Function(%v) generic TypeParam absent", function)
		}
		constraintScope, constraintOperand, ok := p.Static().Operators().TypeOfs().Get(constraint)
		if !ok || constraintScope != generic {
			t.Fatalf(
				"Function(%v) generic TypeOf = scope %v operand %v ok %v; want TypeParam scope",
				function, constraintScope, constraintOperand, ok,
			)
		}
		if _, source, _, ok := p.Flow().Authored().Storage().Reads().Get(constraintOperand); !ok || source != preFormalsSource {
			t.Fatalf(
				"Function(%v) generic TypeOf source = %v/%v; want pre-formals source %v",
				function, source, ok, preFormalsSource,
			)
		}

		returnType, ok := p.Static().Contracts().Functions().ReturnAt(function, 0)
		if !ok {
			t.Fatalf("Function(%v) missing return type", function)
		}
		returnTypeOf, annotationTarget := keyspace.Term(0), keyspace.Term(0)
		for index := 0; index < 2; index++ {
			member, ok := p.Static().Types().Unions().MemberAt(returnType, index)
			if !ok {
				t.Fatalf("Function(%v) return type is not a two-member Union", function)
			}
			if _, _, ok := p.Static().Operators().TypeOfs().Get(member); ok {
				returnTypeOf = member
			}
			if count, ok := p.Static().Operands().Annotations().ForCount(member); ok && count == 1 {
				annotationTarget = member
			}
		}
		if returnTypeOf == 0 || annotationTarget == 0 {
			t.Fatalf("Function(%v) return type omitted typeof or annotation target", function)
		}
		returnScope, returnOperand, ok := p.Static().Operators().TypeOfs().Get(returnTypeOf)
		if !ok || returnScope != function {
			t.Fatalf(
				"Function(%v) return TypeOf = scope %v operand %v ok %v; want Function scope",
				function, returnScope, returnOperand, ok,
			)
		}
		if _, source, _, ok := p.Flow().Authored().Storage().Reads().Get(returnOperand); !ok || source != formal {
			t.Fatalf(
				"Function(%v) return TypeOf source = %v/%v; want formal %v",
				function, source, ok, formal,
			)
		}
		if !p.Flow().Containment().Static(returnTypeOf) || !p.Flow().Containment().Static(returnOperand) {
			t.Fatalf("Function(%v) return TypeOf did not retain static containment", function)
		}

		annotation, ok := p.Static().Operands().Annotations().ForAt(annotationTarget, 0)
		if !ok {
			t.Fatalf("Function(%v) missing return Annotation", function)
		}
		annotationRow, ok := p.Static().Operands().Annotations().Get(annotation)
		if !ok || annotationRow.Scope != function {
			t.Fatalf(
				"Function(%v) Annotation = scope %v values %v ok %v; want Function scope",
				function, annotationRow.Scope, annotationRow.Values, ok,
			)
		}
		annotationValue := valueAt(t, p, annotationRow.Values, 0)
		if _, source, _, ok := p.Flow().Authored().Storage().Reads().Get(annotationValue); !ok || source != formal {
			t.Fatalf(
				"Function(%v) Annotation source = %v/%v; want formal %v",
				function, source, ok, formal,
			)
		}
		if !p.Flow().Containment().Static(annotation) || !p.Flow().Containment().Static(annotationValue) {
			t.Fatalf("Function(%v) Annotation did not retain static containment", function)
		}
	}

	assertFunctionStaticScope(outerFunction, outer)
	outerFormal, ok := p.Source().Formals().At(outerFunction, 0)
	if !ok {
		t.Fatal("missing outer Function formal")
	}
	assertFunctionStaticScope(nestedFunction, outerFormal)
}

func TestStaticAliasesPrimitivesAndReferencesUseStaticVocabulary(t *testing.T) {
	p := parseBindLower(t, "type A = B\ntype B = number\ntype C = A\ntype Node = C?\ntype Receiver = self")
	aliases := p.Static().Declarations().Aliases()
	if aliases.Count() != 5 {
		t.Fatalf("Static Alias count = %d, want 5", aliases.Count())
	}
	a, _ := aliases.At(0)
	b, _ := aliases.At(1)
	c, _ := aliases.At(2)
	node, _ := aliases.At(3)
	receiver, _ := aliases.At(4)
	_, aTarget, _, _, _ := aliases.Get(a)
	_, bTarget, _, _, _ := aliases.Get(b)
	_, cTarget, _, _, _ := aliases.Get(c)
	_, nodeTarget, _, _, _ := aliases.Get(node)
	_, receiverTarget, _, _, _ := aliases.Get(receiver)
	assertStaticDeclarationRef(t, p, aTarget, b)
	if primitive, ok := p.Static().Types().Primitives().Get(bTarget); !ok || primitive != static.PrimitiveNumber {
		t.Fatalf("B primitive = %v/%v, want number", primitive, ok)
	}
	assertStaticDeclarationRef(t, p, cTarget, a)
	inner, optionalOK := p.Static().Types().Optionals().Get(nodeTarget)
	if !optionalOK {
		t.Fatal("missing Optional Node target")
	}
	assertStaticDeclarationRef(t, p, inner, c)
	if primitive, ok := p.Static().Types().Primitives().Get(receiverTarget); !ok || primitive != static.PrimitiveSelf {
		t.Fatalf("Receiver primitive = %v/%v, want self", primitive, ok)
	}
}

func TestStaticCompositeRowsKeepExactChildren(t *testing.T) {
	p := parseBindLower(t, "type Box<T: typeof(subject)> = T\ntype Values = number | string | boolean | nil\ntype Nested = readonly number[][]\ntype Dictionary<K, V> = {[K]: V}\ntype Shape = { readonly name: string, optional count: number }")
	aliases := p.Static().Declarations().Aliases()
	box, _ := aliases.At(0)
	values, _ := aliases.At(1)
	nested, _ := aliases.At(2)
	dictionary, _ := aliases.At(3)
	shape, _ := aliases.At(4)
	param, paramOK := aliases.ParamAt(box, 0)
	if !paramOK {
		t.Fatal("missing Box parameter")
	}
	_, _, constraint, paramRowOK := p.Static().Declarations().TypeParams().Get(param)
	if !paramRowOK || constraint == 0 {
		t.Fatal("missing Box parameter constraint")
	}
	if _, _, ok := p.Static().Operators().TypeOfs().Get(constraint); !ok {
		t.Fatal("Box constraint is not Static TypeOf")
	}
	_, valuesTarget, _, _, _ := aliases.Get(values)
	if count, ok := p.Static().Types().Unions().MemberCount(valuesTarget); !ok || count != 4 {
		t.Fatalf("Values union members = %d/%v, want 4", count, ok)
	}
	_, nestedTarget, _, _, _ := aliases.Get(nested)
	innerArray, readonly, arrayOK := p.Static().Types().Arrays().Get(nestedTarget)
	if !arrayOK || !readonly {
		t.Fatalf("outer Array = %v/%v readonly=%v", innerArray, arrayOK, readonly)
	}
	element, innerReadonly, innerArrayOK := p.Static().Types().Arrays().Get(innerArray)
	if !innerArrayOK || innerReadonly {
		t.Fatalf("inner Array = %v/%v readonly=%v", element, innerArrayOK, innerReadonly)
	}
	if primitive, ok := p.Static().Types().Primitives().Get(element); !ok || primitive != static.PrimitiveNumber {
		t.Fatalf("Nested element = %v/%v", primitive, ok)
	}
	_, dictionaryTarget, _, _, _ := aliases.Get(dictionary)
	key, value, _, mapOK := p.Static().Types().Maps().Get(dictionaryTarget)
	if !mapOK || key == 0 || value == 0 {
		t.Fatalf("Dictionary map = key %v value %v ok %v", key, value, mapOK)
	}
	_, shapeTarget, _, _, _ := aliases.Get(shape)
	shapeReadonly, fieldCount, recordOK := p.Static().Types().Records().Get(shapeTarget)
	if !recordOK || !shapeReadonly || fieldCount != 2 {
		t.Fatalf("Shape record = readonly %v fields %d ok %v", shapeReadonly, fieldCount, recordOK)
	}
}

func TestStaticAnnotationsAndDeclaredTypesStayInStaticOwner(t *testing.T) {
	p := parseBindLower(t, "local first: number = 1\nlocal second: typeof(first) @note(7) = 2\nlocal third = 3")
	declared := p.Static().Declarations().DeclaredTypes()
	if declared.Count() != 2 {
		t.Fatalf("DeclaredType count = %d, want 2", declared.Count())
	}
	annotations := p.Static().Operands().Annotations()
	annotation, annotationOK := annotations.At(0)
	row, rowOK := annotations.Get(annotation)
	if !annotationOK || !rowOK || row.Target == 0 || row.Values == 0 {
		t.Fatalf("Annotation = %#v/%v/%v", row, annotationOK, rowOK)
	}
	if count, ok := annotations.ForCount(row.Target); !ok || count != 1 {
		t.Fatalf("Annotation target count = %d/%v", count, ok)
	}
	binds := p.Flow().Authored().Storage().Binds()
	secondBind, bindOK := binds.At(1)
	if !bindOK {
		t.Fatal("missing second Bind")
	}
	secondCell := boundCell(t, p, secondBind, 0)
	if term, ok := declared.ForCell(secondCell); !ok || term == 0 {
		t.Fatalf("second Cell declared type = %v/%v", term, ok)
	}
	if !p.Flow().Containment().Static(annotation) || !p.Flow().Containment().Static(row.Values) {
		t.Fatal("Static Annotation escaped Flow static containment")
	}
}

func TestStaticSignatureRowsKeepParametersAndReturns(t *testing.T) {
	p := parseBindLower(t, "type Handler = fun(...: number, value: any): (asserts value is string, number)")
	alias, ok := p.Static().Declarations().Aliases().At(0)
	if !ok {
		t.Fatal("missing Handler Alias")
	}
	_, signature, _, _, aliasOK := p.Static().Declarations().Aliases().Get(alias)
	if !aliasOK {
		t.Fatal("missing Handler target")
	}
	scope, variadic, _, returnsKnown, signatureOK := p.Static().Signatures().TypeFunctions().Get(signature)
	if !signatureOK || scope == 0 || variadic == 0 || !returnsKnown {
		t.Fatalf("Signature = scope %v variadic %v known %v ok %v", scope, variadic, returnsKnown, signatureOK)
	}
	if primitive, ok := p.Static().Types().Primitives().Get(variadic); !ok || primitive != static.PrimitiveNumber {
		t.Fatalf("Signature variadic = %v/%v", primitive, ok)
	}
	if count, ok := p.Static().Signatures().TypeFunctions().ParameterCount(signature); !ok || count != 1 {
		t.Fatalf("Signature parameter count = %d/%v, want one", count, ok)
	}
	if count, ok := p.Static().Signatures().TypeFunctions().ReturnCount(signature); !ok || count != 2 {
		t.Fatalf("Signature return count = %d/%v, want two", count, ok)
	}
}

var _ flow.CellKind
var _ keyspace.Term

func TestNestedTypeOfCastComposition(t *testing.T) {
	p := parseBindLower(t, `
local x = 1
type Snapshot = typeof(x as typeof(x))
`)
	staticView := p.Static()
	if got := staticView.Operators().TypeOfs().Count(); got != 2 {
		t.Fatalf("TypeOfCount = %d, want outer and cast-target typeof", got)
	}
	alias, ok := staticView.Declarations().Aliases().At(0)
	if !ok {
		t.Fatal("missing Snapshot alias")
	}
	_, target, _, _, ok := staticView.Declarations().Aliases().Get(alias)
	if !ok {
		t.Fatal("missing Snapshot target")
	}
	scope, operand, ok := staticView.Operators().TypeOfs().Get(target)
	if !ok || scope != alias {
		t.Fatalf("outer TypeOf = scope %v operand %v ok %v, want Snapshot host", scope, operand, ok)
	}
	_, _, claimKind, ok := p.Flow().Authored().Claims().Get(operand)
	inner, innerOK := staticView.Operands().Claims().Target(operand)
	if !ok || claimKind != kind.ValueClaimTypeAs || !innerOK || inner == 0 {
		t.Fatalf("outer operand = ValueClaim target %v/%v kind %v ok %v, want as-claim", inner, innerOK, claimKind, ok)
	}
	if !p.Flow().Containment().Static(operand) {
		t.Fatalf("nested ValueClaim %v escaped static classification", operand)
	}
	innerScope, innerOperand, ok := staticView.Operators().TypeOfs().Get(inner)
	if !ok || innerScope != operand || innerOperand == 0 {
		t.Fatalf("nested TypeOf = scope %v operand %v ok %v, want ValueClaim host", innerScope, innerOperand, ok)
	}
}

func TestStaticValueClaimNestingKeepsTargetlessNonNilStructural(t *testing.T) {
	p := parseBindLower(t, `
type Snapshot = typeof((false as typeof(false))!)
`)
	flowView := p.Flow()
	claims := flowView.Authored().Claims()
	staticView := p.Static()
	if claims.Count() != 2 || staticView.Operators().TypeOfs().Count() != 2 {
		t.Fatalf("ValueClaims/TypeOfs = %d/%d, want 2/2", claims.Count(), staticView.Operators().TypeOfs().Count())
	}
	typed, _ := claims.At(0)
	nonNil, _ := claims.At(1)
	_, typedOperand, typedKind, typedOK := claims.Get(typed)
	_, nonNilOperand, nonNilKind, nonNilOK := claims.Get(nonNil)
	typedTarget, typedTargetOK := staticView.Operands().Claims().Target(typed)
	nonNilTarget, nonNilTargetOK := staticView.Operands().Claims().Target(nonNil)
	if !typedOK || typedKind != kind.ValueClaimTypeAs || !typedTargetOK || typedTarget == 0 || typedOperand == 0 {
		t.Fatalf("typed ValueClaim = operand %v target %v/%v kind %v ok %v", typedOperand, typedTarget, typedTargetOK, typedKind, typedOK)
	}
	if !nonNilOK || nonNilKind != kind.ValueClaimNonNil || nonNilTargetOK || nonNilTarget != 0 || nonNilOperand != typed {
		t.Fatalf("NonNil ValueClaim = operand %v target %v/%v kind %v ok %v", nonNilOperand, nonNilTarget, nonNilTargetOK, nonNilKind, nonNilOK)
	}
	for _, term := range []keyspace.Term{typed, nonNil, typedOperand} {
		if !flowView.Containment().Static(term) {
			t.Fatalf("static value-claim descendant %v escaped static classification", term)
		}
	}
}

func TestNestedTypeOfInReachableNestedClosureRestoresGlobalEvidence(t *testing.T) {
	p := parseBindLower(t, `
local outer = function()
	return function(x: typeof(x as typeof(x)))
		return external
	end
end
return outer
`)
	flowView := p.Flow()
	if got := flowView.Authored().Functions().Count(); got != 2 {
		t.Fatalf("FunctionCount = %d, want outer and reachable nested closure", got)
	}
	if got := p.Static().Operators().TypeOfs().Count(); got != 2 {
		t.Fatalf("TypeOfCount = %d, want nested function-header composition", got)
	}
	if got := flowView.Authored().Storage().Reads().ImplicitCount(); got != 1 {
		t.Fatalf("implicit reads = %d, want reachable closure body global after typeof closes", got)
	}
	implicit, ok := flowView.Authored().Storage().Reads().ImplicitAt(0)
	if !ok {
		t.Fatal("missing nested closure implicit read")
	}
	_, sourceTerm, _, ok := flowView.Authored().Storage().Reads().Get(implicit)
	if !ok {
		t.Fatal("nested closure implicit evidence is not a Read")
	}
	cellKind, _, key, cellOK := flowView.Authored().Storage().Cells().Get(sourceTerm)
	value, keyOK := p.Source().Keys().Exact(key)
	if !cellOK || cellKind != flow.CellGlobal || !keyOK || value.String != "external" {
		t.Fatalf("nested closure implicit source = cell %v key %#v/%v, want global external", cellKind, value, keyOK)
	}
}

func TestStaticFunctionLabelMetadataStaysOutOfExecutableControl(t *testing.T) {
	p := parseBindLower(t, `
type Snapshot = typeof(function()
::again::
goto again
end)
`)
	flowView := p.Flow()
	function, ok := flowView.Authored().Functions().At(0)
	if !ok {
		t.Fatal("missing static Function")
	}
	_, body, _, ok := flowView.Authored().Functions().Get(function)
	if !ok {
		t.Fatal("missing static Function Body")
	}
	label, labelOK := flowView.Authored().Control().Labels().At(0)
	jump, jumpOK := flowView.Authored().Control().Gotos().At(0)
	if !labelOK || !jumpOK {
		t.Fatal("missing static Label/Goto")
	}
	labelOwner, labelOK := flowView.Authored().Control().Labels().Get(label)
	jumpOwner, target, jumpOK := flowView.Authored().Control().Gotos().Get(jump)
	if !labelOK || !jumpOK || labelOwner != body || jumpOwner != body || target != label {
		t.Fatalf(
			"static control = label owner %v, goto owner %v target %v, ok %v/%v",
			labelOwner, jumpOwner, target, labelOK, jumpOK,
		)
	}
	for _, term := range []keyspace.Term{function, body, label, jump} {
		if !flowView.Containment().Static(term) {
			t.Fatalf("static control descendant %v was classified executable", term)
		}
	}
}

func assertStaticDeclarationRef(t *testing.T, p *program.Program, ref, want keyspace.Term) {
	t.Helper()
	resolution, target, root, ok := p.Static().References().Get(ref)
	if !ok || resolution != static.TypeRefDeclaration || target != want || root != 0 {
		t.Fatalf("Static Reference(%v) = resolution %v target %v root %v ok %v; want declaration %v", ref, resolution, target, root, ok, want)
	}
}

func TestStaticAliasesResolveNearestVisibleBareName(t *testing.T) {
	p := parseBindLower(t, "type T = number\ndo\n  type T = string\n  type Inner = T\nend\ntype Outer = T")
	entry, ok := p.Source().Index().Entry()
	if !ok {
		t.Fatal("missing Source entry")
	}
	outerT := controlSourceAt(t, p, entry, 0)
	block := controlSourceAt(t, p, entry, 1)
	outerUse := controlSourceAt(t, p, entry, 2)
	innerT := controlSourceAt(t, p, block, 0)
	innerUse := controlSourceAt(t, p, block, 1)
	aliases := p.Static().Declarations().Aliases()
	for _, row := range []struct {
		alias keyspace.Term
		want  keyspace.Term
	}{
		{innerUse, innerT}, {outerUse, outerT},
	} {
		_, ref, _, _, ok := aliases.Get(row.alias)
		if !ok {
			t.Fatalf("Static Alias(%v) is absent", row.alias)
		}
		assertStaticDeclarationRef(t, p, ref, row.want)
	}
}

func TestStaticTypeParametersKeepTheirOwnDeclarationRows(t *testing.T) {
	p := parseBindLower(t, "type Pair<T: U, U: T> = T")
	alias, ok := p.Static().Declarations().Aliases().At(0)
	if !ok {
		t.Fatal("missing Static Alias")
	}
	params := p.Static().Declarations().TypeParams()
	first, firstOK := p.Static().Declarations().Aliases().ParamAt(alias, 0)
	second, secondOK := p.Static().Declarations().Aliases().ParamAt(alias, 1)
	if !firstOK || !secondOK {
		t.Fatalf("Alias parameters = %v/%v %v/%v", first, firstOK, second, secondOK)
	}
	_, _, firstConstraint, firstRowOK := params.Get(first)
	_, _, secondConstraint, secondRowOK := params.Get(second)
	if !firstRowOK || !secondRowOK {
		t.Fatal("missing Static TypeParam rows")
	}
	assertStaticDeclarationRef(t, p, firstConstraint, second)
	assertStaticDeclarationRef(t, p, secondConstraint, first)
}

func TestStaticNestedFunctionsRemainInStaticFlowContainment(t *testing.T) {
	p := parseBindLower(t, "type Box<T: typeof(function(value: T): T\n  return value\nend)> = T")
	function, ok := p.Flow().Authored().Functions().At(0)
	if !ok || !p.Flow().Containment().Static(function) {
		t.Fatalf("nested Function = %v/%v static=%v", function, ok, p.Flow().Containment().Static(function))
	}
	if count, ok := p.Source().Formals().Len(function); !ok || count != 1 {
		t.Fatalf("nested Function formal count = %d/%v, want one", count, ok)
	}
	formal, _ := p.Source().Formals().At(function, 0)
	cellKind, _, _, cellOK := p.Flow().Authored().Storage().Cells().Get(formal)
	if !cellOK || cellKind != flow.CellLocal {
		t.Fatalf("nested static Function formal cell = kind %v ok %v", cellKind, cellOK)
	}
}

func TestStaticPublicationKeepsStaticTargetAndFlowAssignSeparate(t *testing.T) {
	p := parseBindLower(t, "type T = number\nlocal M = {}\nM.Schema.T = T")
	publications := p.Static().Publications()
	publication, ok := publications.At(0)
	if !ok || publications.Count() != 1 {
		t.Fatalf("Static publication = %v/%v count=%d", publication, ok, publications.Count())
	}
	assign, pair, target, rowOK := publications.Get(publication)
	if !rowOK || assign == 0 || pair != 0 || target == 0 {
		t.Fatalf("Static publication row = assign %v pair %d target %v ok %v", assign, pair, target, rowOK)
	}
	if _, _, assignOK := p.Flow().Authored().Storage().Assigns().Get(assign); !assignOK {
		t.Fatal("publication Assign is absent from Flow")
	}
	if _, _, _, refOK := p.Static().References().Get(target); !refOK {
		t.Fatal("publication target is absent from Static References")
	}
}

func TestStaticNamespaceScaleIsDeterministic(t *testing.T) {
	const declarations = 256
	var input strings.Builder
	for index := 0; index < declarations; index++ {
		fmt.Fprintf(&input, "type Shared = number -- %d\n", index)
	}
	for index := 0; index < declarations; index++ {
		fmt.Fprintf(&input, "type Use%d = Shared\n", index)
	}
	first := parseBindLower(t, input.String())
	second := parseBindLower(t, input.String())
	firstAliases := first.Static().Declarations().Aliases().Count()
	secondAliases := second.Static().Declarations().Aliases().Count()
	if firstAliases != declarations*2 || secondAliases != firstAliases {
		t.Fatalf("Static Alias counts = %d/%d, want %d", firstAliases, secondAliases, declarations*2)
	}
	if first.ContentID() != second.ContentID() {
		t.Fatal("replayed static namespace source changed Program ContentID")
	}
}

func TestStaticTypePublicationIsAdditiveToRuntimeAssignment(t *testing.T) {
	p := parseBindLower(t, `
type User = { id: string }
local M = {}
M.Schema.User = User
return M
`)
	publications := p.Static().Publications()
	if got := publications.Count(); got != 1 {
		t.Fatalf("TypePublicationCount = %d, want 1", got)
	}
	publication, ok := publications.At(0)
	if !ok {
		t.Fatal("missing TypePublication")
	}
	assign, pair, target, ok := publications.Get(publication)
	if !ok || assign == 0 || pair != 0 || target == 0 {
		t.Fatalf(
			"TypePublication = assign %v pair %d target %v ok %v",
			assign, pair, target, ok,
		)
	}
	if state, declaration, _, ok := p.Static().References().Get(target); !ok ||
		state != static.TypeRefDeclaration || declaration == 0 {
		t.Fatalf("publication TypeRef = state %v declaration %v ok %v", state, declaration, ok)
	}
	assigns := p.Flow().Authored().Storage().Assigns()
	if assigns.Count() != 1 {
		t.Fatalf("AssignCount = %d, want executable assignment retained", assigns.Count())
	}
	if _, _, ok := assigns.Get(assign); !ok {
		t.Fatalf("publication Assign %v is missing", assign)
	}
	write, writeOK := assigns.WriteAt(assign, int(pair))
	_, targetTerm, targetOK := p.Flow().Authored().Storage().Writes().Get(write)
	if !writeOK || !targetOK || targetTerm == 0 {
		t.Fatalf("publication Assign pair %d = Write %v/%v target %v/%v", pair, write, writeOK, targetTerm, targetOK)
	}
	if reads := p.Flow().Authored().Storage().Reads().Count(); reads != 4 {
		t.Fatalf("ReadCount = %d, want ordinary nested-LHS, RHS, and return reads only", reads)
	}
}

func TestStaticTypePublicationUsesPerPairEvidenceWithoutExtraRuntimeWork(t *testing.T) {
	p := parseBindLower(t, `
type User = { id: string }
local M = {}
local value = 1
M.User, M.value = User, value
`)
	assigns := p.Flow().Authored().Storage().Assigns()
	publications := p.Static().Publications()
	if assigns.Count() != 1 || publications.Count() != 1 {
		t.Fatalf("Assigns/Publications = %d/%d, want 1/1", assigns.Count(), publications.Count())
	}
	assign, ok := assigns.At(0)
	if !ok {
		t.Fatal("missing runtime Assign")
	}
	if count, ok := assigns.WriteCount(assign); !ok || count != 2 {
		t.Fatalf("Assign target count = %d/%v, want 2", count, ok)
	}
	_, values, ok := assigns.Get(assign)
	if !ok {
		t.Fatalf("root %v is not Assign", assign)
	}
	if fixed, ok := p.Flow().Authored().Values().Len(values); !ok || fixed != 2 || valuesTail(t, p, values) != 0 {
		t.Fatalf("Assign Values = fixed %d/%v tail %v, want two authored RHS values", fixed, ok, valuesTail(t, p, values))
	}
	publication, _ := publications.At(0)
	gotAssign, pair, _, ok := publications.Get(publication)
	if !ok || gotAssign != assign || pair != 0 {
		t.Fatalf("TypePublication Assign/pair = %v/%d/%v, want %v/0/true", gotAssign, pair, ok, assign)
	}
	write, writeOK := assigns.WriteAt(assign, int(pair))
	_, target, targetOK := p.Flow().Authored().Storage().Writes().Get(write)
	if !writeOK || !targetOK || target == 0 {
		t.Fatalf("TypePublication pair Write = %v/%v target %v/%v", write, writeOK, target, targetOK)
	}
	if p.Flow().Authored().Storage().Reads().Count() != 4 || p.Flow().Authored().Access().Exact().Count() != 2 || p.Flow().Authored().Calls().Count() != 0 {
		t.Fatalf("runtime topology Reads/Lenses/Calls = %d/%d/%d, want 4/2/0", p.Flow().Authored().Storage().Reads().Count(), p.Flow().Authored().Access().Exact().Count(), p.Flow().Authored().Calls().Count())
	}
}

func TestQualifiedAssignmentsRemainRuntime(t *testing.T) {
	p := parseBindLower(t, `
local M = {}
local Runtime = {}
local value = 1
M.User, M.value = Runtime, value
M.new = Factory.new
`)
	if got := p.Static().Publications().Count(); got != 0 {
		t.Fatalf("TypePublicationCount = %d, want no inferred Factory.new metadata", got)
	}
	if got := p.Flow().Authored().Storage().Assigns().Count(); got != 2 {
		t.Fatalf("AssignCount = %d, want 2", got)
	}
}

func TestStaticTypePublicationDeepPathRetainsCompactPublication(t *testing.T) {
	const depth = 2048
	var source strings.Builder
	source.WriteString("type Published = number\nlocal M = {}\nM")
	for index := 0; index < depth; index++ {
		fmt.Fprintf(&source, ".k%04d", index)
	}
	source.WriteString(" = Published\n")

	p := parseBindLower(t, source.String())
	publications := p.Static().Publications()
	if got := publications.Count(); got != 1 {
		t.Fatalf("TypePublicationCount = %d, want one", got)
	}
	publication, ok := publications.At(0)
	if !ok {
		t.Fatal("missing TypePublication")
	}

	assign, pair, target, ok := publications.Get(publication)
	if !ok || assign == 0 || pair != 0 || target == 0 {
		t.Fatalf("deep TypePublication = assign %v pair %d target %v ok %v", assign, pair, target, ok)
	}
	if lenses := p.Flow().Authored().Access().Exact().Count(); lenses != depth {
		t.Fatalf("deep TypePublication exact lenses = %d, want %d", lenses, depth)
	}
}

func TestBracketAssignmentRemainsRuntimeOnly(t *testing.T) {
	p := parseBindLower(t, `
type Published = number
local M = {}
M["Published"] = Published
`)
	if got := p.Static().Publications().Count(); got != 0 {
		t.Fatalf("TypePublicationCount = %d, want no static publication for a bracket key", got)
	}
	if got := p.Flow().Authored().Storage().Assigns().Count(); got != 1 {
		t.Fatalf("AssignCount = %d, want retained runtime assignment", got)
	}
}
