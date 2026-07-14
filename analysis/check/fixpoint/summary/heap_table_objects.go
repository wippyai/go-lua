package summary

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

func requireHeapKeySpace(s Summary, operation string) {
	if len(s.HeapTableObjects) != 0 && s.HeapKeySpace != nil && !s.HeapKeySpace.Valid() ||
		!heapTableObjectsStructuralKeyFree(s.HeapTableObjects) && s.HeapKeySpace == nil {
		panic(fmt.Sprintf("summary %s: non-empty HeapTableObjects has no valid producing HeapKeySpace", operation))
	}
}

func heapTableObjectsStructuralKeyFree(objects map[identity.ID]heapidentity.TableObject) bool {
	for _, object := range objects {
		if !object.StructuralKeyFree() {
			return false
		}
	}
	return true
}

func normalizeHeapTableObjects(reg *axis.Registry, in map[identity.ID]heapidentity.TableObject) map[identity.ID]heapidentity.TableObject {
	if len(in) == 0 {
		return nil
	}
	objectDomain := heapidentity.ObjectDomain(reg)
	out := make(map[identity.ID]heapidentity.TableObject, len(in))
	for id, object := range in {
		if id == (identity.ID{}) || objectDomain.Equal(object, objectDomain.Bottom()) {
			continue
		}
		out[id] = object
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeOwnedHeapTableObjects(reg *axis.Registry, in map[identity.ID]heapidentity.TableObject) map[identity.ID]heapidentity.TableObject {
	if len(in) == 0 {
		return nil
	}
	objectDomain := heapidentity.ObjectDomain(reg)
	for id, object := range in {
		if id == (identity.ID{}) || objectDomain.Equal(object, objectDomain.Bottom()) {
			delete(in, id)
		}
	}
	if len(in) == 0 {
		return nil
	}
	return in
}

func cloneHeapTableObjects(in map[identity.ID]heapidentity.TableObject) map[identity.ID]heapidentity.TableObject {
	return heapidentity.CloneMap(in)
}

func heapTableObjectsEqual(reg *axis.Registry, a, b map[identity.ID]heapidentity.TableObject) bool {
	return heapidentity.MapDomain(reg).Equal(normalizeHeapTableObjects(reg, a), normalizeHeapTableObjects(reg, b))
}

func heapTableObjectsLessOrEq(reg *axis.Registry, a, b map[identity.ID]heapidentity.TableObject) bool {
	return heapidentity.MapDomain(reg).LessOrEq(normalizeHeapTableObjects(reg, a), normalizeHeapTableObjects(reg, b))
}

func summaryHeapTableObjectsEqual(reg *axis.Registry, a, b Summary) bool {
	if len(a.HeapTableObjects) != 0 && a.HeapKeySpace != nil && !a.HeapKeySpace.Valid() ||
		len(b.HeapTableObjects) != 0 && b.HeapKeySpace != nil && !b.HeapKeySpace.Valid() ||
		!heapTableObjectsStructuralKeyFree(a.HeapTableObjects) && a.HeapKeySpace == nil ||
		!heapTableObjectsStructuralKeyFree(b.HeapTableObjects) && b.HeapKeySpace == nil {
		return false
	}
	ks := heapKeySpaceForPair(a, b)
	left, leftOK := heapTableObjectsInKeySpace(a, ks)
	right, rightOK := heapTableObjectsInKeySpace(b, ks)
	if !leftOK || !rightOK {
		return false
	}
	return heapTableObjectsEqual(
		reg,
		left,
		right,
	)
}

func summaryHeapTableObjectsLessOrEq(reg *axis.Registry, a, b Summary) bool {
	if len(a.HeapTableObjects) != 0 && a.HeapKeySpace != nil && !a.HeapKeySpace.Valid() ||
		len(b.HeapTableObjects) != 0 && b.HeapKeySpace != nil && !b.HeapKeySpace.Valid() ||
		!heapTableObjectsStructuralKeyFree(a.HeapTableObjects) && a.HeapKeySpace == nil ||
		!heapTableObjectsStructuralKeyFree(b.HeapTableObjects) && b.HeapKeySpace == nil {
		return false
	}
	ks := heapKeySpaceForPair(a, b)
	left, leftOK := heapTableObjectsInKeySpace(a, ks)
	right, rightOK := heapTableObjectsInKeySpace(b, ks)
	if !leftOK || !rightOK {
		return false
	}
	return heapTableObjectsLessOrEq(
		reg,
		left,
		right,
	)
}

func joinHeapTableObjects(reg *axis.Registry, a, b map[identity.ID]heapidentity.TableObject) map[identity.ID]heapidentity.TableObject {
	return normalizeHeapTableObjects(reg, heapidentity.MapDomain(reg).Join(
		normalizeHeapTableObjects(reg, a),
		normalizeHeapTableObjects(reg, b),
	))
}

func widenHeapTableObjects(reg *axis.Registry, prev, next map[identity.ID]heapidentity.TableObject) map[identity.ID]heapidentity.TableObject {
	return normalizeHeapTableObjects(reg, heapidentity.MapDomain(reg).Widen(
		normalizeHeapTableObjects(reg, prev),
		normalizeHeapTableObjects(reg, next),
	))
}

func heapKeySpaceForPair(a, b Summary) *keyspace.KeySpace {
	if a.HeapKeySpace != nil && a.HeapKeySpace.Valid() {
		return a.HeapKeySpace
	}
	if b.HeapKeySpace != nil && b.HeapKeySpace.Valid() {
		return b.HeapKeySpace
	}
	return nil
}

func heapTableObjectsInKeySpace(s Summary, target *keyspace.KeySpace) (map[identity.ID]heapidentity.TableObject, bool) {
	if len(s.HeapTableObjects) == 0 {
		return nil, true
	}
	rekeyed, err := s.RekeyHeapTableObjects(target)
	if err != nil {
		return nil, false
	}
	return rekeyed.HeapTableObjects, true
}

func joinSummaryHeapTableObjects(reg *axis.Registry, a, b Summary) (map[identity.ID]heapidentity.TableObject, *keyspace.KeySpace) {
	requireHeapKeySpace(a, "join left operand")
	requireHeapKeySpace(b, "join right operand")
	ks := heapKeySpaceForPair(a, b)
	left, leftOK := heapTableObjectsInKeySpace(a, ks)
	right, rightOK := heapTableObjectsInKeySpace(b, ks)
	if !leftOK || !rightOK {
		panic("summary join: cannot import heap table objects into a common keyspace")
	}
	return joinHeapTableObjects(
		reg,
		left,
		right,
	), ks
}

func widenSummaryHeapTableObjects(reg *axis.Registry, prev, next Summary) (map[identity.ID]heapidentity.TableObject, *keyspace.KeySpace) {
	requireHeapKeySpace(prev, "widen previous operand")
	requireHeapKeySpace(next, "widen next operand")
	ks := heapKeySpaceForPair(prev, next)
	left, leftOK := heapTableObjectsInKeySpace(prev, ks)
	right, rightOK := heapTableObjectsInKeySpace(next, ks)
	if !leftOK || !rightOK {
		panic("summary widen: cannot import heap table objects into a common keyspace")
	}
	return widenHeapTableObjects(
		reg,
		left,
		right,
	), ks
}
