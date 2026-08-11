package discover

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAnalyzeFilesFindsTypedMechanicalCandidatesDeterministically(t *testing.T) {
	source := map[string]string{"fixture.go": `package fixture
import (
 "fmt"
 "testing"
)
type Store struct { items []int; byID map[int]int; byCopy map[int]int }
func (s *Store) Count() int { return len(s.items) }
func (s *Store) ItemAt(i int) int { return s.items[i] }
func (s *Store) FindAt(i int) int { return s.ItemAt(i) }
func (s *Store) RebindAt(i int) int { return s.ItemAt(i) }
func (s *Store) BuildOne() { s.byID = make(map[int]int); for _, item := range s.items { s.byID[item] = item } }
func (s *Store) BuildTwo() { s.byCopy = make(map[int]int); for _, item := range s.items { s.byCopy[item] = item } }
func sameOne(a int) int { local := a + 1; return local }
func sameTwo(b int) int { value := b + 1; return value }
func codec(a int) string { switch a { case 1: return fmt.Sprint(a); case 2: return fmt.Sprint(a); default: return "" } }
func TestStore(t *testing.T) { _ = (&Store{}).Count() }
`}
	first, err := AnalyzeFiles("example/fixture", source)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AnalyzeFiles("example/fixture", source)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("nondeterministic report:\n%#v\n%#v", first, second)
	}
	for _, kind := range []Kind{ReceiverCluster, Forwarder, APIFamily, FileCluster, TestCluster, DuplicateBody, SwitchCaseShape, ImportCluster, DuplicateIndex} {
		candidate, ok := candidateKind(first, kind)
		if !ok {
			t.Fatalf("missing %s: %#v", kind, first.Candidates)
		}
		if len(candidate.Symbols) == 0 || len(candidate.Positions) == 0 || len(candidate.Evidence) == 0 || candidate.Confidence == "" {
			t.Fatalf("thin evidence for %s: %#v", kind, candidate)
		}
	}
	forwarder, _ := candidateKind(first, Forwarder)
	if !strings.Contains(forwarder.Reasons[0], "forwarding") {
		t.Fatalf("unexpected forwarder: %#v", forwarder)
	}
}

func TestAnalyzeFilesRejectsTypeErrors(t *testing.T) {
	_, err := AnalyzeFiles("example/bad", map[string]string{"bad.go": `package bad; func f() { missing() }`})
	if err == nil || !strings.Contains(err.Error(), "type-check") {
		t.Fatalf("want type-check failure, got %v", err)
	}
}

func TestCallerPackagesReportsDeclaredImportOnly(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root+"/a/a.go", `package a; import _ "example/target"`)
	writeFixture(t, root+"/a/another.go", `package a; import _ "example/target"`)
	writeFixture(t, root+"/b/b.go", `package b; import "example/other"`)
	got, err := CallerPackages(root, "example/target")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != CallerPackage || got[0].Evidence[0].Code != "declared-import" || got[0].Evidence[0].Count != 2 {
		t.Fatalf("unexpected callers: %#v", got)
	}
}

func candidateKind(report Report, kind Kind) (Candidate, bool) {
	for _, candidate := range report.Candidates {
		if candidate.Kind == kind {
			return candidate, true
		}
	}
	return Candidate{}, false
}

func writeFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
