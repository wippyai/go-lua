package program

import (
	"crypto/sha256"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

func TestProgramRootRetainsOnlyFrozenSurface(t *testing.T) {
	typeInfo := reflect.TypeOf(Program{})
	if typeInfo.NumField() != 6 {
		t.Fatalf("Program field count = %d, want six", typeInfo.NumField())
	}
	wantFields := map[string]reflect.Type{
		"source":          reflect.TypeOf((*source.Component)(nil)),
		"flow":            reflect.TypeOf((*flow.Component)(nil)),
		"static":          reflect.TypeOf((*static.Component)(nil)),
		"module":          reflect.TypeOf((*module.Component)(nil)),
		"id":              reflect.TypeOf(keyspace.ContentID{}),
		"semanticReceipt": reflect.TypeOf(SemanticSourceReceipt{}),
	}
	for index := 0; index < typeInfo.NumField(); index++ {
		field := typeInfo.Field(index)
		want, ok := wantFields[field.Name]
		if !ok || field.Type != want {
			t.Fatalf("Program field %d = %s %s; want frozen owner field", index, field.Name, field.Type)
		}
		delete(wantFields, field.Name)
	}
	if len(wantFields) != 0 {
		t.Fatalf("Program is missing fields: %v", wantFields)
	}

	methodInfo := reflect.TypeOf((*Program)(nil))
	wantMethods := map[string]bool{
		"Source": true, "Flow": true, "Static": true, "Module": true, "ContentID": true,
		"SemanticSourceReceipt": true, "SemanticSourceViews": true,
	}
	if methodInfo.NumMethod() != len(wantMethods) {
		t.Fatalf("Program method count = %d, want %d", methodInfo.NumMethod(), len(wantMethods))
	}
	for index := 0; index < methodInfo.NumMethod(); index++ {
		method := methodInfo.Method(index)
		if !wantMethods[method.Name] {
			t.Fatalf("unexpected Program method %q", method.Name)
		}
		delete(wantMethods, method.Name)
	}
	if len(wantMethods) != 0 {
		t.Fatalf("Program is missing methods: %v", wantMethods)
	}
}

func TestRootContentIDIsCanonicalOrderedAndSensitive(t *testing.T) {
	ids := [...]keyspace.ContentID{
		{0: 1}, {1: 2}, {2: 3}, {3: 4},
	}
	first, err := rootContentID(ids[0], ids[1], ids[2], ids[3])
	if err != nil || !first.Available() {
		t.Fatalf("rootContentID = %x, %v; want available identity", first, err)
	}
	second, err := rootContentID(ids[0], ids[1], ids[2], ids[3])
	if err != nil || second != first {
		t.Fatalf("equivalent rootContentID = %x, %v; want deterministic %x", second, err, first)
	}

	want := canonicalRootIDForLaw(t, ids)
	if first != want {
		t.Fatalf("rootContentID = %x; want canonical stream digest %x", first, want)
	}
	for index := range ids {
		changed := ids
		changed[index][31]++
		got, err := rootContentID(changed[0], changed[1], changed[2], changed[3])
		if err != nil || got == first {
			t.Fatalf("changing child %d retained root identity: %x, %v", index, got, err)
		}
	}
	for left, right := 0, 1; left < len(ids); left++ {
		for right = left + 1; right < len(ids); right++ {
			permuted := ids
			permuted[left], permuted[right] = permuted[right], permuted[left]
			got, err := rootContentID(permuted[0], permuted[1], permuted[2], permuted[3])
			if err != nil || got == first {
				t.Fatalf("permuting children %d/%d retained root identity: %x, %v", left, right, got, err)
			}
		}
	}
}

func canonicalRootIDForLaw(t testing.TB, ids [4]keyspace.ContentID) keyspace.ContentID {
	t.Helper()
	hash := sha256.New()
	var writer canonical.Writer
	if err := writer.Reset(hash, "program/root", 1); err != nil {
		t.Fatalf("canonical root Reset: %v", err)
	}
	for _, id := range ids {
		if err := writer.Bytes(id[:]); err != nil {
			t.Fatalf("canonical root Bytes: %v", err)
		}
	}
	if err := writer.Finish(); err != nil {
		t.Fatalf("canonical root Finish: %v", err)
	}
	var result keyspace.ContentID
	copy(result[:], hash.Sum(nil))
	return result
}

func TestProgramNilReceiverAndNilAssemblyFailClosed(t *testing.T) {
	var program *Program
	if program.Source() != (source.View{}) || program.Flow() != (flow.View{}) ||
		program.Static() != (static.View{}) || program.Module() != (module.View{}) ||
		program.ContentID().Available() {
		t.Fatal("nil Program exposed owner state")
	}
	if published, err := Publish(nil); err == nil || published != nil {
		t.Fatalf("Publish(nil) = %v, %v; want nil Program and error", published, err)
	}
}

func TestPublishRetainsExactOwnerQuartetViews(t *testing.T) {
	assembly := rootAssembly(t, "program-root-law.lua")
	published, err := Publish(assembly)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if published == nil {
		t.Fatal("Publish returned nil Program")
	}
	sourceID := published.Source().Identity().ContentID()
	flowID := published.Flow().ContentID()
	staticID := published.Static().ContentID()
	moduleID := published.Module().ContentID()
	provenance := published.Flow().Provenance()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() {
		t.Fatal("Publish exposed an unavailable owner view")
	}
	if provenance.Source != sourceID || provenance.Flow != flowID ||
		provenance.Static != staticID || provenance.Module != moduleID {
		t.Fatalf("published owner views do not match Flow provenance: %#v", provenance)
	}
	firstID := published.ContentID()
	if firstID != published.ContentID() {
		t.Fatal("Program ContentID is not stable")
	}
	if second, secondErr := Publish(assembly); secondErr == nil || second != nil {
		t.Fatalf("reused Assembly published %#v with nil error", second)
	}
}

func rootAssembly(t *testing.T, name string) *flow.Assembly {
	t.Helper()
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody] = 1

	sourceDraft, err := source.Build(source.Input{
		Name:     name,
		Families: rootFamilySpans(name, counts),
		Bodies:   []source.BodySource{{Body: entry}},
	})
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalizer, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}

	staticDraft, err := static.Build(static.Input{Counts: counts})
	if err != nil {
		_ = sourceFinalizer.Abort()
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		_ = sourceFinalizer.Abort()
		t.Fatalf("static.Finalizer: %v", err)
	}

	moduleDraft, err := module.Build(module.Input{})
	if err != nil {
		_ = staticFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		t.Fatalf("module.Build: %v", err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		_ = staticFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		t.Fatalf("module.Finalizer: %v", err)
	}

	flowDraft, err := flow.Build(flow.Input{Counts: counts})
	if err != nil {
		_ = moduleFinalizer.Abort()
		_ = staticFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		t.Fatalf("flow.Build: %v", err)
	}
	assembly, err := flow.Assemble(sourceFinalizer, staticFinalizer, moduleFinalizer, flowDraft, entry)
	if err != nil {
		t.Fatalf("flow.Assemble: %v", err)
	}
	return assembly
}

func rootFamilySpans(name string, counts [keyspace.FamilyCount]uint32) []source.FamilySpans {
	rows := make([]source.FamilySpans, 0, int(keyspace.FamilyCount)-1)
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, counts[family])
		for index := range spans {
			line := uint32(index + 1)
			spans[index] = source.Span{
				File: name, StartLine: line, StartCol: 1,
				EndLine: line, EndCol: 1,
			}
		}
		rows = append(rows, source.FamilySpans{Family: family, Spans: spans})
	}
	return rows
}
