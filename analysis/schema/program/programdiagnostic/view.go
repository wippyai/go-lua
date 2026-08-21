package programdiagnostic

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programfamily "github.com/wippyai/go-lua/analysis/schema/program/family"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// View is the read capability for one authenticated cold Program publication.
// It carries only immutable state; diagnostic rows and child spans remain in
// the sealed family planes.
type View struct {
	state programstate.State
}

// NewView authenticates diagnostic readers over one sealed program state.
func NewView(state programstate.State) (View, bool) {
	if !state.Available() {
		return View{}, false
	}
	return View{state: state}, true
}

// Available reports whether this reader has an authenticated sealed state.
func (view View) Available() bool { return view.state.Available() }

// State exposes the immutable state capability used by this view.
func (view View) State() programstate.State { return view.state }

func (view View) catalog() (identity.ContentID, bool) {
	if !view.Available() {
		return identity.ContentID{}, false
	}
	return view.state.CatalogID(), true
}

func (view View) frozen() snapshot.Frozen { return view.state.Frozen() }

func diagnosticFamilyCount[V programfamily.Row](view View, family programfamily.Family[V]) (int, bool) {
	catalog, ok := view.catalog()
	if !ok {
		return 0, false
	}
	frozen := view.frozen()
	return family.Count(&frozen, catalog)
}

func diagnosticFamilyAt[V programfamily.Row](view View, family programfamily.Family[V], index int) (V, bool) {
	var absent V
	catalog, ok := view.catalog()
	if !ok {
		return absent, false
	}
	frozen := view.frozen()
	return family.At(&frozen, catalog, index)
}

func (view View) DiagnosticObservationForID(id identity.ContentID) (DiagnosticObservation, bool) {
	if !id.Available() {
		return DiagnosticObservation{}, false
	}
	count, published := view.DiagnosticObservationCount()
	if !published {
		return DiagnosticObservation{}, false
	}
	for index := 0; index < count; index++ {
		observation, held := view.DiagnosticObservationAt(index)
		if held && observation.ID() == id {
			return observation, true
		}
	}
	return DiagnosticObservation{}, false
}

func (view View) DiagnosticObservationOrdinalForID(id identity.ContentID) (int, bool) {
	if !id.Available() {
		return 0, false
	}
	count, published := view.DiagnosticObservationCount()
	if !published {
		return 0, false
	}
	for index := 0; index < count; index++ {
		observation, held := view.DiagnosticObservationAt(index)
		if held && observation.ID() == id {
			return index, true
		}
	}
	return 0, false
}

// DiagnosticObservationCount is the sealed width of the diagnostic parent
// family. Evidence and Path are separate dense child families so a caller
// never receives a copied tagged-union payload.
func (view View) DiagnosticObservationCount() (int, bool) {
	return diagnosticFamilyCount(view, DiagnosticObservationFamily())
}

func (view View) DiagnosticObservationAt(index int) (DiagnosticObservation, bool) {
	return diagnosticFamilyAt(view, DiagnosticObservationFamily(), index)
}

func (view View) DiagnosticEvidenceCount() (int, bool) {
	return diagnosticFamilyCount(view, DiagnosticEvidenceFamily())
}

func (view View) DiagnosticEvidenceAt(index int) (DiagnosticEvidence, bool) {
	return diagnosticFamilyAt(view, DiagnosticEvidenceFamily(), index)
}

func (view View) DiagnosticPathCount() (int, bool) {
	return diagnosticFamilyCount(view, DiagnosticPathFamily())
}

func (view View) DiagnosticPathAt(index int) (DiagnosticPath, bool) {
	return diagnosticFamilyAt(view, DiagnosticPathFamily(), index)
}

func (view View) DiagnosticEvidenceFor(observationIndex, childIndex int) (DiagnosticEvidence, bool) {
	observation, ok := view.DiagnosticObservationAt(observationIndex)
	if !ok || childIndex < 0 {
		return DiagnosticEvidence{}, false
	}
	offset, count, spanOK := observation.EvidenceSpan()
	if !spanOK || uint64(childIndex) >= uint64(count) {
		return DiagnosticEvidence{}, false
	}
	return view.DiagnosticEvidenceAt(int(offset) + childIndex)
}

func (view View) DiagnosticPathFor(observationIndex, childIndex int) (DiagnosticPath, bool) {
	observation, ok := view.DiagnosticObservationAt(observationIndex)
	if !ok || childIndex < 0 {
		return DiagnosticPath{}, false
	}
	offset, count, spanOK := observation.PathSpan()
	if !spanOK || uint64(childIndex) >= uint64(count) {
		return DiagnosticPath{}, false
	}
	return view.DiagnosticPathAt(int(offset) + childIndex)
}

func (view View) DiagnosticEvidencePointAt(observationIndex, childIndex int) (id identity.ContentID, ok bool) {
	evidence, held := view.DiagnosticEvidenceFor(observationIndex, childIndex)
	if !held {
		return identity.ContentID{}, false
	}
	return evidence.PointID(), true
}

func (view View) DiagnosticPathComponentAt(observationIndex, childIndex int) (string, bool) {
	path, held := view.DiagnosticPathFor(observationIndex, childIndex)
	if !held {
		return "", false
	}
	return path.Component(), true
}

func (view View) DiagnosticPathName(observationIndex int) (string, bool) {
	observation, held := view.DiagnosticObservationAt(observationIndex)
	if !held || observation.Kind() != structure.DiagnosticObservationTypeReferenceUnresolved {
		return "", false
	}
	_, count, spanOK := observation.PathSpan()
	if !spanOK || count == 0 {
		return "", false
	}
	path := make([]DiagnosticPath, count)
	for index := uint32(0); index < count; index++ {
		child, childOK := view.DiagnosticPathFor(observationIndex, int(index))
		if !childOK {
			return "", false
		}
		path[index] = child
	}
	return DiagnosticPathName(path)
}
