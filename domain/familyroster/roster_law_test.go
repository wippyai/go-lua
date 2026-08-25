package familyroster_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule/emit"
	"github.com/wippyai/go-lua/analysis/schema/rule/emitlaw"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/domain/familyroster"
	"github.com/wippyai/go-lua/domain/memberroster"
)

// TestEveryEmittedFamilyIsTheOneItsDeclarationDerives is the freshness law and
// the whole enforcement of the executor generator.
//
// A rule's execution family is a function of its Program declaration and the
// axis member vocabulary it names. This law re-derives every rostered family
// and holds the checked-in file to it byte for byte, so a declaration that
// moves without its family being regenerated is a build failure rather than a
// silent disagreement between what a rule declares and what it executes.
func TestEveryEmittedFamilyIsTheOneItsDeclarationDerives(t *testing.T) {
	root := moduleRoot(t)
	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		t.Fatal("member definition roster is not admissible")
	}
	families := familyroster.Families()
	if len(families) == 0 {
		t.Fatal("the emitted family roster is empty")
	}
	for _, family := range families {
		path := filepath.Join(root, family.Directory, familyroster.GeneratedFileName)
		if err := emit.Generate(family.Target, roster, path, true); err != nil {
			t.Errorf("%s: %v", string(family.Key()), err)
		}
	}
}

// TestARosteredPackageAuthorsNoSecondFamily states the cutover's own
// irreversibility. Once a rule's family is emitted, the package holds exactly
// one: an authored installer beside the generated one is a second authority
// over the same rule's execution, and the two would drift the moment the
// declaration moved.
//
// The bind arm is not a second family. It resolves the axis schemas the
// emitted installer is sealed against from its composition's authorities,
// which is the one thing about a family that is not a function of the
// declaration, and it constructs no rows of its own.
func TestARosteredPackageAuthorsNoSecondFamily(t *testing.T) {
	root := moduleRoot(t)
	for _, family := range familyroster.Families() {
		directory := filepath.Join(root, family.Directory)
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatalf("%s: %v", family.Directory, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") ||
				name == familyroster.GeneratedFileName {
				continue
			}
			source, readErr := os.ReadFile(filepath.Join(directory, name))
			if readErr != nil {
				t.Fatalf("%s/%s: %v", family.Directory, name, readErr)
			}
			if strings.Contains(string(source), "InstallRuleFamily") {
				t.Errorf("%s/%s authors a family beside the one %s declares", family.Directory, name, string(family.Key()))
			}
		}
	}
}

// TestOneRuleDeclaresOneEmittedFamily keeps the roster a registry rather than
// a list. Two rows claiming one rule key, or one directory, would put two
// generated files in disagreement over the same declaration.
func TestOneRuleDeclaresOneEmittedFamily(t *testing.T) {
	keys := map[schema.Key]struct{}{}
	directories := map[string]struct{}{}
	for _, family := range familyroster.Families() {
		if !family.Key().Available() {
			t.Fatalf("a rostered family declares no rule key: %s", family.Directory)
		}
		if _, duplicate := keys[family.Key()]; duplicate {
			t.Fatalf("two rostered families claim rule %s", string(family.Key()))
		}
		keys[family.Key()] = struct{}{}
		if _, duplicate := directories[family.Directory]; duplicate {
			t.Fatalf("two rostered families are generated into %s", family.Directory)
		}
		directories[family.Directory] = struct{}{}
	}
}

// TestEveryEmittedLawSuiteIsTheOneItsDeclarationDerives is the freshness law of
// the structural law suite, and carries the same weight for laws that
// TestEveryEmittedFamilyIsTheOneItsDeclarationDerives carries for executors.
//
// A declaration's structural obligations are a function of the declaration. If
// the checked-in suite could drift from the declaration it was emitted from,
// its geometry law would be holding the declaration to a form nobody has
// re-derived and its mutation table would be asserting verdicts against a
// declaration that no longer has those terms. Re-deriving every rostered suite
// and holding the checked-in file to it byte for byte is what makes the
// emitted laws laws.
func TestEveryEmittedLawSuiteIsTheOneItsDeclarationDerives(t *testing.T) {
	root := moduleRoot(t)
	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		t.Fatal("member definition roster is not admissible")
	}
	declarations := familyroster.Declarations()
	if len(declarations) == 0 {
		t.Fatal("the emitted law suite roster is empty")
	}
	for _, declaration := range declarations {
		path := filepath.Join(root, declaration.Directory, familyroster.GeneratedLawFileName)
		if err := emitlaw.Generate(declaration.Target, roster, path, true); err != nil {
			t.Errorf("%s: %v", string(declaration.Key()), err)
		}
	}
}

// TestOneRuleDeclaresOneEmittedLawSuite keeps the law roster a registry for the
// same reason the family roster is one: two rows over a rule or a directory
// would put two generated suites in disagreement over one declaration.
func TestOneRuleDeclaresOneEmittedLawSuite(t *testing.T) {
	keys := map[schema.Key]struct{}{}
	directories := map[string]struct{}{}
	for _, declaration := range familyroster.Declarations() {
		if !declaration.Key().Available() {
			t.Fatalf("a rostered law suite declares no rule key: %s", declaration.Directory)
		}
		if _, duplicate := keys[declaration.Key()]; duplicate {
			t.Fatalf("two rostered law suites claim rule %s", string(declaration.Key()))
		}
		keys[declaration.Key()] = struct{}{}
		if _, duplicate := directories[declaration.Directory]; duplicate {
			t.Fatalf("two rostered law suites are generated into %s", declaration.Directory)
		}
		directories[declaration.Directory] = struct{}{}
	}
}

// TestARosteredDeclarationAuthorsNoSecondStructuralLaw is the law cutover's own
// irreversibility, and the counterpart of TestARosteredPackageAuthorsNoSecondFamily.
//
// Once a declaration's structural laws are emitted, the package holds exactly
// one statement of them. An authored geometry restatement beside the generated
// one is a second authority over what the declaration is, and the two drift the
// moment the declaration moves - the emitted half regenerates and the authored
// half does not.
//
// The two spellings held here are the ones that are only ever the structural
// suite's: the declared join count, and the reducer call-shape agreement. A
// package may still call Check itself, because a law about what the upward seal
// refuses has to establish that the declaration is data-local valid first.
func TestARosteredDeclarationAuthorsNoSecondStructuralLaw(t *testing.T) {
	root := moduleRoot(t)
	restatements := []string{"JoinCount()", "CheckAgainst("}
	for _, declaration := range familyroster.Declarations() {
		directory := filepath.Join(root, declaration.Directory)
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatalf("%s: %v", declaration.Directory, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, "_test.go") || name == familyroster.GeneratedLawFileName {
				continue
			}
			source, readErr := os.ReadFile(filepath.Join(directory, name))
			if readErr != nil {
				t.Fatalf("%s/%s: %v", declaration.Directory, name, readErr)
			}
			for _, restatement := range restatements {
				if strings.Contains(string(source), restatement) {
					t.Errorf("%s/%s restates %s, which %s emits",
						declaration.Directory, name, restatement, familyroster.GeneratedLawFileName)
				}
			}
		}
	}
}

// TestAReadFreeExactRuleIsNeverRostered states where the Z form's execution
// lives, by holding the roster to the one place it is not.
//
// A rule that declares no read publishes what its own axis already
// materialized: the owner seals a typed source column at bind, and the whole
// invocation is that column answered at the issued candidate ordinal. There is
// no cell to reduce, so there is no fold to generate, so there is no family -
// the engine's generic read-free builder is the complete execution and the
// emitter refuses such a declaration outright rather than emitting a second,
// worse path to it.
//
// The one read-free shape that IS emitted is the transformed carry, because a
// carry applies an owner-issued transition that no materialized column holds.
// So the law is not "a rostered declaration reads something"; it is that a
// rostered declaration which reads nothing carries a transform.
func TestAReadFreeExactRuleIsNeverRostered(t *testing.T) {
	for _, family := range familyroster.Families() {
		declaration := family.Target.Spec.Program
		if declaration.JoinCount() != 0 {
			continue
		}
		if declaration.Carry == nil || declaration.Carry.Mode != program.CarryTransform {
			t.Errorf("%s is rostered while declaring no read and no transformed carry; a read-free exact rule is answered by its owner's materialized source column and emits no family",
				string(family.Key()))
		}
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(directory, "go.mod")); statErr == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("module root was not found")
		}
		directory = parent
	}
}
