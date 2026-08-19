package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

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
	if kind, ok := primitives.Get(variadic); !ok || kind != statictypes.PrimitiveBoolean {
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
		if !ok || resolution != staticrefs.Unresolved || target != 0 || root != want.root {
			t.Fatalf("%s TypeRef = resolution %v target %v root %v ok %v; want unresolved/0/%v", want.name, resolution, target, root, ok, want.root)
		}
	}

	if path := sourceTypeRefPath(t, p, bare, 1); path[0] == 0 {
		t.Fatal("bare TypeRef lost its authored source component")
	}
	resolution, target, root, ok := references.Get(bare)
	if !ok || resolution != staticrefs.Declaration || target != outerAlias || root != 0 {
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
		kind       statictypes.PrimitiveKind
		methodCall bool
	}{
		{kind: statictypes.PrimitiveString},
		{kind: statictypes.PrimitiveInteger, methodCall: true},
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
	if kind, ok := primitives.Get(narrow); !ok || kind != statictypes.PrimitiveString {
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
	if kind, ok := primitives.Get(narrow); !ok || kind != statictypes.PrimitiveNumber {
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
	for index, want := range []staticdecl.InterfaceMemberKind{
		staticdecl.InterfaceField,
		staticdecl.InterfaceMethod,
		staticdecl.InterfaceField,
		staticdecl.InterfaceMethod,
	} {
		member, ok := interfaces.MemberAt(iface, index)
		if !ok || member.Kind != want {
			t.Fatalf("member[%d] = %#v/%v, want kind %v", index, member, ok, want)
		}
		switch want {
		case staticdecl.InterfaceField:
			if member.Field == 0 || member.Name != 0 || member.Signature != 0 {
				t.Fatalf("field member[%d] = %#v", index, member)
			}
			span, ok := p.Source().Identity().Span(member.Field)
			if !ok || span.StartLine != uint32([]int{2, 0, 4, 0}[index]) || span.StartCol != 3 {
				t.Fatalf("field member[%d] span = %#v/%v", index, span, ok)
			}
		case staticdecl.InterfaceMethod:
			if member.Field != 0 || member.Name == 0 || member.Signature == 0 {
				t.Fatalf("method member[%d] = %#v", index, member)
			}
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
