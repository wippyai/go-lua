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
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/internal/framing"
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
	content identity.ContentID
	spec    Spec
	counts  denominator.CountRows
	fence   *coldFence
}

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

// compositionEntry is the private parent-issued relation consumed by the
// publication phase.  It deliberately carries no authored module names and
// no Program/source authority: the source Shard, issued Import term, and
// owner-fenced root coordinates are all sealed while this Component is built.
// Publication later joins those coordinates to mounted Program rows.
type compositionEntry struct {
	sourceShard       linkproject.Shard
	sourceRootOrdinal uint32
	importTerm        keyspace.Term
	fromRootOrdinal   uint32
	toRootOrdinal     uint32
	fromRootID        identity.ContentID
	toRootID          identity.ContentID
	actorID           identity.ContentID
	representativeID  identity.ContentID
}
type rootRange struct{ start, end uint32 }
type authority struct {
	component       *Component
	project         *linkproject.Component
	boundary        *linkboundary.Component
	actors          []actorRow
	instances       []instanceRow
	roots           []rootRow
	rootRanges      []rootRange
	rootIngress     []uint32
	authoredEntries []ModuleCacheEntrySpec
	composition     []compositionEntry
	spec            Spec
	content         identity.ContentID
	counts          denominator.CountRows
	hostRelation    identity.ContentID
	fence           *coldFence
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
	return Cold{content: a.content, spec: cloneSpec(a.spec), counts: a.counts, fence: a.fence}
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
	if len(spec.Actors) == 0 && len(spec.ModuleCacheAliases) == 0 && len(spec.AnalysisRoots) == 0 {
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
	// Module-cache entries remain authored Link configuration. Validate only
	// their local source/root shape and Import term family here. Snapshot
	// composition owns exact Program request-target joins, completeness, and
	// cycle admission after Artifact facts are available.
	type authoredEntryKey struct {
		module     string
		importTerm keyspace.Term
		fromRoot   string
	}
	seenEntries := make(map[authoredEntryKey]struct{}, len(spec.ModuleCacheEntries))
	for _, row := range spec.ModuleCacheEntries {
		shard, moduleOK := moduleByName[row.Module]
		from, fromOK := rootByName[row.FromRoot]
		to, toOK := rootByName[row.ToRoot]
		if !moduleOK || !fromOK || !toOK || row.Import == 0 || keyspace.TermFamily(row.Import) != keyspace.FamilyImport {
			return errors.New("link/module: malformed authored cache entry")
		}
		fromRoot, toRoot := a.roots[from-1], a.roots[to-1]
		if fromRoot.shard != shard || fromRoot.actor != toRoot.actor {
			return errors.New("link/module: malformed authored cache entry roots")
		}
		key := authoredEntryKey{module: row.Module, importTerm: row.Import, fromRoot: row.FromRoot}
		if _, duplicate := seenEntries[key]; duplicate {
			return errors.New("link/module: duplicate authored cache entry")
		}
		seenEntries[key] = struct{}{}
	}
	sort.Slice(spec.ModuleCacheEntries, func(i, j int) bool {
		left, right := spec.ModuleCacheEntries[i], spec.ModuleCacheEntries[j]
		if left.Module != right.Module {
			return left.Module < right.Module
		}
		if left.Import != right.Import {
			return left.Import < right.Import
		}
		if left.FromRoot != right.FromRoot {
			return left.FromRoot < right.FromRoot
		}
		return left.ToRoot < right.ToRoot
	})
	a.authoredEntries = append([]ModuleCacheEntrySpec(nil), spec.ModuleCacheEntries...)
	a.spec = canonicalSpec(a)
	a.content = content(a)
	if !a.content.Available() {
		return errors.New("link/module: unavailable content identity")
	}
	composition, compositionOK := buildCompositionEntries(a, spec, moduleByName, rootByName)
	if !compositionOK {
		return errors.New("link/module: unavailable parent composition relation")
	}
	a.composition = composition
	a.hostRelation = hostRelationID(a)
	if !a.hostRelation.Available() {
		return errors.New("link/module: unavailable Host relation identity")
	}
	if err := a.indexes(); err != nil {
		return err
	}
	var ok bool
	if a.counts, ok = buildCountRows(a.component); !ok {
		return errors.New("link/module: unavailable module denominator rows")
	}
	return nil
}

// buildCompositionEntries seals the narrow relation that the later Snapshot
// publication phase is allowed to consume.  This is intentionally a Link
// child construction step: authored names are resolved to the exact Project
// Shard and root ordinals here, while Program request/target joins remain a
// publication concern.
func buildCompositionEntries(a *authority, spec Spec, moduleByName map[string]linkproject.Shard, rootByName map[string]uint32) ([]compositionEntry, bool) {
	if a == nil || a.project == nil || !a.content.Available() {
		return nil, false
	}
	entries := make([]compositionEntry, 0, len(spec.ModuleCacheEntries))
	for _, authored := range spec.ModuleCacheEntries {
		sourceShard, sourceOK := moduleByName[authored.Module]
		fromOrdinal, fromOK := rootByName[authored.FromRoot]
		toOrdinal, toOK := rootByName[authored.ToRoot]
		if !sourceOK || !fromOK || !toOK || authored.Import == 0 ||
			keyspace.TermFamily(authored.Import) != keyspace.FamilyImport || keyspace.TermOrdinal(authored.Import) == 0 ||
			fromOrdinal > uint32(len(a.roots)) || toOrdinal > uint32(len(a.roots)) {
			return nil, false
		}
		fromRoot := a.roots[fromOrdinal-1]
		toRoot := a.roots[toOrdinal-1]
		if fromRoot.shard != sourceShard || fromRoot.actor != toRoot.actor || fromRoot.actor == 0 ||
			fromRoot.instance == 0 || uint64(fromRoot.instance) > uint64(len(a.instances)) {
			return nil, false
		}
		representative := a.instances[fromRoot.instance-1].representative
		if representative == 0 || uint64(representative) > uint64(len(a.instances)) {
			return nil, false
		}
		fromRootID := denseID(a.content, 0x6d6f64756c652d72, uint64(fromOrdinal))
		toRootID := denseID(a.content, 0x6d6f64756c652d72, uint64(toOrdinal))
		actorID := denseID(a.content, 0x6d6f64756c652d61, uint64(fromRoot.actor))
		representativeID := denseID(a.content, 0x6d6f64756c652d69, uint64(representative))
		if !fromRootID.Available() || !toRootID.Available() || !actorID.Available() || !representativeID.Available() {
			return nil, false
		}
		entries = append(entries, compositionEntry{
			sourceShard: sourceShard, sourceRootOrdinal: fromOrdinal, importTerm: authored.Import,
			fromRootOrdinal: fromOrdinal, toRootOrdinal: toOrdinal,
			fromRootID: fromRootID, toRootID: toRootID, actorID: actorID, representativeID: representativeID,
		})
	}
	return entries, true
}

// hostRelationID is the narrow actor/root projection consumed by Host boot
// and globals. Cache aliases, authored ingress configuration, and any
// Snapshot-resolved geometry deliberately cannot churn it.
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
func canonicalSpec(a *authority) Spec {
	s := Spec{Actors: make([]ActorSpec, len(a.actors)), AnalysisRoots: make([]AnalysisRootSpec, len(a.roots)), ModuleCacheEntries: append([]ModuleCacheEntrySpec(nil), a.authoredEntries...)}
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
	if a == nil || a.project == nil {
		return
	}
	mountID, mountOK := a.project.MountRelationID()
	if !mountOK || !mountID.Available() {
		return
	}
	h := sha256.New()
	var w framing.Writer
	if w.Reset(h, "program/link/module", 4) != nil || w.Record(1) != nil || w.Bytes(mountID[:]) != nil {
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
