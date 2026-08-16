package snapshot

import "github.com/wippyai/go-lua/analysis/identity"

// Denominators is the snapshot's sealed denominator publication: which
// published denominator identity proves absence on which column. The
// membership sets themselves live in the columns they prove, so this value
// carries a directory and never a second copy of the key universe.
//
// Its contract is deliberately narrow for now. Later moves widen it as
// diagnostics resolve a declared denominatorID to the published denominator
// it reads against; nothing here may grow into a second storage location.
type Denominators struct {
	slots map[identity.ContentID]uint32
}

// Slot returns the column slot whose sealed denominator is id.
func (d Denominators) Slot(id identity.ContentID) (uint32, bool) {
	slot, published := d.slots[id]
	return slot, published
}

// Len reports how many denominators the snapshot publishes.
func (d Denominators) Len() int { return len(d.slots) }

// Mounts is the snapshot's sealed mount binding set: the mounts whose facts
// this snapshot's columns were written under. A Cross-Link key names these
// bindings, so a consumer can ask whether a binding participated without
// reaching into the engine.
//
// Its contract is deliberately narrow for now. Later moves add the ordered
// binding evidence a Cross-Link key carries and the structural verification
// that a digest collision demands.
type Mounts struct {
	bound map[identity.MountID]struct{}
}

// Bound reports whether mount participated in this snapshot.
func (m Mounts) Bound(mount identity.MountID) bool {
	_, participated := m.bound[mount]
	return participated
}

// Len reports how many mounts the snapshot binds.
func (m Mounts) Len() int { return len(m.bound) }

// Queries is the snapshot's sealed query publication: the registered query
// plans this snapshot answers. A plan that is not published here has no
// authority against this snapshot, which is what keeps a local combination
// from passing itself off as a registered answer.
//
// Its contract is deliberately narrow for now. Later moves attach the
// generated tracked query surfaces and their dependency tokens.
type Queries struct {
	plans map[identity.ContentID]struct{}
}

// Published reports whether plan is registered against this snapshot.
func (q Queries) Published(plan identity.ContentID) bool {
	_, registered := q.plans[plan]
	return registered
}

// Len reports how many query plans the snapshot publishes.
func (q Queries) Len() int { return len(q.plans) }
