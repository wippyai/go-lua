package source

import (
	"github.com/wippyai/go-lua/program/internal/schema/relations"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/semanticsource"
)

// SemanticSourceView is Source's detached, fixed semantic-publication
// interval. It owns no Source rows and exposes only the eight cardinality
// claims that Source seals for Program.
type SemanticSourceView struct {
	owner      keyspace.ContentID
	rangeValue semanticsource.PublicationRange
}

func (view SemanticSourceView) Valid() bool {
	return view.owner.Available() && view.rangeValue.Valid()
}

func (view SemanticSourceView) OwnerID() keyspace.ContentID { return view.owner }

// Range returns the already-sealed owner range by value. Its private rows
// remain shared with the child owner; no claim slice is copied.
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

// SemanticSourceCursor is the shared bounded cursor over this fixed Source
// interval. The alias does not introduce a second semantic registry.
type SemanticSourceCursor = semanticsource.PublicationCursor

func (view SemanticSourceView) Cursor() SemanticSourceCursor {
	if !view.Valid() {
		return semanticsource.PublicationRange{}.Cursor()
	}
	return view.rangeValue.Cursor()
}

// SealSemanticSourceFragment validates the exact Source/Flow publication
// interval and detaches it behind an owner fence. The fixed interval rejects
// foreign, duplicate, missing, and reordered tokens before a fragment can be
// consumed by Program.
func SealSemanticSourceFragment(owner keyspace.ContentID, rows []semanticsource.Publication) (SemanticSourceView, error) {
	if !owner.Available() {
		return SemanticSourceView{}, ErrSemanticSourceUnavailable
	}
	definitions, err := sourceSemanticSourceDefinitions()
	if err != nil {
		return SemanticSourceView{}, err
	}
	rangeValue, err := semanticsource.SealPublicationRange(definitions, rows)
	if err != nil {
		return SemanticSourceView{}, err
	}
	return SemanticSourceView{owner: owner, rangeValue: rangeValue}, nil
}

// SemanticSourceFragmentView returns Source's already-sealed owner-local
// cursor surface without rebuilding or copying its publication claims.
func SemanticSourceFragmentView(view View) (SemanticSourceView, error) {
	if view.authority == nil || view.authority.identity.name == "" || !view.authority.content.Available() {
		return SemanticSourceView{}, ErrSemanticSourceUnavailable
	}
	fragment := view.authority.semantic
	if !fragment.Valid() {
		return SemanticSourceView{}, ErrSemanticSourceIncomplete
	}
	digest, err := sourceSemanticSourceDigest(view)
	if err != nil {
		return SemanticSourceView{}, err
	}
	sealedDigest, ok := fragment.Digest()
	if !ok || digest != sealedDigest {
		return SemanticSourceView{}, ErrSemanticSourceIncomplete
	}
	return SemanticSourceView{owner: view.authority.content, rangeValue: fragment}, nil
}

// sourceSemanticSourceDigest validates the live Source projections and derives
// only their scalar publication seal. It deliberately creates neither
// Publication values nor a second PublicationRange; the committed fragment's
// private range remains the sole claim storage.
func sourceSemanticSourceDigest(view View) ([32]byte, error) {
	direct, roots, bodyCount, err := sourceSemanticSourceCounts(view)
	if err != nil {
		return [32]byte{}, err
	}
	literals, err := sourceLiteralCount(view)
	if err != nil {
		return [32]byte{}, err
	}
	keys, exactKeys, err := sourceKeyCounts(view)
	if err != nil {
		return [32]byte{}, err
	}
	faults, err := sourceFaultCount(view)
	if err != nil {
		return [32]byte{}, err
	}
	definitions, err := sourceSemanticSourceDefinitions()
	if err != nil {
		return [32]byte{}, err
	}
	digest, ok := semanticsource.DigestPublicationCounts(definitions, []int{
		direct,
		direct,
		keys,
		exactKeys,
		faults,
		literals,
		bodyCount,
		roots,
	})
	if !ok {
		return [32]byte{}, ErrSemanticSourceIncomplete
	}
	return digest, nil
}

// SemanticSourceFragment retains the historical detached-slice API. The
// owner-local seal is performed by the component finalizer; this query only
// detaches values for legacy cold callers.
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
	fragment, err := SealSemanticSourceFragment(view.Identity().ContentID(), rows)
	if err != nil {
		return semanticsource.PublicationRange{}, err
	}
	return fragment.Range(), nil
}

func sourceSemanticSourceDefinitions() ([]semanticsource.RelationDef, error) {
	schema, err := relations.CanonicalSchema()
	if err != nil || schema == nil {
		return nil, ErrSemanticSourceIncomplete
	}
	rows := schema.Rows()
	definitions := make([]semanticsource.RelationDef, 0, semanticSourceFragmentPublicationCount)
	for _, row := range rows {
		if row.Owner == relations.OwnerProgramSource {
			definitions = append(definitions, row.Definition)
		}
	}
	if len(definitions) != semanticSourceFragmentPublicationCount {
		return nil, ErrSemanticSourceIncomplete
	}
	return definitions, nil
}
