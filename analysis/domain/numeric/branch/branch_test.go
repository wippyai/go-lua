package branch

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

type branchFixture struct {
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

func newBranchFixture(t *testing.T, name string) branchFixture {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: name, Text: []byte(`
local x = 1
local y = 2
if x == y then x = x + 1 else x = x - 1 end
if x ~= y then x = x * 2 else x = x / 2 end
if x < y then x = x // 1 else x = x % 2 end
if x <= y then x = x ^ 2 else x = x + 3 end
if x > y then x = x - 4 else x = x + 5 end
if x >= y then x = x * 6 else x = x / 7 end
`)})
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
		t.Fatal("Numeric rejected comparison BinaryPrimitives fixture")
	}
	return branchFixture{source: source, program: p, algebra: algebra}
}

type primitiveBucket struct {
	name  string
	count func() int
	at    func(int) (keyspace.Term, bool)
}

func TestPrimitiveComparisonConsumesAllSixNormalFormsAndBothBodies(t *testing.T) {
	fixture := newBranchFixture(t, "numeric_branch_forms")
	primitives := fixture.program.Flow().BinaryPrimitives()
	buckets := []primitiveBucket{
		{name: "equality", count: func() int { return primitives.Equality().Count() }, at: func(i int) (keyspace.Term, bool) { return primitives.Equality().At(i) }},
		{name: "order", count: func() int { return primitives.Order().Count() }, at: func(i int) (keyspace.Term, bool) { return primitives.Order().At(i) }},
	}
	seen := make(map[kind.BinaryOp]int)
	for _, bucket := range buckets {
		for index := 0; index < bucket.count(); index++ {
			binary, ok := bucket.at(index)
			if !ok {
				t.Fatalf("%s primitive %d", bucket.name, index)
			}
			primitive, ok := primitives.Primitive(binary)
			if !ok {
				t.Fatalf("%s primitive handle %v", bucket.name, binary)
			}
			operation, operationOK := primitive.Operation()
			comparison, comparisonOK := primitive.Comparison()
			if !operationOK || !comparisonOK {
				t.Fatalf("%s BinaryPrimitive %v lost Operation/Comparison", bucket.name, binary)
			}
			seen[operation.Op]++
			for branchIndex := 0; branchIndex < 2; branchIndex++ {
				var root numeric.Root
				if operation.Op == kind.BinaryEqual || operation.Op == kind.BinaryNotEqual {
					operand, operandOK := NewEqualityOperand(fixture.source, fixture.algebra, fixtureShard(t, fixture.source), binary, branchIndex)
					if !operandOK {
						t.Fatalf("%s branch %d was not consumed", bucket.name, branchIndex)
					}
					root, operandOK = operand.BranchRoot()
					if !operandOK || root.Body() != map[bool]keyspace.Term{true: comparison.TrueBody, false: comparison.FalseBody}[branchIndex == 0] {
						t.Fatalf("%s branch %d root = %v", bucket.name, branchIndex, root.Body())
					}
				} else {
					operand, operandOK := NewOrderOperand(fixture.source, fixture.algebra, fixtureShard(t, fixture.source), binary, branchIndex)
					if !operandOK {
						t.Fatalf("%s branch %d was not consumed", bucket.name, branchIndex)
					}
					root, operandOK = operand.BranchRoot()
					if !operandOK || root.Body() != map[bool]keyspace.Term{true: comparison.TrueBody, false: comparison.FalseBody}[branchIndex == 0] {
						t.Fatalf("%s branch %d root = %v", bucket.name, branchIndex, root.Body())
					}
				}
			}
		}
	}
	for _, op := range []kind.BinaryOp{kind.BinaryEqual, kind.BinaryNotEqual, kind.BinaryLess, kind.BinaryLessEqual, kind.BinaryGreater, kind.BinaryGreaterEqual} {
		if seen[op] == 0 {
			t.Fatalf("missing comparison operator %v", op)
		}
	}
}

func TestPrimitiveComparisonUsesNormalizedOperandsAndInvertsOnce(t *testing.T) {
	fixture := newBranchFixture(t, "numeric_branch_normalized")
	primitives := fixture.program.Flow().BinaryPrimitives()
	for index := 0; index < primitives.Order().Count(); index++ {
		binary, _ := primitives.Order().At(index)
		primitive, _ := primitives.Primitive(binary)
		operation, _ := primitive.Operation()
		comparison, ok := primitive.Comparison()
		if !ok {
			t.Fatal("order primitive unexpectedly branchless")
		}
		trueOperand, ok := NewOrderOperand(fixture.source, fixture.algebra, fixtureShard(t, fixture.source), binary, 0)
		if !ok {
			t.Fatal("true order operand")
		}
		leftScalar, leftOK := trueOperand.left.Scalar()
		rightScalar, rightOK := trueOperand.right.Scalar()
		if !leftOK || !rightOK || leftScalar.Term() != comparison.Left || rightScalar.Term() != comparison.Right {
			t.Fatalf("order retained raw operands: got %v/%v normalized %v/%v", leftScalar.Term(), rightScalar.Term(), comparison.Left, comparison.Right)
		}
		if operation.Op == kind.BinaryGreater || operation.Op == kind.BinaryGreaterEqual {
			if operation.Left != comparison.Right || operation.Right != comparison.Left {
				t.Fatalf("greater comparison was not normalized exactly once")
			}
		}
		pair, pairOK := fixture.algebra.Pair(trueOperand.left, trueOperand.right)
		if !pairOK {
			t.Fatal("normalized order pair")
		}
		value, valueOK := orderConstraint(fixture.algebra, trueOperand)
		if !valueOK {
			t.Fatal("true order constraint")
		}
		bound, infinite, found := value.Bound(pair)
		want := int64(0)
		if operation.Op == kind.BinaryLess || operation.Op == kind.BinaryGreater {
			want = -1
		}
		if !found || infinite || bound != want {
			t.Fatalf("true %v normalized bound = %d/%t/%t, want %d", operation.Op, bound, infinite, found, want)
		}
		falseOperand, ok := NewOrderOperand(fixture.source, fixture.algebra, fixtureShard(t, fixture.source), binary, 1)
		if !ok {
			t.Fatal("false order operand")
		}
		reverse, reverseOK := fixture.algebra.Pair(falseOperand.right, falseOperand.left)
		if !reverseOK {
			t.Fatal("false normalized pair")
		}
		falseValue, falseOK := orderConstraint(fixture.algebra, falseOperand)
		if !falseOK {
			t.Fatal("false order constraint")
		}
		falseBound, falseInfinite, falseFound := falseValue.Bound(reverse)
		falseWant := int64(-1)
		if operation.Op == kind.BinaryLess || operation.Op == kind.BinaryGreater {
			falseWant = 0
		}
		if !falseFound || falseInfinite || falseBound != falseWant {
			t.Fatalf("false %v normalized bound = %d/%t/%t, want %d", operation.Op, falseBound, falseInfinite, falseFound, falseWant)
		}
	}
	for index := 0; index < primitives.Equality().Count(); index++ {
		binary, _ := primitives.Equality().At(index)
		primitive, _ := primitives.Primitive(binary)
		operation, _ := primitive.Operation()
		comparison, ok := primitive.Comparison()
		if !ok {
			t.Fatal("equality primitive unexpectedly branchless")
		}
		trueOperand, trueOK := NewEqualityOperand(fixture.source, fixture.algebra, fixtureShard(t, fixture.source), binary, 0)
		falseOperand, falseOK := NewEqualityOperand(fixture.source, fixture.algebra, fixtureShard(t, fixture.source), binary, 1)
		if !trueOK || !falseOK {
			t.Fatal("equality branch operands")
		}
		trueValue, trueResult := equalityResult(fixture.algebra, trueOperand)
		falseValue, falseResult := equalityResult(fixture.algebra, falseOperand)
		pair, pairOK := fixture.algebra.Pair(trueOperand.left, trueOperand.right)
		if !pairOK || !trueResult || !falseResult {
			t.Fatal("equality pair/result")
		}
		wantTrueEqual := operation.Op == kind.BinaryEqual
		if comparison.Invert != (operation.Op == kind.BinaryNotEqual) {
			t.Fatalf("equality %v did not retain its sealed invert normal form", operation.Op)
		}
		if wantTrueEqual != trueValue.MustEqual(pair) || wantTrueEqual == trueValue.MustUnequal(pair) {
			t.Fatalf("true equality %v invert=%v has wrong polarity", operation.Op, comparison.Invert)
		}
		if wantTrueEqual == falseValue.MustEqual(pair) || wantTrueEqual != falseValue.MustUnequal(pair) {
			t.Fatalf("false equality %v invert=%v has wrong polarity", operation.Op, comparison.Invert)
		}
	}
}

func TestPrimitiveComparisonForeignAndBranchlessCoordinatesFailClosed(t *testing.T) {
	fixture := newBranchFixture(t, "numeric_branch_owner")
	foreign := newBranchFixture(t, "numeric_branch_foreign")
	primitives := fixture.program.Flow().BinaryPrimitives()
	binary, ok := primitives.Equality().At(0)
	if !ok {
		t.Fatal("equality primitive")
	}
	if _, ok := NewEqualityOperand(foreign.source, fixture.algebra, fixtureShard(t, foreign.source), binary, 0); ok {
		t.Fatal("foreign Link crossed branch owner fence")
	}
	if _, ok := NewEqualityOperand(fixture.source, foreign.algebra, fixtureShard(t, fixture.source), binary, 0); ok {
		t.Fatal("foreign Algebra crossed branch owner fence")
	}
	// A valid primitive operation without a Comparison cannot become a branch
	// operand. The fixture's arithmetic primitive is the branchless witness.
	arithmetic, ok := primitives.Arithmetic().At(0)
	if !ok {
		t.Fatal("arithmetic primitive")
	}
	if _, ok := NewEqualityOperand(fixture.source, fixture.algebra, fixtureShard(t, fixture.source), arithmetic, 0); ok {
		t.Fatal("branchless arithmetic became an equality branch")
	}
	if _, ok := NewOrderOperand(fixture.source, fixture.algebra, fixtureShard(t, fixture.source), arithmetic, 0); ok {
		t.Fatal("branchless arithmetic became an order branch")
	}
}

func TestPrimitiveComparisonRejectsSameContentForeignLiveOwners(t *testing.T) {
	fixture := newBranchFixture(t, "numeric_branch_same_content")
	foreignSource := sameContentBranchLink(t, fixture.source)
	foreignAlgebra, ok := numeric.New(foreignSource)
	if !ok || foreignSource == fixture.source || foreignSource.ContentID() != fixture.source.ContentID() || foreignAlgebra.Link() != foreignSource {
		t.Fatal("same-content independent Numeric fixture")
	}
	binary, ok := fixture.program.Flow().BinaryPrimitives().Equality().At(0)
	if !ok {
		t.Fatal("equality primitive")
	}
	if _, ok := NewEqualityOperand(foreignSource, fixture.algebra, fixtureShard(t, foreignSource), binary, 0); ok {
		t.Fatal("same-content foreign Link crossed Numeric branch owner fence")
	}
	if _, ok := NewEqualityOperand(fixture.source, foreignAlgebra, fixtureShard(t, fixture.source), binary, 0); ok {
		t.Fatal("same-content foreign Algebra crossed Numeric branch owner fence")
	}
}

func sameContentBranchLink(t testing.TB, original *link.Link) *link.Link {
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
