package static

import (
	"math"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	proglink "github.com/wippyai/go-lua/program/link"
	linkstatic "github.com/wippyai/go-lua/program/link/static"
)

func TestDenseOrdinalsRejectUnrepresentableRows(t *testing.T) {
	if ^uint(0)>>32 == 0 {
		t.Skip("upper-bound law requires a 64-bit host int")
	}
	if _, err := denseOrdinal(int(uint64(math.MaxUint32) + 1)); err == nil {
		t.Fatal("unrepresentable dense ordinal was admitted")
	}
}

func TestContainedOperandPreservesKnownRuntimeAndInvalidDispositions(t *testing.T) {
	p, source, authority := sealedStatic(t, staticLawSource)
	ownerID := p.ContentID()
	typeOfs := p.Static().Operators().TypeOfs()
	if typeOfs.Count() != 2 {
		t.Fatalf("typeof count = %d, want 2", typeOfs.Count())
	}
	first, ok := typeOfs.At(0)
	if !ok {
		t.Fatal("first typeof")
	}
	_, runtimeTerm, ok := typeOfs.Get(first)
	if !ok {
		t.Fatal("runtime typeof operand")
	}
	runtimeInput := inputForTypeOf(t, source, first)
	runtime, ok := authority.Input(runtimeInput)
	if !ok || runtime.Kind() != OperandRuntimeSubject {
		t.Fatalf("runtime contained operand = %#v/%v", runtime, ok)
	}
	if _, ok := runtime.RuntimeSubject(); !ok {
		t.Fatal("runtime contained operand lost its exact subject")
	}
	if owner, source, ok := runtime.Source(); !ok || owner != ownerID || source != runtimeTerm ||
		!runtime.Namespace().Available() || !runtime.Law().Available() || !runtime.Dependency().Available() {
		t.Fatalf("runtime contained operand provenance = %x/%d/%v", owner, source, ok)
	}
	second, ok := typeOfs.At(1)
	if !ok {
		t.Fatal("second typeof")
	}
	_, _, ok = typeOfs.Get(second)
	if !ok {
		t.Fatal("literal typeof operand")
	}
	knownInput := inputForTypeOf(t, source, second)
	known, ok := authority.Input(knownInput)
	if !ok || known.Kind() != OperandKnown {
		t.Fatalf("known contained operand = %#v/%v", known, ok)
	}
	if value, ok := known.Known(); !ok || !value.IsClosed() {
		t.Fatal("known contained operand lost its exact Static value")
	}
	foreign, foreignLink, _ := sealedStatic(t, strings.Replace(staticLawSource, "local subject = 1", "local subject = 2", 1))
	if foreign.ContentID() == ownerID {
		t.Fatal("foreign-owner fixture did not change Program identity")
	}
	foreignTypeof, _ := foreign.Static().Operators().TypeOfs().At(1)
	foreignInput := inputForTypeOf(t, foreignLink, foreignTypeof)
	if _, ok := authority.Input(foreignInput); ok {
		t.Fatal("foreign StaticInput exposed a contained operand")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_, _ = authority.Input(knownInput)
	}); allocations != 0 {
		t.Fatalf("contained-operand hot lookup allocates %v", allocations)
	}
}

func TestContainedLiteralTermsUseLiteralIdentity(t *testing.T) {
	p, _, authority := sealedStatic(t, `
type NilLiteral = typeof(nil)
type BoolLiteral = typeof(true)
type IntegerLiteral = typeof(7)
type FloatLiteral = typeof(3.5)
type StringLiteral = typeof("literal")
`)
	cases := map[string]typ.Type{
		"NilLiteral":     typ.Nil,
		"BoolLiteral":    typ.LiteralBool(true),
		"IntegerLiteral": typ.LiteralInt(7),
		"FloatLiteral":   typ.LiteralNumber(3.5),
		"StringLiteral":  typ.LiteralString("literal"),
	}
	for name, expected := range cases {
		term, _ := aliasNamed(t, p, name)
		actual, ok := authority.ClosedType(resultFor(t, authority, p, term))
		if !ok || !typ.TypeEquals(actual, expected) {
			t.Fatalf("%s = %v/%v, want %v", name, actual, ok, expected)
		}
	}
}

func inputForTypeOf(t testing.TB, source *proglink.Link, typeOf keyspace.Term) linkstatic.InputRef {
	t.Helper()
	for index := 0; index < source.Static().Inputs().Count(); index++ {
		input, ok := source.Static().Inputs().At(index)
		if !ok {
			continue
		}
		_, _, expression, _, _, _, ok := source.Static().Inputs().Source(input)
		reference, referenceOK := source.Static().Expressions().Reference(expression)
		if ok && referenceOK && reference.Term() == typeOf {
			return input
		}
	}
	t.Fatalf("missing StaticInput for TypeOf %v", typeOf)
	return linkstatic.InputRef{}
}

func mustFrontierBody(t testing.TB, p *program.Program, site keyspace.Term) keyspace.Term {
	t.Helper()
	body, _, ok := p.Source().Index().Frontier(site)
	if !ok {
		t.Fatal("SourceFrontier")
	}
	return body
}

func TestContainedOperandRejectsUnadmittedStaticExpression(t *testing.T) {
	p, source, authority := sealedStatic(t, `type Bad = typeof({})`)
	typeof, ok := p.Static().Operators().TypeOfs().At(0)
	if !ok {
		t.Fatal("typeof")
	}
	_, term, ok := p.Static().Operators().TypeOfs().Get(typeof)
	if !ok {
		t.Fatal("typeof operand")
	}
	input := inputForTypeOf(t, source, typeof)
	operand, ok := authority.Input(input)
	if !ok || operand.Kind() != OperandInvalid {
		t.Fatalf("unadmitted contained operand = %#v/%v", operand, ok)
	}
	if fault, ok := operand.Fault(); !ok || fault != FaultContainment {
		t.Fatalf("unadmitted contained operand fault = %v/%v", fault, ok)
	}
	owner, operandSource, sourceOK := operand.Source()
	if !sourceOK || owner != p.ContentID() || operandSource != term || !operand.Namespace().Available() || !operand.Law().Available() || !operand.Dependency().Available() {
		t.Fatalf("invalid contained operand provenance = %x/%d/%v", owner, operandSource, sourceOK)
	}
	unknown := unknownOperand(authority, containedKey{
		owner: p.ContentID(), term: term, site: typeof, resolver: operand.Namespace(),
		frontierBody: mustFrontierBody(t, p, typeof),
		env:          operand.Environment(), operation: operand.Operation(),
	}, operand.Dependency(), ReasonStaticUnknown)
	if unknown.Kind() != OperandUnknown {
		t.Fatal("legitimate Static Unknown changed disposition")
	}
	if reason, ok := unknown.UnknownReason(); !ok || reason != ReasonStaticUnknown {
		t.Fatalf("Static Unknown lost structured reason = %v/%v", reason, ok)
	}
	if _, ok := unknown.Fault(); ok {
		t.Fatal("Static Unknown acquired an invalid diagnostic")
	}
}

// A literal require without a project namespace is still a sealed Link
// operand. Static consumes its explicit unresolved disposition; it must not
// infer missing resolution from an absent Link row or fall through to runtime.
func TestContainedOperandRejectsSealedUnresolvedLiteralRequire(t *testing.T) {
	p, source, authority := sealedStatic(t, `type Bad = typeof(require("missing"))`)
	typeof, ok := p.Static().Operators().TypeOfs().At(0)
	if !ok {
		t.Fatal("typeof")
	}
	input := inputForTypeOf(t, source, typeof)
	operand, ok := authority.Input(input)
	if !ok || operand.Kind() != OperandInvalid {
		t.Fatalf("unresolved literal require operand = %#v/%v", operand, ok)
	}
	if fault, ok := operand.Fault(); !ok || fault != FaultContainment {
		t.Fatalf("unresolved literal require fault = %v/%v", fault, ok)
	}
}

func TestInvalidStaticResultRetainsExactStructuredSite(t *testing.T) {
	p, _, authority := sealedStatic(t, `type Bad = keyof(number)`)
	_, target := aliasNamed(t, p, "Bad")
	result := resultFor(t, authority, p, target)
	if fault, ok := result.Fault(); !ok || fault != FaultProjection {
		t.Fatalf("invalid result fault = %v/%v", fault, ok)
	}
	owner, source, ok := result.InvalidSource()
	if !ok || owner != p.ContentID() || source != target {
		t.Fatalf("invalid result source = %x/%d/%v", owner, source, ok)
	}
	if namespace, ok := result.InvalidNamespace(); !ok || !namespace.Available() {
		t.Fatal("invalid result lost namespace identity")
	}
	if law, ok := result.InvalidLaw(); !ok || !law.Available() {
		t.Fatal("invalid result lost evaluator-law identity")
	}
	if dependency, ok := result.InvalidDependency(); !ok || dependency != p.ContentID() {
		t.Fatalf("invalid result dependency = %x/%v", dependency, ok)
	}
}

func TestSymbolicAndInvalidAdmissionRejectSourceLessSites(t *testing.T) {
	p, _, authority := sealedStatic(t, `type Open<T> = T`)
	base := Symbolic{
		namespace: authority.LinkID(), law: authority.lawID,
		dependency: p.ContentID(), reason: ReasonOpenFormal,
	}
	if _, err := authority.addSymbolic(base); err == nil {
		t.Fatal("source-less symbolic site was admitted")
	}
	if _, err := authority.addInvalid(base, FaultReference); err == nil {
		t.Fatal("source-less invalid site was admitted")
	}
	_, target := aliasNamed(t, p, "Open")
	for _, coordinate := range authority.coordinates {
		if coordinate.key.reference.Owner() == p.ContentID() && coordinate.key.reference.Root() == target {
			base.reference = coordinate.key.reference
			break
		}
	}
	base.sourceOwner = p.ContentID()
	if _, err := authority.addSymbolic(base); err == nil {
		t.Fatal("mismatched partial Program source was admitted")
	}
}

func TestNamespaceMaterializationUsesAnExplicitDeepWorkStack(t *testing.T) {
	const depth = 4096
	root := &namespaceNode{children: make(map[keyspace.LiteralValue]*namespaceNode)}
	node := root
	for index := 0; index < depth; index++ {
		next := &namespaceNode{children: make(map[keyspace.LiteralValue]*namespaceNode)}
		node.children[keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "next"}] = next
		node = next
	}
	node.value = typ.String
	value, ok := materializeNamespaceIterative(root)
	if !ok || value == nil {
		t.Fatal("deep namespace materialization")
	}
	for index := 0; index < depth; index++ {
		record, ok := value.(*typ.Record)
		if !ok || len(record.Fields) != 1 || record.Fields[0].Name != "next" {
			t.Fatalf("deep namespace node %d = %T", index, value)
		}
		value = record.Fields[0].Type
	}
	if value.Kind() != typ.String.Kind() {
		t.Fatalf("deep namespace leaf = %v", value)
	}
}

func TestNamespaceMaterializationIsIndependentOfMapInsertionOrder(t *testing.T) {
	build := func(reverse bool) *namespaceNode {
		root := &namespaceNode{children: make(map[keyspace.LiteralValue]*namespaceNode)}
		rows := []struct {
			key   keyspace.LiteralValue
			value typ.Type
		}{
			{keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "z"}, typ.String},
			{keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 7}, typ.Integer},
			{keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "a"}, typ.Boolean},
		}
		for index := range rows {
			at := index
			if reverse {
				at = len(rows) - 1 - index
			}
			root.children[rows[at].key] = &namespaceNode{value: rows[at].value, children: make(map[keyspace.LiteralValue]*namespaceNode)}
		}
		return root
	}
	left, leftOK := materializeNamespaceIterative(build(false))
	right, rightOK := materializeNamespaceIterative(build(true))
	if !leftOK || !rightOK || !subtype.IsSubtype(left, right) || !subtype.IsSubtype(right, left) {
		t.Fatalf("namespace materialization changed with insertion order: %v/%v", left, right)
	}
}

func TestStaticEvaluationRetainsFiniteRecursiveAuthority(t *testing.T) {
	p, _, authority := sealedStatic(t, `type Node = { next: Node? }`)
	node, _ := aliasNamed(t, p, "Node")
	result := resultFor(t, authority, p, node)
	value, ok := authority.ClosedType(result)
	if !ok || typ.ValidateStaticGenericRecurrence(value) != nil {
		t.Fatal("finite recursive Static authority became unresolved or invalid")
	}
}
