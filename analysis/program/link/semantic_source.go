package link

import "github.com/wippyai/go-lua/analysis/program/semanticsource"

// sourcePublications is the Link-child aggregation boundary. Each child has
// already snapshotted its typed Count/At identities at seal time; this method
// authenticates the exact owner and projects only those detached source rows.
// It never opens Project, Boundary, Module, Static, or Host internals.
func (l *Link) childSourcePublications(schema semanticsource.ProgramSchema) ([]semanticsource.Publication, bool) {
	if schema == nil || !l.sealedSemanticSource() || l.project == nil || l.boundary == nil || l.module == nil || l.static == nil || l.host == nil {
		return nil, false
	}
	projectCold := l.project.Cold()
	projectViews, ok := projectCold.SourceViews()
	if !ok || projectViews.OwnerID() != projectCold.ContentID() {
		return nil, false
	}
	boundaryViews, ok := l.boundary.SourceViews()
	if !ok || boundaryViews.OwnerID() != l.boundary.ContentID() {
		return nil, false
	}
	moduleCold := l.module.Cold()
	moduleViews, ok := moduleCold.SourceViews()
	if !ok || moduleViews.OwnerID() != moduleCold.ContentID() {
		return nil, false
	}
	staticCold := l.static.Cold()
	staticRows, ok := staticCold.Publications(schema)
	if !ok || len(staticRows) != 1 {
		return nil, false
	}
	hostCold := l.host.Cold()
	hostViews, ok := hostCold.SourceViews()
	if !ok || hostViews.OwnerID() != hostCold.ContentID() {
		return nil, false
	}
	projectRows, boundaryRows, moduleRows := projectViews.Publications(schema), boundaryViews.Publications(schema), moduleViews.Publications(schema)
	hostRows := hostViews.Publications(schema)
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
// source rows provide the semantic completeness proof at the child boundary.
func (l *Link) sealedSemanticSource() bool {
	return l != nil && l.id.Available() && l.boundary != nil && l.project != nil && l.module != nil && l.static != nil && l.host != nil
}

// SourcePublications returns the detached Link snapshot of all sealed source
// columns. Child views are assembled once at Link seal and never traversed here.
func (l *Link) SourcePublications() (semanticsource.Publications, error) {
	if l == nil || !l.id.Available() || !l.sourcePublications.SchemaDigest().Available() || l.sourcePublications.Count() == 0 {
		return semanticsource.Publications{}, errSemanticSourceAssemblyUnavailable
	}
	return l.sourcePublications.Clone(), nil
}
