package factapply

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type presenceRootAccess struct {
	root        bool
	descendants bool
}

// presenceKeyAccess is the immutable lexical-read relation consumed by the
// implication kernel. Visibility is resolved while the exact coordinate
// inventory is sealed; consequence rounds never retain or query a resolver.
type presenceKeyAccess struct {
	keys  *keyspace.KeySpace
	roots map[keyspace.Key]presenceRootAccess
}

func freezePresenceKeyAccess(
	resolver *visibility.Resolver,
	point cfg.Point,
	keys *keyspace.KeySpace,
	paths []keyspace.Key,
) (presenceKeyAccess, error) {
	if keys == nil || !keys.Valid() {
		return presenceKeyAccess{}, fmt.Errorf("factapply: invalid presence accessibility keyspace")
	}
	out := presenceKeyAccess{keys: keys, roots: make(map[keyspace.Key]presenceRootAccess)}
	for _, path := range paths {
		if path.Kind == keyspace.KindInvalid {
			continue
		}
		root, ok := keys.StructuralRoot(path)
		if !ok {
			return presenceKeyAccess{}, fmt.Errorf("factapply: invalid presence accessibility path")
		}
		entry := out.roots[root]
		readable := false
		if _, formal := keys.DescribeFormalRoot(root); formal {
			// A formal relation keyspace contains only lexical cells admitted by
			// its sealed root rekey. SSA selection happened before transport.
			readable = true
		} else if resolver != nil && resolver.KeySpace() == keys {
			readable = pathKeyCurrentlyVisibleKey(resolver, point, path)
		}
		if path.Segs == 0 {
			entry.root = entry.root || readable
		} else {
			entry.descendants = entry.descendants || readable
		}
		out.roots[root] = entry
	}
	return out, nil
}

func (a presenceKeyAccess) valid() bool {
	return a.keys != nil && a.keys.Valid() && a.roots != nil
}

func (a presenceKeyAccess) readable(path keyspace.Key) bool {
	if !a.valid() || path.Kind == keyspace.KindInvalid {
		return false
	}
	root, ok := a.keys.StructuralRoot(path)
	if !ok {
		return false
	}
	entry, present := a.roots[root]
	if !present {
		return false
	}
	if path.Segs == 0 {
		return entry.root
	}
	return entry.descendants
}

func (a presenceKeyAccess) rekeyFormal(
	domain state.ProductDomain,
	plan state.CoordinateFormalRootRekey,
) (presenceKeyAccess, error) {
	target, ok := domain.CoordinateFormalDestinationKeySpace(plan)
	if !a.valid() || !ok {
		return presenceKeyAccess{}, fmt.Errorf("factapply: invalid presence accessibility rekey")
	}
	out := presenceKeyAccess{keys: target, roots: make(map[keyspace.Key]presenceRootAccess, len(a.roots))}
	for source, access := range a.roots {
		mapped, err := domain.RekeyStructuralKeyFormal(plan, source)
		if err != nil {
			return presenceKeyAccess{}, err
		}
		root, rooted := target.StructuralRoot(mapped)
		if !rooted {
			return presenceKeyAccess{}, fmt.Errorf("factapply: rekeyed presence accessibility root is invalid")
		}
		prior := out.roots[root]
		prior.root = prior.root || access.root
		prior.descendants = prior.descendants || access.descendants
		out.roots[root] = prior
	}
	return out, nil
}

func (a presenceKeyAccess) merge(other presenceKeyAccess) (presenceKeyAccess, error) {
	if !a.valid() || !other.valid() || a.keys != other.keys {
		return presenceKeyAccess{}, fmt.Errorf("factapply: incompatible presence accessibility relations")
	}
	out := presenceKeyAccess{keys: a.keys, roots: make(map[keyspace.Key]presenceRootAccess, len(a.roots)+len(other.roots))}
	for root, access := range a.roots {
		out.roots[root] = access
	}
	for root, access := range other.roots {
		prior := out.roots[root]
		prior.root = prior.root || access.root
		prior.descendants = prior.descendants || access.descendants
		out.roots[root] = prior
	}
	return out, nil
}

func presenceImplicationPaths(rows []pathevidence.PathPresenceImplication) []keyspace.Key {
	out := make([]keyspace.Key, 0, len(rows)*3)
	for _, row := range rows {
		out = append(out, row.Trigger)
		if row.HasTriggerPathEqual {
			out = append(out, row.TriggerOther)
		}
		out = append(out, row.Target)
	}
	return out
}

func freezeConcretePresenceStorageAccess(
	resolver *visibility.Resolver,
	point cfg.Point,
	storage presenceImplicationStorage,
	rows []pathevidence.PathPresenceImplication,
) (presenceKeyAccess, error) {
	if resolver == nil || storage == nil {
		return presenceKeyAccess{}, fmt.Errorf("factapply: invalid concrete presence accessibility source")
	}
	paths := presenceImplicationPaths(rows)
	for _, path := range append([]keyspace.Key(nil), paths...) {
		equivalents, valid := storage.EquivalentKeys(path)
		if !valid {
			return presenceKeyAccess{}, fmt.Errorf("factapply: invalid concrete presence equality class")
		}
		paths = append(paths, equivalents...)
	}
	return freezePresenceKeyAccess(resolver, point, resolver.KeySpace(), paths)
}
