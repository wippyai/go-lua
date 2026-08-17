package artifact_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	artifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/target"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	"github.com/wippyai/go-lua/internal/testfixture"
)

func TestV20RoundTripRebuildsDerivedOwners(t *testing.T) {
	contract := mustProfile(t)
	original := mustLower(t, "v20-roundtrip.lua", `
local function f(x)
  return x
end
return {f = f}, f(1)
`)
	metadata := artifact.Metadata{
		Provenance: "sha256:v20-test",
		Dependencies: []artifact.Dependency{
			{Name: "z-module", ID: original.ContentID()},
			{Name: "a-module", ID: original.ContentID()},
		},
	}
	want, err := artifact.Encode(original, contract, metadata)
	if err != nil {
		t.Fatal(err)
	}
	replayed, got, err := artifact.Decode(want, contract, metadata.Dependencies)
	if err != nil {
		t.Fatal(err)
	}
	if replayed == nil || replayed.ContentID() != original.ContentID() {
		t.Fatalf("replayed Program identity = %v, want %v", replayed.ContentID(), original.ContentID())
	}
	if got.Provenance != metadata.Provenance || len(got.Dependencies) != 2 ||
		got.Dependencies[0].Name != "a-module" || got.Dependencies[1].Name != "z-module" {
		t.Fatalf("decoded metadata = %#v, want canonical dependency order", got)
	}
	if original.Source().Identity().TermCount() != replayed.Source().Identity().TermCount() {
		t.Fatal("Source authored denominator was not rebuilt")
	}
	if original.Flow().Outcomes().Count() != replayed.Flow().Outcomes().Count() ||
		original.Flow().Executable().Count() != replayed.Flow().Executable().Count() {
		t.Fatal("Flow-derived projections were not rebuilt from authored sections")
	}
	if original.Module().Entry().ReturnCount() != replayed.Module().Entry().ReturnCount() {
		t.Fatal("Module Entry projection was not rebuilt")
	}
	again, err := artifact.Encode(replayed, contract, got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, again) {
		t.Fatal("v20 roundtrip changed canonical bytes")
	}
}

func TestV20RejectsV19VersionAndTrailingBytes(t *testing.T) {
	contract := mustProfile(t)
	encoded, err := artifact.Encode(mustLower(t, "v20-version.lua", `return 1`), contract, artifact.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	version := cloneEventPayload(encoded, 1, func(payload []byte) { payload[0] = 19 })
	if _, _, err := artifact.Decode(version, contract, nil); !errors.Is(err, artifact.ErrNoncanonical) {
		t.Fatalf("v19 artifact version error = %v, want noncanonical", err)
	}
	trailing := append(append([]byte(nil), encoded...), 0)
	if _, _, err := artifact.Decode(trailing, contract, nil); !errors.Is(err, artifact.ErrNoncanonical) {
		t.Fatalf("trailing byte error = %v, want noncanonical", err)
	}
}

func TestV20RejectsTargetClaimedIDAndEntryMutation(t *testing.T) {
	contract := mustProfile(t)
	encoded, err := artifact.Encode(mustLower(t, "v20-mutation.lua", `return 1`), contract, artifact.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	other, err := target.Seal(&target.Spec{Semantics: domaincontract.NewSemantics()})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := artifact.Decode(encoded, other, nil); !errors.Is(err, artifact.ErrTargetMismatch) {
		t.Fatalf("different target error = %v, want target mismatch", err)
	}
	targetMutation := cloneEventPayload(encoded, 3, func(payload []byte) { payload[0] ^= 1 })
	if _, _, err := artifact.Decode(targetMutation, contract, nil); !errors.Is(err, artifact.ErrTargetMismatch) {
		t.Fatalf("mutated target error = %v, want target mismatch", err)
	}
	claimedMutation := cloneEventPayload(encoded, 4, func(payload []byte) { payload[0] ^= 1 })
	if _, _, err := artifact.Decode(claimedMutation, contract, nil); !errors.Is(err, artifact.ErrNoncanonical) {
		t.Fatalf("mutated claimed Program ID error = %v, want noncanonical", err)
	}
	entryMutation := cloneEventPayload(encoded, 5, func(payload []byte) { payload[0] ^= 1 })
	if _, _, err := artifact.Decode(entryMutation, contract, nil); !errors.Is(err, artifact.ErrNoncanonical) {
		t.Fatalf("mutated Entry error = %v, want noncanonical", err)
	}
}

func TestV20CanonicalizesAndValidatesDependencies(t *testing.T) {
	contract := mustProfile(t)
	p := mustLower(t, "v20-dependencies.lua", `return 1`)
	first, err := artifact.Encode(p, contract, artifact.Metadata{Dependencies: []artifact.Dependency{
		{Name: "z", ID: p.ContentID()}, {Name: "a", ID: p.ContentID()},
	}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := artifact.Encode(p, contract, artifact.Metadata{Dependencies: []artifact.Dependency{
		{Name: "a", ID: p.ContentID()}, {Name: "z", ID: p.ContentID()},
	}})
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("dependency permutation changed canonical bytes: %v", err)
	}
	duplicate, err := artifact.Encode(p, contract, artifact.Metadata{Dependencies: []artifact.Dependency{
		{Name: "same", ID: p.ContentID()}, {Name: "same", ID: p.ContentID()},
	}})
	if err == nil || duplicate != nil {
		t.Fatalf("duplicate dependency result = %v/%v, want rejection", duplicate, err)
	}
	// Names are one-byte frames in this fixture. Swapping their payloads keeps
	// framing valid but violates the envelope's strict canonical order.
	noncanonical := cloneEventPayload(first, 9, func(payload []byte) { payload[0] = 'z' })
	noncanonical = cloneEventPayload(noncanonical, 12, func(payload []byte) { payload[0] = 'a' })
	if _, _, err := artifact.Decode(noncanonical, contract, []artifact.Dependency{
		{Name: "a", ID: p.ContentID()}, {Name: "z", ID: p.ContentID()},
	}); !errors.Is(err, artifact.ErrNoncanonical) {
		t.Fatalf("out-of-order dependencies error = %v, want noncanonical", err)
	}
}

func TestV20RequiresExactExpectedDependencyManifest(t *testing.T) {
	contract := mustProfile(t)
	p := mustLower(t, "v20-manifest.lua", `return 1`)
	metadata := artifact.Metadata{Dependencies: []artifact.Dependency{{Name: "a", ID: p.ContentID()}}}
	encoded, err := artifact.Encode(p, contract, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := artifact.Decode(encoded, contract, nil); !errors.Is(err, artifact.ErrDependencyMismatch) {
		t.Fatalf("missing expected dependency manifest error = %v, want mismatch", err)
	}
	wrong := []artifact.Dependency{{Name: "b", ID: p.ContentID()}}
	if _, _, err := artifact.Decode(encoded, contract, wrong); !errors.Is(err, artifact.ErrDependencyMismatch) {
		t.Fatalf("wrong expected dependency manifest error = %v, want mismatch", err)
	}
}

func TestV20CanonicalDependencyRowsPrecedeManifestCountMismatch(t *testing.T) {
	contract := mustProfile(t)
	p := mustLower(t, "v20-manifest-order.lua", `return 1`)
	encoded, err := artifact.Encode(p, contract, artifact.Metadata{Dependencies: []artifact.Dependency{
		{Name: "a", ID: p.ContentID()}, {Name: "z", ID: p.ContentID()},
	}})
	if err != nil {
		t.Fatal(err)
	}
	// The envelope remains a two-row stream, but its names are now out of
	// canonical order. Passing an empty manifest must not hide that grammar
	// error behind the count mismatch.
	noncanonical := cloneEventPayload(encoded, 9, func(payload []byte) { payload[0] = 'z' })
	noncanonical = cloneEventPayload(noncanonical, 12, func(payload []byte) { payload[0] = 'a' })
	if _, _, err := artifact.Decode(noncanonical, contract, nil); !errors.Is(err, artifact.ErrNoncanonical) {
		t.Fatalf("noncanonical rows with count mismatch error = %v, want noncanonical", err)
	}
}

func TestV20RejectsOversizeProvenanceBeforeWriting(t *testing.T) {
	contract := mustProfile(t)
	p := mustLower(t, "v20-provenance-limit.lua", `return 1`)
	data, err := artifact.Encode(p, contract, artifact.Metadata{Provenance: strings.Repeat("p", (64<<20)+1)})
	if !errors.Is(err, artifact.ErrLimit) || data != nil {
		t.Fatalf("oversize provenance result = %d/%v, want nil/resource limit", len(data), err)
	}
}

type wireEvent struct {
	payloadStart int
	payloadEnd   int
}

func cloneEventPayload(data []byte, index int, mutate func([]byte)) []byte {
	copyData := append([]byte(nil), data...)
	// The helper is called only with indexes from a valid v20 envelope. Keep
	// the panic useful if a future envelope layout changes this test law.
	parsed := parseEvents(copyData)
	if index < 0 || index >= len(parsed) {
		panic("artifact test event index out of range")
	}
	payload := copyData[parsed[index].payloadStart:parsed[index].payloadEnd]
	mutate(payload)
	return copyData
}

func parseEvents(data []byte) []wireEvent {
	var result []wireEvent
	for offset := 0; offset < len(data); {
		offset++
		length, width := binary.Uvarint(data[offset:])
		if width <= 0 {
			panic("artifact test malformed event length")
		}
		offset += width
		end := offset + int(length)
		if end < offset || end > len(data) {
			panic("artifact test malformed event payload")
		}
		result = append(result, wireEvent{payloadStart: offset, payloadEnd: end})
		offset = end
	}
	return result
}

func mustProfile(t *testing.T) *target.Contract {
	t.Helper()
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func mustLower(t *testing.T, name, text string) *program.Program {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: name, Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
