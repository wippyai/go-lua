package engine

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func escapeTestPartition(t *testing.T, facts ...equation.Fact) equation.Partition {
	t.Helper()
	partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: facts})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	return partition
}

func encodedFunctionValue(t *testing.T) []byte {
	t.Helper()
	value, ok := shapefact.EncodeTarget(typ.Func().Param("m", typ.NewArray(typ.Number)).Build())
	if !ok {
		t.Fatalf("encoding a function contract failed")
	}
	return value
}

// TestOpaqueCalleeEffectFiresForADeclaredFunctionContract pins the widening: a
// declared function type states the callee's signature, not its body, so a
// callee whose only authority is that contract admits the same unmodeled effect
// on its arguments that a top-like callee admits.
func TestOpaqueCalleeEffectFiresForADeclaredFunctionContract(t *testing.T) {
	callee := []byte("path/sink")
	contract := encodedFunctionValue(t)
	partition := escapeTestPartition(t,
		equation.Fact{Key: "value/" + string(callee) + "/op-00000001", Value: contract},
		equation.Fact{Key: "declared-type/" + string(callee) + "/op-00000000", Value: contract},
	)
	if !opaqueCalleeEffect(callee, partition) {
		t.Fatalf("a fun(...)-typed callee with no body kept its arguments' proofs")
	}
}

// TestOpaqueCalleeEffectFiresForAClaimedCallee pins the declared-but-unwritten
// local: its current value is a claim refinement, which carries no body either.
func TestOpaqueCalleeEffectFiresForAClaimedCallee(t *testing.T) {
	callee := []byte("path/sink")
	partition := escapeTestPartition(t,
		equation.Fact{Key: "value/" + string(callee) + "/op-00000001", Value: []byte(`scalar/claim/claim-kind/3/claim-type/"fun(m: number[])"`)},
	)
	if !opaqueCalleeEffect(callee, partition) {
		t.Fatalf("a claimed callee with no body kept its arguments' proofs")
	}
}

// TestOpaqueCalleeEffectLeavesAProviderApplicationToItsBoundary pins the split
// between the two lanes: a callee with no current value is a provider call, and
// the external boundary answers it against the published effect row.
func TestOpaqueCalleeEffectLeavesAProviderApplicationToItsBoundary(t *testing.T) {
	if opaqueCalleeEffect([]byte("path/table.concat"), escapeTestPartition(t)) {
		t.Fatalf("a provider application was decided by the call kernel")
	}
}

// TestProviderContractDischargesOnlyWhatItProves walks the published effect
// rows the external boundary reads. A read-only disposition and an append-only
// length change keep the argument's proofs; a shrink, an unqualified mutation, a
// callback row and an unpublished provider each leave them unproven.
func TestProviderContractDischargesOnlyWhatItProves(t *testing.T) {
	tests := []struct {
		provider string
		arity    int
		index    int
		want     bool
	}{
		{"table.concat", 2, 0, true},
		{"print", 1, 0, true},
		{"ipairs", 1, 0, true},
		{"pairs", 1, 0, true},
		{"rawlen", 1, 0, true},
		{"table.insert", 2, 0, true},
		{"table.sort", 1, 0, false},
		{"table.remove", 1, 0, false},
		{"rawset", 3, 0, false},
		{"setmetatable", 2, 1, false},
		{"pcall", 2, 1, false},
		{"unpublished_writer", 1, 0, false},
	}
	for _, test := range tests {
		provider := []byte(`provider/global/"` + test.provider + `"`)
		if got := signatureArgumentProofsSurvive(provider, test.arity, test.index); got != test.want {
			t.Fatalf("%s argument %d discharge = %v, want %v", test.provider, test.index, got, test.want)
		}
	}
}

// TestEscapeReachesTheGraphBeneathAnArgument pins that a callee receiving a
// container reaches every table published under it, so a nested sequence is
// revoked alongside the root it travelled in.
func TestEscapeReachesTheGraphBeneathAnArgument(t *testing.T) {
	root, nested := []byte("sealed-table/root"), []byte("sealed-table/nested")
	partition := escapeTestPartition(t,
		equation.Fact{Key: factkey.Epoch.Key().String() + "path/box/op-00000001", Value: []byte("op-00000001")},
		heapIdentityFact("path/box", "op-00000001", root),
		heapMemberIdentityFact(root, ".items", "op-00000002", nested),
	)
	subjects := escapeReachableSubjects([]byte("path/box"), partition)
	want := factkey.BuildKey(factkey.HeapIndexRevoke, []factkey.Part{factkey.TaggedIdentityPart(nested)}, "op").String()
	found := false
	for _, subject := range subjects {
		found = found || factkey.BuildKey(factkey.HeapIndexRevoke, []factkey.Part{subject}, "op").String() == want
	}
	if !found {
		t.Fatalf("subjects %v do not reach the nested container", subjects)
	}
}

// TestImmutableArgumentKeepsItsFloorAcrossAnEscape pins the boundary of the
// revocation. The length-floor family carries a Lua string's position bound
// alongside a sequence border, and a string cannot be changed by the callee
// that receives it, so its floor survives every application.
func TestImmutableArgumentKeepsItsFloorAcrossAnEscape(t *testing.T) {
	argument := []byte("path/s")
	stringWitness, ok := shapefact.EncodeTarget(typ.String)
	if !ok {
		t.Fatalf("encoding the string contract failed")
	}
	cases := []struct {
		name  string
		value []byte
		want  bool
	}{
		{"literal", []byte(`scalar/string/"abc"`), true},
		{"contract", stringWitness, true},
		{"array", func() []byte {
			value, ok := shapefact.EncodeTarget(typ.NewArray(typ.Number))
			if !ok {
				t.Fatalf("encoding the array contract failed")
			}
			return value
		}(), false},
	}
	for _, test := range cases {
		partition := escapeTestPartition(t,
			equation.Fact{Key: "value/" + string(argument) + "/op-00000001", Value: test.value},
		)
		if got := immutableEscapeArgument(argument, partition); got != test.want {
			t.Fatalf("%s argument immutability = %v, want %v", test.name, got, test.want)
		}
	}
}

// TestEscapedIdentityOrdersAgainstTheCellItInvalidates pins the ordering rule
// shared with the length-floor lane: a cell published before the escape is
// stale, and one published after it is the live slot again.
func TestEscapedIdentityOrdersAgainstTheCellItInvalidates(t *testing.T) {
	identity := []byte("sealed-table/root")
	partition := escapeTestPartition(t,
		equation.Fact{
			Key:   factkey.BuildKey(factkey.HeapTableEscape, []factkey.Part{factkey.TaggedIdentityPart(identity)}, "op-00000005").String(),
			Value: []byte("escaped"),
		},
	)
	if !heapIdentityEscapedAfter(identity, "op-00000003", partition) {
		t.Fatalf("a cell published before the escape survived it")
	}
	if heapIdentityEscapedAfter(identity, "op-00000009", partition) {
		t.Fatalf("a cell published after the escape was revoked by it")
	}
}
