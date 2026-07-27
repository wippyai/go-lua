package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
)

func TestRevokerFamilySubsetsStayDeclarationOwned(t *testing.T) {
	root := wireFenceRepositoryRoot(t)
	analysisRoot := filepath.Join(root, "analysis")
	ownedDeclarations := filepath.Join(root, "analysis", "check", "fixpoint", "factkey")
	ownedConsumer := filepath.Join(root, "analysis", "check", "engine", "license.go")
	err := filepath.WalkDir(analysisRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == ownedDeclarations {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || path == ownedConsumer {
			return nil
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if literal, ok := node.(*ast.CompositeLit); ok && fenceRevokerSubsetLiteral(literal) {
				t.Errorf("%s:%d constructs a handwritten revoker-family subset; use the factkey declaration through familyReadLicense", path, literal.Pos())
			}
			if loop, ok := node.(*ast.RangeStmt); ok && fenceHandwrittenRevokerRange(loop) {
				t.Errorf("%s:%d enumerates a handwritten revoker-family subset; use the factkey declaration through familyReadLicense", path, loop.Pos())
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRevocationFenceRejectsHandwrittenFamilyIDSubset(t *testing.T) {
	file, err := fenceParseSource(`package engine
var revokers = factkey.RevocationSet{factkey.FamilyHeapIndexRevoke}
var alternate = []factkey.FamilyID{factkey.FamilyHeapTableEscape}
func handwrittenRevocation(partition Partition, proof string) bool {
	for _, family := range []factkey.Family{factkey.HeapIndexRevoke} {
		values := partition.FamilyValues(factkey.BuildKey(family, nil, ""))
		for fact, ok := values.Next(); ok; fact, ok = values.Next() {
			if fact.Occurrence >= proof {
				return false
			}
		}
	}
	return true
}`)
	if err != nil {
		t.Fatal(err)
	}
	matches := 0
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if ok && fenceRevokerSubsetLiteral(literal) {
			matches++
		}
		return true
	})
	if matches != 2 {
		t.Fatalf("handwritten revoker subset matches = %d, want 2", matches)
	}
	ranges := 0
	ast.Inspect(file, func(node ast.Node) bool {
		loop, ok := node.(*ast.RangeStmt)
		if ok && fenceHandwrittenRevokerRange(loop) {
			ranges++
		}
		return true
	})
	if ranges != 1 {
		t.Fatalf("handwritten revoker family-list ranges = %d, want 1", ranges)
	}
}

func TestDeclaredRevokerInvalidatesLengthFloorFamilyRead(t *testing.T) {
	identity := []byte("sealed-table/license")
	subject := factkey.TaggedIdentityPart(identity)
	floor := func(point string) equation.Fact {
		return equation.Fact{
			Key: factkey.BuildKey(
				factkey.HeapLengthFloor, []factkey.Part{subject}, point,
			).String(),
			Value: []byte("1"),
		}
	}
	revoker := func(point string) equation.Fact {
		return equation.Fact{
			Key: factkey.BuildKey(
				factkey.HeapIndexRevoke, []factkey.Part{subject}, point,
			).String(),
			Value: []byte("revoked"),
		}
	}

	if got := subjectLengthFloorProven(subject, joinTestPartition(t, nil, floor("op-00000002"))); got != 1 {
		t.Fatalf("unrevoked length floor = %d, want 1", got)
	}
	if got := subjectLengthFloorProven(subject, joinTestPartition(t, nil, revoker("op-00000001"), floor("op-00000002"))); got != 1 {
		t.Fatal("a revoker before the proof invalidated a later publication")
	}
	for _, point := range []string{"op-00000002", "op-00000003"} {
		if got := subjectLengthFloorProven(subject, joinTestPartition(t, nil, floor("op-00000002"), revoker(point))); got != 0 {
			t.Fatalf("declared index revoker at %s left length floor %d, want 0", point, got)
		}
	}
}
