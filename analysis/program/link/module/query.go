package module

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

// The views are deliberately narrow.  A Component issues only owner views;
// it does not become a god-object facade for every relation it stores.
type Actors struct{ component *Component }
type Roots struct{ component *Component }
type Cache struct{ component *Component }
type Coordinates struct{ component *Component }
type Generations struct{ component *Component }
type Outcomes struct{ component *Component }
type Terminals struct{ component *Component }

func (c *Component) Actors() Actors           { return Actors{c} }
func (c *Component) Roots() Roots             { return Roots{c} }
func (c *Component) Cache() Cache             { return Cache{c} }
func (c *Component) Coordinates() Coordinates { return Coordinates{c} }
func (c *Component) Generations() Generations { return Generations{c} }
func (c *Component) Outcomes() Outcomes       { return Outcomes{c} }
func (c *Component) Terminals() Terminals     { return Terminals{c} }

// MatchesProject and MatchesBoundary are exact prerequisite fences for later
// owners.  Content equality is deliberately insufficient for hot wiring.
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

type ownerView interface{ owner() *Component }

func (c *Component) owner() *Component  { return c }
func (c Actors) owner() *Component      { return c.component }
func (c Roots) owner() *Component       { return c.component }
func (c Cache) owner() *Component       { return c.component }
func (c Coordinates) owner() *Component { return c.component }
func (c Generations) owner() *Component { return c.component }
func (c Outcomes) owner() *Component    { return c.component }
func (c Terminals) owner() *Component   { return c.component }
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
func (c Cache) InstanceCount() int {
	if !live(c) {
		return 0
	}
	return len(c.component.authority.instances)
}
func (c Cache) EntryCount() int {
	if !live(c) {
		return 0
	}
	return len(c.component.authority.entries)
}
func (c Cache) EntryAt(index int) (ModuleCacheEntry, bool) {
	if !live(c.component) || index < 0 || index >= len(c.component.authority.entries) {
		return ModuleCacheEntry{}, false
	}
	return ModuleCacheEntry{c.component, uint32(index + 1)}, true
}
func (c Cache) EntryMapping(entry ModuleCacheEntry) (linkproject.Application, AnalysisRoot, AnalysisRoot, bool) {
	if !valid(c.component, entry.ordinal, len(c.component.authority.entries)) || entry.component != c.component {
		return linkproject.Application{}, AnalysisRoot{}, AnalysisRoot{}, false
	}
	r := c.component.authority.entries[entry.ordinal-1]
	return r.application, AnalysisRoot{c.component, r.from}, AnalysisRoot{c.component, r.to}, true
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
func (c Cache) EntryID(entry ModuleCacheEntry) (identity.ContentID, bool) {
	if !valid(c.component, entry.ordinal, len(c.component.authority.entries)) || entry.component != c.component {
		return identity.ContentID{}, false
	}
	return denseID(c.component.authority.content, 0x6d6f64756c652d65, uint64(entry.ordinal)), true
}

func (c Coordinates) Count() int {
	if !live(c) {
		return 0
	}
	return len(c.component.authority.coordinates)
}
func (c *Component) coordinate(row coordinateRow) (ModuleCoordinate, bool) {
	if !live(c) {
		return ModuleCoordinate{}, false
	}
	ord, ok := c.authority.coordinateOrdinals[row]
	if !ok {
		return ModuleCoordinate{}, false
	}
	return ModuleCoordinate{c, ord}, true
}

func (c Generations) Count() int {
	if !live(c) {
		return 0
	}
	return len(c.component.authority.entries)
}
func (c Generations) At(index int) (ModuleInitGeneration, bool) {
	if !live(c.component) || index < 0 || index >= len(c.component.authority.entries) {
		return ModuleInitGeneration{}, false
	}
	return ModuleInitGeneration{c.component, uint32(index + 1)}, true
}
func (c Generations) Entry(g ModuleInitGeneration) (ModuleCacheEntry, ModuleCoordinate, linkproject.Shard, keyspace.Term, bool) {
	if !valid(c.component, g.ordinal, len(c.component.authority.entries)) || g.component != c.component {
		return ModuleCacheEntry{}, ModuleCoordinate{}, linkproject.Shard{}, 0, false
	}
	e := c.component.authority.entries[g.ordinal-1]
	to := c.component.authority.roots[e.to-1]
	from := c.component.authority.roots[e.from-1]
	rep := c.component.authority.instances[from.instance-1].representative
	coordinate, ok := c.component.coordinate(coordinateRow{actor: from.actor, shard: to.shard, representative: rep})
	if !ok {
		return ModuleCacheEntry{}, ModuleCoordinate{}, linkproject.Shard{}, 0, false
	}
	p, ok := c.component.authority.project.Mounts().Program(to.shard)
	if !ok || p == nil {
		return ModuleCacheEntry{}, ModuleCoordinate{}, linkproject.Shard{}, 0, false
	}
	term, ok := p.Source().Index().Entry()
	if !ok {
		return ModuleCacheEntry{}, ModuleCoordinate{}, linkproject.Shard{}, 0, false
	}
	return ModuleCacheEntry{c.component, g.ordinal}, coordinate, to.shard, term, true
}
func (c Generations) Ref(g ModuleInitGeneration) (ModuleInitGenerationRef, bool) {
	if !valid(c.component, g.ordinal, len(c.component.authority.entries)) || g.component != c.component {
		return ModuleInitGenerationRef{}, false
	}
	entry, ok := c.component.Cache().EntryID(ModuleCacheEntry{c.component, g.ordinal})
	if !ok {
		return ModuleInitGenerationRef{}, false
	}
	return ModuleInitGenerationRef{c.component.authority.content, entry}, true
}

func (c Outcomes) Count(g ModuleInitGeneration) int {
	_, _, shard, entry, ok := c.component.Generations().Entry(g)
	if !ok {
		return 0
	}
	p, ok := c.component.authority.project.Mounts().Program(shard)
	if !ok || p == nil {
		return 0
	}
	n := p.Module().Entry().ReturnCount()
	for _, kind := range []flowkind.OutcomeKind{flowkind.OutcomeNormal, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel} {
		if moduleExit(p, entry, kind) != 0 {
			n++
		}
	}
	return n
}
func (c Outcomes) At(g ModuleInitGeneration, index int) (ModuleInitOutcome, bool) {
	if index < 0 {
		return ModuleInitOutcome{}, false
	}
	_, _, shard, entry, ok := c.component.Generations().Entry(g)
	if !ok {
		return ModuleInitOutcome{}, false
	}
	p, ok := c.component.authority.project.Mounts().Program(shard)
	if !ok {
		return ModuleInitOutcome{}, false
	}
	if moduleExit(p, entry, flowkind.OutcomeNormal) != 0 {
		if index == 0 {
			return ModuleInitOutcome{c.component, g.ordinal, flowkind.OutcomeNormal, 0}, true
		}
		index--
	}
	if n := p.Module().Entry().ReturnCount(); index < n {
		return ModuleInitOutcome{c.component, g.ordinal, flowkind.OutcomeReturn, uint32(index)}, true
	} else {
		index -= n
	}
	for _, kind := range []flowkind.OutcomeKind{flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel} {
		if moduleExit(p, entry, kind) != 0 {
			if index == 0 {
				return ModuleInitOutcome{c.component, g.ordinal, kind, 0}, true
			}
			index--
		}
	}
	return ModuleInitOutcome{}, false
}
func (c Outcomes) Ref(o ModuleInitOutcome) (ModuleInitOutcomeRef, bool) {
	if !c.component.validOutcome(o) {
		return ModuleInitOutcomeRef{}, false
	}
	g, ok := c.component.Generations().Ref(ModuleInitGeneration{c.component, o.generation})
	if !ok {
		return ModuleInitOutcomeRef{}, false
	}
	return ModuleInitOutcomeRef{g, o.kind, o.ordinal}, true
}
func (c Outcomes) ID(o ModuleInitOutcome) (identity.ContentID, bool) {
	ref, ok := c.Ref(o)
	if !ok {
		return identity.ContentID{}, false
	}
	return hashOutcome(ref), true
}

func (c Terminals) Count() int {
	if !live(c) {
		return 0
	}
	return len(c.component.authority.terminals)
}
func (c Terminals) ID(t ModuleInitTerminal) (identity.ContentID, bool) {
	if !c.component.validTerminal(t) {
		return identity.ContentID{}, false
	}
	id, ok := c.component.Outcomes().ID(t.outcome)
	if !ok {
		return identity.ContentID{}, false
	}
	return denseID(id, 0x6d6f64756c652d74, 1), true
}

func (a *authority) indexes() error {
	coords := make([]coordinateRow, 0, len(a.roots)+len(a.entries))
	for _, r := range a.roots {
		coords = append(coords, coordinateRow{r.actor, r.shard, a.instances[r.instance-1].representative})
	}
	for _, e := range a.entries {
		from, to := a.roots[e.from-1], a.roots[e.to-1]
		if from.actor != to.actor {
			return errors.New("link/module: cross actor coordinate")
		}
		coords = append(coords, coordinateRow{from.actor, to.shard, a.instances[from.instance-1].representative})
	}
	sort.Slice(coords, func(i, j int) bool {
		left, right := coords[i], coords[j]
		if left.actor != right.actor {
			return left.actor < right.actor
		}
		ai, _ := a.shardOrdinal(left.shard)
		bi, _ := a.shardOrdinal(right.shard)
		if ai != bi {
			return ai < bi
		}
		return left.representative < right.representative
	})
	out := coords[:0]
	for _, row := range coords {
		if len(out) == 0 || out[len(out)-1] != row {
			out = append(out, row)
		}
	}
	a.coordinates = out
	a.coordinateOrdinals = make(map[coordinateRow]uint32, len(out))
	seenCoordinate := make(map[identity.ContentID]struct{}, len(out))
	for i, row := range out {
		a.coordinateOrdinals[row] = uint32(i + 1)
		id := denseID(a.content, 0x6d6f64756c652d63, uint64(i+1))
		if _, duplicate := seenCoordinate[id]; duplicate {
			return errors.New("link/module: duplicate coordinate identity")
		}
		seenCoordinate[id] = struct{}{}
	}
	seenRoot := make(map[identity.ContentID]struct{}, len(a.roots))
	for i := range a.roots {
		id := denseID(a.content, 0x6d6f64756c652d72, uint64(i+1))
		if _, duplicate := seenRoot[id]; duplicate {
			return errors.New("link/module: duplicate root identity")
		}
		seenRoot[id] = struct{}{}
	}
	seenEntry := make(map[identity.ContentID]struct{}, len(a.entries))
	for i := range a.entries {
		id := denseID(a.content, 0x6d6f64756c652d65, uint64(i+1))
		if _, dup := seenEntry[id]; dup {
			return errors.New("link/module: duplicate entry identity")
		}
		seenEntry[id] = struct{}{}
	}
	seenOutcome := map[identity.ContentID]struct{}{}
	seenTerminal := map[identity.ContentID]struct{}{}
	for i := range a.entries {
		g := ModuleInitGeneration{a.component, uint32(i + 1)}
		for j := 0; j < a.component.Outcomes().Count(g); j++ {
			o, ok := a.component.Outcomes().At(g, j)
			if !ok {
				return errors.New("link/module: malformed outcome")
			}
			id, ok := a.component.Outcomes().ID(o)
			if !ok {
				return errors.New("link/module: unavailable outcome identity")
			}
			if _, dup := seenOutcome[id]; dup {
				return errors.New("link/module: duplicate outcome identity")
			}
			seenOutcome[id] = struct{}{}
			if terminalKind(o.kind) {
				a.terminals = append(a.terminals, outcomeCoordinate{o.generation, o.kind, o.ordinal})
				terminalID, terminalOK := a.component.Terminals().ID(ModuleInitTerminal{o})
				if !terminalOK || !terminalID.Available() {
					return errors.New("link/module: unavailable terminal identity")
				}
				if _, duplicate := seenTerminal[terminalID]; duplicate {
					return errors.New("link/module: duplicate terminal identity")
				}
				seenTerminal[terminalID] = struct{}{}
			}
		}
	}
	return nil
}
func (a *authority) shardOrdinal(shard linkproject.Shard) (int, bool) {
	return a.project.Mounts().Index(shard)
}
func (c *Component) validOutcome(o ModuleInitOutcome) bool {
	if !live(c) || o.component != c || o.generation == 0 || int(o.generation) > len(c.authority.entries) {
		return false
	}
	g := ModuleInitGeneration{c, o.generation}
	_, _, shard, entry, ok := c.Generations().Entry(g)
	if !ok {
		return false
	}
	p, ok := c.authority.project.Mounts().Program(shard)
	if !ok || p == nil {
		return false
	}
	switch o.kind {
	case flowkind.OutcomeReturn:
		if uint64(o.ordinal) >= uint64(p.Module().Entry().ReturnCount()) {
			return false
		}
		term, ok := p.Module().Entry().ReturnAt(int(o.ordinal))
		return ok && term != 0
	case flowkind.OutcomeNormal, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel:
		return o.ordinal == 0 && moduleExit(p, entry, o.kind) != 0
	default:
		return false
	}
}
func (c *Component) validTerminal(t ModuleInitTerminal) bool {
	return c.validOutcome(t.outcome) && terminalKind(t.outcome.kind)
}
func terminalKind(k flowkind.OutcomeKind) bool {
	return k == flowkind.OutcomeThrow || k == flowkind.OutcomeCancel
}
func moduleExit(p *program.Program, entry keyspace.Term, kind flowkind.OutcomeKind) keyspace.Term {
	if p == nil || entry == 0 {
		return 0
	}
	switch kind {
	case flowkind.OutcomeNormal, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel:
		v, _ := p.Flow().Outcomes().BodyExit(entry, kind)
		return v
	}
	return 0
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
func hashGeneration(r ModuleInitGenerationRef) identity.ContentID {
	if !r.component.Available() || !r.entry.Available() {
		return identity.ContentID{}
	}
	var payload [32 + 32 + 2*8]byte
	copy(payload[:32], r.component[:])
	copy(payload[32:64], r.entry[:])
	binary.BigEndian.PutUint64(payload[64:72], 0x6d6f64756c652d67) // module-gen
	binary.BigEndian.PutUint64(payload[72:80], 2)
	return sha256.Sum256(payload[:])
}
func hashOutcome(r ModuleInitOutcomeRef) identity.ContentID {
	g := hashGeneration(r.generation)
	var p [48]byte
	copy(p[:32], g[:])
	binary.BigEndian.PutUint64(p[32:40], uint64(r.kind))
	binary.BigEndian.PutUint64(p[40:48], uint64(r.ordinal))
	return sha256.Sum256(p[:])
}
