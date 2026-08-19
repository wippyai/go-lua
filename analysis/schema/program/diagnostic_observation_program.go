package programschema

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

func (row Program) DiagnosticObservationForID(id identity.ContentID) (DiagnosticObservation, bool) {
	if !id.Available() {
		return DiagnosticObservation{}, false
	}
	count, published := row.DiagnosticObservationCount()
	if !published {
		return DiagnosticObservation{}, false
	}
	for index := 0; index < count; index++ {
		observation, held := row.DiagnosticObservationAt(index)
		if held && observation.ID() == id {
			return observation, true
		}
	}
	return DiagnosticObservation{}, false
}

func (row Program) DiagnosticObservationOrdinalForID(id identity.ContentID) (int, bool) {
	if !id.Available() {
		return 0, false
	}
	count, published := row.DiagnosticObservationCount()
	if !published {
		return 0, false
	}
	for index := 0; index < count; index++ {
		observation, held := row.DiagnosticObservationAt(index)
		if held && observation.ID() == id {
			return index, true
		}
	}
	return 0, false
}

// DiagnosticObservationCount is the sealed width of the diagnostic parent
// family. Evidence and Path are separate dense child families so a caller
// never receives a copied tagged-union payload.
func (row Program) DiagnosticObservationCount() (int, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return 0, false
	}
	return DiagnosticObservationFamily().Count(&row.Frozen, catalog)
}

func (row Program) DiagnosticObservationAt(index int) (DiagnosticObservation, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return DiagnosticObservation{}, false
	}
	return DiagnosticObservationFamily().At(&row.Frozen, catalog, index)
}

func (row Program) DiagnosticEvidenceCount() (int, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return 0, false
	}
	return DiagnosticEvidenceFamily().Count(&row.Frozen, catalog)
}

func (row Program) DiagnosticEvidenceAt(index int) (DiagnosticEvidence, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return DiagnosticEvidence{}, false
	}
	return DiagnosticEvidenceFamily().At(&row.Frozen, catalog, index)
}

func (row Program) DiagnosticPathCount() (int, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return 0, false
	}
	return DiagnosticPathFamily().Count(&row.Frozen, catalog)
}

func (row Program) DiagnosticPathAt(index int) (DiagnosticPath, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return DiagnosticPath{}, false
	}
	return DiagnosticPathFamily().At(&row.Frozen, catalog, index)
}

func (row Program) DiagnosticEvidenceFor(observationIndex, childIndex int) (DiagnosticEvidence, bool) {
	observation, ok := row.DiagnosticObservationAt(observationIndex)
	if !ok || childIndex < 0 {
		return DiagnosticEvidence{}, false
	}
	offset, count, spanOK := observation.EvidenceSpan()
	if !spanOK || uint64(childIndex) >= uint64(count) {
		return DiagnosticEvidence{}, false
	}
	return row.DiagnosticEvidenceAt(int(offset) + childIndex)
}

func (row Program) DiagnosticPathFor(observationIndex, childIndex int) (DiagnosticPath, bool) {
	observation, ok := row.DiagnosticObservationAt(observationIndex)
	if !ok || childIndex < 0 {
		return DiagnosticPath{}, false
	}
	offset, count, spanOK := observation.PathSpan()
	if !spanOK || uint64(childIndex) >= uint64(count) {
		return DiagnosticPath{}, false
	}
	return row.DiagnosticPathAt(int(offset) + childIndex)
}

func (row Program) DiagnosticEvidencePointAt(observationIndex, childIndex int) (id identity.ContentID, ok bool) {
	evidence, held := row.DiagnosticEvidenceFor(observationIndex, childIndex)
	if !held {
		return identity.ContentID{}, false
	}
	return evidence.PointID(), true
}

func (row Program) DiagnosticPathComponentAt(observationIndex, childIndex int) (string, bool) {
	path, held := row.DiagnosticPathFor(observationIndex, childIndex)
	if !held {
		return "", false
	}
	return path.Component(), true
}

func (row Program) DiagnosticPathName(observationIndex int) (string, bool) {
	observation, held := row.DiagnosticObservationAt(observationIndex)
	if !held || observation.Kind() != structure.DiagnosticObservationTypeReferenceUnresolved {
		return "", false
	}
	_, count, spanOK := observation.PathSpan()
	if !spanOK || count == 0 {
		return "", false
	}
	path := make([]DiagnosticPath, count)
	for index := uint32(0); index < count; index++ {
		child, childOK := row.DiagnosticPathFor(observationIndex, int(index))
		if !childOK {
			return "", false
		}
		path[index] = child
	}
	return DiagnosticPathName(path)
}
