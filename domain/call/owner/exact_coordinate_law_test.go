package owner_test

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// A Call fact published by a routed rule has no committed exact write to
// recover a read coordinate from, so a consumer that must observe the Call
// cell one occurrence publishes at states the coordinate itself. The only
// admissible statement is the one Call already issued: the key a row of its
// own sealed coordinate projection names, resolved by the sealed Factor into
// an opaque capability. These laws hold that issuance to its owner - one
// capability per occurrence, none for a row this Algebra did not issue, and
// none at all before the Factor is sealed.

const exactCoordinateSource = "local function apply(value) return value end\nreturn apply(1), apply(2)\n"

// exactCoordinateAlgebra seals one Call Algebra over a module with call sites,
// the way the composition mounts it.
func exactCoordinateAlgebra(t testing.TB, name string) *calldomain.Algebra {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: name + ".lua", Text: []byte(exactCoordinateSource)})
	if err != nil {
		t.Fatalf("lower %s: %v", name, err)
	}
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatalf("standard target: %v", err)
	}
	linked, err := link.Seal(&link.Spec{Target: target, Modules: []linkproject.Module{{Name: name, Program: program}}})
	if err != nil {
		t.Fatalf("seal %s: %v", name, err)
	}
	receipt, receiptOK := composite.Build()
	grammar := receipt.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(receipt)
	if !receiptOK || !grammar.Available() || !issuanceOK {
		t.Fatal("composite execution schema")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile %s: %s", name, failure.Error())
	}
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	if !shardOK || !moduleOK {
		t.Fatal("module mount")
	}
	snapshot := snapshottest.MustLower(t, artifact)
	mounted := snapshottest.MustMount(t, artifact, module)
	calls, callsOK := calldomain.NewWithMountedArtifacts(linked, []calldomain.MountedArtifact{{Program: mounted, Snapshot: snapshot}})
	if !callsOK || calls == nil || calls.CallCoordinateCount() == 0 {
		t.Fatalf("Call algebra over %s carries no coordinate projection", name)
	}
	return calls
}

// exactCoordinateOwner binds and seals one Call Factor over an Algebra.
func exactCoordinateOwner(t testing.TB, calls *calldomain.Algebra, seed byte) *callowner.HotOwner {
	t.Helper()
	owner, binding := exactCoordinateOpenOwner(t, calls, seed)
	if !binding.Seal() {
		t.Fatal("seal Call binding")
	}
	return owner
}

// exactCoordinateOpenOwner is the same owner before its binding is sealed. An
// unsealed binding publishes no Factor implementation, which is what makes the
// capability a property of the sealed Factor rather than of the owner value.
func exactCoordinateOpenOwner(t testing.TB, calls *calldomain.Algebra, seed byte) (*callowner.HotOwner, *engine.SchemaBinding) {
	t.Helper()
	builder := engine.NewSchema()
	fragment, fragmentOK := callowner.DeclareSchema(builder, exactCoordinateSemantic(seed))
	cold, coldOK := builder.Seal()
	if !fragmentOK || !coldOK || cold == nil {
		t.Fatal("declare Call factor")
	}
	binding := engine.NewSchemaBinding(cold)
	owner, ownerOK := callowner.BindHot(binding, fragment, calls)
	if !ownerOK || owner == nil {
		t.Fatal("bind Call factor")
	}
	return owner, binding
}

func exactCoordinateSemantic(seed byte) identity.SemanticKey {
	digest := sha256.Sum256([]byte{0xC1, seed})
	key, ok := identity.NewSemanticKey(digest, 1)
	if !ok {
		panic("Call exact-coordinate semantic key")
	}
	return key
}

// TestACallCoordinateIssuesTheExactReadCapabilityItHolds states the mapping.
// Every row of the sealed coordinate projection resolves the exact-read
// capability for its own occurrence, and the dense index the row publishes is
// the coordinate that capability reads at: the projection and the key
// directory answer one coordinate, not two that happen to agree. Distinct
// occurrences resolve distinct coordinates, so an observation admitted at one
// occurrence can never read the cell of another.
func TestACallCoordinateIssuesTheExactReadCapabilityItHolds(t *testing.T) {
	calls := exactCoordinateAlgebra(t, "call-exact-coordinate")
	owner := exactCoordinateOwner(t, calls, 1)
	seen := map[engine.RuleReadSurface]int{}
	for ordinal := 0; ordinal < calls.CallCoordinateCount(); ordinal++ {
		row, rowOK := calls.CallCoordinateAt(ordinal)
		if !rowOK {
			t.Fatalf("coordinate row %d", ordinal)
		}
		key, keyOK := row.Key()
		if !keyOK {
			t.Fatalf("row %d names no key", ordinal)
		}
		ref, refOK := owner.Ref(key)
		if !refOK {
			t.Fatalf("row %d resolved no exact-read capability", ordinal)
		}
		surface, surfaceOK := engine.ExactReadSurface(ref)
		if !surfaceOK {
			t.Fatalf("row %d resolved a capability with no read coordinate", ordinal)
		}
		index, indexOK := row.CoordinateIndex()
		dense, denseOK := calls.DenseKeyIndex(key)
		if !indexOK || !denseOK || index != uint64(dense) {
			t.Fatalf("row %d publishes index %d/%t and its key densifies to %d/%t", ordinal, index, indexOK, dense, denseOK)
		}
		if previous, duplicate := seen[surface]; duplicate {
			t.Fatalf("rows %d and %d issued one coordinate", previous, ordinal)
		}
		seen[surface] = ordinal
	}
	if len(seen) != calls.CallCoordinateCount() {
		t.Fatalf("the projection issued %d coordinates for %d rows", len(seen), calls.CallCoordinateCount())
	}
}

// TestACallExactCoordinateRefusesARowItsAlgebraDidNotIssue is the owner fence.
// A coordinate is a capability, so the only keys that resolve one are the keys
// this Algebra sealed: an unset key, and the key of another Algebra over the
// same source, name nothing here rather than naming a cell by coincidence of
// index.
func TestACallExactCoordinateRefusesARowItsAlgebraDidNotIssue(t *testing.T) {
	calls := exactCoordinateAlgebra(t, "call-exact-coordinate")
	foreign := exactCoordinateAlgebra(t, "call-exact-coordinate-foreign")
	owner := exactCoordinateOwner(t, calls, 2)

	if _, refused := owner.Ref(calldomain.Key{}); refused {
		t.Fatal("an unset key resolved an exact-read capability")
	}
	foreignRow, foreignRowOK := foreign.CallCoordinateAt(0)
	if !foreignRowOK {
		t.Fatal("foreign coordinate row")
	}
	foreignKey, foreignKeyOK := foreignRow.Key()
	if !foreignKeyOK {
		t.Fatal("foreign coordinate key")
	}
	if _, refused := owner.Ref(foreignKey); refused {
		t.Fatal("a key another Algebra issued resolved against this Factor")
	}
	own, ownOK := calls.CallCoordinateAt(0)
	if !ownOK {
		t.Fatal("own coordinate row")
	}
	ownKey, ownKeyOK := own.Key()
	if !ownKeyOK {
		t.Fatal("own coordinate key")
	}
	if _, issued := owner.Ref(ownKey); !issued {
		t.Fatal("the Algebra's own row resolved no capability")
	}
}

// TestACallExactCoordinateIsAPropertyOfTheSealedFactor states when the
// capability exists. Before the binding is sealed there is no Factor
// implementation to authenticate against, so a row that will resolve a
// coordinate resolves none yet - the capability is the sealed Factor's, not
// the owner value's.
func TestACallExactCoordinateIsAPropertyOfTheSealedFactor(t *testing.T) {
	calls := exactCoordinateAlgebra(t, "call-exact-coordinate")
	owner, binding := exactCoordinateOpenOwner(t, calls, 3)
	row, rowOK := calls.CallCoordinateAt(0)
	if !rowOK {
		t.Fatal("coordinate row")
	}
	key, keyOK := row.Key()
	if !keyOK {
		t.Fatal("coordinate key")
	}
	if _, issued := owner.Ref(key); issued {
		t.Fatal("an unsealed binding resolved an exact-read capability")
	}
	if !binding.Seal() {
		t.Fatal("seal Call binding")
	}
	if _, issued := owner.Ref(key); !issued {
		t.Fatal("the sealed Factor resolved no exact-read capability")
	}
}
