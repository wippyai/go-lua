package publicationfreeze

import (
	"testing"

	packdomain "github.com/wippyai/go-lua/domain/pack"

	"github.com/wippyai/go-lua/analysis/identity"
)

// A publication's subject is a semantic the program authored; a call's actuals
// are the semantics Pack mounted for that call. The two are not the same set:
// Value keys a mounted coordinate by (module, semantic) over every mounted
// semantic of the module, not over the call's actuals alone, so a subject that
// is no actual of its call is a shape this rule must answer for rather than a
// case it may assume away.
//
// The answer is the empty valid plan - the same answer the rule already gives
// a subject whose Value fact is not exact - and it is stated here by name so a
// change to it is a change to this law rather than a silent consequence of how
// the subject vector is read.
func TestASubjectThatIsNoActualOfItsCallIsNotCarriedByTheVector(t *testing.T) {
	semantic := identity.ContentID([32]byte{1})

	// A projection that mounts no actual carries no subject, which is the
	// boundary case of the same statement: the vector has no cell to offer.
	if _, carried := actualOrdinalFor(packdomain.MountedActualProjection{}, semantic); carried {
		t.Fatal("a projection with no actuals reported a subject as carried")
	}

	// An unavailable semantic is not a subject at all, and is never reported
	// as carried by some ordinal.
	if _, carried := actualOrdinalFor(packdomain.MountedActualProjection{}, identity.ContentID{}); carried {
		t.Fatal("an unavailable semantic resolved to an actual ordinal")
	}
}
