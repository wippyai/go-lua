package observation

import (
	"fmt"
	"testing"

	seal "github.com/wippyai/go-lua/analysis/schema/seal"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/internal/framing"
)

type scratchEntry struct{ key schema.Key }

func (entry scratchEntry) Key() schema.Key { return entry.key }

func (entry scratchEntry) EntryAvailable() bool { return entry.key.Available() }

func (entry scratchEntry) EntryContent(*framing.Writer) error { return nil }

func (scratchEntry) ProducerEnvelope() (query.ProducerEnvelope, bool) {
	codec, codecOK := vocabulary.Key("query-result/test")
	envelope := query.ProducerEnvelope{Population: query.PopulationKindSelectedPoint, Codec: codec}
	return envelope, codecOK && envelope.Available()
}

// producerEntry lets a boundary law issue a deliberately different owner
// envelope without weakening the ordinary scratch producer used by the rest
// of this file.
type producerEntry struct {
	scratchEntry
	envelope query.ProducerEnvelope
	ok       bool
}

func (entry producerEntry) ProducerEnvelope() (query.ProducerEnvelope, bool) {
	return entry.envelope, entry.ok
}

type producerSurface struct {
	envelope query.ProducerEnvelope
	ok       bool
}

func (producerSurface) Kind() schema.SurfaceKind { return schema.SurfaceKindQuery }

func (surface producerSurface) Entries() []schema.Entry {
	return []schema.Entry{producerEntry{
		scratchEntry: scratchEntry{key: testProducer},
		envelope:     surface.envelope,
		ok:           surface.ok,
	}}
}

func (producerSurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

type scratchSurface struct {
	kind schema.SurfaceKind
	keys []schema.Key
}

func (contribution scratchSurface) Kind() schema.SurfaceKind { return contribution.kind }

func (contribution scratchSurface) Entries() []schema.Entry {
	entries := make([]schema.Entry, len(contribution.keys))
	for index, key := range contribution.keys {
		entries[index] = scratchEntry{key: key}
	}
	return entries
}

func (scratchSurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

const (
	testProducer    schema.Key = "producer"
	testRelation    schema.Key = "relation"
	testObservation            = "test/branch-observation"
	testPopulation             = "test/diagnostic-observation"
	testCodec                  = "semantic/query-result/test"
	testGeometry               = "semantic/observation/geometry/test"
	testAnchor                 = "semantic/observation/anchor/test"
)

func testStructure(t *testing.T) seal.Surface {
	t.Helper()
	specs := make([]structure.Spec, 0, int(structure.CategoryNativeSendSafety))
	for category := structure.CategoryArm; category <= structure.CategoryNativeSendSafety; category++ {
		specs = append(specs, structure.Spec{
			Key:      schema.Key(fmt.Sprintf("test/category/%d", category)),
			Category: category,
			Spelling: fmt.Sprintf("test-%d", category),
			Accepted: true,
		})
	}
	specs = append(specs,
		structure.Spec{Key: testPopulation, Category: structure.CategoryDiagnosticObservation, Spelling: "test-observation", Accepted: true},
		structure.Spec{Key: schema.Key(testCodec), Category: structure.CategorySemanticRole, Spelling: "query-result/test", Accepted: true},
		structure.Spec{Key: schema.Key(testGeometry), Category: structure.CategorySemanticRole, Spelling: "observation/geometry/test", Accepted: true},
		structure.Spec{Key: schema.Key(testAnchor), Category: structure.CategorySemanticRole, Spelling: "observation/anchor/test", Accepted: true},
	)
	entries, ok := structure.Collect(specs)
	if !ok {
		t.Fatal("test structure rejected")
	}
	return structure.NewSurface(entries)
}

func sealObservation(t *testing.T, contribution seal.Surface) (*seal.Schema, schema.SealFailure) {
	t.Helper()
	return sealObservationWithProducer(t, contribution, scratchSurface{kind: schema.SurfaceKindQuery, keys: []schema.Key{testProducer}})
}

func sealObservationWithProducer(t *testing.T, contribution seal.Surface, producer seal.Surface) (*seal.Schema, schema.SealFailure) {
	t.Helper()
	builder := seal.NewBuilder()
	for kind := schema.SurfaceKindInvalid + 1; kind.Available(); kind++ {
		switch kind {
		case schema.SurfaceKindStructure:
			builder.Register(testStructure(t))
		case schema.SurfaceKindQuery:
			builder.Register(producer)
		case schema.SurfaceKindDenominator:
			builder.Register(scratchSurface{kind: kind, keys: []schema.Key{testRelation}})
		case schema.SurfaceKindObservation:
			builder.Register(contribution)
		default:
			builder.Register(scratchSurface{kind: kind, keys: []schema.Key{"scratch"}})
		}
	}
	return builder.Seal()
}

func testSpec() Spec {
	return Spec{
		Key:      testObservation,
		Producer: Reference{Surface: schema.SurfaceKindQuery, Key: testProducer},
		Population: Population{
			Relation: Reference{Surface: schema.SurfaceKindDenominator, Key: testRelation},
			Kind:     Reference{Surface: schema.SurfaceKindStructure, Key: testPopulation},
		},
		Geometry: Reference{Surface: schema.SurfaceKindStructure, Key: vocabulary.RoleKey("observation/geometry/test")},
		Anchor:   Reference{Surface: schema.SurfaceKindStructure, Key: vocabulary.RoleKey("observation/anchor/test")},
		Codec:    Reference{Surface: schema.SurfaceKindStructure, Key: schema.Key(testCodec)},
	}
}

func mustEntry(t *testing.T, spec Spec) *Entry {
	t.Helper()
	entry, ok := New(spec)
	if !ok {
		t.Fatal("observation spec rejected")
	}
	return entry
}

func TestObservationAdmissionAndSeal(t *testing.T) {
	entry := mustEntry(t, testSpec())
	sealed, failure := sealObservation(t, NewSurface([]*Entry{entry}))
	if failure.Available() || sealed == nil {
		t.Fatalf("observation row rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindObservation)
	if !viewOK || view.Count() != 1 {
		t.Fatal("sealed observation row is not reachable")
	}
}

func TestObservationRejectsWrongProducerSurfaceAtAdmission(t *testing.T) {
	spec := testSpec()
	spec.Producer.Surface = schema.SurfaceKindAxis
	if _, ok := New(spec); ok {
		t.Fatal("observation admitted a producer that is not a query family")
	}
}

func TestObservationReportsUnresolvedProducer(t *testing.T) {
	spec := testSpec()
	spec.Producer.Key = "missing"
	sealed, failure := sealObservation(t, NewSurface([]*Entry{mustEntry(t, spec)}))
	if sealed != nil || failure.Law != LawProducerResolves || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("unresolved producer verdict = table=%v law=%d disposition=%s", sealed != nil, failure.Law, failure.Disposition)
	}
}

func TestObservationRejectsProducerCodecDrift(t *testing.T) {
	spec := testSpec()
	spec.Codec = Reference{Surface: schema.SurfaceKindStructure, Key: schema.Key(testAnchor)}
	sealed, failure := sealObservation(t, NewSurface([]*Entry{mustEntry(t, spec)}))
	if sealed != nil || failure.Law != LawProducerCompatibility || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("producer codec drift verdict = table=%v law=%d disposition=%s", sealed != nil, failure.Law, failure.Disposition)
	}
}

// TestObservationDoesNotConflateProducerAndDiagnosticPopulation states the
// positive cross-lane law: either query execution lane can produce an
// observation. The observation's diagnostic population is not imposed on the
// producer's own population contract.
func TestObservationDoesNotConflateProducerAndDiagnosticPopulation(t *testing.T) {
	codec, codecOK := vocabulary.Key("query-result/test")
	if !codecOK {
		t.Fatal("test codec role did not resolve")
	}
	for _, producerPopulation := range []query.PopulationKind{
		query.PopulationKindSelectedPoint,
		query.PopulationKindObservation,
	} {
		producer := producerSurface{
			envelope: query.ProducerEnvelope{Population: producerPopulation, Codec: codec},
			ok:       true,
		}
		sealed, failure := sealObservationWithProducer(t, NewSurface([]*Entry{mustEntry(t, testSpec())}), producer)
		if sealed == nil || failure.Available() {
			t.Fatalf("producer population %v rejected valid observation: law=%d disposition=%s", producerPopulation, failure.Law, failure.Disposition)
		}
	}
}

func TestObservationRejectsUnavailableProducerPopulation(t *testing.T) {
	codec, codecOK := vocabulary.Key("query-result/test")
	if !codecOK {
		t.Fatal("test codec role did not resolve")
	}
	producer := producerSurface{
		envelope: query.ProducerEnvelope{Population: query.PopulationKindInvalid, Codec: codec},
		ok:       true,
	}
	sealed, failure := sealObservationWithProducer(t, NewSurface([]*Entry{mustEntry(t, testSpec())}), producer)
	if sealed != nil || failure.Law != LawProducerCompatibility || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("invalid producer population verdict = table=%v law=%d disposition=%s", sealed != nil, failure.Law, failure.Disposition)
	}
}

func TestObservationReportsUnresolvedGeometryRole(t *testing.T) {
	spec := testSpec()
	spec.Geometry.Key = "semantic/observation/geometry/missing"
	sealed, failure := sealObservation(t, NewSurface([]*Entry{mustEntry(t, spec)}))
	if sealed != nil || failure.Law != LawGeometryResolves || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("unresolved geometry verdict = table=%v law=%d disposition=%s", sealed != nil, failure.Law, failure.Disposition)
	}
}

func TestObservationReportsDuplicateRowsAtRoot(t *testing.T) {
	entry := mustEntry(t, testSpec())
	sealed, failure := sealObservation(t, NewSurface([]*Entry{entry, entry}))
	if sealed != nil || failure.Law != seal.LawEntryUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("duplicate observation verdict = table=%v law=%d disposition=%s", sealed != nil, failure.Law, failure.Disposition)
	}
}
