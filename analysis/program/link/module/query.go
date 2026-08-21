package module

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

// These views are deliberately limited to authored actor/cache-alias/root
// configuration. Resolved module-cache rows and all init geometry belong to
// the schema-declared Snapshot composition, not this Link child.
type Actors struct{ component *Component }
type Roots struct{ component *Component }
type Cache struct{ component *Component }

func (c *Component) Actors() Actors { return Actors{c} }
func (c *Component) Roots() Roots   { return Roots{c} }
func (c *Component) Cache() Cache   { return Cache{c} }

// MatchesProject and MatchesBoundary are exact prerequisite fences for later
// owners. Content equality is deliberately insufficient for hot wiring.
func (c *Component) MatchesProject(project *linkproject.Component) bool {
	return live(c) && project != nil && c.authority.project == project
}
func (c *Component) MatchesBoundary(boundary *linkboundary.Component) bool {
	return live(c) && boundary != nil && c.authority.boundary == boundary
}

// HostRelationID is the immutable actor/root relation consumed by Host.
func (c *Component) HostRelationID() (identity.ContentID, bool) {
	if !live(c) || !c.authority.hostRelation.Available() {
		return identity.ContentID{}, false
	}
	return c.authority.hostRelation, true
}

// compositionEntries is the private handoff from Module authority to the
// publication phase.  It is deliberately not a public module query: resolved
// rows belong to the general immutable Snapshot, while this relation is only
// the sealed parent-issued join witness needed to publish them.
func (c *Component) compositionEntries() ([]compositionEntry, bool) {
	if !live(c) || c.authority.fence == nil || !c.authority.fence.sealed || len(c.authority.composition) != len(c.authority.spec.ModuleCacheEntries) {
		return nil, false
	}
	entries := append([]compositionEntry(nil), c.authority.composition...)
	for _, entry := range entries {
		if !c.authority.validCompositionEntry(entry) {
			return nil, false
		}
	}
	if !c.authority.compositionComplete(entries) {
		return nil, false
	}
	return entries, true
}

// siblingImportKey names one authored Import occurrence by its owning Shard
// and exact Import term, which is the coordinate a module-cache entry resolves.
type siblingImportKey struct {
	shard linkproject.Shard
	term  keyspace.Term
}

// compositionComplete is the Snapshot-composition completeness law model.go's
// build defers here: a mounted-by-name sibling is a reachable declaration, but
// its exact Import is only a resolvable value if some authored module-cache
// entry names that same (Shard, Import) coordinate. An Import naming a mounted
// sibling with no such entry passes admission by name alone and would
// otherwise link with no resolved row backing it, so this relation refuses it
// before publication rather than letting it stand as an unbacked value.
func (a *authority) compositionComplete(entries []compositionEntry) bool {
	if a == nil || a.project == nil {
		return false
	}
	mounts := a.project.Mounts()
	moduleByName := make(map[string]linkproject.Shard, mounts.Count())
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		name, nameOK := mounts.Name(shard)
		if !shardOK || !nameOK {
			return false
		}
		moduleByName[name] = shard
	}
	present := make(map[siblingImportKey]struct{}, len(entries))
	for _, entry := range entries {
		present[siblingImportKey{shard: entry.sourceShard, term: entry.importTerm}] = struct{}{}
	}
	complete := true
	err := a.forEachExecutableModuleRequest(func(shard linkproject.Shard, name string, term keyspace.Term, request string) error {
		if _, mounted := moduleByName[request]; !mounted {
			return nil
		}
		if _, backed := present[siblingImportKey{shard: shard, term: term}]; !backed {
			complete = false
		}
		return nil
	})
	return err == nil && complete
}

func (a *authority) validCompositionEntry(entry compositionEntry) bool {
	if a == nil || a.project == nil || !a.content.Available() || entry.sourceShard == (linkproject.Shard{}) ||
		entry.sourceRootOrdinal == 0 || entry.fromRootOrdinal == 0 || entry.toRootOrdinal == 0 ||
		entry.sourceRootOrdinal != entry.fromRootOrdinal || entry.importTerm == 0 ||
		keyspace.TermFamily(entry.importTerm) != keyspace.FamilyImport || keyspace.TermOrdinal(entry.importTerm) == 0 ||
		uint64(entry.fromRootOrdinal) > uint64(len(a.roots)) || uint64(entry.toRootOrdinal) > uint64(len(a.roots)) {
		return false
	}
	fromRoot := a.roots[entry.fromRootOrdinal-1]
	toRoot := a.roots[entry.toRootOrdinal-1]
	if fromRoot.shard != entry.sourceShard || fromRoot.actor == 0 || fromRoot.actor != toRoot.actor ||
		fromRoot.instance == 0 || uint64(fromRoot.instance) > uint64(len(a.instances)) {
		return false
	}
	representative := a.instances[fromRoot.instance-1].representative
	if representative == 0 || uint64(representative) > uint64(len(a.instances)) {
		return false
	}
	return entry.fromRootID == denseID(a.content, 0x6d6f64756c652d72, uint64(entry.fromRootOrdinal)) &&
		entry.toRootID == denseID(a.content, 0x6d6f64756c652d72, uint64(entry.toRootOrdinal)) &&
		entry.actorID == denseID(a.content, 0x6d6f64756c652d61, uint64(fromRoot.actor)) &&
		entry.representativeID == denseID(a.content, 0x6d6f64756c652d69, uint64(representative))
}

type ownerView interface{ owner() *Component }

func (c *Component) owner() *Component { return c }
func (c Actors) owner() *Component     { return c.component }
func (c Roots) owner() *Component      { return c.component }
func (c Cache) owner() *Component      { return c.component }
func live(view ownerView) bool {
	c := view.owner()
	return c != nil && c.authority != nil && c.authority.component == c
}
func valid(view ownerView, ordinal uint32, n int) bool {
	return live(view) && ordinal != 0 && uint64(ordinal) <= uint64(n)
}

func (c Actors) Count() int {
	if !live(c) {
		return 0
	}
	return len(c.component.authority.actors)
}
func (c Actors) At(index int) (Actor, bool) {
	if !live(c.component) || index < 0 || index >= len(c.component.authority.actors) {
		return Actor{}, false
	}
	return Actor{c.component, uint32(index + 1)}, true
}
func (c Actors) Index(actor Actor) (int, bool) {
	if !valid(c.component, actor.ordinal, len(c.component.authority.actors)) || actor.component != c.component {
		return 0, false
	}
	return int(actor.ordinal - 1), true
}
func (c Actors) ID(actor Actor) (identity.ContentID, bool) {
	if !valid(c.component, actor.ordinal, len(c.component.authority.actors)) || actor.component != c.component {
		return identity.ContentID{}, false
	}
	return denseID(c.component.authority.content, 0x6d6f64756c652d61, uint64(actor.ordinal)), true
}

func (c Roots) Count() int {
	if !live(c) {
		return 0
	}
	return len(c.component.authority.roots)
}
func (c Roots) At(index int) (AnalysisRoot, bool) {
	if !live(c.component) || index < 0 || index >= len(c.component.authority.roots) {
		return AnalysisRoot{}, false
	}
	return AnalysisRoot{c.component, uint32(index + 1)}, true
}
func (c Roots) Index(root AnalysisRoot) (int, bool) {
	if !valid(c, root.ordinal, len(c.component.authority.roots)) || root.component != c.component {
		return 0, false
	}
	return int(root.ordinal - 1), true
}
func (c Roots) Compare(left, right AnalysisRoot) (int, bool) {
	if !valid(c.component, left.ordinal, len(c.component.authority.roots)) || !valid(c.component, right.ordinal, len(c.component.authority.roots)) || left.component != c.component || right.component != c.component {
		return 0, false
	}
	if left.ordinal < right.ordinal {
		return -1, true
	}
	if left.ordinal > right.ordinal {
		return 1, true
	}
	return 0, true
}
func (c Roots) ID(root AnalysisRoot) (identity.ContentID, bool) {
	if !valid(c, root.ordinal, len(c.component.authority.roots)) || root.component != c.component {
		return identity.ContentID{}, false
	}
	return denseID(c.component.authority.content, 0x6d6f64756c652d72, uint64(root.ordinal)), true
}

// InstanceCount reports authored cache-alias instances. There is deliberately
// no cache-entry query: resolved rows are Snapshot-owned.
func (c Cache) InstanceCount() int {
	if !live(c) {
		return 0
	}
	return len(c.component.authority.instances)
}
func (c Cache) InstanceID(instance ModuleCacheInstance) (identity.ContentID, bool) {
	if !valid(c.component, instance.ordinal, len(c.component.authority.instances)) || instance.component != c.component {
		return identity.ContentID{}, false
	}
	return denseID(c.component.authority.content, 0x6d6f64756c652d69, uint64(instance.ordinal)), true
}
func (c Cache) Representative(instance ModuleCacheInstance) (ModuleCacheInstance, bool) {
	if !valid(c.component, instance.ordinal, len(c.component.authority.instances)) || instance.component != c.component {
		return ModuleCacheInstance{}, false
	}
	representative := c.component.authority.instances[instance.ordinal-1].representative
	if representative == 0 || uint64(representative) > uint64(len(c.component.authority.instances)) {
		return ModuleCacheInstance{}, false
	}
	return ModuleCacheInstance{component: c.component, ordinal: representative}, true
}

func (c Roots) Mapping(root AnalysisRoot) (linkproject.Shard, Actor, ModuleCacheInstance, bool) {
	if !valid(c, root.ordinal, len(c.component.authority.roots)) || root.component != c.component {
		return linkproject.Shard{}, Actor{}, ModuleCacheInstance{}, false
	}
	r := c.component.authority.roots[root.ordinal-1]
	return r.shard, Actor{c.component, r.actor}, ModuleCacheInstance{c.component, r.instance}, true
}
func (c Roots) ForShardCount(shard linkproject.Shard) int {
	if !live(c) {
		return 0
	}
	i, ok := c.component.authority.project.Mounts().Index(shard)
	if !ok || i >= len(c.component.authority.rootRanges) {
		return 0
	}
	r := c.component.authority.rootRanges[i]
	return int(r.end - r.start)
}
func (c Roots) ForShardAt(shard linkproject.Shard, index int) (AnalysisRoot, bool) {
	if index < 0 || !live(c.component) {
		return AnalysisRoot{}, false
	}
	i, ok := c.component.authority.project.Mounts().Index(shard)
	if !ok || i >= len(c.component.authority.rootRanges) {
		return AnalysisRoot{}, false
	}
	r := c.component.authority.rootRanges[i]
	at := uint64(r.start) + uint64(index)
	if at >= uint64(r.end) {
		return AnalysisRoot{}, false
	}
	return AnalysisRoot{c.component, c.component.authority.rootIngress[at]}, true
}

// indexes enforces structural identity laws for the authored rows only.
func (a *authority) indexes() error {
	if a == nil || !a.content.Available() || a.project == nil {
		return errors.New("link/module: unavailable authored identity")
	}
	seenActors := make(map[identity.ContentID]struct{}, len(a.actors))
	for i, actor := range a.actors {
		if actor.name == "" {
			return errors.New("link/module: malformed actor")
		}
		id := denseID(a.content, 0x6d6f64756c652d61, uint64(i+1))
		if !id.Available() {
			return errors.New("link/module: unavailable actor identity")
		}
		if _, duplicate := seenActors[id]; duplicate {
			return errors.New("link/module: duplicate actor identity")
		}
		seenActors[id] = struct{}{}
	}
	seenInstances := make(map[identity.ContentID]struct{}, len(a.instances))
	for i, instance := range a.instances {
		if instance.name == "" || instance.actor == 0 || uint64(instance.actor) > uint64(len(a.actors)) || instance.representative == 0 || uint64(instance.representative) > uint64(len(a.instances)) {
			return errors.New("link/module: malformed cache instance")
		}
		if a.instances[instance.representative-1].actor != instance.actor {
			return errors.New("link/module: foreign cache representative")
		}
		id := denseID(a.content, 0x6d6f64756c652d69, uint64(i+1))
		if !id.Available() {
			return errors.New("link/module: unavailable cache instance identity")
		}
		if _, duplicate := seenInstances[id]; duplicate {
			return errors.New("link/module: duplicate cache instance identity")
		}
		seenInstances[id] = struct{}{}
	}
	seenRoots := make(map[identity.ContentID]struct{}, len(a.roots))
	for i, root := range a.roots {
		if root.name == "" || root.actor == 0 || uint64(root.actor) > uint64(len(a.actors)) || root.instance == 0 || uint64(root.instance) > uint64(len(a.instances)) {
			return errors.New("link/module: malformed analysis root")
		}
		if a.instances[root.instance-1].actor != root.actor {
			return errors.New("link/module: analysis root actor mismatch")
		}
		if _, ok := a.project.Mounts().Index(root.shard); !ok {
			return errors.New("link/module: foreign analysis root shard")
		}
		id := denseID(a.content, 0x6d6f64756c652d72, uint64(i+1))
		if !id.Available() {
			return errors.New("link/module: unavailable root identity")
		}
		if _, duplicate := seenRoots[id]; duplicate {
			return errors.New("link/module: duplicate root identity")
		}
		seenRoots[id] = struct{}{}
	}
	return nil
}

func denseID(seed identity.ContentID, tag uint64, ordinal uint64) identity.ContentID {
	if !seed.Available() {
		return identity.ContentID{}
	}
	var p [48]byte
	copy(p[:32], seed[:])
	binary.BigEndian.PutUint64(p[32:40], tag)
	binary.BigEndian.PutUint64(p[40:48], ordinal)
	return sha256.Sum256(p[:])
}
