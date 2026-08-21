package observation

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/internal/framing"
)

type scratchEntry struct{ key schema.Key }

func (entry scratchEntry) Key() schema.Key { return entry.key }

func (entry scratchEntry) EntryAvailable() bool { return entry.key.Available() }

func (entry scratchEntry) EntryContent(*framing.Writer) error { return nil }

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

func (scratchSurface) Seal(schema.View, schema.Sealed) schema.SealFailure {
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

func testStructure(t *testing.T) schema.Surface {
	t.Helper()
	specs := make([]structure.Spec, 0, int(structure.CategoryConformanceVerdict))
	for category := structure.CategoryArm; category <= structure.CategoryConformanceVerdict; category++ {
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

func sealObservation(t *testing.T, contribution schema.Surface) (*schema.Schema, schema.SealFailure) {
	t.Helper()
	builder := schema.NewBuilder()
	for kind := schema.SurfaceKindInvalid + 1; kind.Available(); kind++ {
		switch kind {
		case schema.SurfaceKindStructure:
			builder.Register(testStructure(t))
		case schema.SurfaceKindQuery:
			builder.Register(scratchSurface{kind: kind, keys: []schema.Key{testProducer}})
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
	if sealed != nil || failure.Law != schema.LawEntryUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("duplicate observation verdict = table=%v law=%d disposition=%s", sealed != nil, failure.Law, failure.Disposition)
	}
}
