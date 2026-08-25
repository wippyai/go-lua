package call

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestMaySuspendUsesCallTargetAuthority(t *testing.T) {
	_, _, algebra := targetOperationLawAlgebra(t)
	application := identity.ContentID{90}
	algebra.keys = []keyRow{{kind: keyApplication, id: application}}
	algebra.keyIndex = map[identity.ContentID]uint32{application: 1}
	key, keyOK := algebra.KeyForApplicationID(application)
	operation, operationOK := algebra.TargetForSeedID(identity.ContentID{1})
	if !keyOK || !operationOK {
		t.Fatal("call suspension fixture")
	}
	closed, closedOK := algebra.DispatchValue(key, []Target{operation}, false)
	if !closedOK {
		t.Fatal("closed call fact")
	}
	if may, ok := algebra.MaySuspend(key, closed); !ok || may {
		t.Fatalf("closed synchronous operation = %t/%t, want false/true", may, ok)
	}
	open, openOK := algebra.DispatchValue(key, []Target{operation}, true)
	if !openOK {
		t.Fatal("open call fact")
	}
	if may, ok := algebra.MaySuspend(key, open); !ok || !may {
		t.Fatalf("opaque call alternative = %t/%t, want true/true", may, ok)
	}

	bodyRole, roleOK := newTargetRoleID(TargetRoleBody, identity.ContentID{91})
	if !roleOK {
		t.Fatal("body role")
	}
	algebra.targets[1] = targetRow{kind: targetBody, role: bodyRole}
	body, bodyOK := algebra.targetForSelector(2)
	bodyFact, factOK := algebra.DispatchValue(key, []Target{body}, false)
	if !bodyOK || !factOK {
		t.Fatal("body call fact")
	}
	if may, ok := algebra.MaySuspend(key, bodyFact); !ok || !may {
		t.Fatalf("body alternative = %t/%t, want true/true", may, ok)
	}
}
