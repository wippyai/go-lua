package summary

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/lattice/factset"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

type keyMembershipFactKey struct {
	key   pathdom.PathKey
	table pathdom.PathKey
}

var keyMembershipLane = factset.Set[keyMembershipFactKey, callboundary.KeyMembershipFact]{
	Key: keyMembershipFactKeyOf,
	EqualFact: func(a, b callboundary.KeyMembershipFact) bool {
		return keyMembershipFactKeyOf(a) == keyMembershipFactKeyOf(b)
	},
	Less: func(a, b callboundary.KeyMembershipFact) bool {
		if !a.Key.Equal(b.Key) {
			return a.Key.Less(b.Key)
		}
		return a.Table.Less(b.Table)
	},
	Valid: func(f callboundary.KeyMembershipFact) bool {
		return boundaryFactPath(f.Key) && boundaryFactPath(f.Table)
	},
	CloneFact: func(f callboundary.KeyMembershipFact) callboundary.KeyMembershipFact {
		f.Key = f.Key.Clone()
		f.Table = f.Table.Clone()
		return f
	},
	Prefer:    func(kept, incoming callboundary.KeyMembershipFact) bool { return true },
	Intersect: true,
}

func keyMembershipFactKeyOf(f callboundary.KeyMembershipFact) keyMembershipFactKey {
	return keyMembershipFactKey{key: f.Key.Key(), table: f.Table.Key()}
}

type dynamicValueKeyMembershipFactKey struct {
	container pathdom.PathKey
	site      dynamicindex.Site
	table     pathdom.PathKey
}

var dynamicValueKeyMembershipLane = factset.Set[dynamicValueKeyMembershipFactKey, callboundary.DynamicValueKeyMembershipFact]{
	Key: dynamicValueKeyMembershipFactKeyOf,
	EqualFact: func(a, b callboundary.DynamicValueKeyMembershipFact) bool {
		return dynamicValueKeyMembershipFactKeyOf(a) == dynamicValueKeyMembershipFactKeyOf(b)
	},
	Less: func(a, b callboundary.DynamicValueKeyMembershipFact) bool {
		if !a.Container.Equal(b.Container) {
			return a.Container.Less(b.Container)
		}
		if a.Site != b.Site {
			return a.Site < b.Site
		}
		return a.Table.Less(b.Table)
	},
	Valid: func(f callboundary.DynamicValueKeyMembershipFact) bool {
		return boundaryFactPath(f.Container) && f.Site != "" && boundaryFactPath(f.Table)
	},
	CloneFact: func(f callboundary.DynamicValueKeyMembershipFact) callboundary.DynamicValueKeyMembershipFact {
		f.Container = f.Container.Clone()
		f.Table = f.Table.Clone()
		return f
	},
	Prefer:    func(kept, incoming callboundary.DynamicValueKeyMembershipFact) bool { return true },
	Intersect: true,
}

func dynamicValueKeyMembershipFactKeyOf(f callboundary.DynamicValueKeyMembershipFact) dynamicValueKeyMembershipFactKey {
	return dynamicValueKeyMembershipFactKey{container: f.Container.Key(), site: f.Site, table: f.Table.Key()}
}

type dynamicAllValueKeyMembershipFactKey struct {
	container pathdom.PathKey
	table     pathdom.PathKey
}

var dynamicAllValueKeyMembershipLane = factset.Set[dynamicAllValueKeyMembershipFactKey, callboundary.DynamicAllValueKeyMembershipFact]{
	Key: dynamicAllValueKeyMembershipFactKeyOf,
	EqualFact: func(a, b callboundary.DynamicAllValueKeyMembershipFact) bool {
		return dynamicAllValueKeyMembershipFactKeyOf(a) == dynamicAllValueKeyMembershipFactKeyOf(b)
	},
	Less: func(a, b callboundary.DynamicAllValueKeyMembershipFact) bool {
		if !a.Container.Equal(b.Container) {
			return a.Container.Less(b.Container)
		}
		return a.Table.Less(b.Table)
	},
	Valid: func(f callboundary.DynamicAllValueKeyMembershipFact) bool {
		return boundaryFactPath(f.Container) && boundaryFactPath(f.Table)
	},
	CloneFact: func(f callboundary.DynamicAllValueKeyMembershipFact) callboundary.DynamicAllValueKeyMembershipFact {
		f.Container = f.Container.Clone()
		f.Table = f.Table.Clone()
		return f
	},
	Prefer:    func(kept, incoming callboundary.DynamicAllValueKeyMembershipFact) bool { return true },
	Intersect: true,
}

func dynamicAllValueKeyMembershipFactKeyOf(f callboundary.DynamicAllValueKeyMembershipFact) dynamicAllValueKeyMembershipFactKey {
	return dynamicAllValueKeyMembershipFactKey{container: f.Container.Key(), table: f.Table.Key()}
}

func boundaryFactPath(p pathdom.Path) bool {
	return p.IsPlaceholder() || boundaryReturnSlotPath(p) || p.Symbol != 0
}

func boundaryReturnSlotPath(p pathdom.Path) bool {
	if p.Symbol != 0 || p.Root == "" || !strings.HasPrefix(p.Root, "ret[") || !strings.HasSuffix(p.Root, "]") {
		return false
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(p.Root, "ret["), "]")
	index, err := strconv.Atoi(raw)
	return err == nil && index >= 0 && p.Root == "ret["+strconv.Itoa(index)+"]"
}
