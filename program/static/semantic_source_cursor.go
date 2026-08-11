package static

import (
	"github.com/wippyai/go-lua/program/internal/schema/relations"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/semanticsource"
)

const semanticSourceFragmentPublicationCount = 10

// SemanticSourceView is Static's detached, fixed ten-publication interval.
type SemanticSourceView struct {
	owner      keyspace.ContentID
	rangeValue semanticsource.PublicationRange
}

func (view SemanticSourceView) Valid() bool {
	return view.owner.Available() && view.rangeValue.Valid()
}

func (view SemanticSourceView) OwnerID() keyspace.ContentID { return view.owner }

func (view SemanticSourceView) Range() semanticsource.PublicationRange {
	if !view.Valid() {
		return semanticsource.PublicationRange{}
	}
	return view.rangeValue.Snapshot()
}

func (view SemanticSourceView) Count() int {
	if !view.Valid() {
		return 0
	}
	return view.rangeValue.Count()
}

func (view SemanticSourceView) At(index int) (semanticsource.Publication, bool) {
	if !view.Valid() {
		return semanticsource.Publication{}, false
	}
	return view.rangeValue.At(index)
}

func (view SemanticSourceView) Publications() []semanticsource.Publication {
	if !view.Valid() {
		return nil
	}
	return view.rangeValue.Publications()
}

// SemanticSourceCursor is the shared bounded cursor over Static's fixed
// publication interval.
type SemanticSourceCursor = semanticsource.PublicationCursor

func (view SemanticSourceView) Cursor() SemanticSourceCursor {
	if !view.Valid() {
		return semanticsource.PublicationRange{}.Cursor()
	}
	return view.rangeValue.Cursor()
}

// SealSemanticSourceFragment validates and detaches Static's exact fixed
// schema interval. It accepts zero-count rows but no missing, duplicate,
// foreign, or reordered token.
func SealSemanticSourceFragment(owner keyspace.ContentID, rows []semanticsource.Publication) (SemanticSourceView, error) {
	if !owner.Available() {
		return SemanticSourceView{}, errSemanticSourceUnavailable
	}
	definitions, err := staticSemanticSourceDefinitions()
	if err != nil {
		return SemanticSourceView{}, err
	}
	rangeValue, err := semanticsource.SealPublicationRange(definitions, rows)
	if err != nil {
		return SemanticSourceView{}, err
	}
	return SemanticSourceView{owner: owner, rangeValue: rangeValue}, nil
}

// SemanticSourceFragmentView returns Static's already-sealed owner-local
// interval without rebuilding or copying its publication claims.
func SemanticSourceFragmentView(view View) (SemanticSourceView, error) {
	if view.ContentID() == (keyspace.ContentID{}) {
		return SemanticSourceView{}, errSemanticSourceUnavailable
	}
	component := view.componentOf()
	if component == nil || !component.semantic.Valid() || contentID(component) != component.contentID {
		return SemanticSourceView{}, errSemanticSourceIncomplete
	}
	counts, err := staticSemanticSourceCounts(view)
	if err != nil {
		return SemanticSourceView{}, err
	}
	definitions, err := staticSemanticSourceDefinitions()
	if err != nil {
		return SemanticSourceView{}, err
	}
	digest, ok := semanticsource.DigestPublicationCounts(definitions, counts[:])
	sealedDigest, sealed := component.semantic.Digest()
	if !ok || !sealed || digest != sealedDigest {
		return SemanticSourceView{}, errSemanticSourceIncomplete
	}
	return SemanticSourceView{owner: component.contentID, rangeValue: component.semantic}, nil
}

func SemanticSourceFragment(view View) ([]semanticsource.Publication, error) {
	fragment, err := SemanticSourceFragmentView(view)
	if err != nil {
		return nil, err
	}
	return fragment.Publications(), nil
}

func sealSemanticSourceFragment(view View) (semanticsource.PublicationRange, error) {
	rows, err := buildSemanticSourceFragment(view)
	if err != nil {
		return semanticsource.PublicationRange{}, err
	}
	fragment, err := SealSemanticSourceFragment(view.ContentID(), rows)
	if err != nil {
		return semanticsource.PublicationRange{}, err
	}
	return fragment.Range(), nil
}

func staticSemanticSourceDefinitions() ([]semanticsource.RelationDef, error) {
	schema, err := relations.CanonicalSchema()
	if err != nil || schema == nil {
		return nil, errSemanticSourceIncomplete
	}
	definitions := make([]semanticsource.RelationDef, 0, semanticSourceFragmentPublicationCount)
	for _, row := range schema.Rows() {
		if row.Owner == relations.OwnerProgramStatic {
			definitions = append(definitions, row.Definition)
		}
	}
	if len(definitions) != semanticSourceFragmentPublicationCount {
		return nil, errSemanticSourceIncomplete
	}
	return definitions, nil
}
