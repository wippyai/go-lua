package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

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
	ks := heapKeySpaceForPair(a, b)
	return heapTableObjectsEqual(
		reg,
		heapTableObjectsInKeySpace(a, ks),
		heapTableObjectsInKeySpace(b, ks),
	)
}

func summaryHeapTableObjectsLessOrEq(reg *axis.Registry, a, b Summary) bool {
	ks := heapKeySpaceForPair(a, b)
	return heapTableObjectsLessOrEq(
		reg,
		heapTableObjectsInKeySpace(a, ks),
		heapTableObjectsInKeySpace(b, ks),
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
	if a.HeapKeySpace != nil {
		return a.HeapKeySpace
	}
	return b.HeapKeySpace
}

func heapTableObjectsInKeySpace(s Summary, target *keyspace.KeySpace) map[identity.ID]heapidentity.TableObject {
	if len(s.HeapTableObjects) == 0 {
		return nil
	}
	if target == nil || s.HeapKeySpace == nil || s.HeapKeySpace == target {
		return s.HeapTableObjects
	}
	return s.RekeyHeapTableObjects(target).HeapTableObjects
}

func joinSummaryHeapTableObjects(reg *axis.Registry, a, b Summary) (map[identity.ID]heapidentity.TableObject, *keyspace.KeySpace) {
	ks := heapKeySpaceForPair(a, b)
	return joinHeapTableObjects(
		reg,
		heapTableObjectsInKeySpace(a, ks),
		heapTableObjectsInKeySpace(b, ks),
	), ks
}

func widenSummaryHeapTableObjects(reg *axis.Registry, prev, next Summary) (map[identity.ID]heapidentity.TableObject, *keyspace.KeySpace) {
	ks := heapKeySpaceForPair(prev, next)
	return widenHeapTableObjects(
		reg,
		heapTableObjectsInKeySpace(prev, ks),
		heapTableObjectsInKeySpace(next, ks),
	), ks
}
