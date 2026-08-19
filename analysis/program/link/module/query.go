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

func (c *Component) owner() *Component    { return c }
func (c Actors) owner() *Component        { return c.component }
func (c Roots) owner() *Component         { return c.component }
func (c Cache) owner() *Component         { return c.component }
func (c Coordinates) owner() *Component   { return c.component }
func (c Generations) owner() *Component   { return c.component }
func (c Outcomes) owner() *Component      { return c.component }
func (c ReadySubjects) owner() *Component { return c.component }
func (c Terminals) owner() *Component     { return c.component }
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
func (c Roots) Find(id identity.ContentID) (AnalysisRoot, bool) {
	if !live(c.component) || !id.Available() {
		return AnalysisRoot{}, false
	}
	ordinal, ok := c.component.authority.rootByID[id]
	if !ok {
		return AnalysisRoot{}, false
	}
	return AnalysisRoot{c.component, ordinal}, true
}
func (c Roots) Rebind(root AnalysisRoot) (AnalysisRoot, bool) {
	if root.component == nil {
		return AnalysisRoot{}, false
	}
	id, ok := root.component.Roots().ID(root)
	if !ok {
		return AnalysisRoot{}, false
	}
	return c.Find(id)
}
func (c Cache) InstanceCount() int {
	if !live(c) {
		return 0
	}
	return len(c.component.authority.instances)
}
func (c Cache) InstanceAt(index int) (ModuleCacheInstance, bool) {
	if !live(c.component) || index < 0 || index >= len(c.component.authority.instances) {
		return ModuleCacheInstance{}, false
	}
	return ModuleCacheInstance{c.component, uint32(index + 1)}, true
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
func (c Cache) Representative(instance ModuleCacheInstance) (ModuleCacheInstance, bool) {
	if !valid(c.component, instance.ordinal, len(c.component.authority.instances)) || instance.component != c.component {
		return ModuleCacheInstance{}, false
	}
	return ModuleCacheInstance{c.component, c.component.authority.instances[instance.ordinal-1].representative}, true
}
func (c Cache) CallRepresentative(root AnalysisRoot, representative ModuleCacheInstance, application linkproject.Application) (ModuleCacheInstance, bool) {
	if !c.component.cacheRepresentativeAtRoot(root, representative) {
		return ModuleCacheInstance{}, false
	}
	shard, _, ok := c.component.authority.project.Applications().Call(application)
	if !ok || c.component.authority.roots[root.ordinal-1].shard != shard {
		return ModuleCacheInstance{}, false
	}
	return representative, true
}
func (c Cache) EntryMapping(entry ModuleCacheEntry) (linkproject.Application, AnalysisRoot, AnalysisRoot, bool) {
	if !valid(c.component, entry.ordinal, len(c.component.authority.entries)) || entry.component != c.component {
		return linkproject.Application{}, AnalysisRoot{}, AnalysisRoot{}, false
	}
	r := c.component.authority.entries[entry.ordinal-1]
	return r.application, AnalysisRoot{c.component, r.from}, AnalysisRoot{c.component, r.to}, true
}
func (c *Component) cacheRepresentativeAtRoot(root AnalysisRoot, rep ModuleCacheInstance) bool {
	if !valid(c, root.ordinal, len(c.authority.roots)) || !valid(c, rep.ordinal, len(c.authority.instances)) || root.component != c || rep.component != c {
		return false
	}
	instance := c.authority.instances[rep.ordinal-1]
	return instance.representative == rep.ordinal && instance.actor == c.authority.roots[root.ordinal-1].actor
}

func (c Cache) EntryID(entry ModuleCacheEntry) (identity.ContentID, bool) {
	if !valid(c.component, entry.ordinal, len(c.component.authority.entries)) || entry.component != c.component {
		return identity.ContentID{}, false
	}
	return denseID(c.component.authority.content, 0x6d6f64756c652d65, uint64(entry.ordinal)), true
}

// InstanceID is the detached identity of one exact cache instance. The typed
// owner helper is intentionally narrow: the source rows needs a stable
// identity, while no generic relation registry is introduced.
func (c Cache) InstanceID(instance ModuleCacheInstance) (identity.ContentID, bool) {
	if !valid(c.component, instance.ordinal, len(c.component.authority.instances)) || instance.component != c.component {
		return identity.ContentID{}, false
	}
	return denseID(c.component.authority.content, 0x6d6f64756c652d69, uint64(instance.ordinal)), true
}
func (c Cache) FindEntry(id identity.ContentID) (ModuleCacheEntry, bool) {
	if !live(c.component) || !id.Available() {
		return ModuleCacheEntry{}, false
	}
	ord, ok := c.component.authority.entryByID[id]
	if !ok {
		return ModuleCacheEntry{}, false
	}
	return ModuleCacheEntry{c.component, ord}, true
}
func (c Cache) RebindEntry(entry ModuleCacheEntry) (ModuleCacheEntry, bool) {
	if entry.component == nil {
		return ModuleCacheEntry{}, false
	}
	id, ok := entry.component.Cache().EntryID(entry)
	if !ok {
		return ModuleCacheEntry{}, false
	}
	return c.FindEntry(id)
}

func (c Coordinates) Count() int {
	if !live(c) {
		return 0
	}
	return len(c.component.authority.coordinates)
}
func (c Coordinates) At(index int) (ModuleCoordinate, bool) {
	if !live(c.component) || index < 0 || index >= len(c.component.authority.coordinates) {
		return ModuleCoordinate{}, false
	}
	return ModuleCoordinate{c.component, uint32(index + 1)}, true
}
func (c Coordinates) ForRoot(root AnalysisRoot) (ModuleCoordinate, bool) {
	shard, actor, instance, ok := c.component.Roots().Mapping(root)
	if !ok {
		return ModuleCoordinate{}, false
	}
	rep, ok := c.component.Cache().Representative(instance)
	if !ok {
		return ModuleCoordinate{}, false
	}
	return c.component.coordinate(coordinateRow{actor: actor.ordinal, shard: shard, representative: rep.ordinal})
}
func (c Coordinates) Mapping(coordinate ModuleCoordinate) (Actor, linkproject.Shard, ModuleCacheInstance, bool) {
	if !valid(c.component, coordinate.ordinal, len(c.component.authority.coordinates)) || coordinate.component != c.component {
		return Actor{}, linkproject.Shard{}, ModuleCacheInstance{}, false
	}
	r := c.component.authority.coordinates[coordinate.ordinal-1]
	return Actor{c.component, r.actor}, r.shard, ModuleCacheInstance{c.component, r.representative}, true
}
func (c Coordinates) ID(coordinate ModuleCoordinate) (identity.ContentID, bool) {
	if !valid(c.component, coordinate.ordinal, len(c.component.authority.coordinates)) || coordinate.component != c.component {
		return identity.ContentID{}, false
	}
	return denseID(c.component.authority.content, 0x6d6f64756c652d63, uint64(coordinate.ordinal)), true
}
func (c Coordinates) Find(id identity.ContentID) (ModuleCoordinate, bool) {
	if !live(c.component) || !id.Available() {
		return ModuleCoordinate{}, false
	}
	ordinal, ok := c.component.authority.coordinateByID[id]
	if !ok {
		return ModuleCoordinate{}, false
	}
	return ModuleCoordinate{c.component, ordinal}, true
}
func (c Coordinates) Rebind(coordinate ModuleCoordinate) (ModuleCoordinate, bool) {
	if coordinate.component == nil {
		return ModuleCoordinate{}, false
	}
	id, ok := coordinate.component.Coordinates().ID(coordinate)
	if !ok {
		return ModuleCoordinate{}, false
	}
	return c.Find(id)
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
func (c Generations) ID(g ModuleInitGeneration) (identity.ContentID, bool) {
	ref, ok := c.Ref(g)
	if !ok {
		return identity.ContentID{}, false
	}
	return hashGeneration(ref), true
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
func (c Generations) FindRef(ref ModuleInitGenerationRef) (ModuleInitGeneration, bool) {
	if !live(c.component) || ref.component != c.component.authority.content {
		return ModuleInitGeneration{}, false
	}
	entry, ok := c.component.Cache().FindEntry(ref.entry)
	if !ok {
		return ModuleInitGeneration{}, false
	}
	return ModuleInitGeneration{c.component, entry.ordinal}, true
}
func (c Generations) Rebind(g ModuleInitGeneration) (ModuleInitGeneration, bool) {
	if g.component == nil {
		return ModuleInitGeneration{}, false
	}
	ref, ok := g.component.Generations().Ref(g)
	if !ok {
		return ModuleInitGeneration{}, false
	}
	return c.FindRef(ref)
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
func (c Outcomes) FindRef(ref ModuleInitOutcomeRef) (ModuleInitOutcome, bool) {
	g, ok := c.component.Generations().FindRef(ref.generation)
	if !ok {
		return ModuleInitOutcome{}, false
	}
	o := ModuleInitOutcome{c.component, g.ordinal, ref.kind, ref.ordinal}
	return o, c.component.validOutcome(o)
}
func (c Outcomes) ID(o ModuleInitOutcome) (identity.ContentID, bool) {
	ref, ok := c.Ref(o)
	if !ok {
		return identity.ContentID{}, false
	}
	return hashOutcome(ref), true
}
func (c Outcomes) Find(id identity.ContentID) (ModuleInitOutcome, bool) {
	if !live(c) {
		return ModuleInitOutcome{}, false
	}
	row, ok := c.component.authority.outcomeByID[id]
	if !ok {
		return ModuleInitOutcome{}, false
	}
	return ModuleInitOutcome{c.component, row.generation, row.kind, row.ordinal}, true
}
func (c Outcomes) Rebind(o ModuleInitOutcome) (ModuleInitOutcome, bool) {
	if o.component == nil {
		return ModuleInitOutcome{}, false
	}
	ref, ok := o.component.Outcomes().Ref(o)
	if !ok {
		return ModuleInitOutcome{}, false
	}
	return c.FindRef(ref)
}
func (c Outcomes) Source(o ModuleInitOutcome) (linkproject.Shard, keyspace.Term, bool) {
	if !c.component.validOutcome(o) {
		return linkproject.Shard{}, 0, false
	}
	_, _, shard, entry, ok := c.component.Generations().Entry(ModuleInitGeneration{c.component, o.generation})
	if !ok {
		return linkproject.Shard{}, 0, false
	}
	p, ok := c.component.authority.project.Mounts().Program(shard)
	if !ok {
		return linkproject.Shard{}, 0, false
	}
	if o.kind == flowkind.OutcomeReturn {
		term, ok := p.Module().Entry().ReturnAt(int(o.ordinal))
		return shard, term, ok && term != 0
	}
	term := moduleExit(p, entry, o.kind)
	return shard, term, term != 0
}
func (c Outcomes) Provenance(o ModuleInitOutcome) (ModuleInitGeneration, linkproject.Shard, keyspace.Term, bool) {
	shard, term, ok := c.Source(o)
	if !ok {
		return ModuleInitGeneration{}, linkproject.Shard{}, 0, false
	}
	return ModuleInitGeneration{c.component, o.generation}, shard, term, true
}
func (c Outcomes) Kind(o ModuleInitOutcome) (flowkind.OutcomeKind, bool) {
	return o.kind, c.component.validOutcome(o)
}
func (c Outcomes) ReadySubject(o ModuleInitOutcome) (ModuleReadySubject, bool) {
	if !c.component.validOutcome(o) {
		return ModuleReadySubject{}, false
	}
	if o.kind == flowkind.OutcomeNormal {
		return ModuleReadySubject{c.component, ModuleReadySubjectDefaultTrue, linkboundary.Value{}}, true
	}
	if o.kind != flowkind.OutcomeReturn {
		return ModuleReadySubject{}, false
	}
	shard, term, ok := c.Source(o)
	if !ok {
		return ModuleReadySubject{}, false
	}
	p, ok := c.component.authority.project.Mounts().Program(shard)
	if !ok {
		return ModuleReadySubject{}, false
	}
	_, values, ok := p.Flow().Authored().Control().Returns().Get(term)
	if !ok {
		return ModuleReadySubject{}, false
	}
	view := p.Flow().Authored().Values()
	n, ok := view.Len(values)
	if !ok {
		return ModuleReadySubject{}, false
	}
	var subject keyspace.Term
	if n == 0 {
		_, tail, ok := view.Get(values)
		if !ok || tail == 0 {
			return ModuleReadySubject{c.component, ModuleReadySubjectDefaultTrue, linkboundary.Value{}}, ok
		}
		subject = tail
	} else {
		subject, ok = view.Member(values, 0)
		if !ok {
			return ModuleReadySubject{}, false
		}
		if keyspace.TermFamily(subject) == keyspace.FamilyNil {
			return ModuleReadySubject{c.component, ModuleReadySubjectDefaultTrue, linkboundary.Value{}}, true
		}
	}
	value, ok := c.component.authority.boundary.Values().Of(shard, subject)
	if !ok {
		return ModuleReadySubject{}, false
	}
	return ModuleReadySubject{c.component, ModuleReadySubjectExistingValue, value}, true
}
func (c ReadySubjects) Value(s ModuleReadySubject) (linkboundary.Value, bool) {
	if !c.component.validSubject(s) || s.kind != ModuleReadySubjectExistingValue {
		return linkboundary.Value{}, false
	}
	return s.value, true
}
func (c ReadySubjects) Compare(a, b ModuleReadySubject) (int, bool) {
	if !c.component.validSubject(a) || !c.component.validSubject(b) {
		return 0, false
	}
	if a.kind < b.kind {
		return -1, true
	}
	if a.kind > b.kind {
		return 1, true
	}
	if a.kind != ModuleReadySubjectExistingValue {
		return 0, true
	}
	return c.component.authority.boundary.Values().Compare(a.value, b.value)
}
func (c Generations) Compare(a, b ModuleInitGeneration) (int, bool) {
	if !valid(c.component, a.ordinal, len(c.component.authority.entries)) || !valid(c.component, b.ordinal, len(c.component.authority.entries)) || a.component != c.component || b.component != c.component {
		return 0, false
	}
	if a.ordinal < b.ordinal {
		return -1, true
	}
	if a.ordinal > b.ordinal {
		return 1, true
	}
	return 0, true
}

func (c Terminals) Count() int {
	if !live(c) {
		return 0
	}
	return len(c.component.authority.terminals)
}
func (c Terminals) At(index int) (ModuleInitTerminal, bool) {
	if !live(c.component) || index < 0 || index >= len(c.component.authority.terminals) {
		return ModuleInitTerminal{}, false
	}
	o := c.component.authority.terminals[index]
	return ModuleInitTerminal{ModuleInitOutcome{c.component, o.generation, o.kind, o.ordinal}}, true
}
func (c Terminals) Outcome(t ModuleInitTerminal) (ModuleInitOutcome, bool) {
	if !c.component.validTerminal(t) {
		return ModuleInitOutcome{}, false
	}
	return t.outcome, true
}
func (c Terminals) Provenance(t ModuleInitTerminal) (ModuleInitOutcome, ModuleInitGeneration, ModuleCoordinate, flowkind.OutcomeKind, bool) {
	if !c.component.validTerminal(t) {
		return ModuleInitOutcome{}, ModuleInitGeneration{}, ModuleCoordinate{}, 0, false
	}
	g := ModuleInitGeneration{c.component, t.outcome.generation}
	_, coord, _, _, ok := c.component.Generations().Entry(g)
	if !ok {
		return ModuleInitOutcome{}, ModuleInitGeneration{}, ModuleCoordinate{}, 0, false
	}
	return t.outcome, g, coord, t.outcome.kind, true
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
func (c Terminals) Ref(t ModuleInitTerminal) (ModuleInitTerminalRef, bool) {
	if !c.component.validTerminal(t) {
		return ModuleInitTerminalRef{}, false
	}
	o, ok := c.component.Outcomes().Ref(t.outcome)
	return ModuleInitTerminalRef{o}, ok
}
func (c Terminals) Find(id identity.ContentID) (ModuleInitTerminal, bool) {
	if !live(c) {
		return ModuleInitTerminal{}, false
	}
	ordinal, ok := c.component.authority.terminalByID[id]
	if !ok || ordinal == 0 || uint64(ordinal) > uint64(len(c.component.authority.terminals)) {
		return ModuleInitTerminal{}, false
	}
	row := c.component.authority.terminals[ordinal-1]
	return ModuleInitTerminal{ModuleInitOutcome{c.component, row.generation, row.kind, row.ordinal}}, true
}
func (c Terminals) FindRef(ref ModuleInitTerminalRef) (ModuleInitTerminal, bool) {
	o, ok := c.component.Outcomes().FindRef(ref.outcome)
	if !ok {
		return ModuleInitTerminal{}, false
	}
	t := ModuleInitTerminal{o}
	return t, c.component.validTerminal(t)
}
func (c Terminals) Rebind(t ModuleInitTerminal) (ModuleInitTerminal, bool) {
	if t.outcome.component == nil {
		return ModuleInitTerminal{}, false
	}
	ref, ok := t.outcome.component.Terminals().Ref(t)
	if !ok {
		return ModuleInitTerminal{}, false
	}
	return c.FindRef(ref)
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
	a.coordinateByID = make(map[identity.ContentID]uint32, len(out))
	for i, row := range out {
		a.coordinateOrdinals[row] = uint32(i + 1)
		id := denseID(a.content, 0x6d6f64756c652d63, uint64(i+1))
		if _, duplicate := a.coordinateByID[id]; duplicate {
			return errors.New("link/module: duplicate coordinate identity")
		}
		a.coordinateByID[id] = uint32(i + 1)
	}
	a.rootByID = make(map[identity.ContentID]uint32, len(a.roots))
	for i := range a.roots {
		id := denseID(a.content, 0x6d6f64756c652d72, uint64(i+1))
		if _, duplicate := a.rootByID[id]; duplicate {
			return errors.New("link/module: duplicate root identity")
		}
		a.rootByID[id] = uint32(i + 1)
	}
	a.entryByID = make(map[identity.ContentID]uint32, len(a.entries))
	for i := range a.entries {
		id := denseID(a.content, 0x6d6f64756c652d65, uint64(i+1))
		if _, dup := a.entryByID[id]; dup {
			return errors.New("link/module: duplicate entry identity")
		}
		a.entryByID[id] = uint32(i + 1)
	}
	a.outcomeByID = map[identity.ContentID]outcomeCoordinate{}
	a.terminalByID = map[identity.ContentID]uint32{}
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
			row := outcomeCoordinate{o.generation, o.kind, o.ordinal}
			if _, dup := a.outcomeByID[id]; dup {
				return errors.New("link/module: duplicate outcome identity")
			}
			a.outcomeByID[id] = row
			if terminalKind(o.kind) {
				a.terminals = append(a.terminals, row)
				terminalID, terminalOK := a.component.Terminals().ID(ModuleInitTerminal{o})
				if !terminalOK || !terminalID.Available() {
					return errors.New("link/module: unavailable terminal identity")
				}
				if _, duplicate := a.terminalByID[terminalID]; duplicate {
					return errors.New("link/module: duplicate terminal identity")
				}
				a.terminalByID[terminalID] = uint32(len(a.terminals))
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
func (c *Component) validSubject(s ModuleReadySubject) bool {
	if !live(c) || s.component != c {
		return false
	}
	if s.kind == ModuleReadySubjectDefaultTrue {
		return s.value == (linkboundary.Value{})
	}
	if s.kind == ModuleReadySubjectExistingValue {
		_, _, ok := c.authority.boundary.Values().Origin(s.value)
		return ok
	}
	return false
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
