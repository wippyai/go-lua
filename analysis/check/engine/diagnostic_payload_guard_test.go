package engine

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestEngineDoesNotParseDiagnosticMessageContent(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locating engine source")
	}
	root := filepath.Dir(current)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("reading engine source: %v", err)
	}
	parsers := regexp.MustCompile(`(?m)strings\.(?:Cut(?:Prefix|Suffix)?|Trim(?:Prefix|Suffix)|Has(?:Prefix|Suffix)|Contains|Index|LastIndex|IndexByte|LastIndexByte)\([^\n]*(?:\.Message|\bmessage\b)`)
	comparisons := regexp.MustCompile(`(?m)\.Message\s*(?:==|!=)`)
	switches := regexp.MustCompile(`(?m)\bswitch\s+[^\n]*\.Message\b`)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		source, readErr := os.ReadFile(filepath.Join(root, entry.Name()))
		if readErr != nil {
			t.Fatalf("reading %s: %v", entry.Name(), readErr)
		}
		for _, pattern := range []*regexp.Regexp{parsers, comparisons, switches} {
			if match := pattern.FindIndex(source); match != nil {
				line := 1 + strings.Count(string(source[:match[0]]), "\n")
				t.Errorf("%s:%d parses diagnostic Message content: %q", entry.Name(), line, source[match[0]:match[1]])
			}
		}
		removedSentinel := "assignment-map-read-" + "missing/v1/"
		if strings.Contains(string(source), removedSentinel) {
			t.Errorf("%s retains the removed assignment sentinel channel", entry.Name())
		}
	}
}

func TestDiagnosticPayloadWireRoundTripsTypedFields(t *testing.T) {
	want := DiagnosticPayload{
		Kind:  diagnosticCallGenericConflict,
		Flags: DiagnosticAnyBoundary | DiagnosticMapReadMissing,
		Conflict: &DiagnosticConflict{
			Parameter: "T", Bound: "string", BoundAt: "argument 1.id",
			Demanded: "number", DemandedAt: "argument 2.id",
		},
	}
	encoded, err := encodeDiagnosticPayload(want)
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	got, ok := decodeDiagnosticPayload(encoded)
	if !ok {
		t.Fatal("decode payload failed")
	}
	if got.Version != 1 || got.Kind != want.Kind || got.Flags != want.Flags || got.Conflict == nil || *got.Conflict != *want.Conflict {
		t.Fatalf("payload round trip = %#v, want %#v", got, want)
	}
}
