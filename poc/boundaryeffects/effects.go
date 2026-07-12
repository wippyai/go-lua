// Package boundaryeffects is an isolated proof that an acyclic lexical body
// can be reduced to guarded boundary effects. It is deliberately not imported
// by production.
package boundaryeffects

import (
	"errors"
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

var ErrStateDependent = errors.New("boundaryeffects: state-dependent effect is outside exact slice")

// Root is a dense lexical boundary-root index. It avoids maps during binding
// and instantiation.
type Root uint8

const (
	Left Root = iota
	Right
	Output
	rootCount
)

// Points names the observation points in the acyclic diamond used by the
// function-level proof. Result follows transfer.Result's point-input convention.
type Points struct {
	Entry, Branch, Then, Else, Join, Final, Exit cfg.Point
}

// Plan is the compact, immutable semantic result of compilation. The three
// lexical suffixes are the only syntax retained. Control flow has disappeared:
// execution applies two guarded outcomes and one joined tail.
type Plan struct {
	points Points
	suffix [4]string // left, right, chosen, final
}

// CompileDiamond validates the exact shape represented by this proof.
func CompileDiamond(points Points) (Plan, error) {
	ids := []cfg.Point{points.Entry, points.Branch, points.Then, points.Else, points.Join, points.Final, points.Exit}
	seen := make(map[cfg.Point]struct{}, len(ids))
	for _, point := range ids {
		if _, ok := seen[point]; ok {
			return Plan{}, fmt.Errorf("boundaryeffects: duplicate point %d", point)
		}
		seen[point] = struct{}{}
	}
	return Plan{points: points, suffix: [4]string{"left.value", "right.value", "result.chosen", "result.final"}}, nil
}

// RootBinding binds one lexical root to a caller root. Caller paths are borrowed
// immutably; PackBindings itself allocates no maps or slices.
type RootBinding struct {
	Lexical Root
	Caller  pathdom.Path
}

// PackedBindings is fixed-size and searched by direct index.
type PackedBindings struct {
	caller [rootCount]pathdom.Path
	mask   uint8
}

func PackBindings(bindings ...RootBinding) (PackedBindings, error) {
	var out PackedBindings
	for _, binding := range bindings {
		if binding.Lexical >= rootCount || binding.Caller.IsEmpty() {
			return PackedBindings{}, fmt.Errorf("boundaryeffects: invalid root binding")
		}
		bit := uint8(1) << binding.Lexical
		if out.mask&bit != 0 {
			return PackedBindings{}, fmt.Errorf("boundaryeffects: duplicate root %d", binding.Lexical)
		}
		out.mask |= bit
		out.caller[binding.Lexical] = binding.Caller
	}
	if out.mask != (uint8(1)<<rootCount)-1 {
		return PackedBindings{}, fmt.Errorf("boundaryeffects: incomplete root bindings")
	}
	return out, nil
}

type boundPath struct {
	path  pathdom.Path
	key   pathdom.PathKey
	local keyspace.Key
	state pathaddr.StateKey
}

// Bound is caller-shaped and contains no lookup maps.
type Bound struct {
	plan  Plan
	paths [4]boundPath
}

func (p Plan) Bind(bindings PackedBindings, resolver *visibility.Resolver) (Bound, error) {
	if resolver == nil || resolver.KeySpace() == nil {
		return Bound{}, fmt.Errorf("boundaryeffects: nil resolver")
	}
	defs := [4]struct {
		root   Root
		point  cfg.Point
		suffix string
	}{
		{Left, p.points.Branch, p.suffix[0]}, {Right, p.points.Branch, p.suffix[1]},
		{Output, p.points.Then, p.suffix[2]}, {Output, p.points.Final, p.suffix[3]},
	}
	var out Bound
	out.plan = p
	for i, def := range defs {
		path := appendFields(bindings.caller[def.root], def.suffix)
		key, ok := visibility.AddressAt(resolver, def.point, path).VisiblePathKey()
		if !ok {
			return Bound{}, fmt.Errorf("boundaryeffects: path %d is not visible", i)
		}
		stateKey, ok := pathaddr.StateKeyFromPathKey(key)
		if !ok {
			return Bound{}, fmt.Errorf("boundaryeffects: invalid state key %q", key)
		}
		local, ok := resolver.KeySpace().FromPathKey(key)
		if !ok {
			return Bound{}, fmt.Errorf("boundaryeffects: uninterned path %q", key)
		}
		out.paths[i] = boundPath{path: path, key: key, local: local, state: stateKey}
	}
	return out, nil
}

func appendFields(base pathdom.Path, dotted string) pathdom.Path {
	out := base
	start := 0
	for i := 0; i <= len(dotted); i++ {
		if i == len(dotted) || dotted[i] == '.' {
			out = out.Field(dotted[start:i])
			start = i + 1
		}
	}
	return out
}

type Config struct {
	Registry   *axis.Registry
	Resolver   *visibility.Resolver
	Entry      state.State
	StateLanes []state.LaneID
}

type Result map[cfg.Point]state.State

// Observation identifies a point-input snapshot without retaining CFG maps.
type Observation uint8

const (
	ObserveEntry Observation = iota
	ObserveBranch
	ObserveThen
	ObserveElse
	ObserveJoin
	ObserveFinal
	ObserveExit
	observationCount
)

// ObservationPlan is a packed selection of snapshots required by a consumer.
// Summary construction normally requests only Exit; materialization can select
// exactly the diagnostic points it needs.
type ObservationPlan uint8

func Observe(points ...Observation) ObservationPlan {
	var plan ObservationPlan
	for _, point := range points {
		if point < observationCount {
			plan |= 1 << point
		}
	}
	return plan
}

func ObserveAll() ObservationPlan { return (1 << observationCount) - 1 }

// Observations is caller-owned fixed storage. ExecuteObserved overwrites it and
// performs no observation-map allocation.
type Observations struct {
	states [observationCount]state.State
	mask   ObservationPlan
}

func (o Observations) Get(point Observation) (state.State, bool) {
	if point >= observationCount || o.mask&(1<<point) == 0 {
		return state.State{}, false
	}
	return o.states[point], true
}

func (o *Observations) record(plan ObservationPlan, point Observation, value state.State) {
	if o != nil && plan&(1<<point) != 0 {
		o.states[point], o.mask = value, o.mask|(1<<point)
	}
}

// Execute applies the composed effects and materializes the compatibility map.
// Summary consumers should use ExecuteExit; sparse diagnostic consumers should
// use ExecuteObserved.
func (b Bound) Execute(config Config) (Result, error) {
	var observed Observations
	_, err := b.ExecuteObserved(config, ObserveAll(), &observed)
	if err != nil {
		return nil, err
	}
	result := make(Result, observationCount)
	points := [...]cfg.Point{b.plan.points.Entry, b.plan.points.Branch, b.plan.points.Then, b.plan.points.Else, b.plan.points.Join, b.plan.points.Final, b.plan.points.Exit}
	for i, point := range points {
		result[point] = observed.states[i]
	}
	return result, nil
}

// ExecuteExit computes only the summary boundary and avoids all point-result
// storage. The semantic operations remain exact and identical.
func (b Bound) ExecuteExit(config Config) (state.State, error) {
	return b.ExecuteObserved(config, 0, nil)
}

// ExecuteObserved computes the boundary and stores only selected point inputs.
func (b Bound) ExecuteObserved(config Config, plan ObservationPlan, observed *Observations) (state.State, error) {
	if config.Registry == nil || config.Resolver == nil {
		return state.State{}, fmt.Errorf("boundaryeffects: incomplete config")
	}
	domain := state.Domain(config.Registry)
	if config.StateLanes != nil {
		domain = state.DomainWithLanes(config.Registry, config.StateLanes)
	}
	entry := config.Entry
	if config.StateLanes != nil {
		entry = state.NormalizeForDomain(domain, entry)
	}
	reachableEmpty := state.Reachable(state.State{})
	if config.StateLanes != nil {
		reachableEmpty = state.NormalizeForDomain(domain, reachableEmpty)
	}
	if !domain.Equal(reachableEmpty, domain.Bottom()) {
		entry = state.Reachable(entry)
		if config.StateLanes != nil {
			entry = state.NormalizeForDomain(domain, entry)
		}
	}
	config.Entry = entry
	if err := b.admit(config); err != nil {
		return state.State{}, err
	}
	if observed != nil {
		*observed = Observations{}
	}
	if domain.Equal(entry, domain.Bottom()) {
		for point := Observation(0); point < observationCount; point++ {
			observed.record(plan, point, domain.Bottom())
		}
		return domain.Bottom(), nil
	}
	normalize := func(value state.State) state.State {
		if config.StateLanes != nil {
			return state.NormalizeForDomain(domain, value)
		}
		return value
	}
	observed.record(plan, ObserveEntry, entry)
	observed.record(plan, ObserveBranch, entry)

	// Equality outcome: the guard is one meet, then chosen and final are pure
	// copies. Point observations are retained without replaying CFG kernels.
	eq, reachable := b.equalGuard(config.Registry, config.Resolver.KeySpace(), entry)
	if !reachable {
		eq = domain.Bottom()
	} else {
		eq = normalize(eq)
	}
	observed.record(plan, ObserveThen, eq)
	thenOut := eq
	if reachable {
		thenOut = normalize(b.copy(config, eq, b.paths[2], b.paths[0]))
	}

	// Inequality has no concrete effect for the admitted non-origin values.
	observed.record(plan, ObserveElse, entry)
	elseOut := normalize(b.copy(config, entry, b.paths[2], b.paths[1]))

	joined := domain.Join(thenOut, elseOut)
	observed.record(plan, ObserveJoin, joined)
	observed.record(plan, ObserveFinal, joined)
	exit := normalize(b.copy(config, joined, b.paths[3], b.paths[2]))
	observed.record(plan, ObserveExit, exit)
	return exit, nil
}

func (b Bound) equalGuard(reg *axis.Registry, ks *keyspace.KeySpace, in state.State) (state.State, bool) {
	left := in.ReadPathKey(reg, ks, b.paths[0].key)
	right := in.ReadPathKey(reg, ks, b.paths[1].key)
	if product.Equal(reg, left, product.Bottom(reg)) || product.Equal(reg, right, product.Bottom(reg)) {
		return in, true
	}
	meet := product.Meet(reg, left, right)
	if product.Equal(reg, meet, product.Bottom(reg)) {
		return state.Domain(reg).Bottom(), false
	}
	edit := in.EditPathEvidence(reg)
	edit.WriteLocalPathKey(b.paths[0].local, meet)
	edit.WriteLocalPathKey(b.paths[1].local, meet)
	return edit.Done(), true
}

func (b Bound) copy(config Config, in state.State, target, source boundPath) state.State {
	ks := config.Resolver.KeySpace()
	value := in.ReadPathKey(config.Registry, ks, source.key)
	if product.Equal(config.Registry, value, product.Bottom(config.Registry)) {
		return in
	}
	out, _ := in.InvalidatePathKeySubtree(ks, target.key)
	edit := out.EditPathEvidence(config.Registry)
	edit.WriteLocalPathKey(target.local, value)
	if canonical, ok := ks.FieldCanonical(target.local); ok {
		edit.WriteLocalPathKey(canonical, value)
	}
	out = edit.Done()
	out = out.AddBranchProof(pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: target.local, Other: source.local})
	out = out.CanonicalizeTypestateResources(ks)
	return out.PropagateUserAssignmentFrom(config.Registry, ks, target.state, in, source.state)
}

// admit rejects the concrete features whose effects depend on hidden heap or
// variant structure. They need symbolic lane adapters; treating them as fixed
// kill/gen deltas would be unsound.
func (b Bound) admit(config Config) error {
	ks := config.Resolver.KeySpace()
	if snap := config.Entry.HeapTableObjectsSnapshot(); len(snap.Objects) != 0 {
		return ErrStateDependent
	}
	if proofs := config.Entry.BranchProofsSnapshot(ks); len(proofs.Proofs) != 0 {
		return ErrStateDependent
	}
	for _, path := range b.paths[:2] {
		value := config.Entry.ReadPathKey(config.Registry, ks, path.key)
		if _, ok := product.Get(config.Registry, value, identity.Key).ID(); ok {
			return ErrStateDependent
		}
		if origin := product.Get(config.Registry, value, variantorigin.Key); !origin.IsBottom() && !origin.IsTop() {
			return ErrStateDependent
		}
		root := config.Entry.ReadValue(config.Registry, statekey.SymbolValue(path.path.Symbol))
		if origin := product.Get(config.Registry, root, variantorigin.Key); !origin.IsBottom() && !origin.IsTop() {
			return ErrStateDependent
		}
	}
	staticDependent := false
	config.Entry.ForEachPathStaticMember(func(member keyspace.Key, _ product.Value) bool {
		for _, source := range b.paths[:2] {
			if member == source.local || ks.HasStrictPrefix(member, source.local) {
				staticDependent = true
				return false
			}
		}
		return true
	})
	if staticDependent {
		return ErrStateDependent
	}
	// Result roots must be fresh at entry; otherwise root-origin invalidation
	// and alias closure are state-dependent rather than fixed boundary effects.
	rootSlot := statekey.SymbolValue(b.paths[2].path.Symbol)
	if value := config.Entry.ReadValue(config.Registry, rootSlot); !product.Equal(config.Registry, value, product.Bottom(config.Registry)) {
		return ErrStateDependent
	}
	return nil
}

// SortedPoints provides deterministic observation iteration in tests/tools.
func (r Result) SortedPoints() []cfg.Point {
	out := make([]cfg.Point, 0, len(r))
	for point := range r {
		out = append(out, point)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Keep the boundary vocabulary anchored to lexical identities in this POC.
var _ symbol.ID
