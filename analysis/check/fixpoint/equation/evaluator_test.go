package equation

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
)

func stage3Artifact(t *testing.T) (Artifact, EntryBinding, []ContentID) {
	t.Helper()
	body := testBody(31)
	entry := EntryParameter{Body: body, Name: "entry"}
	contracts := []ContentID{testID(41), testID(42), testID(43)}
	artifact := Artifact{Equations: []Equation{
		{Target: Coordinate{Body: body, Name: "identity"}, Entry: entry, Occurrence: Occurrence{Kind: "entry", ContractID: contracts[0]}, KernelID: "canonical/identity", Operands: []Operand{{Role: "entry", Term: EntryTerm(entry)}}},
		{Target: Coordinate{Body: body, Name: "guarded-return"}, Entry: entry, Guards: []Guard{{Body: body, Encoding: []byte("not-nil")}}, Occurrence: Occurrence{Kind: "outcome", ContractID: contracts[1]}, KernelID: "canonical/guarded-return", Operands: []Operand{{Role: "flow", Term: ClosedTerm([]byte("identity"))}}},
		{Target: Coordinate{Body: body, Name: "copied-store"}, Entry: entry, Dependencies: []Coordinate{{Body: body, Name: "identity"}}, Occurrence: Occurrence{Kind: "environment-write", ContractID: contracts[2]}, KernelID: "canonical/copied-store", Operands: []Operand{{Role: "store", Term: ClosedTerm([]byte("source"))}}},
	}}
	if artifact.CanonicalBytes() == nil {
		t.Fatal("stage-3 fixture artifact is invalid")
	}
	return artifact, EntryBinding{Parameter: entry, Value: []byte("caller-entry")}, contracts
}

func stage3VM(t *testing.T, contracts []ContentID) *AcyclicVM {
	t.Helper()
	registry, err := NewKernelRegistry([]KernelBinding{
		{KernelID: "canonical/identity", ContractID: contracts[0], Kernel: KernelFunc(func(equation BoundEquation, _ Partition) (TransactionResult, error) {
			if len(equation.Operands) != 1 || !bytes.Equal(equation.Operands[0].Value, []byte("caller-entry")) {
				return TransactionResult{}, fmt.Errorf("entry was not closed")
			}
			return TransactionResult{Complete: true, Closure: OutputClosure{Values: []Fact{{Key: "identity", Value: equation.Operands[0].Value}}}}, nil
		})},
		{KernelID: "canonical/guarded-return", ContractID: contracts[1], Kernel: KernelFunc(func(equation BoundEquation, _ Partition) (TransactionResult, error) {
			if len(equation.Guards) != 1 || string(equation.Guards[0].Encoding) != "not-nil" {
				return TransactionResult{}, fmt.Errorf("guard was not retained")
			}
			return TransactionResult{Complete: true, Closure: OutputClosure{Outcomes: []Fact{{Key: "return", Value: []byte("normal")}}, Diagnostics: []Fact{{Key: "guard-witness", Value: []byte("not-nil")}}}}, nil
		})},
		{KernelID: "canonical/copied-store", ContractID: contracts[2], Kernel: KernelFunc(func(_ BoundEquation, partition Partition) (TransactionResult, error) {
			values := partition.Values()
			if len(values) != 1 || values[0].Key != "identity" || !bytes.Equal(values[0].Value, []byte("caller-entry")) {
				return TransactionResult{}, fmt.Errorf("completed partition was not supplied")
			}
			return TransactionResult{Complete: true, Closure: OutputClosure{Values: []Fact{{Key: "copied-store", Value: values[0].Value}}, AllocationRekeys: []AllocationRekey{{From: "formal:table", To: "caller:table"}}}}, nil
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	vm, err := NewAcyclicVM(registry)
	if err != nil {
		t.Fatal(err)
	}
	return vm
}

func TestAcyclicBoundEvaluatorIdentityWitness(t *testing.T) {
	artifact, entry, contracts := stage3Artifact(t)
	bound, err := BindEntry(artifact, entry)
	if err != nil {
		t.Fatal(err)
	}
	evaluation, err := stage3VM(t, contracts).Evaluate(bound)
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.Transactions != 3 || len(evaluation.Closure.Values) != 2 || string(evaluation.Closure.Values[0].Value) != "caller-entry" {
		t.Fatalf("identity witness closure = %#v", evaluation)
	}
}

func TestAcyclicBoundEvaluatorGuardedReturnWitness(t *testing.T) {
	artifact, entry, contracts := stage3Artifact(t)
	bound, err := BindEntry(artifact, entry)
	if err != nil {
		t.Fatal(err)
	}
	evaluation, err := stage3VM(t, contracts).Evaluate(bound)
	if err != nil {
		t.Fatal(err)
	}
	if len(evaluation.Closure.Outcomes) != 1 || evaluation.Closure.Outcomes[0].Key != "return" || !bytes.Equal(evaluation.Closure.Outcomes[0].Value, []byte("normal")) ||
		len(evaluation.Closure.Diagnostics) != 1 || evaluation.Closure.Diagnostics[0].Key != "guard-witness" {
		t.Fatalf("guarded-return witness closure = %#v", evaluation.Closure)
	}
}

func TestAcyclicBoundEvaluatorCopiedStoreWitness(t *testing.T) {
	artifact, entry, contracts := stage3Artifact(t)
	bound, err := BindEntry(artifact, entry)
	if err != nil {
		t.Fatal(err)
	}
	evaluation, err := stage3VM(t, contracts).Evaluate(bound)
	if err != nil {
		t.Fatal(err)
	}
	want := OutputClosure{
		Values: []Fact{
			{Key: "copied-store", Value: []byte("caller-entry")},
			{Key: "identity", Value: []byte("caller-entry")},
		},
		Outcomes:         []Fact{{Key: "return", Value: []byte("normal"), Guards: []Guard{{Body: entry.Parameter.Body, Encoding: []byte("not-nil")}}}},
		Diagnostics:      []Fact{{Key: "guard-witness", Value: []byte("not-nil"), Guards: []Guard{{Body: entry.Parameter.Body, Encoding: []byte("not-nil")}}}},
		AllocationRekeys: []AllocationRekey{{From: "formal:table", To: "caller:table"}},
	}
	if !reflect.DeepEqual(evaluation.Closure, want) {
		t.Fatalf("copied-store published closure = %#v, want %#v", evaluation.Closure, want)
	}
}

func TestPartitionFromClosuresMatchesSequentialClosedJoin(t *testing.T) {
	body := testBody(51)
	guard := Guard{Body: body, Encoding: []byte("branch")}
	closures := []OutputClosure{
		{Values: []Fact{{Key: "seed", Value: []byte("one")}}, Diagnostics: []Fact{{Key: "note", Value: []byte("seen"), Guards: []Guard{guard}}}},
		{Values: []Fact{{Key: "next", Value: []byte("two")}, {Key: "seed", Value: []byte("one")}}, Outcomes: []Fact{{Key: "return", Value: []byte("ok")}}, AllocationRekeys: []AllocationRekey{{From: "formal", To: "actual"}}},
	}
	want := OutputClosure{}
	for _, closure := range closures {
		var err error
		want, err = joinClosure(want, closure)
		if err != nil {
			t.Fatalf("sequential closed join: %v", err)
		}
	}
	partition, err := PartitionFromClosuresWithGuards(nil, closures...)
	if err != nil {
		t.Fatalf("aggregate closed join: %v", err)
	}
	if !want.Equal(partition.closure) {
		t.Fatalf("aggregate closure = %#v, want sequential %#v", partition.closure, want)
	}
}

func TestPartitionPointLookupsKeepGuardedFactsPrivate(t *testing.T) {
	body := testBody(52)
	visible := Guard{Body: body, Encoding: []byte("visible")}
	hidden := Guard{Body: body, Encoding: []byte("hidden")}
	partition := newPartition(OutputClosure{Values: []Fact{
		{Key: "epoch/path/item/0001", Value: []byte("old"), Guards: []Guard{visible}},
		{Key: "epoch/path/item/0002", Value: []byte("current"), Guards: []Guard{visible}},
		{Key: "epoch/path/item/9999", Value: []byte("hidden"), Guards: []Guard{hidden}},
	}}, []Guard{visible})

	latest, ok := partition.LatestValuePrefix("epoch/path/item/")
	if !ok || latest.Key != "epoch/path/item/0002" || string(latest.Value) != "current" {
		t.Fatalf("latest visible point lookup = %#v, %v", latest, ok)
	}
	value, ok := partition.Value("epoch/path/item/0002")
	if !ok || string(value.Value) != "current" {
		t.Fatalf("point lookup = %#v, %v, want the current publication", value, ok)
	}
	if _, ok := partition.Value("epoch/path/item/9999"); ok {
		t.Fatal("point lookup exposed a fact outside the active guards")
	}
	if _, ok := partition.LatestValuePrefix("epoch/path/missing/"); ok {
		t.Fatal("latest point lookup answered a prefix that publishes nothing")
	}
}

// TestPartitionReadsShareSealedSnapshotRows pins the read contract: a partition
// presents its published rows rather than restating them, and what it presents
// is sealed.  A consumer that appends to a returned lane, or to a payload it
// holds, allocates instead of writing into the snapshot the next consumer reads.
func TestPartitionReadsShareSealedSnapshotRows(t *testing.T) {
	body := testBody(54)
	visible := Guard{Body: body, Encoding: []byte("visible")}
	hidden := Guard{Body: body, Encoding: []byte("hidden")}
	closure, err := joinClosure(OutputClosure{Values: []Fact{
		{Key: "value/a/0001", Value: []byte("first"), Guards: []Guard{visible}},
		{Key: "value/a/0002", Value: []byte("second"), Guards: []Guard{visible}},
		{Key: "value/a/9999", Value: []byte("guarded"), Guards: []Guard{hidden}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	partition := newPartition(closure, []Guard{visible})

	values := partition.Values()
	if len(values) != 2 {
		t.Fatalf("visible values = %#v, want the two rows the active cube admits", values)
	}
	if &values[0] != &partition.Values()[0] {
		t.Fatal("two reads of one partition built two lanes")
	}
	for _, lane := range [][]Fact{values, partition.AllValues(), partition.ValuesPrefix("value/a/000")} {
		if cap(lane) != len(lane) {
			t.Fatalf("returned lane capacity %d exceeds its length %d, so an append could write into the snapshot", cap(lane), len(lane))
		}
	}
	extended := append(partition.ValuesPrefix("value/a/0001"), Fact{Key: "value/a/0003", Value: []byte("appended")})
	if len(extended) != 2 || len(partition.Values()) != 2 || partition.Values()[1].Key != "value/a/0002" {
		t.Fatalf("appending to a prefix read disturbed the snapshot: %#v", partition.Values())
	}
	payload := append(values[0].Value, '!')
	if string(payload) != "first!" || string(partition.Values()[0].Value) != "first" {
		t.Fatalf("appending to a published payload disturbed the snapshot: %q", partition.Values()[0].Value)
	}
}

func TestPartitionValuesPrefixMatchesFilteredVisibleValues(t *testing.T) {
	body := testBody(53)
	visible := Guard{Body: body, Encoding: []byte("visible")}
	hidden := Guard{Body: body, Encoding: []byte("hidden")}
	values := []Fact{
		{Key: "heap/member/a/0001", Value: []byte("first"), Guards: []Guard{visible}},
		{Key: "heap/member/a/0002", Value: []byte("second"), Guards: []Guard{visible}},
		{Key: "heap/member/a/9999", Value: []byte("guarded"), Guards: []Guard{hidden}},
		{Key: "heap/member/b/0001", Value: []byte("other"), Guards: []Guard{visible}},
	}
	// The same lane is read in published order and in an order no canonical
	// merge produces: an ordered lane answers a prefix by its bounds, an
	// arbitrary one by scanning, and the two must agree fact for fact.
	shuffled := []Fact{values[3], values[1], values[2], values[0]}
	for name, lane := range map[string][]Fact{"ordered": values, "unordered": shuffled} {
		partition := newPartition(OutputClosure{Values: lane}, []Guard{visible})
		got := partition.ValuesPrefix("heap/member/a/")
		want := make([]Fact, 0, 2)
		for _, fact := range partition.Values() {
			if strings.HasPrefix(fact.Key, "heap/member/a/") {
				want = append(want, fact)
			}
		}
		if len(got) != len(want) {
			t.Fatalf("%s prefix scan = %#v, want the same facts as a filtered full scan %#v", name, got, want)
		}
		for index := range got {
			if got[index].Key != want[index].Key || string(got[index].Value) != string(want[index].Value) {
				t.Fatalf("%s prefix scan item %d = %#v, want %#v", name, index, got[index], want[index])
			}
		}
		latest, ok := partition.LatestValuePrefix("heap/member/a/")
		if !ok || latest.Key != "heap/member/a/0002" {
			t.Fatalf("%s latest under prefix = %#v, %v", name, latest, ok)
		}
		if fact, ok := partition.Value("heap/member/a/0001"); !ok || string(fact.Value) != "first" {
			t.Fatalf("%s point lookup = %#v, %v", name, fact, ok)
		}
		if _, ok := partition.Value("heap/member/a/9999"); ok {
			t.Fatalf("%s point lookup exposed a fact outside the active guards", name)
		}
	}
}

func TestPartitionFamilyValuesParsesSelectedRowsWithoutAllocation(t *testing.T) {
	identity := []byte("heap")
	prefix := factkey.BuildKey(factkey.HeapMember, []factkey.Part{factkey.IdentityPart(identity)}, "")
	first := factkey.BuildKey(factkey.HeapMember, []factkey.Part{
		factkey.IdentityPart(identity), factkey.EncodedOpaquePart(".a"),
	}, "op-1")
	second := factkey.BuildKey(factkey.HeapMember, []factkey.Part{
		factkey.IdentityPart(identity), factkey.EncodedOpaquePart(".b"),
	}, "op-2")
	other := factkey.BuildKey(factkey.HeapTableClosed, []factkey.Part{factkey.IdentityPart(identity)}, "op-3")
	partition, err := PartitionFromClosuresWithGuards(nil, OutputClosure{Values: []Fact{
		{Key: first.String(), Value: []byte("one")},
		{Key: second.String(), Value: []byte("two")},
		{Key: other.String(), Value: []byte("closed")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	read := func() {
		iterator := partition.FamilyValues(prefix)
		count := 0
		for {
			value, ok := iterator.Next()
			if !ok {
				break
			}
			qualifier, present := value.Qualifier(0)
			if !present || qualifier.Kind() != factkey.EncodedOpaque || value.Occurrence == "" || len(value.Payload) == 0 {
				t.Fatalf("malformed family value: %+v", value)
			}
			count++
		}
		if count != 2 {
			t.Fatalf("family values = %d, want 2", count)
		}
	}
	read() // Warm the partition view before measuring the read itself.
	if allocations := testing.AllocsPerRun(100, read); allocations != 0 {
		t.Fatalf("FamilyValues allocated %v times per read", allocations)
	}
}

func TestAcyclicBoundEvaluatorDoesNotPublishPartialTransaction(t *testing.T) {
	artifact, entry, contracts := stage3Artifact(t)
	registry, err := NewKernelRegistry([]KernelBinding{
		{KernelID: "canonical/identity", ContractID: contracts[0], Kernel: KernelFunc(func(BoundEquation, Partition) (TransactionResult, error) {
			return TransactionResult{Complete: true, Closure: OutputClosure{Values: []Fact{{Key: "would-leak", Value: []byte("x")}}}}, nil
		})},
		{KernelID: "canonical/guarded-return", ContractID: contracts[1], Kernel: KernelFunc(func(BoundEquation, Partition) (TransactionResult, error) {
			return TransactionResult{Complete: false, Closure: OutputClosure{Outcomes: []Fact{{Key: "must-not-publish", Value: []byte("x")}}}}, nil
		})},
		{KernelID: "canonical/copied-store", ContractID: contracts[2], Kernel: KernelFunc(func(BoundEquation, Partition) (TransactionResult, error) {
			return TransactionResult{Complete: true}, nil
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	vm, err := NewAcyclicVM(registry)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := BindEntry(artifact, entry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vm.Evaluate(bound); !errors.Is(err, ErrIncompleteTransaction) {
		t.Fatalf("partial transaction error = %v", err)
	}
}

func TestAcyclicBoundEvaluatorRejectsMissingContractBoundKernel(t *testing.T) {
	artifact, entry, contracts := stage3Artifact(t)
	registry, err := NewKernelRegistry([]KernelBinding{{KernelID: "canonical/identity", ContractID: contracts[0], Kernel: KernelFunc(func(BoundEquation, Partition) (TransactionResult, error) {
		return TransactionResult{Complete: true}, nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	vm, err := NewAcyclicVM(registry)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := BindEntry(artifact, entry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vm.Evaluate(bound); err == nil {
		t.Fatal("missing contract-bound kernel was accepted")
	}
}

func TestAcyclicBoundEvaluatorRejectsCyclicArtifact(t *testing.T) {
	artifact, entry, contracts := stage3Artifact(t)
	artifact.Equations[0].Dependencies = []Coordinate{{Body: entry.Parameter.Body, Name: "guarded-return"}}
	artifact.Equations[1].Dependencies = []Coordinate{{Body: entry.Parameter.Body, Name: "identity"}}
	bound, err := BindEntry(artifact, entry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stage3VM(t, contracts).Evaluate(bound); err == nil {
		t.Fatal("cyclic artifact was accepted")
	}
}

func TestAcyclicBoundEvaluatorShadowDifferentialCorpus(t *testing.T) {
	artifact, entry, contracts := stage3Artifact(t)
	production := OutputClosure{
		Values:           []Fact{{Key: "identity", Value: []byte("caller-entry")}, {Key: "copied-store", Value: []byte("caller-entry")}},
		Outcomes:         []Fact{{Key: "return", Value: []byte("normal"), Guards: []Guard{{Body: entry.Parameter.Body, Encoding: []byte("not-nil")}}}},
		Diagnostics:      []Fact{{Key: "guard-witness", Value: []byte("not-nil"), Guards: []Guard{{Body: entry.Parameter.Body, Encoding: []byte("not-nil")}}}},
		AllocationRekeys: []AllocationRekey{{From: "formal:table", To: "caller:table"}},
	}
	cases := []ShadowCase{
		{Name: "identity", Artifact: artifact, Entry: entry, Production: func() (OutputClosure, error) { return production, nil }},
		{Name: "guarded-return", Artifact: artifact, Entry: entry, Production: func() (OutputClosure, error) { return production, nil }},
		{Name: "copied-store", Artifact: artifact, Entry: entry, Production: func() (OutputClosure, error) { return production, nil }},
	}
	for _, shadow := range cases {
		t.Run(shadow.Name, func(t *testing.T) {
			want, err := shadow.Production()
			if err != nil {
				t.Fatalf("production: %v", err)
			}
			bound, err := BindEntry(shadow.Artifact, shadow.Entry)
			if err != nil {
				t.Fatalf("BindEntry: %v", err)
			}
			evaluation, err := stage3VM(t, contracts).Evaluate(bound)
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			if !want.Equal(evaluation.Closure) {
				t.Fatalf("published closure = %#v, want %#v", evaluation.Closure, want)
			}
		})
	}
	report, err := RunShadow(stage3VM(t, contracts), cases)
	if err != nil {
		t.Fatal(err)
	}
	if report.Cases != len(cases) || report.Passed != len(cases) {
		t.Fatalf("shadow report = %#v, want every per-case output comparison to pass", report)
	}
}

func TestAcyclicBoundEvaluatorShadowRejectsPublishedDifference(t *testing.T) {
	artifact, entry, contracts := stage3Artifact(t)
	_, err := RunShadow(stage3VM(t, contracts), []ShadowCase{{
		Name: "published-difference", Artifact: artifact, Entry: entry,
		Production: func() (OutputClosure, error) {
			return OutputClosure{Values: []Fact{{Key: "identity", Value: []byte("wrong-entry")}}}, nil
		},
	}})
	if err == nil {
		t.Fatal("shadow accepted unequal published output")
	}
}

// guardOrderCases spells the cube shapes a canonical sort has to separate:
// differing bodies, one encoding extending another, encodings carrying the zero
// byte that terminates them in a key, and cubes of different length.
func guardOrderCases() [][]Guard {
	first, second := testBody(11), testBody(12)
	encodings := [][]byte{
		[]byte("front/branch/op-1/true"),
		[]byte("front/branch/op-1/false"),
		[]byte("front/branch/op-1"),
		[]byte("front/branch/op-10/true"),
		{'a'},
		{'a', 0},
		{'a', 0, 'b'},
		{'a', 'b'},
		{0},
	}
	sets := [][]Guard{nil}
	for _, body := range []BodyID{first, second} {
		for _, encoding := range encodings {
			single := []Guard{{Body: body, Encoding: encoding}}
			sets = append(sets, single)
			for _, tail := range encodings {
				sets = append(sets, []Guard{{Body: body, Encoding: encoding}, {Body: second, Encoding: tail}})
			}
		}
	}
	return sets
}

func TestGuardsCompareMatchesGuardsKeyOrder(t *testing.T) {
	sets := guardOrderCases()
	for _, left := range sets {
		for _, right := range sets {
			want := strings.Compare(guardsKey(left), guardsKey(right))
			if got := guardsCompare(left, right); got != want {
				t.Fatalf("guardsCompare(%q, %q) = %d, want %d", guardsKey(left), guardsKey(right), got, want)
			}
		}
	}
}

// TestCanonicalGuardsOwnsItsResult pins the ownership rule a compiled
// evaluation depends on: an operation binds its guards in worker scratch that
// the next operation rebinds and release zeroes, so a canonical cube must never
// point back at the storage it was read from -- including when that storage
// already held the canonical form.
func TestCanonicalGuardsOwnsItsResult(t *testing.T) {
	body := testBody(13)
	high := Guard{Body: body, Encoding: []byte("z")}
	low := Guard{Body: body, Encoding: []byte("a")}
	canonical := canonicalGuards([]Guard{high, low, high})
	if len(canonical) != 2 || string(canonical[0].Encoding) != "a" || string(canonical[1].Encoding) != "z" {
		t.Fatalf("canonicalGuards = %v, want the sorted duplicate-free cube", canonical)
	}
	scratch := []Guard{low, high}
	var sets guardSets
	cubes := [][]Guard{canonical, canonicalGuards(scratch), sets.canonical(scratch)}
	for index := range scratch {
		scratch[index] = Guard{}
	}
	for _, cube := range cubes {
		if len(cube) != 2 || string(cube[0].Encoding) != "a" || string(cube[1].Encoding) != "z" {
			t.Fatalf("cube %v aliases the scratch it was canonicalized from", cube)
		}
		if cap(cube) != len(cube) {
			t.Fatalf("cube capacity %d exceeds its length %d, so an append could write into shared state", cap(cube), len(cube))
		}
	}
}

func TestGuardSetsInternSharesEqualCubes(t *testing.T) {
	body := testBody(14)
	first := []Guard{{Body: body, Encoding: []byte("a")}, {Body: body, Encoding: []byte("b")}}
	second := []Guard{{Body: body, Encoding: []byte("b")}, {Body: body, Encoding: []byte("a")}}
	other := []Guard{{Body: body, Encoding: []byte("a")}}
	var sets guardSets
	left, right := sets.canonical(first), sets.canonical(second)
	if !sameGuards(left, right) {
		t.Fatalf("interned cubes %v and %v differ", left, right)
	}
	if &left[0] != &right[0] {
		t.Fatal("equal cubes were interned as separate sets")
	}
	if single := sets.canonical(other); len(single) != 1 || &single[0] == &left[0] {
		t.Fatalf("a shorter cube was interned as the longer one: %v", single)
	}
	if sets.canonical(nil) != nil {
		t.Fatal("the empty cube is not interned")
	}
}

func TestGuardSetsUnionMatchesCanonicalAppend(t *testing.T) {
	body := testBody(15)
	stamp := []Guard{{Body: body, Encoding: []byte("outer/true")}}
	cases := [][]Guard{
		nil,
		{{Body: body, Encoding: []byte("inner/true")}},
		{{Body: body, Encoding: []byte("outer/true")}},
		{{Body: body, Encoding: []byte("outer/true")}, {Body: body, Encoding: []byte("inner/false")}},
	}
	var sets guardSets
	for _, existing := range cases {
		want := canonicalGuards(append(append([]Guard(nil), existing...), stamp...))
		if got := sets.union(existing, stamp); !sameGuards(got, want) {
			t.Fatalf("union(%v, %v) = %v, want %v", existing, stamp, got, want)
		}
		if got := sets.union(existing, nil); !sameGuards(got, canonicalGuards(existing)) {
			t.Fatalf("union with the empty cube changed %v", existing)
		}
	}
}
