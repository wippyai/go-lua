package candidates

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestCandidateDispositionMatrix(t *testing.T) {
	for _, row := range []struct {
		op    kind.UnaryOp
		class uint8
	}{
		{kind.UnaryNeg, unaryNumericCandidate},
		{kind.UnaryNot, unaryNoCandidate},
		{kind.UnaryLen, unaryLengthCandidate},
		{kind.UnaryBitNot, unaryNumericCandidate},
	} {
		class, err := classifyUnary(row.op)
		if err != nil || class != row.class {
			t.Fatalf("classifyUnary(%d) = %d, %v; want %d, nil", row.op, class, err, row.class)
		}
	}
	for _, op := range []kind.UnaryOp{0, kind.UnaryBitNot + 1, ^kind.UnaryOp(0)} {
		if _, err := classifyUnary(op); err == nil {
			t.Fatalf("classifyUnary(%d) accepted an invalid enum", op)
		}
	}

	for _, row := range []struct {
		op    kind.BinaryOp
		class uint8
	}{
		{kind.BinaryAdd, binaryArithmeticCandidate},
		{kind.BinarySub, binaryArithmeticCandidate},
		{kind.BinaryMul, binaryArithmeticCandidate},
		{kind.BinaryDiv, binaryArithmeticCandidate},
		{kind.BinaryIDiv, binaryArithmeticCandidate},
		{kind.BinaryMod, binaryArithmeticCandidate},
		{kind.BinaryPow, binaryArithmeticCandidate},
		{kind.BinaryConcat, binaryConcatCandidate},
		{kind.BinaryBitAnd, binaryBitwiseCandidate},
		{kind.BinaryBitOr, binaryBitwiseCandidate},
		{kind.BinaryBitXor, binaryBitwiseCandidate},
		{kind.BinaryShiftLeft, binaryBitwiseCandidate},
		{kind.BinaryShiftRight, binaryBitwiseCandidate},
		{kind.BinaryEqual, binaryEqualityCandidate},
		{kind.BinaryNotEqual, binaryEqualityCandidate},
		{kind.BinaryLess, binaryOrderCandidate},
		{kind.BinaryLessEqual, binaryOrderCandidate},
		{kind.BinaryGreater, binaryOrderCandidate},
		{kind.BinaryGreaterEqual, binaryOrderCandidate},
	} {
		class, err := classifyBinary(row.op)
		if err != nil || class != row.class {
			t.Fatalf("classifyBinary(%d) = %d, %v; want %d, nil", row.op, class, err, row.class)
		}
	}
	for _, op := range []kind.BinaryOp{0, kind.BinaryGreaterEqual + 1, ^kind.BinaryOp(0)} {
		if _, err := classifyBinary(op); err == nil {
			t.Fatalf("classifyBinary(%d) accepted an invalid enum", op)
		}
	}

	for _, op := range []kind.SelectOp{kind.SelectAnd, kind.SelectOr} {
		if err := classifySelect(op); err != nil {
			t.Fatalf("classifySelect(%d) = %v", op, err)
		}
	}
	for _, op := range []kind.SelectOp{0, kind.SelectOr + 1, ^kind.SelectOp(0)} {
		if err := classifySelect(op); err == nil {
			t.Fatalf("classifySelect(%d) accepted an invalid enum", op)
		}
	}
}

func TestCandidatePartitionLensAndExclusionLaws(t *testing.T) {
	result := &Result{
		sourceID: candidateValidID(1),
		flowID:   candidateValidID(2),
		staticID: candidateValidID(3),
		moduleID: candidateValidID(4),
		buckets: bucketStore{
			unaryNumeric: []keyspace.Term{term(keyspace.FamilyUnary, 1), term(keyspace.FamilyUnary, 4)},
			length:       []keyspace.Term{term(keyspace.FamilyUnary, 3)},
			arithmetic: []keyspace.Term{
				term(keyspace.FamilyBinary, 1), term(keyspace.FamilyBinary, 2),
				term(keyspace.FamilyBinary, 3), term(keyspace.FamilyBinary, 4),
				term(keyspace.FamilyBinary, 5), term(keyspace.FamilyBinary, 6),
				term(keyspace.FamilyBinary, 7),
			},
			bitwise: []keyspace.Term{
				term(keyspace.FamilyBinary, 9), term(keyspace.FamilyBinary, 10),
			},
			concat:   []keyspace.Term{term(keyspace.FamilyBinary, 8)},
			equality: []keyspace.Term{term(keyspace.FamilyBinary, 14), term(keyspace.FamilyBinary, 15)},
			order: []keyspace.Term{
				term(keyspace.FamilyBinary, 16), term(keyspace.FamilyBinary, 17),
				term(keyspace.FamilyBinary, 18), term(keyspace.FamilyBinary, 19),
			},
			indexGet: []keyspace.Term{term(keyspace.FamilyRead, 1), term(keyspace.FamilyRead, 2)},
			indexSet: []keyspace.Term{term(keyspace.FamilyWrite, 1), term(keyspace.FamilyWrite, 2)},
		},
		classes: classStore{
			unaryClass: []uint8{
				unaryNumericCandidate, unaryNoCandidate, unaryLengthCandidate, unaryNumericCandidate,
			},
			binaryClass: []uint8{
				binaryArithmeticCandidate, binaryArithmeticCandidate, binaryArithmeticCandidate,
				binaryArithmeticCandidate, binaryArithmeticCandidate, binaryArithmeticCandidate,
				binaryArithmeticCandidate,
				binaryConcatCandidate, binaryBitwiseCandidate, binaryBitwiseCandidate,
				binaryBitwiseCandidate, binaryBitwiseCandidate, binaryBitwiseCandidate,
				binaryEqualityCandidate, binaryEqualityCandidate, binaryOrderCandidate,
				binaryOrderCandidate, binaryOrderCandidate, binaryOrderCandidate,
			},
			readClass:  []uint8{accessIndexCandidate, accessIndexCandidate},
			writeClass: []uint8{accessIndexCandidate, accessIndexCandidate},
		},
	}

	checks := []struct {
		name string
		view func() (func() int, func(int) (keyspace.Term, bool), func(keyspace.Term) bool)
		want []keyspace.Term
	}{
		{"UnaryNumeric", func() (func() int, func(int) (keyspace.Term, bool), func(keyspace.Term) bool) {
			v := result.UnaryNumeric()
			return v.Count, v.At, v.Contains
		}, result.buckets.unaryNumeric},
		{"Length", func() (func() int, func(int) (keyspace.Term, bool), func(keyspace.Term) bool) {
			v := result.Length()
			return v.Count, v.At, v.Contains
		}, result.buckets.length},
		{"Arithmetic", func() (func() int, func(int) (keyspace.Term, bool), func(keyspace.Term) bool) {
			v := result.Arithmetic()
			return v.Count, v.At, v.Contains
		}, result.buckets.arithmetic},
		{"Bitwise", func() (func() int, func(int) (keyspace.Term, bool), func(keyspace.Term) bool) {
			v := result.Bitwise()
			return v.Count, v.At, v.Contains
		}, result.buckets.bitwise},
		{"Concat", func() (func() int, func(int) (keyspace.Term, bool), func(keyspace.Term) bool) {
			v := result.Concat()
			return v.Count, v.At, v.Contains
		}, result.buckets.concat},
		{"Equality", func() (func() int, func(int) (keyspace.Term, bool), func(keyspace.Term) bool) {
			v := result.Equality()
			return v.Count, v.At, v.Contains
		}, result.buckets.equality},
		{"Order", func() (func() int, func(int) (keyspace.Term, bool), func(keyspace.Term) bool) {
			v := result.Order()
			return v.Count, v.At, v.Contains
		}, result.buckets.order},
		{"IndexGet", func() (func() int, func(int) (keyspace.Term, bool), func(keyspace.Term) bool) {
			v := result.IndexGet()
			return v.Count, v.At, v.Contains
		}, result.buckets.indexGet},
		{"IndexSet", func() (func() int, func(int) (keyspace.Term, bool), func(keyspace.Term) bool) {
			v := result.IndexSet()
			return v.Count, v.At, v.Contains
		}, result.buckets.indexSet},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			count, at, contains := check.view()
			if count() != len(check.want) {
				t.Fatalf("Count() = %d, want %d", count(), len(check.want))
			}
			for index, want := range check.want {
				got, ok := at(index)
				if !ok || got != want || !contains(want) {
					t.Fatalf("candidate[%d] = %08x/%v, Contains = %v; want %08x/true", index, uint32(got), ok, contains(want), uint32(want))
				}
				if index > 0 && check.want[index-1] >= want {
					t.Fatalf("candidate order is not strictly increasing at %d", index)
				}
			}
			if _, ok := at(-1); ok {
				t.Fatal("At(-1) accepted")
			}
			if _, ok := at(count()); ok {
				t.Fatal("At(Count()) accepted")
			}
			for _, foreign := range []keyspace.Term{
				0,
				term(keyspace.FamilyCell, 1),
				term(keyspace.FamilyTableField, 1),
				term(keyspace.FamilySelect, 1),
				term(keyspace.FamilyCount, 1),
			} {
				if contains(foreign) {
					t.Fatalf("Contains(%08x) accepted foreign/excluded Term", uint32(foreign))
				}
			}
		})
	}

	// A Cell and a TableField are deliberately never index candidates, while
	// the two read/write rows stand for exact and dynamic lens sources/targets.
	if result.IndexGet().Contains(term(keyspace.FamilyRead, 1)) != result.IndexGet().Contains(term(keyspace.FamilyRead, 2)) {
		t.Fatal("exact and dynamic Lens reads did not share the IndexGet disposition")
	}
	if result.IndexSet().Contains(term(keyspace.FamilyWrite, 1)) != result.IndexSet().Contains(term(keyspace.FamilyWrite, 2)) {
		t.Fatal("exact and dynamic Lens writes did not share the IndexSet disposition")
	}
	if result.IndexGet().Contains(term(keyspace.FamilyRead, 3)) || result.IndexSet().Contains(term(keyspace.FamilyWrite, 3)) {
		t.Fatal("static/dead read or write entered a bucket")
	}
}

func TestCandidatePermutationAndCapacityLaws(t *testing.T) {
	left := &Result{
		sourceID: candidateValidID(1),
		flowID:   candidateValidID(2),
		staticID: candidateValidID(3),
		moduleID: candidateValidID(4),
		buckets:  bucketStore{unaryNumeric: []keyspace.Term{term(keyspace.FamilyUnary, 1)}},
		classes:  classStore{unaryClass: []uint8{unaryNumericCandidate}},
	}
	right := &Result{
		sourceID: candidateValidID(1),
		flowID:   candidateValidID(2),
		staticID: candidateValidID(3),
		moduleID: candidateValidID(4),
		buckets:  bucketStore{unaryNumeric: append([]keyspace.Term(nil), left.buckets.unaryNumeric...)},
		classes:  classStore{unaryClass: append([]uint8(nil), left.classes.unaryClass...)},
	}
	if left.UnaryNumeric().Count() != right.UnaryNumeric().Count() ||
		left.UnaryNumeric().Contains(term(keyspace.FamilyUnary, 1)) != right.UnaryNumeric().Contains(term(keyspace.FamilyUnary, 1)) {
		t.Fatal("permutation/reseal changed the typed membership result")
	}

	const members = 10_000
	scaled := Result{sourceID: candidateValidID(1), flowID: candidateValidID(2), staticID: candidateValidID(3), moduleID: candidateValidID(4)}
	for ordinal := 1; ordinal <= members; ordinal++ {
		scaled.buckets.arithmetic = append(scaled.buckets.arithmetic, term(keyspace.FamilyBinary, uint32(ordinal)))
	}
	if len(scaled.buckets.arithmetic) != members || cap(scaled.buckets.arithmetic) > members*2 {
		t.Fatalf("arithmetic retained capacity %d for %d members", cap(scaled.buckets.arithmetic), members)
	}
	if cap(scaled.buckets.unaryNumeric)+cap(scaled.buckets.length)+cap(scaled.buckets.arithmetic)+cap(scaled.buckets.bitwise)+cap(scaled.buckets.concat)+cap(scaled.buckets.equality)+cap(scaled.buckets.order) > members*2 {
		t.Fatal("mutually exclusive candidate buckets retained overcapacity")
	}
}

func TestCandidateNegativeExactKeyStaticIsExcluded(t *testing.T) {
	// UnaryNeg is a numeric candidate when it is executable. The same
	// authored Unary used only to spell a negative exact key is static, so its
	// zero membership code must not leak into UnaryNumeric.
	static := &Result{
		sourceID: candidateValidID(1),
		flowID:   candidateValidID(2),
		staticID: candidateValidID(3),
		moduleID: candidateValidID(4),
		classes:  classStore{unaryClass: []uint8{unaryNoCandidate}},
	}
	if static.UnaryNumeric().Contains(term(keyspace.FamilyUnary, 1)) {
		t.Fatal("negative exact-key static Unary entered UnaryNumeric")
	}
}

func TestCandidateQueriesAreAllocationFree(t *testing.T) {
	result := &Result{
		sourceID: candidateValidID(1),
		flowID:   candidateValidID(2),
		staticID: candidateValidID(3),
		moduleID: candidateValidID(4),
		buckets:  bucketStore{unaryNumeric: []keyspace.Term{term(keyspace.FamilyUnary, 1)}},
		classes:  classStore{unaryClass: []uint8{unaryNumericCandidate}},
	}
	term := term(keyspace.FamilyUnary, 1)
	view := result.UnaryNumeric()
	if allocations := testing.AllocsPerRun(1000, func() {
		if view.Count() != 1 || !view.Contains(term) {
			t.Fatal("stable typed query returned an incorrect result")
		}
		_, _ = view.At(0)
	}); allocations != 0 {
		t.Fatalf("typed candidate query allocated %v objects per run", allocations)
	}
}

func TestCandidateQueriesFailClosedForUnavailableAndMalformedStorage(t *testing.T) {
	newProjection := func(sourceID, flowID identity.ContentID) *Result {
		return &Result{
			sourceID: sourceID,
			flowID:   flowID,
			staticID: candidateValidID(3),
			moduleID: candidateValidID(4),
			buckets: bucketStore{
				unaryNumeric: []keyspace.Term{term(keyspace.FamilyUnary, 1)},
				length:       []keyspace.Term{term(keyspace.FamilyUnary, 2)},
				arithmetic:   []keyspace.Term{term(keyspace.FamilyBinary, 1)},
				bitwise:      []keyspace.Term{term(keyspace.FamilyBinary, 2)},
				concat:       []keyspace.Term{term(keyspace.FamilyBinary, 3)},
				equality:     []keyspace.Term{term(keyspace.FamilyBinary, 4)},
				order:        []keyspace.Term{term(keyspace.FamilyBinary, 5)},
				indexGet:     []keyspace.Term{term(keyspace.FamilyRead, 1)},
				indexSet:     []keyspace.Term{term(keyspace.FamilyWrite, 1)},
				genericLoop:  []keyspace.Term{term(keyspace.FamilyLoop, 1)},
			},
			classes: classStore{
				unaryClass:  []uint8{unaryNumericCandidate, unaryLengthCandidate},
				binaryClass: []uint8{binaryArithmeticCandidate, binaryBitwiseCandidate, binaryConcatCandidate, binaryEqualityCandidate, binaryOrderCandidate},
				readClass:   []uint8{accessIndexCandidate},
				writeClass:  []uint8{accessIndexCandidate},
				loopClass:   []uint8{genericLoopCandidate},
			},
		}
	}

	type query struct {
		name     string
		count    func() int
		at       func(int) (keyspace.Term, bool)
		contains func(keyspace.Term) bool
	}
	queries := func(result *Result) []query {
		unaryNumeric, length := result.UnaryNumeric(), result.Length()
		arithmetic, bitwise := result.Arithmetic(), result.Bitwise()
		concat, equality, order := result.Concat(), result.Equality(), result.Order()
		indexGet, indexSet, genericLoop := result.IndexGet(), result.IndexSet(), result.GenericLoop()
		return []query{
			{"UnaryNumeric", unaryNumeric.Count, unaryNumeric.At, unaryNumeric.Contains},
			{"Length", length.Count, length.At, length.Contains},
			{"Arithmetic", arithmetic.Count, arithmetic.At, arithmetic.Contains},
			{"Bitwise", bitwise.Count, bitwise.At, bitwise.Contains},
			{"Concat", concat.Count, concat.At, concat.Contains},
			{"Equality", equality.Count, equality.At, equality.Contains},
			{"Order", order.Count, order.At, order.Contains},
			{"IndexGet", indexGet.Count, indexGet.At, indexGet.Contains},
			{"IndexSet", indexSet.Count, indexSet.At, indexSet.Contains},
			{"GenericLoop", genericLoop.Count, genericLoop.At, genericLoop.Contains},
		}
	}

	for _, owner := range []struct {
		name   string
		result *Result
	}{
		{name: "nil", result: nil},
		{name: "zero", result: newProjection(identity.ContentID{}, identity.ContentID{})},
		{name: "zero-source", result: newProjection(identity.ContentID{}, candidateValidID(2))},
		{name: "zero-flow", result: newProjection(candidateValidID(1), identity.ContentID{})},
	} {
		t.Run(owner.name, func(t *testing.T) {
			for _, query := range queries(owner.result) {
				t.Run(query.name, func(t *testing.T) {
					if query.count() != 0 {
						t.Fatalf("Count() = %d for unavailable Result", query.count())
					}
					if _, ok := query.at(0); ok {
						t.Fatal("At(0) accepted unavailable Result")
					}
					if query.contains(term(keyspace.FamilyUnary, 1)) {
						t.Fatal("Contains accepted unavailable Result")
					}
				})
			}
		})
	}

	malformed := newProjection(candidateValidID(1), candidateValidID(2))
	// A bucket row without a corresponding dense class row is not a lawful
	// internal projection row and must not become query-visible.
	malformed.classes = classStore{}
	for _, query := range queries(malformed) {
		t.Run("malformed-"+query.name, func(t *testing.T) {
			if query.count() != 0 {
				t.Fatalf("Count() = %d for malformed row bounds", query.count())
			}
			if _, ok := query.at(0); ok {
				t.Fatal("At(0) accepted malformed row bounds")
			}
		})
	}

	valid := newProjection(candidateValidID(1), candidateValidID(2))
	for _, query := range queries(valid) {
		if query.contains(0) || query.contains(keyspace.MakeTerm(keyspace.FamilyLoop, keyspace.MaxTermOrdinal)) {
			t.Fatalf("%s.Contains accepted a malformed/out-of-range Term", query.name)
		}
	}
}

func TestCandidateSealRejectsZeroForeignAndExpiredAuthority(t *testing.T) {
	if _, err := Seal(source.Identity{}, authored.View{}, nil, identity.ContentID{}, identity.ContentID{}); err == nil {
		t.Fatal("zero identity/proof accepted")
	}

	sourceIdentity, view, finish := minimalOwners(t)
	defer finish()
	if _, err := Seal(sourceIdentity, view, nil, identity.ContentID{}, identity.ContentID{}); err == nil {
		t.Fatal("nil proof accepted for a valid identity/view")
	}
	if _, err := Seal(sourceIdentity, view, &executable.Result{}, identity.ContentID{}, identity.ContentID{}); err == nil {
		t.Fatal("foreign zero proof accepted")
	}

	// The identity captured from the preimage is expired by Commit and must
	// fail closed even when the caller still holds the old value.
	expired, expiredView, expiredFinish := minimalOwnersFromPreimage(t)
	expiredFinish()
	if _, err := Seal(expired, expiredView, nil, identity.ContentID{}, identity.ContentID{}); err == nil {
		t.Fatal("expired identity accepted")
	}
}

func term(family keyspace.Family, ordinal uint32) keyspace.Term {
	return keyspace.MakeTerm(family, ordinal)
}

func candidateValidID(seed byte) identity.ContentID {
	var id identity.ContentID
	id[0] = seed
	return id
}

func minimalOwners(t *testing.T) (source.Identity, authored.View, func()) {
	t.Helper()
	sourceDraft, err := source.Build(minimalSourceInput())
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalizer, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	sourceID := sourceFinalizer.Preimage().Identity().ContentID()
	sourceComponent, err := sourceFinalizer.Commit(source.IndexInput{
		SourceID: sourceID,
		Bodies:   []source.BodyRoots{{Body: term(keyspace.FamilyBody, 1)}},
		Entry:    term(keyspace.FamilyBody, 1),
	})
	if err != nil {
		t.Fatalf("source.Commit: %v", err)
	}
	flowDraft, err := authored.Build(authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1,
	}})
	if err != nil {
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalizer, err := flowDraft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView, err := flowFinalizer.Commit()
	if err != nil {
		t.Fatalf("authored.Commit: %v", err)
	}
	return sourceComponent.View().Identity(), flowView, func() {}
}

func minimalOwnersFromPreimage(t *testing.T) (source.Identity, authored.View, func()) {
	t.Helper()
	sourceDraft, err := source.Build(minimalSourceInput())
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalizer, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	preimage := sourceFinalizer.Preimage()
	identity := preimage.Identity()
	if _, err := sourceFinalizer.Commit(source.IndexInput{
		SourceID: identity.ContentID(),
		Bodies:   []source.BodyRoots{{Body: term(keyspace.FamilyBody, 1)}},
		Entry:    term(keyspace.FamilyBody, 1),
	}); err != nil {
		t.Fatalf("source.Commit: %v", err)
	}
	flowDraft, err := authored.Build(authored.Input{Counts: [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1,
	}})
	if err != nil {
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalizer, err := flowDraft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	flowView, err := flowFinalizer.Commit()
	if err != nil {
		t.Fatalf("authored.Commit: %v", err)
	}
	return identity, flowView, func() {}
}

func minimalSourceInput() source.Input {
	input := source.Input{Name: "candidates.lua"}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		input.Families = append(input.Families, source.FamilySpans{Family: family})
	}
	input.Families[keyspace.FamilyBody-1].Spans = []source.Span{{
		File: input.Name, StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1,
	}}
	input.Bodies = []source.BodySource{{Body: term(keyspace.FamilyBody, 1)}}
	return input
}
