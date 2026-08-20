package imports

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// View returns the immutable direct Module surface. During construction it
// exposes authored Term/Call/Alias/Request; derived Key and Entry become
// visible only after the owner Finalizer commits.
func (c *Component) View() View {
	if c == nil {
		return View{}
	}
	return View{component: c}
}

// ContentID returns the immutable authored Module identity directly from its
// canonical owner.
func (c *Component) ContentID() identity.ContentID {
	if c == nil {
		return identity.ContentID{}
	}
	return c.content
}

// ContentID returns the authored Module identity through the direct view.
func (v View) ContentID() identity.ContentID {
	if component, ok := v.componentForRead(); ok {
		return component.content
	}
	if authored, ok := v.authoredForRead(); ok {
		return authored.content
	}
	return identity.ContentID{}
}

// Count reports the dense authored Import cardinality.
func (v View) Count() int {
	if component, ok := v.componentForRead(); ok {
		return len(component.imports)
	}
	if authored, ok := v.authoredForRead(); ok {
		return len(authored.imports)
	}
	return 0
}

// ImportAt returns one dense Import row in canonical source order. On an
// active Finalizer its derived Key is zero by construction.
func (v View) ImportAt(index int) (Import, bool) {
	imports, ok := v.importsForRead()
	if !ok || index < 0 || index >= len(imports) {
		return Import{}, false
	}
	return imports[index], true
}

// Import returns one row by its canonical dense Import identity.
func (v View) Import(term keyspace.Term) (Import, bool) {
	imports, ok := v.importsForRead()
	if !ok {
		return Import{}, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if keyspace.TermFamily(term) != keyspace.FamilyImport || ordinal == 0 || uint64(ordinal) > uint64(len(imports)) {
		return Import{}, false
	}
	row := imports[ordinal-1]
	if row.Term != term {
		return Import{}, false
	}
	return row, true
}

// Entry returns the committed derived chunk-entry projection. An active
// authored Finalizer has no Entry yet and returns an empty view.
func (v View) Entry() EntryView {
	component, ok := v.componentForRead()
	if !ok {
		return EntryView{}
	}
	return EntryView{data: &component.entry}
}

func (v View) importsForRead() ([]Import, bool) {
	if component, ok := v.componentForRead(); ok {
		return component.imports, true
	}
	if authored, ok := v.authoredForRead(); ok {
		return authored.imports, true
	}
	return nil, false
}

func (v View) componentForRead() (*Component, bool) {
	return v.component, v.component != nil
}

func (v View) authoredForRead() (*authored, bool) {
	if v.state == nil {
		return nil, false
	}
	state := v.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.terminal || !state.claimed || state.authored == nil {
		return nil, false
	}
	return state.authored, true
}
