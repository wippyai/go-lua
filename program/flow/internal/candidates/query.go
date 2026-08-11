package candidates

import "github.com/wippyai/go-lua/program/keyspace"

// The Result bucket accessors are the compact form used by Flow assembly.
func (r *Result) UnaryNumeric() UnaryNumeric { return UnaryNumeric{result: r} }
func (r *Result) Length() Length             { return Length{result: r} }
func (r *Result) Arithmetic() Arithmetic     { return Arithmetic{result: r} }
func (r *Result) Bitwise() Bitwise           { return Bitwise{result: r} }
func (r *Result) Concat() Concat             { return Concat{result: r} }
func (r *Result) Equality() Equality         { return Equality{result: r} }
func (r *Result) Order() Order               { return Order{result: r} }
func (r *Result) IndexGet() IndexGet         { return IndexGet{result: r} }
func (r *Result) IndexSet() IndexSet         { return IndexSet{result: r} }
func (r *Result) GenericLoop() GenericLoop   { return GenericLoop{result: r} }

func (r *Result) bucketCount(bucket []keyspace.Term, classes []uint8) int {
	if !r.available() || len(bucket) > len(classes) {
		return 0
	}
	return len(bucket)
}

func (r *Result) bucketAt(bucket []keyspace.Term, classes []uint8, family keyspace.Family, class uint8, index int) (keyspace.Term, bool) {
	if index < 0 || index >= r.bucketCount(bucket, classes) {
		return 0, false
	}
	term := bucket[index]
	if !keyspace.ValidTerm(term, family, len(classes)) {
		return 0, false
	}
	return term, classes[keyspace.TermOrdinal(term)-1] == class
}

func (view UnaryNumeric) Count() int {
	if view.result == nil {
		return 0
	}
	return view.result.bucketCount(view.result.buckets.unaryNumeric, view.result.classes.unaryClass)
}
func (view Length) Count() int {
	if view.result == nil {
		return 0
	}
	return view.result.bucketCount(view.result.buckets.length, view.result.classes.unaryClass)
}
func (view Arithmetic) Count() int {
	if view.result == nil {
		return 0
	}
	return view.result.bucketCount(view.result.buckets.arithmetic, view.result.classes.binaryClass)
}
func (view Bitwise) Count() int {
	if view.result == nil {
		return 0
	}
	return view.result.bucketCount(view.result.buckets.bitwise, view.result.classes.binaryClass)
}
func (view Concat) Count() int {
	if view.result == nil {
		return 0
	}
	return view.result.bucketCount(view.result.buckets.concat, view.result.classes.binaryClass)
}
func (view Equality) Count() int {
	if view.result == nil {
		return 0
	}
	return view.result.bucketCount(view.result.buckets.equality, view.result.classes.binaryClass)
}
func (view Order) Count() int {
	if view.result == nil {
		return 0
	}
	return view.result.bucketCount(view.result.buckets.order, view.result.classes.binaryClass)
}
func (view IndexGet) Count() int {
	if view.result == nil {
		return 0
	}
	return view.result.bucketCount(view.result.buckets.indexGet, view.result.classes.readClass)
}
func (view IndexSet) Count() int {
	if view.result == nil {
		return 0
	}
	return view.result.bucketCount(view.result.buckets.indexSet, view.result.classes.writeClass)
}
func (view GenericLoop) Count() int {
	if view.result == nil {
		return 0
	}
	return view.result.bucketCount(view.result.buckets.genericLoop, view.result.classes.loopClass)
}

func (view UnaryNumeric) At(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.bucketAt(view.result.buckets.unaryNumeric, view.result.classes.unaryClass, keyspace.FamilyUnary, unaryNumericCandidate, index)
}
func (view Length) At(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.bucketAt(view.result.buckets.length, view.result.classes.unaryClass, keyspace.FamilyUnary, unaryLengthCandidate, index)
}
func (view Arithmetic) At(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.bucketAt(view.result.buckets.arithmetic, view.result.classes.binaryClass, keyspace.FamilyBinary, binaryArithmeticCandidate, index)
}
func (view Bitwise) At(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.bucketAt(view.result.buckets.bitwise, view.result.classes.binaryClass, keyspace.FamilyBinary, binaryBitwiseCandidate, index)
}
func (view Concat) At(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.bucketAt(view.result.buckets.concat, view.result.classes.binaryClass, keyspace.FamilyBinary, binaryConcatCandidate, index)
}
func (view Equality) At(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.bucketAt(view.result.buckets.equality, view.result.classes.binaryClass, keyspace.FamilyBinary, binaryEqualityCandidate, index)
}
func (view Order) At(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.bucketAt(view.result.buckets.order, view.result.classes.binaryClass, keyspace.FamilyBinary, binaryOrderCandidate, index)
}
func (view IndexGet) At(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.bucketAt(view.result.buckets.indexGet, view.result.classes.readClass, keyspace.FamilyRead, accessIndexCandidate, index)
}
func (view IndexSet) At(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.bucketAt(view.result.buckets.indexSet, view.result.classes.writeClass, keyspace.FamilyWrite, accessIndexCandidate, index)
}
func (view GenericLoop) At(index int) (keyspace.Term, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.result.bucketAt(view.result.buckets.genericLoop, view.result.classes.loopClass, keyspace.FamilyLoop, genericLoopCandidate, index)
}

// Contains is constant time. It checks the private source-family class by the
// existing Term's family ordinal; no candidate slice scan or reverse map is
// involved.
func (view UnaryNumeric) Contains(term keyspace.Term) bool {
	return view.result.unaryContains(term, unaryNumericCandidate)
}
func (view Length) Contains(term keyspace.Term) bool {
	return view.result.unaryContains(term, unaryLengthCandidate)
}
func (view Arithmetic) Contains(term keyspace.Term) bool {
	return view.result.binaryContains(term, binaryArithmeticCandidate)
}
func (view Bitwise) Contains(term keyspace.Term) bool {
	return view.result.binaryContains(term, binaryBitwiseCandidate)
}
func (view Concat) Contains(term keyspace.Term) bool {
	return view.result.binaryContains(term, binaryConcatCandidate)
}
func (view Equality) Contains(term keyspace.Term) bool {
	return view.result.binaryContains(term, binaryEqualityCandidate)
}
func (view Order) Contains(term keyspace.Term) bool {
	return view.result.binaryContains(term, binaryOrderCandidate)
}
func (view IndexGet) Contains(term keyspace.Term) bool {
	return view.result.readContains(term)
}
func (view IndexSet) Contains(term keyspace.Term) bool {
	return view.result.writeContains(term)
}
func (view GenericLoop) Contains(term keyspace.Term) bool {
	return view.result.loopContains(term)
}
