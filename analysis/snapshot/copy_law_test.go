package snapshot

import "testing"

// TestSnapshotCopiesAnswerIdentically is the copy law of the published value.
// A Snapshot is a value: assigning one, placing it in a slice, putting it in
// a map, passing it through a function, and round-tripping it through an
// interface all produce something that answers exactly what the original
// answers. Consumer retention is unbounded and structurally shared, so a copy
// must never be a partial or detached view.
func TestSnapshotCopiesAnswerIdentically(t *testing.T) {
	original := newFixture(t)

	assigned := original
	slotted := []Snapshot{{}, original}
	mapped := map[string]Snapshot{"published": original}
	passed := through(original)
	boxed := any(original).(Snapshot)

	copies := map[string]*Snapshot{
		"assigned": &assigned,
		"slice":    &slotted[1],
		"map": func() *Snapshot {
			held := mapped["published"]
			return &held
		}(),
		"parameter": &passed,
		"interface": &boxed,
	}
	for name, copied := range copies {
		t.Run(name, func(t *testing.T) {
			if copied.Schema() != original.Schema() ||
				copied.Store() != original.Store() ||
				copied.Generation() != original.Generation() ||
				copied.Columns() != original.Columns() {
				t.Fatalf("copy anchors differ: %+v", copied)
			}
			for key, want := range map[string]ReadStatus{
				"present": ReadHit,
				"absent":  ReadProvenAbsent,
				"unknown": ReadMiss,
			} {
				value, status := Read(copied, totalAxis, key)
				wantValue, wantStatus := Read(&original, totalAxis, key)
				if status != want || status != wantStatus || value != wantValue {
					t.Fatalf("copy read %q = (%d, %v), want (%d, %v)", key, value, status, wantValue, wantStatus)
				}
			}
			if copied.Denominators().Len() != original.Denominators().Len() ||
				!copied.Queries().Published(fixtureQueryPlan) {
				t.Fatal("copy lost its sealed sub-values")
			}
		})
	}
}

// through passes a snapshot by value so the copy under test is a real
// parameter copy rather than an alias the compiler could elide.
func through(s Snapshot) Snapshot { return s }
