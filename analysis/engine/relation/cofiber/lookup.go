package cofiber

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Lookup is the opaque owner-issued neutral-to-runtime decision capability.
// Its atom vector is aligned with an already prepared guard Manager; neither
// the dense guard spelling nor a semantic-to-physical map is exposed. The
// mounted witness and runtime fence are retained so a lookup cannot cross a
// sibling mount or generation.
type Lookup struct{ data *lookupData }

type lookupData struct {
	mounted witness.Mounted
	fence   binding.Fence
	manager *guard.Manager
	atoms   []region.Atom
	sealed  bool
}

// NewLookup seals an opaque lookup over the existing runtime guard manager.
// Atoms are copied in the manager's existing order and must be one-to-one;
// this constructor does not sort, hash, or derive an identity for any row.
func NewLookup(mounted witness.Mounted, manager *guard.Manager, atoms []region.Atom) (Lookup, bool) {
	if !mounted.Available() || manager == nil || !manager.Valid(manager.True()) {
		return Lookup{}, false
	}
	fence := mounted.RuntimeFence()
	if !fence.Available() {
		return Lookup{}, false
	}
	count := 0
	for {
		if _, ok := manager.AtomAt(uint64(count)); !ok {
			break
		}
		count++
	}
	if len(atoms) != count {
		return Lookup{}, false
	}
	copyAtoms := append([]region.Atom(nil), atoms...)
	for index, atom := range copyAtoms {
		if !atom.Available() {
			return Lookup{}, false
		}
		for prior := 0; prior < index; prior++ {
			if copyAtoms[prior].ID() == atom.ID() {
				return Lookup{}, false
			}
		}
	}
	data := &lookupData{mounted: mounted, fence: fence, manager: manager, atoms: copyAtoms, sealed: true}
	if !data.available() {
		return Lookup{}, false
	}
	return Lookup{data: data}, true
}

// Available reports whether this lookup retains a complete sealed manager and
// owner-issued atom vector.
func (lookup Lookup) Available() bool { return lookup.data != nil && lookup.data.available() }

// ValidFor reports whether the lookup is bound to the exact supplied mount.
func (lookup Lookup) ValidFor(mounted witness.Mounted) bool {
	if !lookup.Available() || !mounted.Available() || !lookup.data.mounted.Same(mounted) {
		return false
	}
	want, got := lookup.data.mounted.Arrangement(), mounted.Arrangement()
	return want.Available() && got.Available() && want.Digest() == got.Digest()
}

func (data *lookupData) available() bool {
	return data != nil && data.sealed && data.mounted.Available() && data.fence.Available() && data.mounted.RuntimeFence().Same(data.fence) && data.manager != nil && data.manager.Valid(data.manager.True())
}

func (lookup Lookup) manager() *guard.Manager {
	if !lookup.Available() {
		return nil
	}
	return lookup.data.manager
}

// physicalIndex seals a transient neutral-to-physical index for one cold
// bootstrap proof. The returned map is owned by the caller and must not be
// retained after that proof; Lookup itself retains only the owner-issued atom
// vector and manager.
func (lookup Lookup) physicalIndex() (map[identity.ContentID]guard.Atom, bool) {
	if !lookup.Available() {
		return nil, false
	}
	index := make(map[identity.ContentID]guard.Atom, len(lookup.data.atoms))
	for rank, neutral := range lookup.data.atoms {
		physical, physicalOK := lookup.data.manager.AtomAt(uint64(rank))
		if !neutral.Available() || !physicalOK {
			return nil, false
		}
		index[neutral.ID()] = physical
	}
	return index, true
}
