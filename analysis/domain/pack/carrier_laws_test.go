package pack

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/test/fixture/staticclass"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
)

type carrierFixture struct {
	schema     *Schema
	owner      *algebra
	first      *relation
	second     *relation
	firstRoot  Root
	secondRoot Root
	firstEnd   Endpoint
	secondEnd  Endpoint
	firstPort  Port
	secondPort Port
	any        static.Class
}

func newCarrierFixture(t testing.TB, classes *static.ClassSet) *carrierFixture {
	t.Helper()
	if classes == nil {
		t.Fatal("nil real ClassSet")
	}
	any := classes.AnyValue()
	owner, ok := newAlgebraWithOffsets(classes, []static.Class{any, classes.Nil()}, []nat{natFromUint64(0), natFromUint64(1), natFromUint64(2), natFromUint64(3)})
	if !ok {
		t.Fatal("seal Pack-local algebra")
	}
	firstEnd, ok := newEndpoint(owner, 1, any)
	if !ok {
		t.Fatal("first endpoint")
	}
	secondEnd, ok := newEndpoint(owner, 2, any)
	if !ok {
		t.Fatal("second endpoint")
	}
	firstPort, ok := newPort(owner, 1, any, true)
	if !ok {
		t.Fatal("first port")
	}
	secondPort, ok := newPort(owner, 2, any, true)
	if !ok {
		t.Fatal("second port")
	}
	first := &relation{owner: owner, index: 1, targets: []equationTarget{{kind: EquationScalar, index: firstEnd.index}, {kind: EquationPack, index: firstPort.index}}}
	second := &relation{owner: owner, index: 2, targets: []equationTarget{{kind: EquationScalar, index: secondEnd.index}, {kind: EquationPack, index: secondPort.index}}}
	if !first.valid() || !second.valid() {
		t.Fatal("seal Pack-local relations")
	}
	state := &schema{owner: owner, relations: []*relation{first, second}}
	fixture := &carrierFixture{
		schema: &Schema{state: state}, owner: owner, first: first, second: second,
		firstRoot: Root{schema: state, index: 0}, secondRoot: Root{schema: state, index: 1},
		firstEnd: firstEnd, secondEnd: secondEnd, firstPort: firstPort, secondPort: secondPort, any: any,
	}
	if !fixture.firstRoot.valid() || !fixture.secondRoot.valid() {
		t.Fatal("seal Pack-local roots")
	}
	return fixture
}

func realClasses(t testing.TB) *static.ClassSet {
	t.Helper()
	capsule, err := staticclass.Seal()
	if err != nil {
		t.Fatal(err)
	}
	return capsule.Classes
}

func (fixture *carrierFixture) exactValue(t *testing.T, relation *relation, scalar Scalar, term Term) Value {
	t.Helper()
	endpoint, port := fixture.firstEnd, fixture.firstPort
	if relation == fixture.second {
		endpoint, port = fixture.secondEnd, fixture.secondPort
	}
	scalarEquation, ok := scalarEquation(endpoint, scalar)
	if !ok {
		t.Fatal("scalar equation")
	}
	packEquation, ok := packEquation(port, term)
	if !ok {
		t.Fatal("Pack equation")
	}
	caseValue, ok := exactCase(relation, []Equation{scalarEquation, packEquation})
	if !ok {
		t.Fatal("complete exact case")
	}
	value, ok := valueFromCases(relation, []Case{caseValue})
	if !ok {
		t.Fatal("exact value")
	}
	return value
}

func (fixture *carrierFixture) closedValue(t *testing.T) Value {
	t.Helper()
	scalar, ok := endpointScalar(fixture.firstEnd)
	if !ok {
		t.Fatal("endpoint scalar")
	}
	term, ok := closedTerm(fixture.owner, []Scalar{scalar})
	if !ok {
		t.Fatal("closed Pack term")
	}
	return fixture.exactValue(t, fixture.first, scalar, term)
}

func (fixture *carrierFixture) unknownValue(t *testing.T) Value {
	t.Helper()
	scalar, ok := anyScalar(fixture.owner, fixture.any)
	if !ok {
		t.Fatal("unknown scalar")
	}
	term, ok := anyTerm(fixture.owner)
	if !ok {
		t.Fatal("unknown Pack term")
	}
	return fixture.exactValue(t, fixture.first, scalar, term)
}

func (fixture *carrierFixture) openValue(t *testing.T) Value {
	t.Helper()
	tail, ok := freeTail(fixture.secondPort)
	if !ok {
		t.Fatal("free tail")
	}
	offset, ok := zeroOffset(fixture.owner)
	if !ok {
		t.Fatal("zero offset")
	}
	head, ok := headScalar(tail, offset)
	if !ok {
		t.Fatal("head scalar")
	}
	rest, ok := tailRest(tail, offset)
	if !ok {
		t.Fatal("tail rest")
	}
	term, ok := openTerm(fixture.owner, []Scalar{head}, rest, nil)
	if !ok {
		t.Fatal("open Pack term")
	}
	return fixture.exactValue(t, fixture.first, head, term)
}

func TestPackLatticeLaws(t *testing.T) {
	fixture := newCarrierFixture(t, realClasses(t))
	latticelaws.LawSuite[Value]{
		Name:   "pack",
		Domain: fixture.schema.Lattice(),
		Sample: []Value{fixture.schema.Bottom(), fixture.closedValue(t), fixture.unknownValue(t), fixture.openValue(t), fixture.schema.Top()},
	}.Run(t)
}

func TestPackExactRelationCompleteness(t *testing.T) {
	fixture := newCarrierFixture(t, realClasses(t))
	closed := fixture.closedValue(t)
	caseValue := closed.cases[0]
	if !caseValue.valid() || caseValue.relation != fixture.first {
		t.Fatal("missing complete relation case")
	}
	if _, ok := exactCase(fixture.first, caseValue.equations[:1]); ok {
		t.Fatal("partial target vector admitted")
	}
	wrongScalar, ok := scalarEquation(fixture.secondEnd, caseValue.equations[0].scalar)
	if !ok {
		t.Fatal("mistarget scalar setup")
	}
	if _, ok := exactCase(fixture.first, []Equation{wrongScalar, caseValue.equations[1]}); ok {
		t.Fatal("mistargeted vector admitted")
	}
	top, ok := exactCase(fixture.first, nil)
	if !ok || !top.top || top.relation != nil {
		t.Fatal("empty conjunction did not canonicalize to global top")
	}
}

func TestPackRootRelationFence(t *testing.T) {
	fixture := newCarrierFixture(t, realClasses(t))
	for _, root := range []Root{fixture.firstRoot, fixture.secondRoot} {
		if !fixture.schema.Admit(root, fixture.schema.Bottom()) || !fixture.schema.Admit(root, fixture.schema.Top()) {
			t.Fatal("global extremum rejected at root")
		}
	}
	value := fixture.closedValue(t)
	if !fixture.schema.Admit(fixture.firstRoot, value) {
		t.Fatal("exact relation value rejected")
	}
	if fixture.schema.Admit(fixture.secondRoot, value) {
		t.Fatal("nonextreme value crossed relation fence")
	}
}

func TestPackForeignAuthorityFence(t *testing.T) {
	local := newCarrierFixture(t, realClasses(t))
	foreign := newCarrierFixture(t, realClasses(t))
	if _, ok := newEndpoint(local.owner, 3, foreign.any); ok {
		t.Fatal("foreign class entered local algebra")
	}
	localValue := local.closedValue(t)
	foreignValue := foreign.closedValue(t)
	if equalValue(localValue, foreignValue) || lessOrEqualValue(localValue, foreignValue) {
		t.Fatal("foreign value compared as local")
	}
	if _, ok := joinValue(localValue, foreignValue); ok {
		t.Fatal("foreign join silently succeeded")
	}
	if _, ok := widenValue(localValue, foreignValue); ok {
		t.Fatal("foreign widening silently succeeded")
	}
}

func TestPackNormalizationAndFingerprints(t *testing.T) {
	fixture := newCarrierFixture(t, realClasses(t))
	value := fixture.closedValue(t)
	forward, ok := exactCase(fixture.first, value.cases[0].equations)
	if !ok {
		t.Fatal("forward case")
	}
	reverse, ok := exactCase(fixture.first, []Equation{value.cases[0].equations[1], value.cases[0].equations[0]})
	if !ok || !equalCase(forward, reverse) || forward.hash != reverse.hash {
		t.Fatal("permuted equations did not normalize")
	}
	left := fixture.boundCase(t, 7)
	right := fixture.boundCase(t, 91)
	if !equalCase(left, right) || left.hash != right.hash {
		t.Fatal("alpha-renamed tails did not normalize")
	}
	leftValue, ok := valueFromCases(fixture.first, []Case{left})
	if !ok {
		t.Fatal("left alpha value")
	}
	rightValue, ok := valueFromCases(fixture.first, []Case{right})
	if !ok || !equalValue(leftValue, rightValue) || valueFingerprint(leftValue) != valueFingerprint(rightValue) {
		t.Fatal("alpha-normalized values lost stable fingerprint")
	}
	other := &relation{owner: fixture.owner, index: 3, targets: append([]equationTarget(nil), fixture.first.targets...)}
	otherCase, ok := exactCase(other, forward.equations)
	if !ok {
		t.Fatal("same geometry distinct relation")
	}
	otherValue, ok := valueFromCases(other, []Case{otherCase})
	if !ok || equalValue(leftValue, otherValue) || valueFingerprint(leftValue) == valueFingerprint(otherValue) {
		t.Fatal("relation identity did not fence normalized value")
	}
	replayed := newCarrierFixture(t, realClasses(t)).closedValue(t)
	if valueFingerprint(value) != valueFingerprint(replayed) {
		t.Fatal("replayed semantic fixture changed fingerprint")
	}
}

func (fixture *carrierFixture) boundCase(t *testing.T, index uint32) Case {
	t.Helper()
	tail, ok := boundTail(fixture.owner, index, fixture.any)
	if !ok {
		t.Fatal("bound tail")
	}
	offset, ok := zeroOffset(fixture.owner)
	if !ok {
		t.Fatal("bound zero offset")
	}
	head, ok := headScalar(tail, offset)
	if !ok {
		t.Fatal("bound head")
	}
	rest, ok := tailRest(tail, offset)
	if !ok {
		t.Fatal("bound rest")
	}
	term, ok := openTerm(fixture.owner, []Scalar{head}, rest, nil)
	if !ok {
		t.Fatal("bound open term")
	}
	scalarEquation, ok := scalarEquation(fixture.firstEnd, head)
	if !ok {
		t.Fatal("bound scalar equation")
	}
	packEquation, ok := packEquation(fixture.firstPort, term)
	if !ok {
		t.Fatal("bound Pack equation")
	}
	caseValue, ok := exactCase(fixture.first, []Equation{scalarEquation, packEquation})
	if !ok {
		t.Fatal("bound exact case")
	}
	return caseValue
}

func TestPackWidenOverApproximatesOpenClosedHeadTail(t *testing.T) {
	fixture := newCarrierFixture(t, realClasses(t))
	closed := fixture.closedValue(t)
	open := fixture.openValue(t)
	unknown := fixture.unknownValue(t)
	result, ok := widenValue(closed, open)
	if !ok || !lessOrEqualValue(closed, result) || !lessOrEqualValue(open, result) {
		t.Fatal("widen did not over-approximate closed/head-tail inputs")
	}
	result, ok = widenValue(result, unknown)
	if !ok || !lessOrEqualValue(closed, result) || !lessOrEqualValue(open, result) || !lessOrEqualValue(unknown, result) {
		t.Fatal("widen did not preserve upper-bound law without a cap")
	}
}
