package binaryprimitive

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/flowtest"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func binaryPrimitiveTestResult() *Result {
	add := keyspace.MakeTerm(keyspace.FamilyBinary, 1)
	equal := keyspace.MakeTerm(keyspace.FamilyBinary, 2)
	greater := keyspace.MakeTerm(keyspace.FamilyBinary, 3)
	return &Result{
		sourceID: flowtest.ContentIDAt(1), flowID: flowtest.ContentIDAt(2),
		staticID: flowtest.ContentIDAt(3), moduleID: flowtest.ContentIDAt(4),
		slots: []uint32{0, 1, 2, 3},
		primitives: []primitiveRow{
			{source: add, operation: Operation{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Op: kind.BinaryAdd, Left: keyspace.MakeTerm(keyspace.FamilyInteger, 1), Right: keyspace.MakeTerm(keyspace.FamilyInteger, 2)}},
			{source: equal, operation: Operation{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Op: kind.BinaryEqual, Left: keyspace.MakeTerm(keyspace.FamilyInteger, 1), Right: keyspace.MakeTerm(keyspace.FamilyInteger, 2)}, comparison: Comparison{Branch: keyspace.MakeTerm(keyspace.FamilyBranch, 1), TrueBody: keyspace.MakeTerm(keyspace.FamilyBody, 2), FalseBody: keyspace.MakeTerm(keyspace.FamilyBody, 3), Left: keyspace.MakeTerm(keyspace.FamilyInteger, 1), Right: keyspace.MakeTerm(keyspace.FamilyInteger, 2)}, hasCompare: true},
			{source: greater, operation: Operation{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Op: kind.BinaryGreater, Left: keyspace.MakeTerm(keyspace.FamilyInteger, 1), Right: keyspace.MakeTerm(keyspace.FamilyInteger, 2)}, comparison: Comparison{Branch: keyspace.MakeTerm(keyspace.FamilyBranch, 2), TrueBody: keyspace.MakeTerm(keyspace.FamilyBody, 2), FalseBody: keyspace.MakeTerm(keyspace.FamilyBody, 3), Left: keyspace.MakeTerm(keyspace.FamilyInteger, 2), Right: keyspace.MakeTerm(keyspace.FamilyInteger, 1)}, hasCompare: true},
		},
		buckets: bucketStore{
			arithmetic: []keyspace.Term{add},
			equality:   []keyspace.Term{equal},
			order:      []keyspace.Term{greater},
		},
	}
}

func TestPrimitiveQueriesExposeOnlyTypedCanonicalBuckets(t *testing.T) {
	result := binaryPrimitiveTestResult()
	if result.Arithmetic().Count() != 1 || result.Equality().Count() != 1 || result.Order().Count() != 1 || result.Bitwise().Count() != 0 {
		t.Fatalf("bucket counts = %d/%d/%d/%d", result.Arithmetic().Count(), result.Bitwise().Count(), result.Equality().Count(), result.Order().Count())
	}
	if got, ok := result.Arithmetic().At(0); !ok || got != keyspace.MakeTerm(keyspace.FamilyBinary, 1) {
		t.Fatalf("Arithmetic.At = %v/%v", got, ok)
	}
	if _, ok := result.Arithmetic().At(1); ok {
		t.Fatal("Arithmetic.At accepted its end boundary")
	}
	if _, ok := result.Arithmetic().At(-1); ok {
		t.Fatal("Arithmetic.At accepted a negative index")
	}
	primitive, ok := result.Primitive(keyspace.MakeTerm(keyspace.FamilyBinary, 2))
	if !ok {
		t.Fatal("Equality primitive was not retained")
	}
	if source, ok := primitive.Source(); !ok || source != keyspace.MakeTerm(keyspace.FamilyBinary, 2) {
		t.Fatalf("Primitive.Source = %v/%v", source, ok)
	}
	operation, ok := primitive.Operation()
	if !ok || operation.Op != kind.BinaryEqual || operation.Left != keyspace.MakeTerm(keyspace.FamilyInteger, 1) || operation.Right != keyspace.MakeTerm(keyspace.FamilyInteger, 2) {
		t.Fatalf("Primitive.Operation = %#v/%v", operation, ok)
	}
	comparison, ok := primitive.Comparison()
	if !ok || comparison.Branch != keyspace.MakeTerm(keyspace.FamilyBranch, 1) || comparison.Left != operation.Left || comparison.Right != operation.Right || comparison.Invert {
		t.Fatalf("Primitive.Comparison = %#v/%v", comparison, ok)
	}
}

func TestPrimitiveComparisonNormalFormsAndBranchlessQueries(t *testing.T) {
	ops := []struct {
		op          kind.BinaryOp
		left, right uint32
		invert      bool
	}{
		{kind.BinaryEqual, 1, 2, false},
		{kind.BinaryNotEqual, 1, 2, true},
		{kind.BinaryLess, 1, 2, false},
		{kind.BinaryLessEqual, 1, 2, false},
		{kind.BinaryGreater, 2, 1, false},
		{kind.BinaryGreaterEqual, 2, 1, false},
	}
	for _, test := range ops {
		comparison := Comparison{Branch: keyspace.MakeTerm(keyspace.FamilyBranch, 1), TrueBody: keyspace.MakeTerm(keyspace.FamilyBody, 1), FalseBody: keyspace.MakeTerm(keyspace.FamilyBody, 2), Left: keyspace.MakeTerm(keyspace.FamilyInteger, test.left), Right: keyspace.MakeTerm(keyspace.FamilyInteger, test.right), Invert: test.invert}
		operation := Operation{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Op: test.op, Left: keyspace.MakeTerm(keyspace.FamilyInteger, 1), Right: keyspace.MakeTerm(keyspace.FamilyInteger, 2)}
		if !validComparison(operation, comparison) {
			t.Fatalf("validComparison rejected %v", test.op)
		}
	}
	result := binaryPrimitiveTestResult()
	branchless, ok := result.Primitive(keyspace.MakeTerm(keyspace.FamilyBinary, 1))
	if !ok {
		t.Fatal("branchless arithmetic primitive was not retained")
	}
	if _, ok := branchless.Comparison(); ok {
		t.Fatal("branchless primitive exposed a comparison")
	}
	if _, ok := result.Primitive(keyspace.MakeTerm(keyspace.FamilyBinary, 99)); ok {
		t.Fatal("Primitive accepted an out-of-denominator Binary")
	}
	if _, ok := result.Primitive(keyspace.MakeTerm(keyspace.FamilyUnary, 1)); ok {
		t.Fatal("Primitive accepted a wrong-family term")
	}
}

func TestPrimitiveComparisonRejectsMalformedNormalization(t *testing.T) {
	rawLeft := keyspace.MakeTerm(keyspace.FamilyInteger, 1)
	rawRight := keyspace.MakeTerm(keyspace.FamilyInteger, 2)
	base := Comparison{Branch: keyspace.MakeTerm(keyspace.FamilyBranch, 1), TrueBody: keyspace.MakeTerm(keyspace.FamilyBody, 1), FalseBody: keyspace.MakeTerm(keyspace.FamilyBody, 2), Left: rawLeft, Right: rawRight}
	operation := Operation{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Op: kind.BinaryEqual, Left: rawLeft, Right: rawRight}
	bad := base
	bad.Invert = true
	if validComparison(operation, bad) {
		t.Fatal("equal comparison admitted an inverted normal form")
	}
	bad = base
	bad.Left, bad.Right = rawRight, rawLeft
	operation.Op = kind.BinaryLess
	if validComparison(operation, bad) {
		t.Fatal("less comparison admitted a swapped normal form")
	}
	bad = base
	bad.Left, bad.Right = rawLeft, rawRight
	operation.Op = kind.BinaryGreater
	if validComparison(operation, bad) {
		t.Fatal("greater comparison admitted an unswapped normal form")
	}
	bad = base
	bad.TrueBody = bad.FalseBody
	operation.Op = kind.BinaryEqual
	if validComparison(operation, bad) {
		t.Fatal("comparison admitted duplicate Branch bodies")
	}
	operation.Left = keyspace.MakeTerm(keyspace.FamilyInteger, 3)
	bad = base
	if validComparison(operation, bad) {
		t.Fatal("comparison admitted operands copied from a different raw operation")
	}
}

func TestPrimitiveQueriesRejectMalformedOperationAndCrossedBucketSlot(t *testing.T) {
	result := binaryPrimitiveTestResult()
	result.primitives[0].operation.Owner = keyspace.MakeTerm(keyspace.FamilyInteger, 1)
	primitive, ok := result.Primitive(keyspace.MakeTerm(keyspace.FamilyBinary, 1))
	if !ok {
		t.Fatal("malformed operation lost its structural handle")
	}
	if _, ok := primitive.Operation(); ok {
		t.Fatal("Operation accepted an owner outside FamilyBody")
	}

	result = binaryPrimitiveTestResult()
	result.primitives[0].operation.Left = keyspace.MakeTerm(keyspace.FamilyInvalid, 1)
	primitive, ok = result.Primitive(keyspace.MakeTerm(keyspace.FamilyBinary, 1))
	if !ok {
		t.Fatal("malformed operation lost its structural handle")
	}
	if _, ok := primitive.Operation(); ok {
		t.Fatal("Operation accepted an invalid left operand")
	}

	result = binaryPrimitiveTestResult()
	result.primitives[0].operation.Right = keyspace.MakeTerm(keyspace.FamilyInvalid, 1)
	primitive, ok = result.Primitive(keyspace.MakeTerm(keyspace.FamilyBinary, 1))
	if !ok {
		t.Fatal("malformed operation lost its structural handle")
	}
	if _, ok := primitive.Operation(); ok {
		t.Fatal("Operation accepted an invalid right operand")
	}

	result = binaryPrimitiveTestResult()
	other := keyspace.MakeTerm(keyspace.FamilyBinary, 4)
	result.slots = append(result.slots, 4)
	result.primitives = append(result.primitives, primitiveRow{
		source: other,
		operation: Operation{
			Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Op: kind.BinaryAdd,
			Left: keyspace.MakeTerm(keyspace.FamilyInteger, 1), Right: keyspace.MakeTerm(keyspace.FamilyInteger, 2),
		},
	})
	result.slots[1] = 4
	if _, ok := result.Arithmetic().At(0); ok {
		t.Fatal("Arithmetic bucket accepted a term whose slot points to another same-category primitive")
	}
}

func TestPrimitiveQueriesFailClosedAndDoNotAllocate(t *testing.T) {
	result := binaryPrimitiveTestResult()
	result.slots[2] = 0
	if _, ok := result.Primitive(keyspace.MakeTerm(keyspace.FamilyBinary, 2)); ok {
		t.Fatal("malformed slot remained queryable")
	}
	result = binaryPrimitiveTestResult()
	result.sourceID = identity.ContentID{}
	if result.Arithmetic().Count() != 0 {
		t.Fatal("unavailable provenance exposed bucket rows")
	}
	if _, ok := result.Primitive(keyspace.MakeTerm(keyspace.FamilyBinary, 1)); ok {
		t.Fatal("unavailable provenance exposed a primitive")
	}
	result = binaryPrimitiveTestResult()
	arithmetic := result.Arithmetic()
	if allocations := testing.AllocsPerRun(1000, func() {
		if _, ok := arithmetic.At(0); !ok {
			t.Fatal("scaled bucket query failed")
		}
		primitive, ok := result.Primitive(keyspace.MakeTerm(keyspace.FamilyBinary, 1))
		if !ok {
			t.Fatal("scaled primitive lookup failed")
		}
		if _, ok := primitive.Operation(); !ok {
			t.Fatal("scaled operation query failed")
		}
	}); allocations != 0 {
		t.Fatalf("primitive queries allocated %v objects per run", allocations)
	}
}

func TestPrimitiveQueriesScaleWithDenseBuckets(t *testing.T) {
	const members = 10000
	result := &Result{
		sourceID: flowtest.ContentIDAt(1), flowID: flowtest.ContentIDAt(2),
		staticID: flowtest.ContentIDAt(3), moduleID: flowtest.ContentIDAt(4),
		slots:      make([]uint32, members+1),
		primitives: make([]primitiveRow, members),
		buckets: bucketStore{
			arithmetic: make([]keyspace.Term, members),
		},
	}
	for index := 0; index < members; index++ {
		binary := keyspace.MakeTerm(keyspace.FamilyBinary, uint32(index+1))
		result.slots[index+1] = uint32(index + 1)
		result.primitives[index] = primitiveRow{source: binary, operation: Operation{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Op: kind.BinaryAdd, Left: keyspace.MakeTerm(keyspace.FamilyInteger, 1), Right: keyspace.MakeTerm(keyspace.FamilyInteger, 1)}}
		result.buckets.arithmetic[index] = binary
	}
	view := result.Arithmetic()
	if view.Count() != members {
		t.Fatalf("scaled arithmetic count = %d, want %d", view.Count(), members)
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		term, ok := view.At(members - 1)
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyBinary, members) {
			t.Fatal("scaled arithmetic At failed")
		}
		primitive, ok := result.Primitive(term)
		if !ok {
			t.Fatal("scaled primitive lookup failed")
		}
		if _, ok := primitive.Operation(); !ok {
			t.Fatal("scaled operation lookup failed")
		}
	}); allocations != 0 {
		t.Fatalf("scaled primitive queries allocated %v objects per run", allocations)
	}
}
