package result

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResultGeometryReadsSealedSnapshotBodies(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("result source location")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "project.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	fn := "func Project("
	start := strings.Index(text, fn)
	if start < 0 {
		t.Fatal("Project missing from analysis/result/project.go")
	}
	rest := text[start:]
	end := strings.Index(rest[len(fn):], "\nfunc ")
	if end < 0 {
		t.Fatal("Project body unbound")
	}
	body := rest[:len(fn)+end]
	if strings.Contains(body, "mount.artifact.") {
		t.Fatal("result geometry still reopens ProgramArtifact interiors")
	}
	if !strings.Contains(body, "mount.Snapshot.BodyCount()") || !strings.Contains(body, "mount.Snapshot.OccurrenceCount()") {
		t.Fatal("result geometry does not walk sealed snapshot bodies and occurrences")
	}
}

func TestNativeSummariesReadSealedSnapshot(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("result source location")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "native_artifact.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if strings.Contains(text, "func exactNativeScalarRulePoint(artifact *programartifact.Artifact") {
		t.Fatal("native summaries still reopen ProgramArtifact columns")
	}
	if !strings.Contains(text, "func exactNativeScalarRulePoint(snapshot *ingress.Snapshot") {
		t.Fatal("native rule-point lookup does not take the sealed snapshot")
	}
}
