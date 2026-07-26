package engine

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// TestEngineFactKeysStayBehindFactkey is deliberately a crude source fence.
// A migrated key literal or resurrected family-prefix identifier in production
// engine code means a caller has started assembling or parsing the wire form
// beside factkey again.
func TestEngineFactKeysStayBehindFactkey(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate engine source")
	}
	directory := filepath.Dir(source)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []*regexp.Regexp{
		regexp.MustCompile("[\"`]heap/"),
		regexp.MustCompile("[\"`]value/"),
		regexp.MustCompile("[\"`](call-result|call-argument|local-call-result)/"),
		regexp.MustCompile("[\"`](type|declared-type|summary-type|method-return-summary)/"),
		regexp.MustCompile("[\"`](branch-proof|front/branch)/"),
		regexp.MustCompile("[\"`](iterator-element|iterator-key|iterator-key-source)/"),
		regexp.MustCompile(`equation\.Guard\{`),
		regexp.MustCompile(`\bheap[A-Za-z0-9_]*Prefix\b`),
		regexp.MustCompile(`factkey\.[A-Za-z0-9_]+\.Prefix`),
		regexp.MustCompile(`strings\.(HasPrefix|CutPrefix|TrimPrefix|Split|SplitN|Cut)\([^\n]*heap`),
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Errorf("read %s: %v", entry.Name(), readErr)
			continue
		}
		for _, pattern := range forbidden {
			if match := pattern.Find(content); match != nil {
				t.Errorf("%s contains forbidden fact-key handling %q", entry.Name(), match)
			}
		}
	}
	frontPath := filepath.Join(directory, "..", "fixpoint", "front", "front.go")
	front, err := os.ReadFile(frontPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile("[\"`]front/branch/"),
		regexp.MustCompile(`equation\.Guard\{`),
	} {
		if match := pattern.Find(front); match != nil {
			t.Errorf("front.go contains forbidden branch guard handling %q", match)
		}
	}
	exporterPath := filepath.Join(directory, "..", "exporter", "exporter.go")
	exporter, err := os.ReadFile(exporterPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile("[\"`]value/"),
		regexp.MustCompile(`factkey\.Value\.Prefix`),
	} {
		if match := pattern.Find(exporter); match != nil {
			t.Errorf("exporter.go contains forbidden value-key handling %q", match)
		}
	}
}
