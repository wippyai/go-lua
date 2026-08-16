// Package module owns actor-local module-cache geometry.  It is deliberately
// below Project and Boundary: it consumes their sealed relations but never
// imports the Link composition root or any later Host/Static authority.
package module

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

// Actor, AnalysisRoot, and ModuleCacheInstance are opaque identities issued by
// exactly one Component.  A dense ordinal without its component is never a
// valid semantic coordinate.
type Actor struct {
	component *Component
	ordinal   uint32
}
type AnalysisRoot struct {
	component *Component
	ordinal   uint32
}
type ModuleCacheInstance struct {
	component *Component
	ordinal   uint32
}
type ModuleCacheEntry struct {
	component *Component
	ordinal   uint32
}
type ModuleCoordinate struct {
	component *Component
	ordinal   uint32
}
type ModuleInitGeneration struct {
	component *Component
	ordinal   uint32
}
type ModuleInitOutcome struct {
	component  *Component
	generation uint32
	kind       flowkind.OutcomeKind
	ordinal    uint32
}
type ModuleInitTerminal struct{ outcome ModuleInitOutcome }

type ActorSpec struct{ Name string }
type ModuleCacheAliasClassSpec struct {
	Actor          string
	Instances      []string
	Representative string
}
type AnalysisRootSpec struct{ Name, Module, Actor, Instance string }
type ModuleCacheEntrySpec struct {
	Module           string
	Import           keyspace.Term
	FromRoot, ToRoot string
}

// Spec is the entire replayable authored module relation.  It is cold input,
// not a parallel hot authority.
type Spec struct {
	Actors             []ActorSpec
	ModuleCacheAliases []ModuleCacheAliasClassSpec
	AnalysisRoots      []AnalysisRootSpec
	ModuleCacheEntries []ModuleCacheEntrySpec
}

// Input names the two exact prerequisite children and the authored module
// relation.  Equal content from another Project/Boundary seal is rejected.
type Input struct {
	Project  *linkproject.Component
	Boundary *linkboundary.Component
	Spec     Spec
}
type Draft struct{ state *draftState }
type Component struct{ authority *authority }

// Cold is detached replay data.  Its tiny fence is deliberately separate from
// authority, so a persisted snapshot cannot retain the hot Project/Boundary
// graph through an incidental pointer.
type Cold struct {
	content         identity.ContentID
	spec            Spec
	semanticReceipt SemanticSourceReceipt
	fence           *coldFence
}

type ModuleInitGenerationRef struct {
	component identity.ContentID
	entry     identity.ContentID
}

func (r ModuleInitGenerationRef) ComponentID() identity.ContentID { return r.component }
func (r ModuleInitGenerationRef) EntryID() identity.ContentID     { return r.entry }

type ModuleInitOutcomeRef struct {
	generation ModuleInitGenerationRef
	kind       flowkind.OutcomeKind
	ordinal    uint32
}

func (r ModuleInitOutcomeRef) Generation() ModuleInitGenerationRef { return r.generation }
func (r ModuleInitOutcomeRef) Kind() flowkind.OutcomeKind          { return r.kind }
func (r ModuleInitOutcomeRef) ReturnOrdinal() uint32               { return r.ordinal }

type ModuleInitTerminalRef struct{ outcome ModuleInitOutcomeRef }

func (r ModuleInitTerminalRef) ComponentID() identity.ContentID {
	return r.outcome.generation.component
}

type ModuleReadySubjectKind uint8

const (
	ModuleReadySubjectInvalid ModuleReadySubjectKind = iota
	ModuleReadySubjectExistingValue
	ModuleReadySubjectDefaultTrue
)

type ModuleReadySubject struct {
	component *Component
	kind      ModuleReadySubjectKind
	value     linkboundary.Value
}

func (s ModuleReadySubject) Kind() ModuleReadySubjectKind { return s.kind }

type actorRow struct{ name string }
type instanceRow struct {
	name                  string
	actor, representative uint32
}
type rootRow struct {
	name            string
	shard           linkproject.Shard
	actor, instance uint32
}
type entryRow struct {
	application linkproject.Application
	from, to    uint32
}
type coordinateRow struct {
	actor          uint32
	shard          linkproject.Shard
	representative uint32
}
type outcomeCoordinate struct {
	generation uint32
	kind       flowkind.OutcomeKind
	ordinal    uint32
}
type rootRange struct{ start, end uint32 }
type authority struct {
	component          *Component
	project            *linkproject.Component
	boundary           *linkboundary.Component
	actors             []actorRow
	instances          []instanceRow
	roots              []rootRow
	entries            []entryRow
	rootRanges         []rootRange
	rootIngress        []uint32
	coordinates        []coordinateRow
	coordinateOrdinals map[coordinateRow]uint32
	rootByID           map[identity.ContentID]uint32
	coordinateByID     map[identity.ContentID]uint32
	entryByID          map[identity.ContentID]uint32
	outcomeByID        map[identity.ContentID]outcomeCoordinate
	terminals          []outcomeCoordinate // direct projection index; At is O(1)
	terminalByID       map[identity.ContentID]uint32
	spec               Spec
	content            identity.ContentID
	semanticReceipt    SemanticSourceReceipt
	hostRelation       identity.ContentID
	fence              *coldFence
}
type draftState struct {
	authority *authority
	consumed  bool
}
type coldFence struct{ sealed bool }

func (d *Draft) Finalize() (*Component, error) {
	if d == nil || d.state == nil || d.state.consumed || d.state.authority == nil {
		return nil, errors.New("link/module: unavailable draft")
	}
	d.state.consumed = true
	c := d.state.authority.component
	if c == nil || c.authority != d.state.authority {
		return nil, errors.New("link/module: malformed draft")
	}
	if c.authority.fence == nil {
		return nil, errors.New("link/module: missing cold fence")
	}
	c.authority.fence.sealed = true
	return c, nil
}
func (d *Draft) Cold() Cold {
	if d == nil || d.state == nil || d.state.consumed {
		return Cold{}
	}
	return cold(d.state.authority)
}
func (c *Component) Cold() Cold {
	if c == nil {
		return Cold{}
	}
	return cold(c.authority)
}
func cold(a *authority) Cold {
	if a == nil || !a.content.Available() || a.fence == nil {
		return Cold{}
	}
	return Cold{content: a.content, spec: cloneSpec(a.spec), semanticReceipt: a.semanticReceipt, fence: a.fence}
}
func (c Cold) ContentID() identity.ContentID { return c.content }
func (c Cold) Spec() (Spec, bool) {
	if c.fence == nil || !c.fence.sealed || !c.content.Available() {
		return Spec{}, false
	}
	return cloneSpec(c.spec), true
}
func (c *Component) ContentID() identity.ContentID {
	if c == nil || c.authority == nil {
		return identity.ContentID{}
	}
	return c.authority.content
}

func Build(input Input) (*Draft, error) {
	if input.Project == nil || input.Boundary == nil {
		return nil, errors.New("link/module: missing prerequisite authority")
	}
	mounts := input.Project.Mounts()
	if mounts.Count() == 0 {
		return nil, errors.New("link/module: empty project")
	}
	if !input.Boundary.MatchesProject(input.Project) {
		return nil, errors.New("link/module: foreign boundary")
	}
	a := &authority{project: input.Project, boundary: input.Boundary, fence: &coldFence{}}
	c := &Component{authority: a}
	a.component = c
	if err := a.build(cloneSpec(input.Spec)); err != nil {
		return nil, err
	}
	return &Draft{state: &draftState{authority: a}}, nil
}

func (a *authority) build(spec Spec) error {
	if len(spec.Actors) == 0 && len(spec.ModuleCacheAliases) == 0 && len(spec.AnalysisRoots) == 0 && len(spec.ModuleCacheEntries) == 0 {
		spec = defaultSpec(a.project.Mounts())
	}
	if len(spec.Actors) == 0 || len(spec.ModuleCacheAliases) == 0 || len(spec.AnalysisRoots) == 0 {
		return errors.New("link/module: incomplete actor-local deployment")
	}
	sort.Slice(spec.Actors, func(i, j int) bool { return spec.Actors[i].Name < spec.Actors[j].Name })
	actors := make(map[string]uint32, len(spec.Actors))
	a.actors = make([]actorRow, len(spec.Actors))
	for i, row := range spec.Actors {
		if row.Name == "" || (i > 0 && spec.Actors[i-1].Name == row.Name) {
			return errors.New("link/module: duplicate or empty actor")
		}
		a.actors[i] = actorRow{row.Name}
		actors[row.Name] = uint32(i + 1)
	}
	type instanceDraft struct {
		name           string
		actor          uint32
		representative string
	}
	var drafts []instanceDraft
	seen := map[string]struct{}{}
	for _, alias := range spec.ModuleCacheAliases {
		actor, ok := actors[alias.Actor]
		if !ok || len(alias.Instances) == 0 || alias.Representative == "" {
			return errors.New("link/module: malformed cache alias")
		}
		members := append([]string(nil), alias.Instances...)
		sort.Slice(members, func(i, j int) bool { return compareInstance(members[i], members[j]) < 0 })
		if members[0] != alias.Representative {
			return errors.New("link/module: noncanonical cache representative")
		}
		for i, name := range members {
			if name == "" || (i > 0 && members[i-1] == name) {
				return errors.New("link/module: duplicate cache instance")
			}
			if _, dup := seen[name]; dup {
				return errors.New("link/module: cache instance crosses alias classes")
			}
			seen[name] = struct{}{}
			drafts = append(drafts, instanceDraft{name, actor, alias.Representative})
		}
	}
	sort.Slice(drafts, func(i, j int) bool { return compareInstance(drafts[i].name, drafts[j].name) < 0 })
	a.instances = make([]instanceRow, len(drafts))
	byName := make(map[string]uint32, len(drafts))
	for i, x := range drafts {
		a.instances[i] = instanceRow{name: x.name, actor: x.actor}
		byName[x.name] = uint32(i + 1)
	}
	for i, x := range drafts {
		rep, ok := byName[x.representative]
		if !ok || a.instances[rep-1].actor != x.actor {
			return errors.New("link/module: foreign cache representative")
		}
		a.instances[i].representative = rep
	}
	mounts := a.project.Mounts()
	moduleByName := make(map[string]linkproject.Shard, mounts.Count())
	for i := 0; i < mounts.Count(); i++ {
		s, ok := mounts.At(i)
		if !ok {
			return errors.New("link/module: malformed project mount")
		}
		name, ok := mounts.Name(s)
		if !ok {
			return errors.New("link/module: malformed project mount")
		}
		moduleByName[name] = s
	}
	sort.Slice(spec.AnalysisRoots, func(i, j int) bool { return spec.AnalysisRoots[i].Name < spec.AnalysisRoots[j].Name })
	a.roots = make([]rootRow, len(spec.AnalysisRoots))
	rootByName := make(map[string]uint32, len(spec.AnalysisRoots))
	counts := make([]uint32, mounts.Count())
	for i, row := range spec.AnalysisRoots {
		shard, ok1 := moduleByName[row.Module]
		actor, ok2 := actors[row.Actor]
		instance, ok3 := byName[row.Instance]
		if row.Name == "" || !ok1 || !ok2 || !ok3 || a.instances[instance-1].actor != actor || (i > 0 && spec.AnalysisRoots[i-1].Name == row.Name) {
			return errors.New("link/module: malformed analysis root")
		}
		index, ok := mounts.Index(shard)
		if !ok {
			return errors.New("link/module: foreign root shard")
		}
		a.roots[i] = rootRow{row.Name, shard, actor, instance}
		rootByName[row.Name] = uint32(i + 1)
		counts[index]++
	}
	for i, n := range counts {
		if n == 0 {
			return errors.New("link/module: incomplete analysis roots")
		}
		_ = i
	}
	a.rootRanges = make([]rootRange, mounts.Count())
	next := make([]uint32, mounts.Count())
	var start uint32
	for i, n := range counts {
		a.rootRanges[i] = rootRange{start, start + n}
		next[i] = start
		start += n
	}
	a.rootIngress = make([]uint32, len(a.roots))
	for i, row := range a.roots {
		index, _ := mounts.Index(row.shard)
		at := next[index]
		a.rootIngress[at] = uint32(i + 1)
		next[index]++
	}
	apps := a.project.Applications()
	imports := apps.Imports()
	if imports.Count() != 0 {
		if _, ok := a.boundary.RequireOperation(); !ok {
			return errors.New("link/module: Program require has no target authority")
		}
	}
	type required struct {
		app      linkproject.Application
		from, to uint32
	}
	type importOccurrence struct {
		shard linkproject.Shard
		term  keyspace.Term
	}
	needed := make([]required, 0)
	// These resolvers exist only during admission.  They remove the former
	// Import×Mount and Entry×Import rescans and are discarded before Finalize.
	importsByOccurrence := make(map[importOccurrence]linkproject.Application, imports.Count())
	targetByOccurrence := make(map[importOccurrence]linkproject.Shard, imports.Count())
	for i := 0; i < imports.Count(); i++ {
		app, ok := imports.At(i)
		if !ok {
			return errors.New("link/module: malformed project import")
		}
		shard, term, _, ok := apps.Import(app)
		if !ok {
			return errors.New("link/module: malformed project import")
		}
		occurrence := importOccurrence{shard, term}
		if _, duplicate := importsByOccurrence[occurrence]; duplicate {
			return errors.New("link/module: duplicate project import")
		}
		importsByOccurrence[occurrence] = app
		loaded, ok := moduleImportTarget(moduleByName, mounts, shard, term)
		if !ok {
			continue
		}
		targetByOccurrence[occurrence] = loaded
		index, _ := mounts.Index(shard)
		for _, from := range a.rootIngress[a.rootRanges[index].start:a.rootRanges[index].end] {
			needed = append(needed, required{app, from, 0})
		}
	}
	// Resolve explicit ingress declarations against exact Program Import applications.
	entryByPair := make(map[[2]uint32]entryRow, len(spec.ModuleCacheEntries))
	for _, row := range spec.ModuleCacheEntries {
		shard, ok1 := moduleByName[row.Module]
		from, ok2 := rootByName[row.FromRoot]
		to, ok3 := rootByName[row.ToRoot]
		if !ok1 || !ok2 || !ok3 || row.Import == 0 {
			return errors.New("link/module: malformed cache entry")
		}
		if a.roots[from-1].shard != shard || a.roots[from-1].actor != a.roots[to-1].actor {
			return errors.New("link/module: cross-actor cache entry")
		}
		occurrence := importOccurrence{shard, row.Import}
		app, ok := importsByOccurrence[occurrence]
		if !ok {
			return errors.New("link/module: unsealed cache import")
		}
		loaded, ok := targetByOccurrence[occurrence]
		if !ok || loaded != a.roots[to-1].shard {
			return errors.New("link/module: cache entry target mismatch")
		}
		index, ok := apps.Index(app)
		if !ok {
			return errors.New("link/module: malformed cache application")
		}
		key := [2]uint32{uint32(index + 1), from}
		if _, dup := entryByPair[key]; dup {
			return errors.New("link/module: duplicate cache entry")
		}
		entryByPair[key] = entryRow{application: app, from: from, to: to}
	}
	if len(entryByPair) != len(needed) {
		return errors.New("link/module: incomplete cache ingress")
	}
	a.entries = make([]entryRow, 0, len(entryByPair))
	for _, need := range needed {
		index, _ := apps.Index(need.app)
		row, ok := entryByPair[[2]uint32{uint32(index + 1), need.from}]
		if !ok {
			return errors.New("link/module: incomplete cache ingress")
		}
		a.entries = append(a.entries, row)
	}
	sort.Slice(a.entries, func(i, j int) bool {
		ai, _ := apps.Index(a.entries[i].application)
		aj, _ := apps.Index(a.entries[j].application)
		if ai != aj {
			return ai < aj
		}
		return a.entries[i].from < a.entries[j].from
	})
	if err := a.acyclic(); err != nil {
		return err
	}
	a.spec = canonicalSpec(a)
	a.content = content(a)
	if !a.content.Available() {
		return errors.New("link/module: unavailable content identity")
	}
	a.hostRelation = hostRelationID(a)
	if !a.hostRelation.Available() {
		return errors.New("link/module: unavailable Host relation identity")
	}
	if err := a.indexes(); err != nil {
		return err
	}
	var ok bool
	if a.semanticReceipt, ok = a.component.buildSemanticSourceReceipt(); !ok {
		return errors.New("link/module: unavailable semantic-source receipt")
	}
	return nil
}

// hostRelationID is the narrow actor/root projection consumed by Host boot
// and globals. Cache aliases, ingress entries, coordinates, and init outcome
// geometry deliberately cannot churn it.
func hostRelationID(a *authority) (id identity.ContentID) {
	if a == nil || a.project == nil {
		return identity.ContentID{}
	}
	mountID, ok := a.project.MountRelationID()
	if !ok || !mountID.Available() {
		return identity.ContentID{}
	}
	h := sha256.New()
	var w framing.Writer
	if w.Reset(h, "program/link/module/host", 1) != nil || w.Record(1) != nil || w.Bytes(mountID[:]) != nil || w.Count(uint64(len(a.actors))) != nil {
		return identity.ContentID{}
	}
	for index, actor := range a.actors {
		if actor.name == "" || w.Uint(uint64(index+1)) != nil || w.String(actor.name) != nil {
			return identity.ContentID{}
		}
	}
	if w.Count(uint64(len(a.roots))) != nil {
		return identity.ContentID{}
	}
	for index, root := range a.roots {
		mount, ok := a.project.Mounts().Index(root.shard)
		if !ok || root.name == "" || root.actor == 0 || uint64(root.actor) > uint64(len(a.actors)) ||
			w.Uint(uint64(index+1)) != nil || w.String(root.name) != nil || w.Uint(uint64(mount+1)) != nil || w.Uint(uint64(root.actor)) != nil {
			return identity.ContentID{}
		}
	}
	if w.Finish() != nil {
		return identity.ContentID{}
	}
	if sum := h.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

func defaultSpec(mounts linkproject.Mounts) Spec {
	s := Spec{Actors: []ActorSpec{{Name: "default"}}}
	for i := 0; i < mounts.Count(); i++ {
		sh, ok := mounts.At(i)
		if !ok {
			continue
		}
		name, ok := mounts.Name(sh)
		if !ok {
			continue
		}
		inst := "module:" + name
		s.ModuleCacheAliases = append(s.ModuleCacheAliases, ModuleCacheAliasClassSpec{Actor: "default", Instances: []string{inst}, Representative: inst})
		s.AnalysisRoots = append(s.AnalysisRoots, AnalysisRootSpec{Name: "root:" + name, Module: name, Actor: "default", Instance: inst})
	}
	return s
}
func compareInstance(a, b string) int {
	var ab, bb bytes.Buffer
	var w framing.Writer
	_ = w.Reset(&ab, "program/link/module/instance", 1)
	_ = w.Record(1)
	_ = w.String(a)
	_ = w.Finish()
	_ = w.Reset(&bb, "program/link/module/instance", 1)
	_ = w.Record(1)
	_ = w.String(b)
	_ = w.Finish()
	return bytes.Compare(ab.Bytes(), bb.Bytes())
}
func moduleImportTarget(moduleByName map[string]linkproject.Shard, mounts linkproject.Mounts, shard linkproject.Shard, term keyspace.Term) (linkproject.Shard, bool) {
	p, ok := mounts.Program(shard)
	if !ok || p == nil {
		return linkproject.Shard{}, false
	}
	row, ok := p.Module().Import(term)
	if !ok || row.Key == 0 {
		return linkproject.Shard{}, false
	}
	name, ok := p.Source().Keys().Exact(row.Key)
	if !ok || name.Kind != keyspace.LiteralString {
		return linkproject.Shard{}, false
	}
	target, ok := moduleByName[name.String]
	return target, ok
}
func (a *authority) acyclic() error {
	n := len(a.roots)
	edges := make([][]uint32, n)
	degree := make([]int, n)
	for _, e := range a.entries {
		if e.from == 0 || e.to == 0 || int(e.from) > n || int(e.to) > n {
			return errors.New("link/module: malformed ingress")
		}
		edges[e.from-1] = append(edges[e.from-1], e.to)
		degree[e.to-1]++
	}
	ready := make([]uint32, 0, n)
	for i, d := range degree {
		if d == 0 {
			ready = append(ready, uint32(i+1))
		}
	}
	seen := 0
	for i := 0; i < len(ready); i++ {
		from := ready[i]
		seen++
		for _, to := range edges[from-1] {
			degree[to-1]--
			if degree[to-1] == 0 {
				ready = append(ready, to)
			}
		}
	}
	if seen != n {
		return errors.New("link/module: cache ingress cycle")
	}
	return nil
}
func canonicalSpec(a *authority) Spec {
	s := Spec{Actors: make([]ActorSpec, len(a.actors)), AnalysisRoots: make([]AnalysisRootSpec, len(a.roots)), ModuleCacheEntries: make([]ModuleCacheEntrySpec, len(a.entries))}
	for i, r := range a.actors {
		s.Actors[i] = ActorSpec{r.name}
	}
	groups := map[uint32][]string{}
	for _, x := range a.instances {
		groups[x.representative] = append(groups[x.representative], x.name)
	}
	reps := make([]uint32, 0, len(groups))
	for rep := range groups {
		reps = append(reps, rep)
	}
	sort.Slice(reps, func(i, j int) bool {
		return compareInstance(a.instances[reps[i]-1].name, a.instances[reps[j]-1].name) < 0
	})
	for _, rep := range reps {
		members := groups[rep]
		sort.Slice(members, func(i, j int) bool { return compareInstance(members[i], members[j]) < 0 })
		s.ModuleCacheAliases = append(s.ModuleCacheAliases, ModuleCacheAliasClassSpec{Actor: a.actors[a.instances[rep-1].actor-1].name, Instances: members, Representative: a.instances[rep-1].name})
	}
	mounts := a.project.Mounts()
	for i, r := range a.roots {
		name, _ := mounts.Name(r.shard)
		s.AnalysisRoots[i] = AnalysisRootSpec{Name: r.name, Module: name, Actor: a.actors[r.actor-1].name, Instance: a.instances[r.instance-1].name}
	}
	apps := a.project.Applications()
	for i, e := range a.entries {
		sh, term, _, _ := apps.Import(e.application)
		name, _ := mounts.Name(sh)
		s.ModuleCacheEntries[i] = ModuleCacheEntrySpec{Module: name, Import: term, FromRoot: a.roots[e.from-1].name, ToRoot: a.roots[e.to-1].name}
	}
	return s
}
func cloneSpec(s Spec) Spec {
	out := Spec{Actors: append([]ActorSpec(nil), s.Actors...), AnalysisRoots: append([]AnalysisRootSpec(nil), s.AnalysisRoots...), ModuleCacheEntries: append([]ModuleCacheEntrySpec(nil), s.ModuleCacheEntries...), ModuleCacheAliases: make([]ModuleCacheAliasClassSpec, len(s.ModuleCacheAliases))}
	for i, x := range s.ModuleCacheAliases {
		out.ModuleCacheAliases[i] = x
		out.ModuleCacheAliases[i].Instances = append([]string(nil), x.Instances...)
	}
	return out
}
func content(a *authority) (id identity.ContentID) {
	if a == nil || a.project == nil || a.boundary == nil {
		return
	}
	mountID, mountOK := a.project.MountRelationID()
	applicationID, applicationOK := a.project.ApplicationRelationID()
	moduleID, moduleOK := a.boundary.ModuleRelationID()
	if !mountOK || !applicationOK || !moduleOK || !mountID.Available() || !applicationID.Available() || !moduleID.Available() {
		return
	}
	h := sha256.New()
	var w framing.Writer
	if w.Reset(h, "program/link/module", 3) != nil || w.Record(1) != nil || w.Bytes(mountID[:]) != nil || w.Bytes(applicationID[:]) != nil || w.Bytes(moduleID[:]) != nil {
		return
	}
	s := a.spec
	if w.Count(uint64(len(s.Actors))) != nil {
		return
	}
	for _, x := range s.Actors {
		if w.String(x.Name) != nil {
			return
		}
	}
	if w.Count(uint64(len(s.ModuleCacheAliases))) != nil {
		return
	}
	for _, x := range s.ModuleCacheAliases {
		if w.String(x.Actor) != nil || w.String(x.Representative) != nil || w.Count(uint64(len(x.Instances))) != nil {
			return
		}
		for _, n := range x.Instances {
			if w.String(n) != nil {
				return
			}
		}
	}
	if w.Count(uint64(len(s.AnalysisRoots))) != nil {
		return
	}
	for _, x := range s.AnalysisRoots {
		if w.String(x.Name) != nil || w.String(x.Module) != nil || w.String(x.Actor) != nil || w.String(x.Instance) != nil {
			return
		}
	}
	if w.Count(uint64(len(s.ModuleCacheEntries))) != nil {
		return
	}
	for _, x := range s.ModuleCacheEntries {
		if w.String(x.Module) != nil || w.Uint(uint64(x.Import)) != nil || w.String(x.FromRoot) != nil || w.String(x.ToRoot) != nil {
			return
		}
	}
	if w.Finish() != nil {
		return
	}
	sum := h.Sum(id[:0])
	if len(sum) != len(id) {
		return identity.ContentID{}
	}
	return
}
