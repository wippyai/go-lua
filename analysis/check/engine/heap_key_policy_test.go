package engine

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// TestEngineHeapKeysStayBehindFactkey is deliberately a crude source fence.
// A heap key literal or a resurrected heap *Prefix identifier in production
// engine code means a caller has started assembling or parsing the wire form
// beside factkey again.
func TestEngineHeapKeysStayBehindFactkey(t *testing.T) {
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
				t.Errorf("%s contains forbidden heap key handling %q", entry.Name(), match)
			}
		}
	}
}
