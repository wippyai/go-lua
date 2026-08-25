package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

// requirementCorpus is the call-geometry corpus the declared-admissibility law
// is stated over. Each source names one authored call shape the cold placement
// projection must decide about: the strict unary plain call a runtime-kind
// operand is sealed for, and the method, arity, and tail shapes it is not.
var requirementCorpus = []struct {
	name string
	// runtimeKind is whether the fixture's call carries the strict unary plain
	// geometry Value seals a runtime-kind operand for. Stating it keeps the
	// agreement law from being satisfied by a projection that places nothing.
	runtimeKind bool
	source      string
}{
	{"plain-unary", true, "local function kindof(value) return value end; local held = kindof(1); return held"},
	{"method-unary", false, "local function handle(channel) channel:recv() end; return handle"},
	{"plain-binary", false, "local function pair(left, right) return left end; return pair(1, 2)"},
	{"plain-nullary", false, "local function now() return 1 end; return now()"},
	{"plain-tail", false, "local function spread(...) return ... end; local function pass(...) return spread(...) end; return pass"},
	{"method-binary", false, "local function put(store, key) store:set(key, 1) end; return put"},
}

// TestSealedPlacementSetEqualsOwnerOperandSet is the declared-admissibility
// law at this package's altitude. A cold issuance placement and the sealed
// rule table are one denominator: every placement the artifact carries names a
// rule the table publishes a construction primitive for, and no placement is
// left for the construction plane to discover and drop.
//
// A placement no rule publishes a primitive for is a rule the artifact
// promised rows for and no owner can execute, so the disagreement is a defect
// in the cold projection rather than a case the admission walk may skip. The
// activation plane is the one declared exception: it admits through its own
// request and publishes no primitive.
func TestSealedPlacementSetEqualsOwnerOperandSet(t *testing.T) {
	for _, fixture := range requirementCorpus {
		t.Run(fixture.name, func(t *testing.T) {
			record := mountedRecord(t, "issuance-requirement-"+fixture.name, fixture.source)
			rules := materializerBinding(t, record).Rules()
			if rules == nil {
				t.Fatal("sealed rule binding")
			}
			const runtimeKind = schema.Key("value-runtime-kind-call")
			placements, runtimeKindPlacements := 0, 0
			unsealed := make(map[schema.Key]int)
			key, ok := WalkSealedPlacements(record.Artifacts, func(key schema.Key, mount, point, occurrence identity.ContentID) bool {
				placements++
				if key == runtimeKind {
					runtimeKindPlacements++
				}
				// Every rule reaches this walk as a generated cell under a
				// mounted capability, activation included. The exemption that
				// stood here was for a HAND-WIRED activation cell, whose
				// payload was a rule-owned callback rather than a slot and
				// which therefore had no mounted capability to be sealed
				// under. There is no such cell any more.
				cell, cellOK := rules.cellByKey(key)
				capability, capabilityOK := rules.CapabilityByKey(key)
				if !cellOK || !cell.Available() || !capabilityOK || !capability.Mounted() || capability.Activation() {
					unsealed[key]++
				}
				return true
			})
			if !ok {
				t.Fatalf("sealed placement walk refused at %q", key)
			}
			if placements == 0 {
				t.Fatal("fixture issued no sealed placements")
			}
			for placed, count := range unsealed {
				t.Errorf("rule %q is placed %d times and publishes no construction primitive", placed, count)
			}
			// Agreement is only worth stating over a projection that still
			// places the shapes it should: a directory that placed nothing
			// would satisfy the loop above and issue no rule at all.
			if (runtimeKindPlacements > 0) != fixture.runtimeKind {
				t.Fatalf("runtime-kind placements=%d, the fixture carries the strict unary plain shape=%v", runtimeKindPlacements, fixture.runtimeKind)
			}
		})
	}
}
