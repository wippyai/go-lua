package operation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/numeric"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

type operationFixture struct {
	source  *link.Link
	program *program.Program
	algebra *numeric.Algebra
}

func fixtureShard(t testing.TB, source *link.Link) linkproject.Shard {
	t.Helper()
	shard, ok := source.Project().Mounts().At(0)
	if !ok {
		t.Fatal("fixture Project shard")
	}
	return shard
}

func newOperationFixture(t *testing.T, name, text string) operationFixture {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: name, Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	algebra, ok := numeric.New(source)
	if !ok {
		t.Fatal("Numeric rejected BinaryPrimitives fixture")
	}
	return operationFixture{source: source, program: p, algebra: algebra}
}

func TestPrimitiveOperationConsumesBranchlessArithmeticAndBitwise(t *testing.T) {
	fixture := newOperationFixture(t, "numeric_operation_direct", `
local arithmetic = 1 + 2
local bitwise = 7 & 3
return arithmetic, bitwise
`)
	primitives := fixture.program.Flow().BinaryPrimitives()
	seen := 0
	for _, bucket := range []struct {
		name  string
		count func() int
		at    func(int) (keyspace.Term, bool)
	}{
		{name: "arithmetic", count: func() int { return primitives.Arithmetic().Count() }, at: func(i int) (keyspace.Term, bool) { return primitives.Arithmetic().At(i) }},
		{name: "bitwise", count: func() int { return primitives.Bitwise().Count() }, at: func(i int) (keyspace.Term, bool) { return primitives.Bitwise().At(i) }},
	} {
		for index := 0; index < bucket.count(); index++ {
			binary, ok := bucket.at(index)
			if !ok {
				t.Fatalf("%s primitive %d", bucket.name, index)
			}
			primitive, ok := primitives.Primitive(binary)
			if !ok {
				t.Fatalf("%s primitive handle %v", bucket.name, binary)
			}
			if _, branchless := primitive.Comparison(); branchless {
				t.Fatalf("branchless %s unexpectedly exposed Comparison", bucket.name)
			}
			operand, ok := NewOperand(fixture.source, fixture.algebra, fixtureShard(t, fixture.source), binary)
			if !ok || !operand.valid() {
				t.Fatalf("%s BinaryPrimitive was not consumed", bucket.name)
			}
			seen++
		}
	}
	if seen != 2 {
		t.Fatalf("direct branchless primitive operations = %d, want 2", seen)
	}
}

func TestPrimitiveOperationRetainsRawProjectionAndExactResults(t *testing.T) {
	fixture := newOperationFixture(t, "numeric_operation_results", `
local arithmetic = 1 + 2
local bitwise = 7 & 3
local dynamicLeft = 7
local dynamicRight = 3
local dynamicBitwise = dynamicLeft & dynamicRight
	return arithmetic, bitwise, dynamicBitwise
`)
	primitives := fixture.program.Flow().BinaryPrimitives()
	addTerm, ok := primitives.Arithmetic().At(0)
	if !ok {
		t.Fatal("add primitive")
	}
	addPrimitive, ok := primitives.Primitive(addTerm)
	if !ok {
		t.Fatal("add primitive projection")
	}
	addOperation, ok := addPrimitive.Operation()
	if !ok || addOperation.Op != kind.BinaryAdd {
		t.Fatalf("add operation = %#v/%v", addOperation, ok)
	}
	addOperand, ok := NewOperand(fixture.source, fixture.algebra, fixtureShard(t, fixture.source), addTerm)
	if !ok {
		t.Fatal("add operand")
	}
	if addOperand.owner != addOperation.Owner || addOperand.leftTerm != addOperation.Left || addOperand.rightTerm != addOperation.Right || addOperand.op != addOperation.Op {
		t.Fatalf("raw operation projection = owner %v/%v left %v/%v right %v/%v op %v/%v", addOperand.owner, addOperation.Owner, addOperand.leftTerm, addOperation.Left, addOperand.rightTerm, addOperation.Right, addOperand.op, addOperation.Op)
	}
	resultScalar, resultOK := addOperand.result.Scalar()
	leftScalar, leftOK := addOperand.left.Scalar()
	rightScalar, rightOK := addOperand.right.Scalar()
	if !resultOK || !leftOK || !rightOK || resultScalar.Term() != addTerm || leftScalar.Term() != addOperation.Left || rightScalar.Term() != addOperation.Right ||
		resultScalar.Body() != addOperation.Owner || leftScalar.Body() != addOperation.Owner || rightScalar.Body() != addOperation.Owner {
		t.Fatalf("raw scalar projection = result %#v left %#v right %#v", resultScalar, leftScalar, rightScalar)
	}
	translated, ok := result(fixture.algebra, addOperand, fixture.algebra.Default())
	if !ok {
		t.Fatal("exact integer addition translation")
	}
	if mask, present := translated.Eligibility(addOperand.result); !present || mask != numeric.MayInteger {
		t.Fatalf("addition result eligibility = %v/%v, want integer", mask, present)
	}
	forward, ok := fixture.algebra.Pair(addOperand.result, addOperand.left)
	if !ok {
		t.Fatal("result-left pair")
	}
	if bound, infinite, present := translated.Bound(forward); !present || infinite || bound != 2 {
		t.Fatalf("result-left bound = %d/%v/%v, want 2", bound, infinite, present)
	}
	reverse, ok := fixture.algebra.Pair(addOperand.left, addOperand.result)
	if !ok {
		t.Fatal("left-result pair")
	}
	if bound, infinite, present := translated.Bound(reverse); !present || infinite || bound != -2 {
		t.Fatalf("left-result bound = %d/%v/%v, want -2", bound, infinite, present)
	}

	bitTerm, ok := primitives.Bitwise().At(0)
	if !ok {
		t.Fatal("bitwise primitive")
	}
	bitPrimitive, ok := primitives.Primitive(bitTerm)
	if !ok {
		t.Fatal("bitwise primitive projection")
	}
	bitOperation, ok := bitPrimitive.Operation()
	if !ok || bitOperation.Op != kind.BinaryBitAnd {
		t.Fatalf("bitwise operation = %#v/%v", bitOperation, ok)
	}
	bitOperand, ok := NewOperand(fixture.source, fixture.algebra, fixtureShard(t, fixture.source), bitTerm)
	if !ok {
		t.Fatal("bitwise operand")
	}
	if bitOperand.owner != bitOperation.Owner || bitOperand.leftTerm != bitOperation.Left || bitOperand.rightTerm != bitOperation.Right || bitOperand.op != bitOperation.Op {
		t.Fatalf("raw bitwise projection = owner %v/%v left %v/%v right %v/%v op %v/%v", bitOperand.owner, bitOperation.Owner, bitOperand.leftTerm, bitOperation.Left, bitOperand.rightTerm, bitOperation.Right, bitOperand.op, bitOperation.Op)
	}
	integerInputs, ok := fixture.algebra.AdmitAt(bitOperand.key, map[numeric.Atom]numeric.Eligibility{
		bitOperand.left:  numeric.MayInteger,
		bitOperand.right: numeric.MayInteger,
	}, nil, nil, nil, nil)
	if !ok {
		t.Fatal("integer bitwise premise")
	}
	bitResult, ok := result(fixture.algebra, bitOperand, integerInputs)
	if !ok {
		t.Fatal("bitwise integer result")
	}
	if mask, present := bitResult.Eligibility(bitOperand.result); !present || mask != numeric.MayInteger {
		t.Fatalf("bitwise result eligibility = %v/%v, want integer", mask, present)
	}
	var mixedOperand Operand
	for index := 0; index < primitives.Bitwise().Count(); index++ {
		candidate, present := primitives.Bitwise().At(index)
		if !present {
			continue
		}
		candidateOperand, accepted := NewOperand(fixture.source, fixture.algebra, fixtureShard(t, fixture.source), candidate)
		if !accepted {
			continue
		}
		leftBase, leftPresent := fixture.algebra.Default().Eligibility(candidateOperand.left)
		rightBase, rightPresent := fixture.algebra.Default().Eligibility(candidateOperand.right)
		if leftPresent && rightPresent && (!leftBase.MustInteger() || !rightBase.MustInteger()) {
			mixedOperand = candidateOperand
			break
		}
	}
	if !mixedOperand.valid() {
		t.Fatal("dynamic bitwise operand")
	}
	leftBase, leftPresent := fixture.algebra.Default().Eligibility(mixedOperand.left)
	rightBase, rightPresent := fixture.algebra.Default().Eligibility(mixedOperand.right)
	if !leftPresent || !rightPresent {
		t.Fatal("dynamic bitwise eligibility")
	}
	leftMask := leftBase &^ numeric.MayInteger
	if !leftMask.Valid() {
		leftMask = leftBase
	}
	nonnumericInputs, ok := fixture.algebra.AdmitAt(mixedOperand.key, map[numeric.Atom]numeric.Eligibility{
		mixedOperand.left:  leftMask,
		mixedOperand.right: rightBase,
	}, nil, nil, nil, nil)
	if !ok {
		t.Fatal("mixed bitwise premise")
	}
	if _, ok := result(fixture.algebra, mixedOperand, nonnumericInputs); ok {
		t.Fatal("bitwise result accepted a non-integer premise")
	}
}

func TestPrimitiveComparisonRejectsOperationOperand(t *testing.T) {
	fixture := newOperationFixture(t, "numeric_operation_comparison", "return 1 < 2")
	primitives := fixture.program.Flow().BinaryPrimitives()
	binary, ok := primitives.Order().At(0)
	if !ok {
		t.Fatal("comparison primitive")
	}
	if _, ok := NewOperand(fixture.source, fixture.algebra, fixtureShard(t, fixture.source), binary); ok {
		t.Fatal("comparison primitive became an operation operand")
	}
}

func TestPrimitiveOperationForeignAndDeadCoordinatesFailClosed(t *testing.T) {
	fixture := newOperationFixture(t, "numeric_operation_owner", "return 1 + 2")
	foreign := newOperationFixture(t, "numeric_operation_foreign", "return 1 + 2")
	primitives := fixture.program.Flow().BinaryPrimitives()
	binary, ok := primitives.Arithmetic().At(0)
	if !ok {
		t.Fatal("arithmetic primitive")
	}
	if _, ok := NewOperand(foreign.source, fixture.algebra, fixtureShard(t, foreign.source), binary); ok {
		t.Fatal("foreign Link crossed Numeric owner fence")
	}
	if _, ok := NewOperand(fixture.source, foreign.algebra, fixtureShard(t, fixture.source), binary); ok {
		t.Fatal("foreign Algebra crossed Numeric owner fence")
	}
	if _, ok := NewOperand(fixture.source, fixture.algebra, fixtureShard(t, fixture.source), keyspace.MakeTerm(keyspace.FamilyBinary, 999)); ok {
		t.Fatal("dead Binary acquired an operation operand")
	}
	if _, ok := NewOperand(fixture.source, fixture.algebra, fixtureShard(t, fixture.source), keyspace.MakeTerm(keyspace.FamilyInteger, 1)); ok {
		t.Fatal("non-Binary term acquired an operation operand")
	}
}

func TestPrimitiveOperationRejectsSameContentForeignLiveOwners(t *testing.T) {
	fixture := newOperationFixture(t, "numeric_operation_same_content", "return 1 + 2")
	foreignSource := sameContentOperationLink(t, fixture.source)
	foreignAlgebra, ok := numeric.New(foreignSource)
	if !ok || foreignSource == fixture.source || foreignSource.ContentID() != fixture.source.ContentID() || foreignAlgebra.Link() != foreignSource {
		t.Fatal("same-content independent Numeric fixture")
	}
	binary, ok := fixture.program.Flow().BinaryPrimitives().Arithmetic().At(0)
	if !ok {
		t.Fatal("arithmetic primitive")
	}
	if _, ok := NewOperand(foreignSource, fixture.algebra, fixtureShard(t, foreignSource), binary); ok {
		t.Fatal("same-content foreign Link crossed Numeric operation owner fence")
	}
	if _, ok := NewOperand(fixture.source, foreignAlgebra, fixtureShard(t, fixture.source), binary); ok {
		t.Fatal("same-content foreign Algebra crossed Numeric operation owner fence")
	}
}

func sameContentOperationLink(t testing.TB, original *link.Link) *link.Link {
	t.Helper()
	contract, ok := original.Boundary().Target()
	if !ok || contract == nil {
		t.Fatal("original Link target")
	}
	mounts := original.Project().Mounts()
	shard, ok := mounts.At(0)
	if !ok {
		t.Fatal("original Link shard")
	}
	name, nameOK := mounts.Name(shard)
	program, programOK := mounts.Program(shard)
	if !nameOK || !programOK || program == nil {
		t.Fatal("original Link module")
	}
	module := linkproject.Module{Name: name, Program: program}
	clone, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{module}})
	if err != nil {
		t.Fatal(err)
	}
	return clone
}
