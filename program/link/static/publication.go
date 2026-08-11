package static

import "github.com/wippyai/go-lua/program/semanticsource"

// Publications publishes the already sealed identity/schema/publication
// snapshot. The hot Component is intentionally not reachable from Cold;
// Build performs the structural validation before taking this snapshot.
func (v Cold) Publications() ([]semanticsource.Publication, bool) {
	views, viewsOK := v.SemanticSourceViews()
	if !viewsOK {
		return nil, false
	}
	schema := semanticsource.CatalogSchema()
	result := make([]semanticsource.Publication, 0, schema.Count())
	expected := 0
	for index := 0; index < schema.Count(); index++ {
		definition, ok := schema.DefinitionAt(index)
		if !ok || definition.Token().Origin() != semanticsource.OriginLinkStatic {
			continue
		}
		expected++
		view, viewOK := views.viewFor(definition.Token())
		if !viewOK || !view.valid() {
			return nil, false
		}
		publication, err := semanticsource.SealPublication(definition, view.Count())
		if err != nil {
			return nil, false
		}
		result = append(result, publication)
	}
	return result, len(result) == expected
}

const maxPublishedCount = int(^uint(0) >> 1)
