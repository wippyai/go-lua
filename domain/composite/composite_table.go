package composite

import "github.com/wippyai/go-lua/analysis/schema/composite"

// compositeSpecs is the authored analyzer composite inventory. It is empty,
// and says so rather than inventing a row: no relation in the analyzer is
// declared as a composite yet, because the half a composite is declared
// against - the typed Frame and its admitted write - lands with the store cut.
//
// Registering the surface with no rows is what makes the omission visible. The
// declaration root's coverage law requires every declared surface to be wired,
// so a composite surface that is absent would be an incomplete table, while one
// that is present and empty is the honest statement that the analyzer has a
// composite surface and has declared nothing on it.
func compositeSpecs() []composite.Spec { return nil }

// compositeEntries admits the authored inventory. A rejected row leaves the
// table unavailable rather than half declared.
func compositeEntries() ([]*composite.Entry, bool) {
	specs := compositeSpecs()
	entries := make([]*composite.Entry, 0, len(specs))
	for _, spec := range specs {
		entry, ok := composite.New(spec)
		if !ok {
			return nil, false
		}
		entries = append(entries, entry)
	}
	return entries, true
}
