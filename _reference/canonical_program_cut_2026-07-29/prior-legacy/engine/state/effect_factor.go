package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/__legacy/analysis/internal/mapedit"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// DynamicIndexMembershipFactorConfig is the complete frozen input to the
// coupled dynamic-index/key-membership endomorphism. Equivalence is resolved
// before sealing and carried by RestoreKeys; applying the factor never scans
// another lane or reconstructs State.
type DynamicIndexMembershipFactorConfig struct {
	Key               dynamicindex.Key
	Fact              dynamicindex.Fact
	TableStateKeys    []pathaddr.StateKey
	AllValueTables    []pathaddr.StateKey
	PendingRestores   []PendingDynamicAllValueRestore
	RestoreKeys       []pathaddr.StateKey
	KeyStateKey       pathaddr.StateKey
	MembershipTable   pathaddr.StateKey
	SourceMemberships []pathaddr.StateKey
	TableSymbol       symbol.ID
	HasKeyStateKey    bool
	DefinitelyPresent bool
	DefinitelyAbsent  bool
	MayBeAbsent       bool
}

// DynamicIndexMembershipEvidenceQuery is the closed factor-native observation
// needed to prepare one dynamic write. It names semantic roles, not axes.
type DynamicIndexMembershipEvidenceQuery struct {
	Container       keyspace.Key
	KeyStateKey     pathaddr.StateKey
	SourceStateKeys []pathaddr.StateKey
	TableStateKeys  []pathaddr.StateKey
}

// DynamicIndexMembershipEvidence is the exact finite membership residue used
// by DynamicIndexMembershipFactorPlan preparation.
type DynamicIndexMembershipEvidence struct {
	AllValueTables    []pathaddr.StateKey
	SourceMemberships []pathaddr.StateKey
	PendingRestores   []PendingDynamicAllValueRestore
	readOrigins       []DynamicIndexReadOrigin
	tableMemberships  []dynamicIndexAllValueMembership
}

type dynamicIndexAllValueMembership struct {
	container keyspace.Key
	table     pathaddr.StateKey
}

// DynamicIndexMembershipFactorPlan is the ProductDomain-sealed factor program
// for one direct dynamic write. Exactly the registered dynamic-index and
// key-membership lanes participate.
type DynamicIndexMembershipFactorPlan struct {
	seal   *productDomainSeal
	reg    *axis.Registry
	keys   *keyspace.KeySpace
	config DynamicIndexMembershipFactorConfig
}

// EffectDeltaFactorPlan is the ProductDomain-sealed replacement of one effect
// delta. Exactly the registered effect-delta lane participates.
type EffectDeltaFactorPlan struct {
	seal  *productDomainSeal
	key   effectdelta.Key
	delta effectdelta.Value
}

func validEffectStateKey(keys *keyspace.KeySpace, key pathaddr.StateKey) bool {
	if keys == nil || !keys.Valid() || key == "" {
		return false
	}
	_, ok := keys.FromStateKey(key.PathKey())
	return ok
}

func uniqueEffectStateKeys(keys *keyspace.KeySpace, values []pathaddr.StateKey) ([]pathaddr.StateKey, bool) {
	out := make([]pathaddr.StateKey, 0, len(values))
	seen := make(map[pathaddr.StateKey]struct{}, len(values))
	for _, value := range values {
		if !validEffectStateKey(keys, value) {
			return nil, false
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out, true
}

func (d ProductDomain) PrepareDynamicIndexMembershipFactorPlan(keys *keyspace.KeySpace, config DynamicIndexMembershipFactorConfig) (DynamicIndexMembershipFactorPlan, error) {
	if !d.Valid() || keys == nil || !keys.Valid() || config.Key.Table.Kind == keyspace.KindInvalid || config.Key.Site == "" {
		return DynamicIndexMembershipFactorPlan{}, fmt.Errorf("state: invalid dynamic-index membership factor")
	}
	var ok bool
	if config.TableStateKeys, ok = uniqueEffectStateKeys(keys, config.TableStateKeys); !ok {
		return DynamicIndexMembershipFactorPlan{}, fmt.Errorf("state: foreign table state key in dynamic-index factor")
	}
	if config.AllValueTables, ok = uniqueEffectStateKeys(keys, config.AllValueTables); !ok {
		return DynamicIndexMembershipFactorPlan{}, fmt.Errorf("state: foreign all-values table in dynamic-index factor")
	}
	if config.SourceMemberships, ok = uniqueEffectStateKeys(keys, config.SourceMemberships); !ok {
		return DynamicIndexMembershipFactorPlan{}, fmt.Errorf("state: foreign source membership in dynamic-index factor")
	}
	if config.RestoreKeys, ok = uniqueEffectStateKeys(keys, config.RestoreKeys); !ok {
		return DynamicIndexMembershipFactorPlan{}, fmt.Errorf("state: foreign restore key in dynamic-index factor")
	}
	if config.HasKeyStateKey && !validEffectStateKey(keys, config.KeyStateKey) {
		return DynamicIndexMembershipFactorPlan{}, fmt.Errorf("state: foreign key state key in dynamic-index factor")
	}
	if config.HasKeyStateKey && !validEffectStateKey(keys, config.MembershipTable) {
		return DynamicIndexMembershipFactorPlan{}, fmt.Errorf("state: foreign membership table in dynamic-index factor")
	}
	config.PendingRestores = append([]PendingDynamicAllValueRestore(nil), config.PendingRestores...)
	for _, restore := range config.PendingRestores {
		if restore.Container.Kind == keyspace.KindInvalid || !validEffectStateKey(keys, restore.Table) || !validEffectStateKey(keys, restore.Key) {
			return DynamicIndexMembershipFactorPlan{}, fmt.Errorf("state: invalid pending restore in dynamic-index factor")
		}
	}
	return DynamicIndexMembershipFactorPlan{seal: d.seal, reg: d.reg, keys: keys, config: config}, nil
}

func (d ProductDomain) PrepareEffectDeltaFactorPlan(key effectdelta.Key, delta effectdelta.Value) (EffectDeltaFactorPlan, error) {
	if !d.Valid() || key.Target.Kind == keyspace.KindInvalid || key.Site == "" || key.Kind == 0 {
		return EffectDeltaFactorPlan{}, fmt.Errorf("state: invalid effect-delta factor")
	}
	return EffectDeltaFactorPlan{seal: d.seal, key: key, delta: delta}, nil
}

func (d ProductDomain) ownsDynamicIndexMembershipFactor(plan DynamicIndexMembershipFactorPlan) bool {
	return d.Valid() && plan.seal == d.seal && plan.reg == d.reg && plan.keys != nil && plan.keys.Valid()
}

func (d ProductDomain) ownsEffectDeltaFactor(plan EffectDeltaFactorPlan) bool {
	return d.Valid() && plan.seal == d.seal && plan.key.Target.Kind != keyspace.KindInvalid
}

func (d ProductDomain) effectFactorLanes(kind effectFactorKind) []ProductLane {
	if !d.Valid() {
		return nil
	}
	out := make([]ProductLane, 0, 2)
	for index := range d.factorLanes {
		runtime := &d.factorLanes[index]
		law, declared := findLaneSemanticLaw(runtime.semanticLaws, laneSemanticEffectFactor)
		if declared && law.participates && law.effectKinds&kind != 0 {
			out = append(out, runtime.lane)
		}
	}
	return out
}

func (d ProductDomain) DynamicIndexMembershipFactorLanes() []ProductLane {
	return d.effectFactorLanes(effectFactorDynamicIndexMembership)
}

func (d ProductDomain) EffectDeltaFactorLanes() []ProductLane {
	return d.effectFactorLanes(effectFactorDelta)
}

// ObserveDynamicIndexMembershipEvidence dispatches through the sole
// registration that owns membership observation. No State reconstruction or
// lane-name switch is permitted.
func (d ProductDomain) ObserveDynamicIndexMembershipEvidence(factor LaneFactor, query DynamicIndexMembershipEvidenceQuery) (DynamicIndexMembershipEvidence, error) {
	if !d.Valid() || query.Container.Kind == keyspace.KindInvalid {
		return DynamicIndexMembershipEvidence{}, fmt.Errorf("state: invalid dynamic-index membership evidence query")
	}
	runtime, err := d.validateFactor(factor)
	if err != nil {
		return DynamicIndexMembershipEvidence{}, err
	}
	law, declared := findLaneSemanticLaw(runtime.semanticLaws, laneSemanticEffectFactor)
	if !declared || law.effectObserve == nil {
		return DynamicIndexMembershipEvidence{}, fmt.Errorf("%w: lane %q does not own membership observation", ErrInvalidLaneFactor, runtime.lane.id)
	}
	evidence, ok := law.effectObserve(factor.payload, query)
	if !ok {
		return DynamicIndexMembershipEvidence{}, fmt.Errorf("state: membership observation rejected query")
	}
	return evidence, nil
}

// ObserveDynamicIndexMutationEvidence is the single temporal preparation law
// for an IndexMutation. Source membership and read origins are observed before
// mutation; all-values membership is observed after descendant invalidation.
func (d ProductDomain) ObserveDynamicIndexMutationEvidence(input, output LaneFactor, query DynamicIndexMembershipEvidenceQuery) (DynamicIndexMembershipEvidence, error) {
	before, err := d.ObserveDynamicIndexMembershipEvidence(input, DynamicIndexMembershipEvidenceQuery{
		Container: query.Container, KeyStateKey: query.KeyStateKey, SourceStateKeys: query.SourceStateKeys,
	})
	if err != nil {
		return DynamicIndexMembershipEvidence{}, err
	}
	after, err := d.ObserveDynamicIndexMembershipEvidence(output, DynamicIndexMembershipEvidenceQuery{
		Container: query.Container, TableStateKeys: query.TableStateKeys,
	})
	if err != nil {
		return DynamicIndexMembershipEvidence{}, err
	}
	evidence := DynamicIndexMembershipEvidence{
		AllValueTables:    append([]pathaddr.StateKey(nil), after.AllValueTables...),
		SourceMemberships: append([]pathaddr.StateKey(nil), before.SourceMemberships...),
	}
	for _, origin := range before.readOrigins {
		for _, membership := range after.tableMemberships {
			if origin.Container == membership.container {
				evidence.PendingRestores = append(evidence.PendingRestores, PendingDynamicAllValueRestore{
					Container: membership.container, Table: membership.table, Key: origin.Key,
				})
			}
		}
	}
	return evidence, nil
}

func observeDynamicIndexMembershipEvidence(lane keyMembershipLane, query DynamicIndexMembershipEvidenceQuery) (DynamicIndexMembershipEvidence, bool) {
	if query.Container.Kind == keyspace.KindInvalid {
		return DynamicIndexMembershipEvidence{}, false
	}
	out := DynamicIndexMembershipEvidence{}
	if !lane.bottom {
		for membership := range lane.dynamicAll {
			if membership.Kind != KeyMembershipDynamicIndexAllValues {
				continue
			}
			if membership.Container == query.Container {
				out.AllValueTables = append(out.AllValueTables, membership.Table)
			}
			for _, table := range query.TableStateKeys {
				if membership.Table == table {
					out.tableMemberships = append(out.tableMemberships, dynamicIndexAllValueMembership{container: membership.Container, table: table})
				}
			}
		}
		if len(query.SourceStateKeys) != 0 {
			seen := make(map[pathaddr.StateKey]struct{})
			for membership := range lane.path {
				if membership.Kind == KeyMembershipPath && effectStateKeyIn(query.SourceStateKeys, membership.Key) {
					if _, exists := seen[membership.Table]; !exists {
						seen[membership.Table] = struct{}{}
						out.SourceMemberships = append(out.SourceMemberships, membership.Table)
					}
				}
			}
			for origin := range lane.valueOrigins {
				if !effectStateKeyIn(query.SourceStateKeys, origin.Value) {
					continue
				}
				for membership := range lane.dynamicAll {
					if membership.Kind == KeyMembershipDynamicIndexAllValues && membership.Container == origin.Container {
						if _, exists := seen[membership.Table]; !exists {
							seen[membership.Table] = struct{}{}
							out.SourceMemberships = append(out.SourceMemberships, membership.Table)
						}
					}
				}
				if lane.dynamicTop {
					continue
				}
				for membership := range lane.dynamic {
					if membership.Kind == KeyMembershipDynamicIndexValue && membership.Container == origin.Container && membership.Site == origin.Site {
						if _, exists := seen[membership.Table]; !exists {
							seen[membership.Table] = struct{}{}
							out.SourceMemberships = append(out.SourceMemberships, membership.Table)
						}
					}
				}
			}
		}
		if query.KeyStateKey != "" {
			for origin := range lane.readOrigins {
				if origin.Value != query.KeyStateKey {
					continue
				}
				out.readOrigins = append(out.readOrigins, origin)
				for _, table := range query.TableStateKeys {
					for membership := range lane.dynamicAll {
						if membership.Kind == KeyMembershipDynamicIndexAllValues && membership.Table == table && membership.Container == origin.Container {
							out.PendingRestores = append(out.PendingRestores, PendingDynamicAllValueRestore{Container: origin.Container, Table: table, Key: origin.Key})
						}
					}
				}
			}
		}
	}
	sort.Slice(out.AllValueTables, func(i, j int) bool { return out.AllValueTables[i] < out.AllValueTables[j] })
	sort.Slice(out.SourceMemberships, func(i, j int) bool { return out.SourceMemberships[i] < out.SourceMemberships[j] })
	sort.Slice(out.PendingRestores, func(i, j int) bool {
		left, right := out.PendingRestores[i], out.PendingRestores[j]
		if left.Container != right.Container {
			return effectKeyLess(left.Container, right.Container)
		}
		if left.Table != right.Table {
			return left.Table < right.Table
		}
		return left.Key < right.Key
	})
	sort.Slice(out.readOrigins, func(i, j int) bool {
		left, right := out.readOrigins[i], out.readOrigins[j]
		if left.Container != right.Container {
			return effectKeyLess(left.Container, right.Container)
		}
		if left.Value != right.Value {
			return left.Value < right.Value
		}
		return left.Key < right.Key
	})
	sort.Slice(out.tableMemberships, func(i, j int) bool {
		left, right := out.tableMemberships[i], out.tableMemberships[j]
		if left.container != right.container {
			return effectKeyLess(left.container, right.container)
		}
		return left.table < right.table
	})
	return out, true
}

func effectKeyLess(left, right keyspace.Key) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.Sym != right.Sym {
		return left.Sym < right.Sym
	}
	if left.Ver != right.Ver {
		return left.Ver < right.Ver
	}
	if left.Root != right.Root {
		return left.Root < right.Root
	}
	if left.Segs != right.Segs {
		return left.Segs < right.Segs
	}
	return !left.Canon && right.Canon
}

func (d ProductDomain) applyEffectFactor(request effectFactorRequest, current LaneFactor) (LaneFactor, error) {
	runtime, err := d.validateFactor(current)
	if err != nil {
		return LaneFactor{}, err
	}
	law, declared := findLaneSemanticLaw(runtime.semanticLaws, laneSemanticEffectFactor)
	if !declared || !law.participates || law.effectKinds&request.kind == 0 {
		return LaneFactor{}, fmt.Errorf("%w: lane %q does not participate in effect factor", ErrInvalidLaneFactor, runtime.lane.id)
	}
	next, changed, valid := law.applyFactor(current.payload, request)
	if !valid {
		return LaneFactor{}, fmt.Errorf("state: lane %q rejected effect factor", runtime.lane.id)
	}
	if !changed {
		return current, nil
	}
	return LaneFactor{lane: runtime.lane, payload: next}, nil
}

func (d ProductDomain) ApplyDynamicIndexMembershipFactor(plan DynamicIndexMembershipFactorPlan, current LaneFactor) (LaneFactor, error) {
	if !d.ownsDynamicIndexMembershipFactor(plan) {
		return LaneFactor{}, fmt.Errorf("state: foreign dynamic-index membership factor")
	}
	return d.applyEffectFactor(effectFactorRequest{kind: effectFactorDynamicIndexMembership, dynamic: plan}, current)
}

func (d ProductDomain) ApplyEffectDeltaFactor(plan EffectDeltaFactorPlan, current LaneFactor) (LaneFactor, error) {
	if !d.ownsEffectDeltaFactor(plan) {
		return LaneFactor{}, fmt.Errorf("state: foreign effect-delta factor")
	}
	return d.applyEffectFactor(effectFactorRequest{kind: effectFactorDelta, delta: plan}, current)
}

func (d ProductDomain) ApplyDynamicIndexMembership(plan DynamicIndexMembershipFactorPlan, input State) (State, error) {
	if !d.ownsDynamicIndexMembershipFactor(plan) {
		return State{}, fmt.Errorf("state: foreign dynamic-index membership factor")
	}
	lanes := d.DynamicIndexMembershipFactorLanes()
	factors, err := d.DecomposeLanes(input, lanes)
	if err != nil {
		return State{}, err
	}
	for index := range factors {
		factors[index], err = d.ApplyDynamicIndexMembershipFactor(plan, factors[index])
		if err != nil {
			return State{}, err
		}
	}
	return d.PatchLaneFactors(input, factors)
}

func (d ProductDomain) ApplyEffectDelta(plan EffectDeltaFactorPlan, input State) (State, error) {
	if !d.ownsEffectDeltaFactor(plan) {
		return State{}, fmt.Errorf("state: foreign effect-delta factor")
	}
	lanes := d.EffectDeltaFactorLanes()
	factors, err := d.DecomposeLanes(input, lanes)
	if err != nil {
		return State{}, err
	}
	for index := range factors {
		factors[index], err = d.ApplyEffectDeltaFactor(plan, factors[index])
		if err != nil {
			return State{}, err
		}
	}
	return d.PatchLaneFactors(input, factors)
}

func applyDynamicIndexEffectFactor(lane dynamicIndexLane, request effectFactorRequest) (dynamicIndexLane, bool, bool) {
	plan := request.dynamic
	if request.kind != effectFactorDynamicIndexMembership || plan.reg == nil || lane.top {
		return lane, false, false
	}
	config := plan.config
	domain := dynamicindex.Domain(plan.reg)
	if domain.Equal(config.Fact, domain.Bottom()) {
		next, changed := lane.without(config.Key)
		return next, changed, true
	}
	if domain.Equal(lane.read(plan.reg, config.Key), config.Fact) {
		return lane, false, true
	}
	return lane.with(config.Key, config.Fact), true, true
}

func effectStateKeyIn(keys []pathaddr.StateKey, want pathaddr.StateKey) bool {
	for _, key := range keys {
		if key == want {
			return true
		}
	}
	return false
}

func applyKeyMembershipEffectFactor(lane keyMembershipLane, request effectFactorRequest) (keyMembershipLane, bool, bool) {
	plan := request.dynamic
	if request.kind != effectFactorDynamicIndexMembership || plan.keys == nil {
		return lane, false, false
	}
	config := plan.config
	changed := false
	var step bool
	lane, step = lane.clearMatching(func(m KeyMembership) bool {
		return (m.Kind == KeyMembershipDynamicIndexValue || m.Kind == KeyMembershipDynamicIndexAllValues) && m.Container == config.Key.Table
	})
	changed = changed || step
	if config.MayBeAbsent {
		for _, tableKey := range config.TableStateKeys {
			lane, step = lane.clearMatching(func(m KeyMembership) bool { return m.Key == tableKey || m.Table == tableKey })
			changed = changed || step
		}
		if config.TableSymbol != 0 {
			lane, step = lane.clearMatching(func(m KeyMembership) bool {
				if m.Table == "" {
					return false
				}
				key, ok := plan.keys.FromStateKey(m.Table.PathKey())
				return ok && key.Sym == config.TableSymbol
			})
			changed = changed || step
		}
	}
	for _, restore := range config.PendingRestores {
		if _, exists := lane.pendingRestores[restore]; exists {
			continue
		}
		lane = lane.reachable()
		lane.pendingRestores = mapedit.Clone(lane.pendingRestores)
		if lane.pendingRestores == nil {
			lane.pendingRestores = make(map[PendingDynamicAllValueRestore]struct{}, 1)
		}
		lane.pendingRestores[restore] = struct{}{}
		changed = true
	}
	if config.DefinitelyPresent && config.HasKeyStateKey {
		lane, step = lane.add(PathKeyMembership(config.KeyStateKey, config.MembershipTable))
		changed = changed || step
	}
	if config.DefinitelyPresent {
		for _, table := range config.SourceMemberships {
			lane, step = lane.add(DynamicIndexValueKeyMembership(config.Key.Table, config.Key.Site, table))
			changed = changed || step
		}
	}
	for _, table := range config.AllValueTables {
		if config.DefinitelyAbsent || effectStateKeyIn(config.SourceMemberships, table) {
			lane, step = lane.add(DynamicIndexAllValuesKeyMembership(config.Key.Table, table))
			changed = changed || step
		}
	}
	if config.DefinitelyAbsent && config.HasKeyStateKey {
		for _, key := range config.RestoreKeys {
			var restores []PendingDynamicAllValueRestore
			for restore := range lane.pendingRestores {
				if restore.Container == config.Key.Table && restore.Key == key {
					restores = append(restores, restore)
				}
			}
			for _, restore := range restores {
				lane, step = lane.add(DynamicIndexAllValuesKeyMembership(restore.Container, restore.Table))
				changed = changed || step
				lane.pendingRestores = mapedit.Clone(lane.pendingRestores)
				delete(lane.pendingRestores, restore)
				changed = true
			}
		}
	}
	return normalizeKeyMembershipLane(lane), changed, true
}

func applyEffectDeltaFactor(lane effectDeltaLane, request effectFactorRequest) (effectDeltaLane, bool, bool) {
	plan := request.delta
	if request.kind != effectFactorDelta || plan.key.Target.Kind == keyspace.KindInvalid || lane.top {
		return lane, false, false
	}
	if plan.delta.Change == effectdelta.ChangeBottom {
		next, changed := lane.without(plan.key)
		return next, changed, true
	}
	if lane.read(plan.key) == plan.delta {
		return lane, false, true
	}
	return lane.with(plan.key, plan.delta), true, true
}
