package core

import (
	"bytes"
	"testing"
	"unsafe"

	"github.com/wippyai/go-lua/program/internal/canonical"
)

// These are decoder laws, not implementation-composition tests. Each input
// is independently hostile; opening it must fail without a panic, Program,
// or partial byte result.
func TestArtifactRejectsResourceCountsBeforeAllocation(t *testing.T) {
	target := artifactSecurityID(1)
	for _, count := range []uint64{1, uint64(indexMax)} {
		data := artifactRootPrefix(t, target, count)
		artifactMustReject(t, data, target)
	}
}

// TestArtifactEquationAllocationWeightsArePortableAndBounded keeps the
// format-level reconstruction budget independent of the host while proving
// the fixed weights remain conservative for this implementation. The hostile
// count is rejected before a cache slice is made; it is below the generic
// event ceiling, so this exercises typed-memory rather than wire-count
// admission.
func TestArtifactEquationBudgetMathIsPortableAndBounded(t *testing.T) {
	for _, row := range []struct {
		name   string
		weight uint64
		actual uintptr
	}{
		{name: "semantic", weight: artifactEquationSemanticBytes, actual: unsafe.Sizeof(ArtifactSemanticKey{})},
		{name: "read", weight: artifactEquationReadBytes, actual: unsafe.Sizeof(ArtifactEquationRead{})},
		{name: "term", weight: artifactEquationTermBytes, actual: unsafe.Sizeof(Term(0))},
		{name: "body", weight: artifactEquationBodyBytes, actual: unsafe.Sizeof(ArtifactEquationBody{})},
		{name: "edge", weight: artifactEquationEdgeBytes, actual: unsafe.Sizeof(ArtifactEquationEdge{})},
		{name: "boundary", weight: artifactEquationBoundaryBytes, actual: unsafe.Sizeof(ArtifactEquationBoundary{})},
		{name: "cache", weight: artifactEquationCacheBytes, actual: unsafe.Sizeof(ArtifactEquationCache{})},
	} {
		if row.weight < uint64(row.actual) {
			t.Fatalf("%s reconstruction weight %d below implementation width %d", row.name, row.weight, row.actual)
		}
	}
	for _, row := range []struct {
		name   string
		weight uint64
	}{
		{name: "edge", weight: artifactEquationEdgeBytes},
		{name: "read", weight: artifactEquationReadBytes},
		{name: "outer-cache-vector", weight: artifactEquationCacheBytes},
	} {
		count := uint64(artifactMaxEquationBytes/row.weight) + 1
		if count >= artifactMaxEvents {
			t.Fatalf("%s adversary did not isolate the byte boundary", row.name)
		}
		var budget artifactEquationBudget
		if budget.reserve(count, row.weight) {
			t.Fatalf("%s crossed the fixed reconstruction budget", row.name)
		}
	}
}

// A derived semantic cut must not silently accept an artifact written under
// the earlier Program contract. Decode always reseals authored rows, but it
// may do so only after the wire version has selected the same contract.
func TestArtifactRejectsV7Codec(t *testing.T) {
	target := artifactSecurityID(1)
	var data bytes.Buffer
	var writer canonical.Writer
	if err := writer.Reset(&data, artifactCodecDomain, 7); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	artifactMustReject(t, data.Bytes(), target)
}

func TestArtifactEquationReadCoordinatesRejectNativeIntOverflow(t *testing.T) {
	for _, row := range []struct {
		name       string
		inputArity uint64
		position   uint64
	}{
		{name: "arity", inputArity: ^uint64(0)},
		{name: "position", inputArity: 1, position: ^uint64(0)},
	} {
		t.Run(row.name, func(t *testing.T) {
			data := artifactEquationBoundaryOverflowStream(t, row.inputArity, row.position)
			r, err := canonical.NewReader(data, artifactMaxBytes)
			if err != nil {
				t.Fatal(err)
			}
			if err := r.Header(artifactCodecDomain, artifactCodecVersion); err != nil {
				t.Fatal(err)
			}
			decoder := artifactDecoder{r: r}
			if _, err := decoder.equationCache(); err == nil {
				t.Fatal("equation cache accepted a coordinate outside native int")
			}
		})
	}
}

func artifactEquationBoundaryOverflowStream(t *testing.T, inputArity, position uint64) []byte {
	t.Helper()
	var data bytes.Buffer
	var writer canonical.Writer
	if err := writer.Reset(&data, artifactCodecDomain, artifactCodecVersion); err != nil {
		t.Fatal(err)
	}
	programID := artifactSecurityID(1)
	moduleID := artifactSecurityID(2)
	factor := ArtifactSemanticKey{ID: artifactSecurityID(3), Version: 1}
	rule := ArtifactSemanticKey{ID: artifactSecurityID(4), Version: 1}
	engine := ArtifactSemanticKey{ID: artifactSecurityID(5), Version: 1}
	writeSemantic := func(key ArtifactSemanticKey) {
		t.Helper()
		if err := writer.Bytes(key.ID[:]); err != nil {
			t.Fatal(err)
		}
		if err := writer.Uint(key.Version); err != nil {
			t.Fatal(err)
		}
	}
	for _, err := range []error{
		writer.Record(artifactRecordEquationCache),
		writer.Bytes(programID[:]),
		writer.Bytes(moduleID[:]),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	writeSemantic(engine)
	if err := writer.Count(1); err != nil {
		t.Fatal(err)
	}
	writeSemantic(factor)
	if err := writer.Count(1); err != nil {
		t.Fatal(err)
	}
	writeSemantic(rule)
	if err := writer.Count(0); err != nil {
		t.Fatal(err)
	}
	if err := writer.Count(1); err != nil {
		t.Fatal(err)
	}
	for _, err := range []error{
		writer.Record(artifactRecordEquationBoundary),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	writeSemantic(rule)
	writeSemantic(factor)
	for _, err := range []error{
		writer.Uint(uint64(tagBody)),
		writer.Uint(1),
		writer.Uint(uint64(tagBody)),
		writer.Uint(1),
		writer.Uint(inputArity),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if inputArity != ^uint64(0) {
		if err := writer.Count(1); err != nil {
			t.Fatal(err)
		}
		if err := writer.Uint(position); err != nil {
			t.Fatal(err)
		}
		writeSemantic(factor)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestArtifactRejectsEnvelopeOrderAndDuplicateOnDecode(t *testing.T) {
	target := artifactSecurityID(1)
	for _, names := range [][]string{{"z", "a"}, {"same", "same"}} {
		var data bytes.Buffer
		var writer canonical.Writer
		if err := writer.Reset(&data, artifactCodecDomain, artifactCodecVersion); err != nil {
			t.Fatal(err)
		}
		artifactWriteRootPrefix(t, &writer, target, uint64(len(names)))
		for _, name := range names {
			if err := writer.Record(artifactRecordDependency); err != nil {
				t.Fatal(err)
			}
			if err := writer.String(name); err != nil {
				t.Fatal(err)
			}
			dependency := artifactSecurityID(2)
			if err := writer.Bytes(dependency[:]); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Finish(); err != nil {
			t.Fatal(err)
		}
		artifactMustReject(t, data.Bytes(), target)
	}
}

func artifactRootPrefix(t *testing.T, target ContentID, dependencies uint64) []byte {
	t.Helper()
	var data bytes.Buffer
	var writer canonical.Writer
	if err := writer.Reset(&data, artifactCodecDomain, artifactCodecVersion); err != nil {
		t.Fatal(err)
	}
	artifactWriteRootPrefix(t, &writer, target, dependencies)
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func artifactWriteRootPrefix(t *testing.T, writer *canonical.Writer, target ContentID, dependencies uint64) {
	t.Helper()
	claim := artifactSecurityID(2)
	for _, err := range []error{
		writer.Record(artifactRecordRoot),
		writer.Bytes(target[:]),
		writer.Bytes(claim[:]),
		writer.String("revision"),
		writer.Count(dependencies),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func artifactSecurityID(value byte) ContentID {
	var id ContentID
	id[0] = value
	return id
}

func artifactMustReject(t *testing.T, data []byte, target ContentID) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("artifact decode panicked: %v", recovered)
		}
	}()
	p, _, err := DecodeArtifact(data, target)
	if err == nil || p != nil {
		t.Fatalf("artifact decode = %v, %v; want rejection", p, err)
	}
}
