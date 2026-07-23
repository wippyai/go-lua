package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// KeyMembershipKind classifies how a key-membership proof is carried.
type KeyMembershipKind uint8

const (
	KeyMembershipPath KeyMembershipKind = iota + 1
	KeyMembershipDynamicIndexValue
	KeyMembershipDynamicIndexAllValues
)

// KeyMembership records must evidence that a value is a key of Table.
// Path facts attach evidence to a concrete key path. Dynamic-index-value facts
// attach evidence to every value written at one container/site pair, letting
// iteration over that container recover the original table-key proof.
// Dynamic-index-all-values facts attach an inductive invariant to the whole
// container: every present value currently reachable through that dynamic
// container is a key of Table.
type KeyMembership struct {
	Kind      KeyMembershipKind
	Key       pathaddr.StateKey
	Container keyspace.Key
	Site      dynamicindex.Site
	Table     pathaddr.StateKey
}

// DynamicIndexReadOrigin records that Value was read from Container[Key] on all
// paths reaching this state. It lets later writes reason about paired map
// updates without guessing from names.
type DynamicIndexReadOrigin struct {
	Value     pathaddr.StateKey
	Container keyspace.Key
	Key       pathaddr.StateKey
}

func (o DynamicIndexReadOrigin) valid() bool {
	return o.Value != "" && o.Container.Kind != keyspace.KindInvalid && o.Key != ""
}

// DynamicIndexValueOrigin records that Value was assigned from one dynamic
// value source of Container. It is paired with DynamicIndexValueKeyMembership at
// query time so loop variables can recover key evidence after the mutation
// facts that justify that evidence arrive through a later fixpoint iteration.
type DynamicIndexValueOrigin struct {
	Value     pathaddr.StateKey
	Container keyspace.Key
	Site      dynamicindex.Site
}

// PendingDynamicAllValueRestore records a closed all-values invariant that was
// suspended by deleting a primary table key. Deleting the exact reverse-map key
// that produced that value restores the invariant.
type PendingDynamicAllValueRestore struct {
	Container keyspace.Key
	Table     pathaddr.StateKey
	Key       pathaddr.StateKey
}

func PathKeyMembership(key, table pathaddr.StateKey) KeyMembership {
	return KeyMembership{Kind: KeyMembershipPath, Key: key, Table: table}
}

func DynamicIndexValueKeyMembership(container keyspace.Key, site dynamicindex.Site, table pathaddr.StateKey) KeyMembership {
	return KeyMembership{Kind: KeyMembershipDynamicIndexValue, Container: container, Site: site, Table: table}
}

func DynamicIndexAllValuesKeyMembership(container keyspace.Key, table pathaddr.StateKey) KeyMembership {
	return KeyMembership{Kind: KeyMembershipDynamicIndexAllValues, Container: container, Table: table}
}

func (m KeyMembership) valid() bool {
	if m.Table == "" {
		return false
	}
	switch m.Kind {
	case KeyMembershipPath:
		return m.Key != ""
	case KeyMembershipDynamicIndexValue:
		return m.Container.Kind != keyspace.KindInvalid && m.Site != ""
	case KeyMembershipDynamicIndexAllValues:
		return m.Container.Kind != keyspace.KindInvalid
	default:
		return false
	}
}

type keyMembershipLane struct {
	bottom          bool
	path            map[KeyMembership]struct{}
	dynamic         map[KeyMembership]struct{}
	dynamicAll      map[KeyMembership]struct{}
	valueOrigins    map[DynamicIndexValueOrigin]struct{}
	readOrigins     map[DynamicIndexReadOrigin]struct{}
	pendingRestores map[PendingDynamicAllValueRestore]struct{}
	dynamicTop      bool
}

func keyMembershipDomain() lattice.Lattice[keyMembershipLane] {
	return lattice.Lattice[keyMembershipLane]{
		Bottom: func() keyMembershipLane { return keyMembershipLane{bottom: true} },
		Top:    func() keyMembershipLane { return keyMembershipLane{dynamicTop: true} },
		Equal:  keyMembershipLaneEqual,
		LessOrEq: func(a, b keyMembershipLane) bool {
			if a.bottom {
				return true
			}
			if b.bottom {
				return false
			}
			return keyMembershipPathMustLessOrEq(a.path, b.path) &&
				keyMembershipPathMustLessOrEq(a.dynamicAll, b.dynamicAll) &&
				(b.dynamicTop || dynamicIndexValueOriginMayLessOrEq(a.valueOrigins, b.valueOrigins)) &&
				dynamicIndexReadOriginMustLessOrEq(a.readOrigins, b.readOrigins) &&
				pendingDynamicAllRestoreMustLessOrEq(a.pendingRestores, b.pendingRestores) &&
				keyMembershipDynamicMayLessOrEq(a, b)
		},
		Join: func(a, b keyMembershipLane) keyMembershipLane {
			if a.bottom {
				return b.clone()
			}
			if b.bottom {
				return a.clone()
			}
			// dynamicTop is the lane-wide unknown element. In particular,
			// valueOrigins is a may set, so retaining its finite entries beside
			// dynamicTop would make Top fail to absorb a dynamic membership state.
			// Return the canonical top spelling rather than an observationally
			// equivalent-but-order-inconsistent mixed representation.
			if a.dynamicTop || b.dynamicTop {
				return keyMembershipLane{dynamicTop: true}
			}
			return keyMembershipLane{
				path:            keyMembershipSetIntersection(a.path, b.path),
				dynamic:         keyMembershipSetUnion(a.dynamic, b.dynamic),
				dynamicAll:      keyMembershipSetIntersection(a.dynamicAll, b.dynamicAll),
				valueOrigins:    dynamicIndexValueOriginSetUnion(a.valueOrigins, b.valueOrigins),
				readOrigins:     dynamicIndexReadOriginSetIntersection(a.readOrigins, b.readOrigins),
				pendingRestores: pendingDynamicAllRestoreSetIntersection(a.pendingRestores, b.pendingRestores),
			}
		},
		Meet: keyMembershipLaneMeet,
		Widen: func(prev, next keyMembershipLane) keyMembershipLane {
			return keyMembershipDomain().Join(prev, next)
		},
	}
}

func keyMembershipLaneMeet(a, b keyMembershipLane) keyMembershipLane {
	if a.bottom || b.bottom {
		return keyMembershipLane{bottom: true}
	}
	out := keyMembershipLane{
		path:            keyMembershipSetUnion(a.path, b.path),
		dynamicAll:      keyMembershipSetUnion(a.dynamicAll, b.dynamicAll),
		readOrigins:     dynamicIndexReadOriginSetUnion(a.readOrigins, b.readOrigins),
		pendingRestores: pendingDynamicAllRestoreSetUnion(a.pendingRestores, b.pendingRestores),
	}
	switch {
	case a.dynamicTop && b.dynamicTop:
		out.dynamicTop = true
	case a.dynamicTop:
		out.dynamic = mapedit.Clone(b.dynamic)
		out.valueOrigins = mapedit.Clone(b.valueOrigins)
	case b.dynamicTop:
		out.dynamic = mapedit.Clone(a.dynamic)
		out.valueOrigins = mapedit.Clone(a.valueOrigins)
	default:
		out.dynamic = keyMembershipSetIntersection(a.dynamic, b.dynamic)
		out.valueOrigins = dynamicIndexValueOriginSetIntersection(a.valueOrigins, b.valueOrigins)
	}
	return normalizeKeyMembershipLane(out)
}

// normalizeKeyMembershipLane keeps the coupled may coordinates in their one
// canonical spelling. dynamicTop is the shared Top marker for both dynamic
// memberships and their value origins, so finite entries beside it are
// semantically redundant. Must coordinates remain valid refinements below
// lane Top and are deliberately retained.
func normalizeKeyMembershipLane(lane keyMembershipLane) keyMembershipLane {
	if lane.bottom {
		return keyMembershipLane{bottom: true}
	}
	if lane.dynamicTop {
		lane.dynamic = nil
		lane.valueOrigins = nil
	}
	// Empty finite sets have one physical spelling. Boundary and image laws may
	// clone a non-nil empty map; retaining it would make representation identity
	// depend on execution history and grow terminal hash-conses on every no-op.
	if len(lane.path) == 0 {
		lane.path = nil
	}
	if len(lane.dynamic) == 0 {
		lane.dynamic = nil
	}
	if len(lane.dynamicAll) == 0 {
		lane.dynamicAll = nil
	}
	if len(lane.valueOrigins) == 0 {
		lane.valueOrigins = nil
	}
	if len(lane.readOrigins) == 0 {
		lane.readOrigins = nil
	}
	if len(lane.pendingRestores) == 0 {
		lane.pendingRestores = nil
	}
	return lane
}

func (l keyMembershipLane) reachable() keyMembershipLane {
	l.bottom = false
	return l
}

func (l keyMembershipLane) has(m KeyMembership) bool {
	if !m.valid() || l.bottom {
		return false
	}
	switch m.Kind {
	case KeyMembershipPath:
		_, ok := l.path[m]
		return ok
	case KeyMembershipDynamicIndexValue:
		if l.dynamicTop {
			return false
		}
		_, ok := l.dynamic[m]
		return ok
	case KeyMembershipDynamicIndexAllValues:
		_, ok := l.dynamicAll[m]
		return ok
	default:
		return false
	}
}

// hasPathKeyMembership evaluates the lane's complete must theorem for one
// observed key/table pair. A direct path fact and a value-origin paired with
// its dynamic container fact are two representations of the same proposition;
// every demand projector must ask this theorem rather than inspecting only
// the direct-path storage arm.
func (l keyMembershipLane) hasPathKeyMembership(key, table pathaddr.StateKey) bool {
	if l.has(PathKeyMembership(key, table)) {
		return true
	}
	if key == "" || table == "" || l.bottom {
		return false
	}
	for origin := range l.valueOrigins {
		if origin.Value != key {
			continue
		}
		if l.has(DynamicIndexAllValuesKeyMembership(origin.Container, table)) ||
			l.has(DynamicIndexValueKeyMembership(origin.Container, origin.Site, table)) {
			return true
		}
	}
	return false
}

func (l keyMembershipLane) add(m KeyMembership) (keyMembershipLane, bool) {
	if !m.valid() {
		return l, false
	}
	l = l.reachable()
	switch m.Kind {
	case KeyMembershipPath:
		if _, ok := l.path[m]; ok {
			return l, false
		}
		l.path = mapedit.Clone(l.path)
		if l.path == nil {
			l.path = make(map[KeyMembership]struct{}, 1)
		}
		l.path[m] = struct{}{}
		return l, true
	case KeyMembershipDynamicIndexValue:
		if l.dynamicTop {
			return l, false
		}
		if _, ok := l.dynamic[m]; ok {
			return l, false
		}
		l.dynamic = mapedit.Clone(l.dynamic)
		if l.dynamic == nil {
			l.dynamic = make(map[KeyMembership]struct{}, 1)
		}
		l.dynamic[m] = struct{}{}
		return l, true
	case KeyMembershipDynamicIndexAllValues:
		if _, ok := l.dynamicAll[m]; ok {
			return l, false
		}
		l.dynamicAll = mapedit.Clone(l.dynamicAll)
		if l.dynamicAll == nil {
			l.dynamicAll = make(map[KeyMembership]struct{}, 1)
		}
		l.dynamicAll[m] = struct{}{}
		return l, true
	default:
		return l, false
	}
}

func (l keyMembershipLane) addReadOrigin(origin DynamicIndexReadOrigin) (keyMembershipLane, bool) {
	if !origin.valid() {
		return l, false
	}
	if _, ok := l.readOrigins[origin]; ok {
		return l, false
	}
	l = l.reachable()
	l.readOrigins = mapedit.Clone(l.readOrigins)
	if l.readOrigins == nil {
		l.readOrigins = make(map[DynamicIndexReadOrigin]struct{}, 1)
	}
	l.readOrigins[origin] = struct{}{}
	return l, true
}

func (l keyMembershipLane) clearMatching(match func(KeyMembership) bool) (keyMembershipLane, bool) {
	if l.bottom || (len(l.path) == 0 && len(l.dynamic) == 0 && len(l.dynamicAll) == 0 &&
		len(l.valueOrigins) == 0 && len(l.readOrigins) == 0 && len(l.pendingRestores) == 0) {
		return l, false
	}
	pathKept, pathChanged := mapedit.DeleteMatching(l.path, func(membership KeyMembership, _ struct{}) bool {
		return match(membership)
	})
	dynamicKept, dynamicChanged := mapedit.DeleteMatching(l.dynamic, func(membership KeyMembership, _ struct{}) bool {
		return match(membership)
	})
	dynamicAllKept, dynamicAllChanged := mapedit.DeleteMatching(l.dynamicAll, func(membership KeyMembership, _ struct{}) bool {
		return match(membership)
	})
	valueOriginKept, valueOriginChanged := mapedit.DeleteMatching(l.valueOrigins, func(origin DynamicIndexValueOrigin, _ struct{}) bool {
		return match(PathKeyMembership(origin.Value, "")) ||
			match(DynamicIndexValueKeyMembership(origin.Container, origin.Site, ""))
	})
	readOriginKept, readOriginChanged := mapedit.DeleteMatching(l.readOrigins, func(origin DynamicIndexReadOrigin, _ struct{}) bool {
		return match(PathKeyMembership(origin.Value, origin.Key))
	})
	pendingRestoreKept, pendingRestoreChanged := mapedit.DeleteMatching(l.pendingRestores, func(restore PendingDynamicAllValueRestore, _ struct{}) bool {
		return match(PathKeyMembership(restore.Key, restore.Table))
	})
	if !pathChanged && !dynamicChanged && !dynamicAllChanged &&
		!valueOriginChanged && !readOriginChanged && !pendingRestoreChanged {
		return l, false
	}
	l.path = pathKept
	l.dynamic = dynamicKept
	l.dynamicAll = dynamicAllKept
	l.valueOrigins = valueOriginKept
	l.readOrigins = readOriginKept
	l.pendingRestores = pendingRestoreKept
	return l, true
}

func (l keyMembershipLane) clone() keyMembershipLane {
	return keyMembershipLane{
		bottom:          l.bottom,
		path:            mapedit.Clone(l.path),
		dynamic:         mapedit.Clone(l.dynamic),
		dynamicAll:      mapedit.Clone(l.dynamicAll),
		valueOrigins:    mapedit.Clone(l.valueOrigins),
		readOrigins:     mapedit.Clone(l.readOrigins),
		pendingRestores: mapedit.Clone(l.pendingRestores),
		dynamicTop:      l.dynamicTop,
	}
}

func (l keyMembershipLane) rekey(from, to *keyspace.KeySpace) (keyMembershipLane, bool) {
	if from != nil && !from.Valid() || to != nil && !to.Valid() {
		return l, false
	}
	if l.bottom || len(l.path)+len(l.dynamic)+len(l.dynamicAll)+len(l.valueOrigins)+len(l.readOrigins)+len(l.pendingRestores) == 0 {
		return l, true
	}
	if from == nil || to == nil {
		return l, false
	}
	out := l
	var ok bool
	if out.path, ok = rekeySet(l.path, func(value KeyMembership) (KeyMembership, bool) {
		return rekeyMembership(from, to, value)
	}); !ok {
		return l, false
	}
	if out.dynamic, ok = rekeySet(l.dynamic, func(value KeyMembership) (KeyMembership, bool) {
		return rekeyMembership(from, to, value)
	}); !ok {
		return l, false
	}
	if out.dynamicAll, ok = rekeySet(l.dynamicAll, func(value KeyMembership) (KeyMembership, bool) {
		return rekeyMembership(from, to, value)
	}); !ok {
		return l, false
	}
	if out.valueOrigins, ok = rekeySet(l.valueOrigins, func(value DynamicIndexValueOrigin) (DynamicIndexValueOrigin, bool) {
		container, ok := to.ImportKey(from, value.Container)
		value.Container = container
		return value, ok
	}); !ok {
		return l, false
	}
	if out.readOrigins, ok = rekeySet(l.readOrigins, func(value DynamicIndexReadOrigin) (DynamicIndexReadOrigin, bool) {
		container, ok := to.ImportKey(from, value.Container)
		value.Container = container
		return value, ok
	}); !ok {
		return l, false
	}
	if out.pendingRestores, ok = rekeySet(l.pendingRestores, func(value PendingDynamicAllValueRestore) (PendingDynamicAllValueRestore, bool) {
		container, ok := to.ImportKey(from, value.Container)
		value.Container = container
		return value, ok
	}); !ok {
		return l, false
	}
	return out, true
}

func rekeyMembership(from, to *keyspace.KeySpace, value KeyMembership) (KeyMembership, bool) {
	if value.Container.Kind == keyspace.KindInvalid {
		return value, true
	}
	container, ok := to.ImportKey(from, value.Container)
	value.Container = container
	return value, ok
}

func rekeySet[T comparable](in map[T]struct{}, transform func(T) (T, bool)) (map[T]struct{}, bool) {
	if len(in) == 0 {
		return nil, true
	}
	out := make(map[T]struct{}, len(in))
	for value := range in {
		transformed, ok := transform(value)
		if !ok {
			return nil, false
		}
		out[transformed] = struct{}{}
	}
	return out, true
}

func (l keyMembershipLane) snapshot() KeyMembershipsSnapshot {
	if l.bottom {
		return KeyMembershipsSnapshot{Bottom: true}
	}
	values := make(map[KeyMembership]struct{}, len(l.path)+len(l.dynamic)+len(l.dynamicAll))
	for membership := range l.path {
		values[membership] = struct{}{}
	}
	for membership := range l.dynamicAll {
		values[membership] = struct{}{}
	}
	if !l.dynamicTop {
		for membership := range l.dynamic {
			values[membership] = struct{}{}
		}
	}
	items := sortedSetValues(values, keyMembershipLess)
	return KeyMembershipsSnapshot{
		Top:         len(items) == 0,
		Memberships: items,
	}
}

func keyMembershipLaneEqual(a, b keyMembershipLane) bool {
	if a.bottom || b.bottom {
		return a.bottom && b.bottom
	}
	return a.dynamicTop == b.dynamicTop &&
		keyMembershipSetEqual(a.path, b.path) &&
		keyMembershipSetEqual(a.dynamic, b.dynamic) &&
		keyMembershipSetEqual(a.dynamicAll, b.dynamicAll) &&
		dynamicIndexValueOriginSetEqual(a.valueOrigins, b.valueOrigins) &&
		dynamicIndexReadOriginSetEqual(a.readOrigins, b.readOrigins) &&
		pendingDynamicAllRestoreSetEqual(a.pendingRestores, b.pendingRestores)
}

func keyMembershipPathMustLessOrEq(a, b map[KeyMembership]struct{}) bool {
	for membership := range b {
		if _, ok := a[membership]; !ok {
			return false
		}
	}
	return true
}

func dynamicIndexReadOriginMustLessOrEq(a, b map[DynamicIndexReadOrigin]struct{}) bool {
	for origin := range b {
		if _, ok := a[origin]; !ok {
			return false
		}
	}
	return true
}

func dynamicIndexValueOriginMayLessOrEq(a, b map[DynamicIndexValueOrigin]struct{}) bool {
	for origin := range a {
		if _, ok := b[origin]; !ok {
			return false
		}
	}
	return true
}

func pendingDynamicAllRestoreMustLessOrEq(a, b map[PendingDynamicAllValueRestore]struct{}) bool {
	for restore := range b {
		if _, ok := a[restore]; !ok {
			return false
		}
	}
	return true
}

func keyMembershipDynamicMayLessOrEq(a, b keyMembershipLane) bool {
	if b.dynamicTop {
		return true
	}
	if a.dynamicTop {
		return false
	}
	for membership := range a.dynamic {
		if _, ok := b.dynamic[membership]; !ok {
			return false
		}
	}
	return true
}

func keyMembershipSetEqual(a, b map[KeyMembership]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for membership := range a {
		if _, ok := b[membership]; !ok {
			return false
		}
	}
	return true
}

func dynamicIndexReadOriginSetEqual(a, b map[DynamicIndexReadOrigin]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for origin := range a {
		if _, ok := b[origin]; !ok {
			return false
		}
	}
	return true
}

func dynamicIndexValueOriginSetEqual(a, b map[DynamicIndexValueOrigin]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for origin := range a {
		if _, ok := b[origin]; !ok {
			return false
		}
	}
	return true
}

func pendingDynamicAllRestoreSetEqual(a, b map[PendingDynamicAllValueRestore]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for restore := range a {
		if _, ok := b[restore]; !ok {
			return false
		}
	}
	return true
}

func keyMembershipSetIntersection(a, b map[KeyMembership]struct{}) map[KeyMembership]struct{} {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	out := make(map[KeyMembership]struct{})
	for membership := range a {
		if _, ok := b[membership]; ok {
			out[membership] = struct{}{}
		}
	}
	return out
}

func keyMembershipSetUnion(a, b map[KeyMembership]struct{}) map[KeyMembership]struct{} {
	if len(a) == 0 {
		return mapedit.Clone(b)
	}
	if len(b) == 0 {
		return mapedit.Clone(a)
	}
	out := mapedit.Clone(a)
	for membership := range b {
		out[membership] = struct{}{}
	}
	return out
}

func dynamicIndexReadOriginSetIntersection(a, b map[DynamicIndexReadOrigin]struct{}) map[DynamicIndexReadOrigin]struct{} {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	out := make(map[DynamicIndexReadOrigin]struct{})
	for origin := range a {
		if _, ok := b[origin]; ok {
			out[origin] = struct{}{}
		}
	}
	return out
}

func dynamicIndexReadOriginSetUnion(a, b map[DynamicIndexReadOrigin]struct{}) map[DynamicIndexReadOrigin]struct{} {
	if len(a) == 0 {
		return mapedit.Clone(b)
	}
	if len(b) == 0 {
		return mapedit.Clone(a)
	}
	out := mapedit.Clone(a)
	for origin := range b {
		out[origin] = struct{}{}
	}
	return out
}

func dynamicIndexValueOriginSetIntersection(a, b map[DynamicIndexValueOrigin]struct{}) map[DynamicIndexValueOrigin]struct{} {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	out := make(map[DynamicIndexValueOrigin]struct{})
	for origin := range a {
		if _, ok := b[origin]; ok {
			out[origin] = struct{}{}
		}
	}
	return out
}

func dynamicIndexValueOriginSetUnion(a, b map[DynamicIndexValueOrigin]struct{}) map[DynamicIndexValueOrigin]struct{} {
	if len(a) == 0 {
		return mapedit.Clone(b)
	}
	if len(b) == 0 {
		return mapedit.Clone(a)
	}
	out := mapedit.Clone(a)
	for origin := range b {
		out[origin] = struct{}{}
	}
	return out
}

func pendingDynamicAllRestoreSetIntersection(a, b map[PendingDynamicAllValueRestore]struct{}) map[PendingDynamicAllValueRestore]struct{} {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	out := make(map[PendingDynamicAllValueRestore]struct{})
	for restore := range a {
		if _, ok := b[restore]; ok {
			out[restore] = struct{}{}
		}
	}
	return out
}

func pendingDynamicAllRestoreSetUnion(a, b map[PendingDynamicAllValueRestore]struct{}) map[PendingDynamicAllValueRestore]struct{} {
	if len(a) == 0 {
		return mapedit.Clone(b)
	}
	if len(b) == 0 {
		return mapedit.Clone(a)
	}
	out := mapedit.Clone(a)
	for restore := range b {
		out[restore] = struct{}{}
	}
	return out
}

// KeyMembershipsSnapshot is a stable snapshot of finite must key-membership
// facts. Bottom is explicit; Top means no guaranteed memberships.
type KeyMembershipsSnapshot struct {
	Bottom      bool
	Top         bool
	Memberships []KeyMembership
}

func (s State) KeyMembershipsSnapshot() KeyMembershipsSnapshot {
	if !s.laneEnabled(laneKeyMembershipBit) {
		return KeyMembershipsSnapshot{Bottom: true}
	}
	return s.keyMemberships.snapshot()
}

func (s State) HasPathKeyMembership(key, table pathaddr.StateKey) bool {
	if !s.laneEnabled(laneKeyMembershipBit) {
		return false
	}
	return s.keyMemberships.hasPathKeyMembership(key, table)
}

func (s State) AddPathKeyMembership(key, table pathaddr.StateKey) State {
	return s.addKeyMembership(PathKeyMembership(key, table))
}

func (s State) AddDynamicIndexValueKeyMembership(container keyspace.Key, site dynamicindex.Site, table pathaddr.StateKey) State {
	return s.addKeyMembership(DynamicIndexValueKeyMembership(container, site, table))
}

func (s State) AddDynamicIndexAllValuesKeyMembership(container keyspace.Key, table pathaddr.StateKey) State {
	return s.addKeyMembership(DynamicIndexAllValuesKeyMembership(container, table))
}

func (s State) AddDynamicIndexValueOrigin(value pathaddr.StateKey, container keyspace.Key, site dynamicindex.Site) State {
	if value == "" || container.Kind == keyspace.KindInvalid || site == "" || !s.laneEnabled(laneKeyMembershipBit) {
		return s
	}
	origin := DynamicIndexValueOrigin{Value: value, Container: container, Site: site}
	if _, ok := s.keyMemberships.valueOrigins[origin]; ok {
		return s
	}
	lane := s.keyMemberships.reachable()
	lane.valueOrigins = mapedit.Clone(lane.valueOrigins)
	if lane.valueOrigins == nil {
		lane.valueOrigins = make(map[DynamicIndexValueOrigin]struct{}, 1)
	}
	lane.valueOrigins[origin] = struct{}{}
	out := s.reachable()
	out.keyMemberships = lane
	return out
}

func (s State) addKeyMembership(m KeyMembership) State {
	if !s.laneEnabled(laneKeyMembershipBit) {
		return s
	}
	keyMemberships, changed := s.keyMemberships.add(m)
	if !changed {
		return s
	}
	out := s.reachable()
	out.keyMemberships = keyMemberships
	return out
}

func (s State) PathKeyMembershipTables(key pathaddr.StateKey) []pathaddr.StateKey {
	if key == "" || !s.laneEnabled(laneKeyMembershipBit) {
		return nil
	}
	lane := s.keyMemberships
	if lane.bottom {
		return nil
	}
	var out []pathaddr.StateKey
	seen := make(map[pathaddr.StateKey]struct{})
	for membership := range lane.path {
		if membership.Kind == KeyMembershipPath && membership.Key == key {
			if _, ok := seen[membership.Table]; ok {
				continue
			}
			seen[membership.Table] = struct{}{}
			out = append(out, membership.Table)
		}
	}
	for origin := range lane.valueOrigins {
		if origin.Value != key {
			continue
		}
		for membership := range lane.dynamicAll {
			if membership.Kind == KeyMembershipDynamicIndexAllValues && membership.Container == origin.Container {
				if _, ok := seen[membership.Table]; ok {
					continue
				}
				seen[membership.Table] = struct{}{}
				out = append(out, membership.Table)
			}
		}
		if lane.dynamicTop {
			continue
		}
		for membership := range lane.dynamic {
			if membership.Kind == KeyMembershipDynamicIndexValue &&
				membership.Container == origin.Container && membership.Site == origin.Site {
				if _, ok := seen[membership.Table]; ok {
					continue
				}
				seen[membership.Table] = struct{}{}
				out = append(out, membership.Table)
			}
		}
	}
	return out
}

func (s State) DynamicIndexValueKeyMembershipTables(container keyspace.Key, site dynamicindex.Site) []pathaddr.StateKey {
	if container.Kind == keyspace.KindInvalid || site == "" || !s.laneEnabled(laneKeyMembershipBit) {
		return nil
	}
	lane := s.keyMemberships
	if lane.bottom || lane.dynamicTop {
		return nil
	}
	var out []pathaddr.StateKey
	for membership := range lane.dynamic {
		if membership.Kind == KeyMembershipDynamicIndexValue && membership.Container == container && membership.Site == site {
			out = append(out, membership.Table)
		}
	}
	return out
}

func (s State) DynamicIndexAllValuesKeyMembershipTables(container keyspace.Key) []pathaddr.StateKey {
	if container.Kind == keyspace.KindInvalid || !s.laneEnabled(laneKeyMembershipBit) {
		return nil
	}
	lane := s.keyMemberships
	if lane.bottom {
		return nil
	}
	var out []pathaddr.StateKey
	for membership := range lane.dynamicAll {
		if membership.Kind == KeyMembershipDynamicIndexAllValues && membership.Container == container {
			out = append(out, membership.Table)
		}
	}
	return out
}

func (s State) DynamicIndexAllValuesKeyMembershipContainers(table pathaddr.StateKey) []keyspace.Key {
	if table == "" || !s.laneEnabled(laneKeyMembershipBit) {
		return nil
	}
	lane := s.keyMemberships
	if lane.bottom {
		return nil
	}
	var out []keyspace.Key
	for membership := range lane.dynamicAll {
		if membership.Kind == KeyMembershipDynamicIndexAllValues && membership.Table == table {
			out = append(out, membership.Container)
		}
	}
	return out
}

func (s State) AddDynamicIndexReadOrigin(value pathaddr.StateKey, container keyspace.Key, key pathaddr.StateKey) State {
	if value == "" || container.Kind == keyspace.KindInvalid || key == "" || !s.laneEnabled(laneKeyMembershipBit) {
		return s
	}
	origin := DynamicIndexReadOrigin{Value: value, Container: container, Key: key}
	lane, changed := s.keyMemberships.addReadOrigin(origin)
	if !changed {
		return s
	}
	out := s.reachable()
	out.keyMemberships = lane
	return out
}

func (s State) DynamicIndexReadOriginsForValue(value pathaddr.StateKey) []DynamicIndexReadOrigin {
	if value == "" || !s.laneEnabled(laneKeyMembershipBit) {
		return nil
	}
	lane := s.keyMemberships
	if lane.bottom {
		return nil
	}
	var out []DynamicIndexReadOrigin
	for origin := range lane.readOrigins {
		if origin.Value == value {
			out = append(out, origin)
		}
	}
	return out
}

func (s State) AddPendingDynamicAllValueRestore(container keyspace.Key, table, key pathaddr.StateKey) State {
	if container.Kind == keyspace.KindInvalid || table == "" || key == "" || !s.laneEnabled(laneKeyMembershipBit) {
		return s
	}
	restore := PendingDynamicAllValueRestore{Container: container, Table: table, Key: key}
	if _, ok := s.keyMemberships.pendingRestores[restore]; ok {
		return s
	}
	lane := s.keyMemberships.reachable()
	lane.pendingRestores = mapedit.Clone(lane.pendingRestores)
	if lane.pendingRestores == nil {
		lane.pendingRestores = make(map[PendingDynamicAllValueRestore]struct{}, 1)
	}
	lane.pendingRestores[restore] = struct{}{}
	out := s.reachable()
	out.keyMemberships = lane
	return out
}

func (s State) PendingDynamicAllValueRestores(container keyspace.Key, key pathaddr.StateKey) []PendingDynamicAllValueRestore {
	if container.Kind == keyspace.KindInvalid || key == "" || !s.laneEnabled(laneKeyMembershipBit) {
		return nil
	}
	lane := s.keyMemberships
	if lane.bottom {
		return nil
	}
	var out []PendingDynamicAllValueRestore
	for restore := range lane.pendingRestores {
		if restore.Container == container && restore.Key == key {
			out = append(out, restore)
		}
	}
	return out
}

func (s State) ClearPendingDynamicAllValueRestore(restore PendingDynamicAllValueRestore) State {
	if !s.laneEnabled(laneKeyMembershipBit) || s.keyMemberships.bottom {
		return s
	}
	if _, ok := s.keyMemberships.pendingRestores[restore]; !ok {
		return s
	}
	lane := s.keyMemberships
	lane.pendingRestores = mapedit.Clone(lane.pendingRestores)
	delete(lane.pendingRestores, restore)
	out := s.reachable()
	out.keyMemberships = lane
	return out
}

func (s State) ClearKeyMembershipsForPath(pathKey pathaddr.StateKey) State {
	if pathKey == "" || !s.laneEnabled(laneKeyMembershipBit) {
		return s
	}
	lane, changed := s.keyMemberships.clearMatching(func(m KeyMembership) bool {
		return m.Key == pathKey || m.Table == pathKey
	})
	if !changed {
		return s
	}
	out := s.reachable()
	out.keyMemberships = lane
	return out
}

func (s State) ClearKeyMembershipsForTableSymbol(ks *keyspace.KeySpace, table symbol.ID) State {
	if ks == nil || table == 0 || !s.laneEnabled(laneKeyMembershipBit) {
		return s
	}
	lane, changed := s.keyMemberships.clearMatching(func(m KeyMembership) bool {
		if m.Table == "" {
			return false
		}
		key, ok := ks.FromStateKey(m.Table.PathKey())
		return ok && key.Sym == table
	})
	if !changed {
		return s
	}
	out := s.reachable()
	out.keyMemberships = lane
	return out
}

func (s State) ClearDynamicIndexValueKeyMembershipsForContainer(container keyspace.Key) State {
	if container.Kind == keyspace.KindInvalid || !s.laneEnabled(laneKeyMembershipBit) {
		return s
	}
	lane, changed := s.keyMemberships.clearMatching(func(m KeyMembership) bool {
		return (m.Kind == KeyMembershipDynamicIndexValue || m.Kind == KeyMembershipDynamicIndexAllValues) &&
			m.Container == container
	})
	if !changed {
		return s
	}
	out := s.reachable()
	out.keyMemberships = lane
	return out
}

func (s State) ClearKeyMembershipsForPathKeySubtree(ks *keyspace.KeySpace, pathKey pathdom.PathKey) State {
	return s.clearKeyMembershipsForPathKeySubtree(ks, pathKey, true)
}

func (s State) ClearPathKeyMembershipsForPathKeySubtree(ks *keyspace.KeySpace, pathKey pathdom.PathKey) State {
	return s.clearKeyMembershipsForPathKeySubtree(ks, pathKey, false)
}

func (s State) clearKeyMembershipsForPathKeySubtree(ks *keyspace.KeySpace, pathKey pathdom.PathKey, clearDynamicValueMemberships bool) State {
	if ks == nil || pathKey == "" || !s.laneEnabled(laneKeyMembershipBit) {
		return s
	}
	prefix, ok := ks.FromStateKey(pathKey)
	if !ok {
		return s
	}
	match := func(candidate pathaddr.StateKey) bool {
		key, ok := ks.FromStateKey(candidate.PathKey())
		return ok && stateKeyHasPrefix(ks, key, prefix)
	}
	lane, changed := s.keyMemberships.clearMatching(func(m KeyMembership) bool {
		if match(m.Key) || match(m.Table) {
			return true
		}
		return clearDynamicValueMemberships &&
			(m.Kind == KeyMembershipDynamicIndexValue || m.Kind == KeyMembershipDynamicIndexAllValues) &&
			stateKeyHasPrefix(ks, m.Container, prefix)
	})
	if !changed {
		return s
	}
	out := s.reachable()
	out.keyMemberships = lane
	return out
}

func (l keyMembershipLane) clearPathKeySubtree(ks *keyspace.KeySpace, pathKey pathdom.PathKey) (keyMembershipLane, bool, bool) {
	if ks == nil || pathKey == "" {
		return l, false, false
	}
	prefix, ok := ks.FromStateKey(pathKey)
	if !ok {
		return l, false, false
	}
	match := func(candidate pathaddr.StateKey) bool {
		key, keyOK := ks.FromStateKey(candidate.PathKey())
		return keyOK && stateKeyHasPrefix(ks, key, prefix)
	}
	next, changed := l.clearMatching(func(m KeyMembership) bool {
		return match(m.Key) || match(m.Table) ||
			(m.Kind == KeyMembershipDynamicIndexValue || m.Kind == KeyMembershipDynamicIndexAllValues) &&
				stateKeyHasPrefix(ks, m.Container, prefix)
	})
	return next, changed, true
}

func keyMembershipLess(a, b KeyMembership) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Key != b.Key {
		return a.Key < b.Key
	}
	if a.Container != b.Container {
		return keyMembershipKeyLess(a.Container, b.Container)
	}
	if a.Site != b.Site {
		return a.Site < b.Site
	}
	return a.Table < b.Table
}

func keyMembershipKeyLess(a, b keyspace.Key) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Sym != b.Sym {
		return a.Sym < b.Sym
	}
	if a.Ver != b.Ver {
		return a.Ver < b.Ver
	}
	if a.Root != b.Root {
		return a.Root < b.Root
	}
	if a.Segs != b.Segs {
		return a.Segs < b.Segs
	}
	return !a.Canon && b.Canon
}
