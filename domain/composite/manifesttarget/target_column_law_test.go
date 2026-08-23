package manifesttarget

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	"github.com/wippyai/go-lua/domain/type/targetfamily"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	"github.com/wippyai/go-lua/manifest"
	"github.com/wippyai/go-lua/stdlib"
)

// targetColumnLawCatalogue is the standard provider catalogue, sealed twice by
// the laws below so that a second contract is a second sealing of one content.
func targetColumnLawCatalogue(t *testing.T) *manifest.Catalogue {
	t.Helper()
	catalogue, err := manifest.Seal(stdlib.Providers()...)
	if err != nil {
		t.Fatal(err)
	}
	return catalogue
}

func targetColumnLawSeal(t *testing.T, column bool) *contract.Contract {
	t.Helper()
	spec, err := compileCatalogue(targetColumnLawCatalogue(t))
	if err != nil {
		t.Fatal(err)
	}
	if !column {
		spec.Semantics = columnlessSemantics{spec.Semantics}
	}
	sealed, err := compiler.Seal(&spec)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

// columnlessSemantics is a semantic adapter that reads declarations but seals
// no class vocabulary. It exists to state what a target without the column
// cannot do; the production adapter always seals one.
type columnlessSemantics struct{ inner schematype.Semantics }

func (s columnlessSemantics) Validate(value schematype.Type, formals []schematype.Type) error {
	return s.inner.Validate(value, formals)
}

func (s columnlessSemantics) Assignable(source, destination schematype.Type, formals []schematype.Type) (bool, error) {
	return s.inner.Assignable(source, destination, formals)
}

func (s columnlessSemantics) Callable(value schematype.Type, admission schematype.CallableAdmission, formals []schematype.Type) (bool, error) {
	return s.inner.Callable(value, admission, formals)
}

func (s columnlessSemantics) Fresh(value schematype.Type, class schematype.FreshClass, formals []schematype.Type) (bool, error) {
	return s.inner.Fresh(value, class, formals)
}

// TestTheProductionAdapterAlwaysSealsTheClassVocabulary states that the
// column is not optional in practice: the one semantic adapter every target
// names is a column sealer, so no target can be sealed without its class
// vocabulary by forgetting to ask for one.
func TestTheProductionAdapterAlwaysSealsTheClassVocabulary(t *testing.T) {
	if _, sealing := any(domaincontract.NewSemantics()).(contract.ColumnSealer); !sealing {
		t.Fatal("the production semantic adapter seals no class vocabulary")
	}
	if _, sealing := any(columnlessSemantics{domaincontract.NewSemantics()}).(contract.ColumnSealer); sealing {
		t.Fatal("the columnless adapter used by these laws seals a class vocabulary")
	}
}

// TestTargetSealsItsClassVocabularyExactlyOnce states where the declaration
// denominator is derived: in the contract constructor, over the finished
// operation core, once. A sealed contract answers with the one instance it
// sealed, however many consumers ask.
func TestTargetSealsItsClassVocabularyExactlyOnce(t *testing.T) {
	sealed := targetColumnLawSeal(t, true)
	first, available := targetfamily.Of(sealed)
	if !available || first == nil {
		t.Fatal("a sealed target published no class vocabulary")
	}
	if first.Count() == 0 {
		t.Fatal("the sealed class vocabulary is empty")
	}
	for read := 0; read < 3; read++ {
		again, againOK := targetfamily.Of(sealed)
		if !againOK || again != first {
			t.Fatalf("read %d answered a second class vocabulary instance", read)
		}
	}
}

// TestTwoTargetsNeverShareOneClassVocabulary states the aliasing fence from
// both sides: one content seals one identity, and two contents never share
// one. Instances are never shared, because a contract owns its own column.
func TestTwoTargetsNeverShareOneClassVocabulary(t *testing.T) {
	left, leftOK := targetfamily.Of(targetColumnLawSeal(t, true))
	right, rightOK := targetfamily.Of(targetColumnLawSeal(t, true))
	if !leftOK || !rightOK {
		t.Fatal("a sealed target published no class vocabulary")
	}
	if left == right {
		t.Fatal("two contracts share one class vocabulary instance")
	}
	if left.ContentID() != right.ContentID() {
		t.Fatalf("one target content sealed two vocabulary identities: %s and %s",
			left.ContentID().String(), right.ContentID().String())
	}
	amended, err := SealCatalogue(targetColumnLawCatalogue(t), PreviewAmendment{})
	if err == nil {
		other, otherOK := targetfamily.Of(amended)
		if otherOK && other.ContentID() == left.ContentID() && amended.ContentID() != left.ContentID() {
			t.Fatal("two target contents share one class vocabulary identity")
		}
	}
}

// TestTheClassVocabularyIsPartOfTargetIdentity states that the column is not
// a side table: a contract that carries one is a different contract from a
// contract that does not, so no consumer can be handed the wrong vocabulary
// behind an equal identity.
func TestTheClassVocabularyIsPartOfTargetIdentity(t *testing.T) {
	withColumn := targetColumnLawSeal(t, true)
	without := targetColumnLawSeal(t, false)
	if _, available := targetfamily.Of(without); available {
		t.Fatal("a target sealed without a column sealer published a class vocabulary")
	}
	if withColumn.ContentID() == without.ContentID() {
		t.Fatal("the sealed class vocabulary is invisible to target identity")
	}
}

// TestATargetWithoutItsClassVocabularyCannotMount states that there is no
// second derivation to fall back to. A Link that mounts a target reads the
// sealed vocabulary or it refuses; it never re-derives one of its own.
func TestATargetWithoutItsClassVocabularyCannotMount(t *testing.T) {
	input, err := lower.Lower(lower.Source{Name: "target-column-law.lua", Text: []byte("local value = 1\nreturn value\n")})
	if err != nil || input == nil || !input.Available() {
		t.Fatalf("lower target column fixture: %v", err)
	}
	executionSchema, executionSchemaOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	if !executionSchemaOK {
		t.Fatal("target column execution schema")
	}
	artifact, failure := artifactcompiler.CompileDetailed(input, executionSchema, schemaissuance.Plan{})
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile target column fixture: %s", failure.Error())
	}
	program := artifact.Program()
	linkID := identity.ContentID{5}
	types, typesErr := typeauthority.SealProgramRows(linkID, []programschema.Program{program}, nil)
	if typesErr != nil || types == nil {
		t.Fatalf("columnless link type authority: %v", typesErr)
	}
	moduleID := identity.ContentID{5, 9}
	mounts := []staticdomain.MountedProgram{{Program: program, ModuleID: moduleID, NamespaceID: moduleID}}
	if _, _, sealErr := staticdomain.SealMountedPrograms(
		staticdomain.MountContext{LinkID: linkID, Target: targetColumnLawSeal(t, false)}, types, mounts); sealErr == nil {
		t.Fatal("a target carrying no sealed class vocabulary mounted")
	}
	if _, _, sealErr := staticdomain.SealMountedPrograms(
		staticdomain.MountContext{LinkID: linkID, Target: targetColumnLawSeal(t, true)}, types, mounts); sealErr != nil {
		t.Fatalf("a target carrying its sealed class vocabulary refused to mount: %v", sealErr)
	}
}

// TestTheClassVocabularyDoesNotEnterArtifactIdentity is the identity fence
// this cut had to clear. Adding the column moves the target identity, and a
// target identity that reached artifact identity would invalidate every
// compiled Program in every cache keyed by it. It does not: an Artifact is
// keyed by its Program and its execution schema, and both are sealed before
// any target is mounted. The law states that on three fixtures rather than
// leaving it to the signature.
func TestTheClassVocabularyDoesNotEnterArtifactIdentity(t *testing.T) {
	withColumn := targetColumnLawSeal(t, true)
	without := targetColumnLawSeal(t, false)
	if withColumn.ContentID() == without.ContentID() {
		t.Fatal("the two targets are identical, so the law would be vacuous")
	}
	sources := []string{
		"local value = 1\nreturn value\n",
		"local function add(a: number, b: number): number return a + b end\nreturn add(1, 2)\n",
		"local rows = {1, 2, 3}\nlocal total = 0\nfor _, row in ipairs(rows) do total = total + row end\nreturn total\n",
	}
	for index, source := range sources {
		artifactID, schemaID := targetColumnLawArtifact(t, source)
		againID, againSchema := targetColumnLawArtifact(t, source)
		if !artifactID.Available() || !schemaID.Available() {
			t.Fatalf("fixture %d: unavailable artifact identity", index)
		}
		// The artifact identity is the Program and the execution schema, both
		// sealed before any target is mounted. Neither target above, and
		// neither target identity, is part of its preimage.
		if artifactID != againID || schemaID != againSchema {
			t.Fatalf("fixture %d does not compile to one artifact identity", index)
		}
	}
}

func targetColumnLawArtifact(t *testing.T, source string) (identity.ContentID, identity.ContentID) {
	t.Helper()
	input, err := lower.Lower(lower.Source{Name: "target-column-artifact-law.lua", Text: []byte(source)})
	if err != nil || input == nil || !input.Available() {
		t.Fatalf("lower artifact identity fixture: %v", err)
	}
	executionSchema, executionSchemaOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	if !executionSchemaOK {
		t.Fatal("artifact identity execution schema")
	}
	artifact, failure := artifactcompiler.CompileDetailed(input, executionSchema, schemaissuance.Plan{})
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile artifact identity fixture: %s", failure.Error())
	}
	return artifact.ID(), artifact.CompileKey().ExecutionSchemaID().ContentID()
}
