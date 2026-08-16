package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/coordinate"
	"github.com/wippyai/go-lua/analysis/engine/internal/dependency"
	"github.com/wippyai/go-lua/analysis/engine/internal/fiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/link"
)

// TestBodyCacheAdmissionRejectsCombinedSelectorConflict is the exact cache
// admission law for the three-record conflict.  The published X=0 record and
// pending X=1 record establish X as the canonical selector.  A later Y-only
// record is individually acceptable to each old index, but cannot enter the
// evolved canonical index because it has no X read.  It must stay an ordinary
// cold result before its Fiber root is retained.
func TestBodyCacheAdmissionRejectsCombinedSelectorConflict(t *testing.T) {
	key := bodyProjectionKey{
		body:  bodyOrigin{shard: link.Shard(1), body: program.Term(1)},
		term:  program.Term(1),
		local: true,
	}
	projection := bodyProjection{key: key, outputs: []int{0}}
	published := newBodyReuse()
	pending := newBodyAdmissionBatch()

	xZero := bodySpecialization{reads: []bodyProjectionRead{bodyCacheAdmissionRead(0, 0, 0)}}
	xOne := bodySpecialization{reads: []bodyProjectionRead{bodyCacheAdmissionRead(0, 0, 1)}}
	yOnly := bodySpecialization{reads: []bodyProjectionRead{bodyCacheAdmissionRead(1, 0, 0)}}
	if !published.appendKey(key, xZero) {
		t.Fatal("publish first cache record")
	}
	if !published.accepts(projection, xOne) {
		t.Fatal("second record must be individually admissible to old canonical index")
	}
	admission, ok := pending.admit(published, projection, xOne)
	if !ok || !pending.commit(admission) {
		t.Fatal("stage second cache record")
	}
	if !published.accepts(projection, yOnly) || !pending.records.accepts(projection, yOnly) {
		t.Fatal("third record must expose the old independent-precheck defect")
	}
	if _, ok := pending.admit(published, projection, yOnly); ok {
		t.Fatal("combined canonical admission accepted selector-incompatible third record")
	}
}

func bodyCacheAdmissionRead(slot int, key, fingerprint uint64) bodyProjectionRead {
	return bodyProjectionRead{
		slot:        slot,
		key:         key,
		fingerprint: fingerprint,
		observe: func(*transaction, dependency.Equation, coordinate.Coordinate, guard.Guard, fiber.Leaf) (bodyProjectionSample, bool) {
			return bodyProjectionSample{}, false
		},
		validate: func(*transaction, dependency.Equation, coordinate.Coordinate, guard.Guard, fiber.Leaf) bool {
			return false
		},
	}
}
