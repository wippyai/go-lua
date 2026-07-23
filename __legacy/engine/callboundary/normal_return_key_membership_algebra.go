package callboundary

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/lattice/factset"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

type keyMembershipFactKey struct {
	key   pathdom.PathKey
	table pathdom.PathKey
}

var keyMembershipLane = factset.Set[keyMembershipFactKey, KeyMembershipFact]{
	Key: keyMembershipFactKeyOf,
	EqualFact: func(a, b KeyMembershipFact) bool {
		return keyMembershipFactKeyOf(a) == keyMembershipFactKeyOf(b)
	},
	Less: func(a, b KeyMembershipFact) bool {
		if !a.Key.Equal(b.Key) {
			return a.Key.Less(b.Key)
		}
		return a.Table.Less(b.Table)
	},
	Valid: func(f KeyMembershipFact) bool {
		return boundaryFactPath(f.Key) && boundaryFactPath(f.Table)
	},
	CloneFact: func(f KeyMembershipFact) KeyMembershipFact {
		f.Key = f.Key.Clone()
		f.Table = f.Table.Clone()
		return f
	},
	Prefer:    func(kept, incoming KeyMembershipFact) bool { return true },
	Intersect: true,
}

func keyMembershipFactKeyOf(f KeyMembershipFact) keyMembershipFactKey {
	return keyMembershipFactKey{key: f.Key.Key(), table: f.Table.Key()}
}

type dynamicValueKeyMembershipFactKey struct {
	container pathdom.PathKey
	site      dynamicindex.Site
	table     pathdom.PathKey
}

var dynamicValueKeyMembershipLane = factset.Set[dynamicValueKeyMembershipFactKey, DynamicValueKeyMembershipFact]{
	Key: dynamicValueKeyMembershipFactKeyOf,
	EqualFact: func(a, b DynamicValueKeyMembershipFact) bool {
		return dynamicValueKeyMembershipFactKeyOf(a) == dynamicValueKeyMembershipFactKeyOf(b)
	},
	Less: func(a, b DynamicValueKeyMembershipFact) bool {
		if !a.Container.Equal(b.Container) {
			return a.Container.Less(b.Container)
		}
		if a.Site != b.Site {
			return a.Site < b.Site
		}
		return a.Table.Less(b.Table)
	},
	Valid: func(f DynamicValueKeyMembershipFact) bool {
		return boundaryFactPath(f.Container) && f.Site != "" && boundaryFactPath(f.Table)
	},
	CloneFact: func(f DynamicValueKeyMembershipFact) DynamicValueKeyMembershipFact {
		f.Container = f.Container.Clone()
		f.Table = f.Table.Clone()
		return f
	},
	Prefer:    func(kept, incoming DynamicValueKeyMembershipFact) bool { return true },
	Intersect: true,
}

func dynamicValueKeyMembershipFactKeyOf(f DynamicValueKeyMembershipFact) dynamicValueKeyMembershipFactKey {
	return dynamicValueKeyMembershipFactKey{container: f.Container.Key(), site: f.Site, table: f.Table.Key()}
}

type dynamicAllValueKeyMembershipFactKey struct {
	container pathdom.PathKey
	table     pathdom.PathKey
}

var dynamicAllValueKeyMembershipLane = factset.Set[dynamicAllValueKeyMembershipFactKey, DynamicAllValueKeyMembershipFact]{
	Key: dynamicAllValueKeyMembershipFactKeyOf,
	EqualFact: func(a, b DynamicAllValueKeyMembershipFact) bool {
		return dynamicAllValueKeyMembershipFactKeyOf(a) == dynamicAllValueKeyMembershipFactKeyOf(b)
	},
	Less: func(a, b DynamicAllValueKeyMembershipFact) bool {
		if !a.Container.Equal(b.Container) {
			return a.Container.Less(b.Container)
		}
		return a.Table.Less(b.Table)
	},
	Valid: func(f DynamicAllValueKeyMembershipFact) bool {
		return boundaryFactPath(f.Container) && boundaryFactPath(f.Table)
	},
	CloneFact: func(f DynamicAllValueKeyMembershipFact) DynamicAllValueKeyMembershipFact {
		f.Container = f.Container.Clone()
		f.Table = f.Table.Clone()
		return f
	},
	Prefer:    func(kept, incoming DynamicAllValueKeyMembershipFact) bool { return true },
	Intersect: true,
}

func dynamicAllValueKeyMembershipFactKeyOf(f DynamicAllValueKeyMembershipFact) dynamicAllValueKeyMembershipFactKey {
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
