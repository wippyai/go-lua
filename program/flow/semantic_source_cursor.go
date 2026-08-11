package flow

import (
	"github.com/wippyai/go-lua/program/internal/schema/relations"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/semanticsource"
)

const semanticSourceFragmentPublicationCount = 33

// SemanticSourceView is Flow's detached, fixed 33-publication interval.
// Literals and Body remain Source-owned; this interval contains only the
// Flow-owned definitions in their generated order.
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

// SemanticSourceCursor is the shared bounded cursor over Flow's fixed
// publication interval.
type SemanticSourceCursor = semanticsource.PublicationCursor

func (view SemanticSourceView) Cursor() SemanticSourceCursor {
	if !view.Valid() {
		return semanticsource.PublicationRange{}.Cursor()
	}
	return view.rangeValue.Cursor()
}

// SealSemanticSourceFragment validates and detaches Flow's exact fixed range.
func SealSemanticSourceFragment(owner keyspace.ContentID, rows []semanticsource.Publication) (SemanticSourceView, error) {
	if !owner.Available() {
		return SemanticSourceView{}, errUnavailableSemanticSourceFragment
	}
	definitions, err := flowSemanticSourceDefinitions()
	if err != nil {
		return SemanticSourceView{}, err
	}
	rangeValue, err := semanticsource.SealPublicationRange(definitions, rows)
	if err != nil {
		return SemanticSourceView{}, err
	}
	return SemanticSourceView{owner: owner, rangeValue: rangeValue}, nil
}

// SemanticSourceFragmentView returns Flow's already-sealed owner-local
// interval without rebuilding or copying its publication claims.
func SemanticSourceFragmentView(view View) (SemanticSourceView, error) {
	if !view.ContentID().Available() || view.component == nil {
		return SemanticSourceView{}, errUnavailableSemanticSourceFragment
	}
	// Recheck the sealed owner projections before exposing the cached range.
	// This is a scalar/fence validation only: it never rebuilds or reseals the
	// publication claims retained by Component.
	if !view.semanticSourceAvailable() {
		return SemanticSourceView{}, errUnavailableSemanticSourceFragment
	}
	if !view.component.semantic.Valid() {
		return SemanticSourceView{}, errUnavailableSemanticSourceFragment
	}
	counts, err := flowSemanticSourceCounts(view)
	if err != nil {
		return SemanticSourceView{}, err
	}
	definitions, err := flowSemanticSourceDefinitions()
	if err != nil {
		return SemanticSourceView{}, err
	}
	digest, ok := semanticsource.DigestPublicationCounts(definitions, counts[:])
	sealedDigest, sealed := view.component.semantic.Digest()
	if !ok || !sealed || digest != sealedDigest {
		return SemanticSourceView{}, errUnavailableSemanticSourceFragment
	}
	return SemanticSourceView{owner: view.ContentID(), rangeValue: view.component.semantic}, nil
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

func flowSemanticSourceDefinitions() ([]semanticsource.RelationDef, error) {
	schema, err := relations.CanonicalSchema()
	if err != nil || schema == nil {
		return nil, errUnavailableSemanticSourceFragment
	}
	definitions := make([]semanticsource.RelationDef, 0, semanticSourceFragmentPublicationCount)
	for _, row := range schema.Rows() {
		if row.Owner == relations.OwnerProgramFlow {
			definitions = append(definitions, row.Definition)
		}
	}
	if len(definitions) != semanticSourceFragmentPublicationCount {
		return nil, errUnavailableSemanticSourceFragment
	}
	return definitions, nil
}
