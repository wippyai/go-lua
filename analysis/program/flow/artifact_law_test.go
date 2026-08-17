package flow

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/internal/framing"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestFlowArtifactSectionTopLevelPayloadAndDerivedExclusion(t *testing.T) {
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	sourceFinalizer, staticFinalizer, moduleFinalizer, draft := emptyAssemblyOwners(t, "flow-artifact-law.lua")
	assembly, err := Assemble(sourceFinalizer, staticFinalizer, moduleFinalizer, draft, entry)
	if err != nil {
		t.Fatal(err)
	}
	_, component, _, _, err := assembly.Take()
	if err != nil {
		t.Fatal(err)
	}
	if component == nil {
		t.Fatal("assembly returned no Flow component")
	}

	base := encodeTopLevelArtifactSection(t, component)
	changed := *component
	changed.provenance.Source[0] ^= 0xff
	changed.provenance.Flow[0] ^= 0xff
	changed.provenance.Static[0] ^= 0xff
	changed.provenance.Module[0] ^= 0xff
	changed.activation = activationProjection{terms: []keyspace.Term{entry}}
	changed.containment = containmentProjection{
		terms:   []keyspace.Term{entry},
		parents: []keyspace.Term{0},
		static:  []bool{true},
	}
	changed.outcomes = nil
	changed.ports = nil
	changed.pending = nil
	changed.executable = nil
	changed.directFunction = nil
	changed.candidates = nil
	changed.accessGeometry = nil
	changed.programStructure.causal = nil
	changed.continuation = nil
	if got := encodeTopLevelArtifactSection(t, &changed); !bytes.Equal(got, base) {
		t.Fatal("derived or provenance mutation changed authored artifact bytes")
	}

	var framed bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&framed, "program/flow-test", 1); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(0x61); err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifactSection(&writer, component.View()); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(0x62); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	reader, err := framing.NewReader(framed.Bytes(), framed.Len())
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Header("program/flow-test", 1); err != nil {
		t.Fatal(err)
	}
	if got, err := reader.Record(); err != nil || got != 0x61 {
		t.Fatalf("prefix sentinel = %d, %v; want 0x61", got, err)
	}
	decoded, err := ReadArtifactSection(reader)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Counts != ([keyspace.FamilyCount]uint32{}) {
		t.Fatalf("decoded Counts = %#v; want zero", decoded.Counts)
	}
	if got, err := reader.Record(); err != nil || got != 0x62 {
		t.Fatalf("suffix sentinel = %d, %v; want 0x62", got, err)
	}
	if err := reader.Finish(); err != nil {
		t.Fatal(err)
	}
}

func encodeTopLevelArtifactSection(t *testing.T, component *Component) []byte {
	t.Helper()
	var buffer bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&buffer, "program/flow-test", 1); err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifactSection(&writer, component.View()); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), buffer.Bytes()...)
}
