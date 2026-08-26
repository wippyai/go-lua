package terminal

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
)

// Result is the immutable output of one serial relation solve. The database
// root is the only state result; counters and the typed application catalog
// are observations of that solve, not a second fact store.
type Result struct {
	root         database.Version
	evaluations  uint64
	publications uint64
	applications Catalog
	sealed       bool
}

// New seals one complete solve result.
func New(root database.Version, evaluations, publications uint64, applications Catalog) (Result, bool) {
	if !root.Available() || !applications.Available() {
		return Result{}, false
	}
	if !catalogValidForRoot(applications, root) {
		return Result{}, false
	}
	result := Result{root: root, evaluations: evaluations, publications: publications, applications: applications, sealed: true}
	return result, result.Available()
}

// Available reports whether the immutable final root and application catalog
// are sealed.
func (result Result) Available() bool {
	return result.sealed && result.root.Available() && result.applications.Available() && catalogValidForRoot(result.applications, result.root)
}

func catalogValidForRoot(catalog Catalog, root database.Version) bool {
	if !catalog.Available() || !root.Available() || !root.Fence().Available() {
		return false
	}
	for _, application := range catalog.Applications() {
		observed := application.Root()
		if !observed.Available() || !observed.Fence().Same(root.Fence()) || observed.Revision() > root.Revision() {
			return false
		}
	}
	return true
}

// Root returns the final immutable database root.
func (result Result) Root() database.Version {
	if !result.Available() {
		return database.Version{}
	}
	return result.root
}

// Evaluations reports the number of redeemed schedule entries.
func (result Result) Evaluations() uint64 {
	if !result.Available() {
		return 0
	}
	return result.evaluations
}

// Publications reports the number of committed publication deltas. A
// no-write settlement does not advance this count.
func (result Result) Publications() uint64 {
	if !result.Available() {
		return 0
	}
	return result.publications
}

// Applications returns the immutable application directory in sealed mounted
// observation order. Relation-only dependencies are absent by construction;
// callers must redeem an entry by its exact dependency and operation key.
func (result Result) Applications() Catalog {
	if !result.Available() {
		return Catalog{}
	}
	return result.applications
}
