package composite

import (
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/effect/factor"
	"github.com/wippyai/go-lua/domain/value"
)

// TestOnlyTheCompositionSealsAPublicationLayout states where the layout lives.
// A family declares the columns it publishes and names the vocabulary its row
// state is ranked against; what family the payload is interpreted under, and
// whether its rows are keyed, come from the family's own query registration.
// Sealing those together is the composition's act, so a domain that sealed a
// layout of its own would be publishing under a declaration the analyzer's
// seal never saw.
func TestOnlyTheCompositionSealsAPublicationLayout(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	set := token.NewFileSet()
	walked := 0
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case entry.IsDir(), !strings.HasSuffix(path, ".go"), strings.HasSuffix(path, "_test.go"):
			return nil
		case strings.HasPrefix(path, filepath.Join(root, "composite")+string(filepath.Separator)):
			// The composition is the one sealer.
			return nil
		}
		file, parseErr := parser.ParseFile(set, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		walked++
		ast.Inspect(file, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall {
				return true
			}
			selector, isSelector := call.Fun.(*ast.SelectorExpr)
			if !isSelector || selector.Sel.Name != "Seal" {
				return true
			}
			pkg, isIdent := selector.X.(*ast.Ident)
			if !isIdent || pkg.Name != "plane" {
				return true
			}
			relative, _ := filepath.Rel(root, path)
			t.Errorf("%s seals a publication layout of its own at %s", relative, set.Position(call.Pos()))
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if walked == 0 {
		t.Fatal("the domain walk read no sources")
	}
}

// TestEveryPlanedFamilyPublishesUnderItsSealedVocabulary states that the row
// state a published family ranks its wire byte against is a category of the
// sealed structural table and holds exactly the members that table declares.
func TestEveryPlanedFamilyPublishesUnderItsSealedVocabulary(t *testing.T) {
	state, failure := newCatalog()
	if state == nil || failure.Available() || !state.structureOK {
		t.Fatalf("the declaration table did not seal: %+v", failure)
	}
	vocabulary := state.structure
	declared, declaredOK := vocabulary.Vocabulary(structure.CategoryPublicationRowClass)
	if !declaredOK || len(declared) == 0 {
		t.Fatal("the publication row class vocabulary is not declared")
	}
	// The digests are the analyzer's published wire identities. They are pinned
	// beside each domain's own codec laws too, so the layout the composition
	// seals and the layout each codec writes under are two statements of one
	// identity rather than one statement read twice.
	for _, family := range []struct {
		key    schema.Key
		keyed  bool
		digest string
	}{
		{key: value.SummaryResultFamily, keyed: true, digest: "58ee06e07da27d3cb74d5e15cff584861e77628bcd21c65d34e81e4eb01b3976"},
		{key: factor.ExactResultFamily, keyed: false, digest: "5dfd6657e139f6ce6e55a18677ec2c7cfcb9d1fc315bbef05b7218f54dbf3a54"},
	} {
		t.Run(string(family.key), func(t *testing.T) {
			layout, layoutOK := queryResultLayout(state, family.key)
			if !layoutOK || !layout.Available() {
				t.Fatal("the compilation sealed no layout for this family")
			}
			states := layout.States()
			if len(states) != len(declared) {
				t.Fatalf("row states = %v, want the sealed vocabulary %v", states, declared)
			}
			for index, state := range states {
				if state != declared[index] {
					t.Fatalf("row state %d = %q, want the sealed member %q", index, state, declared[index])
				}
			}
			// Keying is the registration's fold, so a summary family sizes a
			// coordinate plane and an exact one refuses to.
			if _, admitted := layout.Size(2, 0); admitted != family.keyed {
				t.Fatalf("a two-row answer was admitted = %v, want %v", admitted, family.keyed)
			}
			digest := layout.Digest()
			if got := hex.EncodeToString(digest[:]); got != family.digest {
				t.Fatalf("layout digest = %s, want the pinned declaration %s", got, family.digest)
			}
		})
	}
}
