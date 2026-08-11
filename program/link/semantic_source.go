package link

import "github.com/wippyai/go-lua/program/semanticsource"

// sourcePublications is the Link-child aggregation boundary. Each child has
// already snapshotted its typed Count/At identities at seal time; this method
// authenticates the exact owner and projects only those detached receipts.
// It never opens Project, Boundary, Module, Static, or Host internals.
func (l *Link) sourcePublications() ([]semanticsource.Publication, bool) {
	if !l.sealedSemanticSource() || l.project == nil || l.boundary == nil || l.module == nil || l.static == nil || l.host == nil {
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
	staticReceipt, ok := staticCold.SemanticSourceReceipt()
	if !ok || staticReceipt.OwnerID() != staticCold.ContentID() {
		return nil, false
	}
	hostCold := l.host.Cold()
	hostReceipt, ok := hostCold.SemanticSourceReceipt()
	if !ok || hostReceipt.OwnerID() != hostCold.ContentID() {
		return nil, false
	}
	projectRows, boundaryRows, moduleRows := projectReceipt.Publications(), boundaryReceipt.Publications(), moduleReceipt.Publications()
	staticRows, hostRows := staticReceipt.Publications(), hostReceipt.Publications()
	if len(projectRows) != 2 || len(boundaryRows) != 1 || len(moduleRows) != 8 || len(staticRows) != 5 || len(hostRows) != 5 {
		return nil, false
	}
	rows := make([]semanticsource.Publication, 0, 21)
	rows = append(rows, projectRows...)
	rows = append(rows, boundaryRows...)
	rows = append(rows, moduleRows...)
	rows = append(rows, staticRows...)
	rows = append(rows, hostRows...)
	return rows, len(rows) == 21
}

// sealedSemanticSource is a narrow lifecycle/owner proof. The individual
// receipts provide the semantic completeness proof at the child boundary.
func (l *Link) sealedSemanticSource() bool {
	return l != nil && l.id.Available() && l.boundary != nil && l.project != nil && l.module != nil && l.static != nil && l.host != nil
}
