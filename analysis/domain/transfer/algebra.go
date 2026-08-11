package transfer

import (
	"crypto/sha256"
	"sort"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	"github.com/wippyai/go-lua/program/target"
)

type Algebra struct{ owner *algebra }
type algebra struct {
	source *link.Link
	linkID keyspace.ContentID
	arms   []armEntry
	id     keyspace.ContentID
	sealed bool
}
type armEntry struct{ arm Arm }

func NewAlgebra(source *link.Link) (Algebra, bool) {
	if source == nil || !source.ContentID().Available() {
		return Algebra{}, false
	}
	contract, ok := source.Boundary().Target()
	if !ok || contract == nil {
		return Algebra{}, false
	}
	entries := make([]armEntry, 0)
	for ai := 0; ai < source.Project().Applications().Count(); ai++ {
		app, ok := source.Project().Applications().At(ai)
		if !ok {
			return Algebra{}, false
		}
		for oi := 0; oi < contract.OperationCount(); oi++ {
			op := target.Operation(oi + 1)
			if !source.Boundary().ApplicationOperationAvailable(contract, app, op) {
				continue
			}
			for ti := 0; ti < contract.TransferCount(op); ti++ {
				transfer, ok := contract.TransferIDAt(op, ti)
				if !ok {
					return Algebra{}, false
				}
				for outcome := 0; outcome < contract.TransferOutcomeCount(op, ti); outcome++ {
					_, disp, ok := contract.TransferOutcomeAt(op, ti, outcome)
					if !ok {
						return Algebra{}, false
					}
					isolations := []Isolation{IsolationMutable, IsolationSealed, IsolationDeepImmutable}
					for _, isolation := range isolations {
						entries = append(entries, armEntry{Arm{application: app, operation: op, transfer: transfer, outcome: uint32(outcome), disposition: disp, isolation: isolation}})
					}
				}
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool { return lessArm(entries[i].arm, entries[j].arm) })
	id := algebraID(source.ContentID())
	return Algebra{&algebra{source: source, linkID: source.ContentID(), arms: entries, id: id, sealed: true}}, true
}
func (a Algebra) valid() bool {
	return a.owner != nil && a.owner.sealed && a.owner.source != nil && a.owner.id.Available() && a.owner.source.ContentID() == a.owner.linkID
}
func (a Algebra) ContentID() keyspace.ContentID {
	if !a.valid() {
		return keyspace.ContentID{}
	}
	return a.owner.id
}
func (a Algebra) LinkContentID() keyspace.ContentID {
	if !a.valid() {
		return keyspace.ContentID{}
	}
	return a.owner.linkID
}
func (a Algebra) ArmCount() int {
	if !a.valid() {
		return 0
	}
	return len(a.owner.arms)
}
func (a Algebra) ArmAt(i int) (Arm, bool) {
	if !a.valid() || i < 0 || i >= len(a.owner.arms) {
		return Arm{}, false
	}
	return a.owner.arms[i].arm, true
}

type Key struct {
	owner *algebra
	arm   uint32
}

func (a Algebra) Key(arm Arm) (Key, bool) {
	i, ok := a.armIndex(arm)
	if !ok {
		return Key{}, false
	}
	return Key{a.owner, uint32(i + 1)}, true
}
func (a Algebra) KeyArm(key Key) (Arm, bool) { entry, ok := a.keyEntry(key); return entry.arm, ok }
func (a Algebra) ArmDisposition(arm Arm) (target.TransferPossibility, bool) {
	if _, ok := a.armIndex(arm); !ok {
		return 0, false
	}
	return arm.disposition, true
}
func (a Algebra) armIndex(arm Arm) (int, bool) {
	if !a.valid() || !arm.validFor(a.owner.source) {
		return 0, false
	}
	i := sort.Search(len(a.owner.arms), func(i int) bool { return !lessArm(a.owner.arms[i].arm, arm) })
	return i, i < len(a.owner.arms) && a.owner.arms[i].arm == arm
}
func (a Algebra) keyEntry(key Key) (armEntry, bool) {
	if !a.valid() || key.owner != a.owner || key.arm == 0 || int(key.arm) > len(a.owner.arms) {
		return armEntry{}, false
	}
	return a.owner.arms[key.arm-1], true
}
func (a Algebra) ownsKey(key Key) bool {
	_, ok := a.keyEntry(key)
	return ok
}
func (a Algebra) Rebind(source *link.Link) (Algebra, bool) {
	if !a.valid() || source == nil || source.ContentID() != a.owner.linkID {
		return Algebra{}, false
	}
	b, ok := NewAlgebra(source)
	return b, ok && b.ContentID() == a.owner.id
}
func algebraID(id keyspace.ContentID) keyspace.ContentID {
	hash := sha256.New()
	_, _ = hash.Write([]byte("wippy.analysis.transfer.algebra.v3"))
	_, _ = hash.Write(id[:])
	var result keyspace.ContentID
	copy(result[:], hash.Sum(nil))
	return result
}
