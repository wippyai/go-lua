package link

import "github.com/wippyai/go-lua/analysis/program/semanticsource"

// sourcePublications is the Link-child aggregation boundary. Each child has
// already snapshotted its typed Count/At identities at seal time; this method
// authenticates the exact owner and projects only those detached receipts.
// It never opens Project, Boundary, Module, Static, or Host internals.
func (l *Link) sourcePublications(schema semanticsource.ProgramSchema) ([]semanticsource.Publication, bool) {
	if schema == nil || !l.sealedSemanticSource() || l.project == nil || l.boundary == nil || l.module == nil || l.static == nil || l.host == nil {
		return nil, false
	}
	projectCold := l.project.Cold()
	projectReceipt, ok := projectCold.SemanticSourceReceipt()
	if !ok || projectReceipt.OwnerID() != projectCold.ContentID() {
		return nil, false
	}
	boundaryReceipt, ok := l.boundary.SemanticSourceReceipt()
	if !ok || boundaryReceipt.OwnerID() != l.boundary.ContentID() {
		return nil, false
	}
	moduleCold := l.module.Cold()
	moduleReceipt, ok := moduleCold.SemanticSourceReceipt()
	if !ok || moduleReceipt.OwnerID() != moduleCold.ContentID() {
		return nil, false
	}
	staticCold := l.static.Cold()
	staticRows, ok := staticCold.Publications(schema)
	if !ok || len(staticRows) != 1 {
		return nil, false
	}
	hostCold := l.host.Cold()
	hostReceipt, ok := hostCold.SemanticSourceReceipt()
	if !ok || hostReceipt.OwnerID() != hostCold.ContentID() {
		return nil, false
	}
	projectRows, boundaryRows, moduleRows := projectReceipt.Publications(schema), boundaryReceipt.Publications(schema), moduleReceipt.Publications(schema)
	hostRows := hostReceipt.Publications(schema)
	if len(projectRows) != 2 || len(boundaryRows) != 1 || len(moduleRows) != 8 || len(staticRows) != 1 || len(hostRows) != 5 {
		return nil, false
	}
	rows := make([]semanticsource.Publication, 0, 17)
	rows = append(rows, projectRows...)
	rows = append(rows, boundaryRows...)
	rows = append(rows, moduleRows...)
	rows = append(rows, staticRows...)
	rows = append(rows, hostRows...)
	return rows, len(rows) == 17
}

// sealedSemanticSource is a narrow lifecycle/owner proof. The individual
// receipts provide the semantic completeness proof at the child boundary.
func (l *Link) sealedSemanticSource() bool {
	return l != nil && l.id.Available() && l.boundary != nil && l.project != nil && l.module != nil && l.static != nil && l.host != nil
}
