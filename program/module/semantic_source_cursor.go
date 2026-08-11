package module

import (
	"github.com/wippyai/go-lua/program/internal/schema/relations"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/semanticsource"
)

const semanticSourceFragmentPublicationCount = 6

// SemanticSourceView is Module's detached, fixed six-publication interval.
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

// SemanticSourceCursor is the shared bounded cursor over Module's fixed
// publication interval.
type SemanticSourceCursor = semanticsource.PublicationCursor

func (view SemanticSourceView) Cursor() SemanticSourceCursor {
	if !view.Valid() {
		return semanticsource.PublicationRange{}.Cursor()
	}
	return view.rangeValue.Cursor()
}

// SealSemanticSourceFragment validates and detaches Module's fixed range.
func SealSemanticSourceFragment(owner keyspace.ContentID, rows []semanticsource.Publication) (SemanticSourceView, error) {
	if !owner.Available() {
		return SemanticSourceView{}, errSemanticSourceUnavailable
	}
	definitions, err := moduleSemanticSourceDefinitions()
	if err != nil {
		return SemanticSourceView{}, err
	}
	rangeValue, err := semanticsource.SealPublicationRange(definitions, rows)
	if err != nil {
		return SemanticSourceView{}, err
	}
	return SemanticSourceView{owner: owner, rangeValue: rangeValue}, nil
}

// SemanticSourceFragmentView returns Module's already-sealed owner-local
// interval without rebuilding or copying its publication claims.
func SemanticSourceFragmentView(view View) (SemanticSourceView, error) {
	component, ok := view.componentForRead()
	if !ok || component == nil || !component.content.Available() {
		return SemanticSourceView{}, errSemanticSourceUnavailable
	}
	if !component.semantic.Valid() || !moduleSemanticSourceOwnerReady(component) {
		return SemanticSourceView{}, errSemanticSourceInconsistent
	}
	return SemanticSourceView{owner: component.content, rangeValue: component.semantic}, nil
}

// moduleSemanticSourceOwnerReady validates the live authored and Entry
// projections against the scalar captured by the committed Component. It
// creates no publication claims and never seals a second fragment.
func moduleSemanticSourceOwnerReady(component *Component) bool {
	if component == nil || !component.content.Available() || authoredContent(component.imports) != component.content || !component.entry.valid() {
		return false
	}
	view := component.View()
	counts, err := moduleSemanticSourceCounts(view)
	if err != nil {
		return false
	}
	definitions, err := moduleSemanticSourceDefinitions()
	if err != nil {
		return false
	}
	digest, ok := semanticsource.DigestPublicationCounts(definitions, counts[:])
	sealedDigest, sealed := component.semantic.Digest()
	return ok && sealed && digest == sealedDigest
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

func moduleSemanticSourceDefinitions() ([]semanticsource.RelationDef, error) {
	schema, err := relations.CanonicalSchema()
	if err != nil || schema == nil {
		return nil, errSemanticSourceInconsistent
	}
	definitions := make([]semanticsource.RelationDef, 0, semanticSourceFragmentPublicationCount)
	for _, row := range schema.Rows() {
		if row.Owner == relations.OwnerProgramModule {
			definitions = append(definitions, row.Definition)
		}
	}
	if len(definitions) != semanticSourceFragmentPublicationCount {
		return nil, errSemanticSourceInconsistent
	}
	return definitions, nil
}
