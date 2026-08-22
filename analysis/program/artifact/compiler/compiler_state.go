package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/allocation"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/bodyboundary"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/exactscalar"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/localtransfer"
	stageplan "github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/stage"
	"github.com/wippyai/go-lua/analysis/program/artifact/issuance"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/program"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
)

// compiler is the private one-shot assembly state used by the artifact
// compiler. It exists only during construction; the sealed Artifact is the
// sole retained result.
type compiler struct {
	input              *program.Program
	key                programartifact.CompileKey
	counts             denominator.CountRows
	publication        programpublication.Publication
	environment        []environmentEdgeDraft
	localTransfer      *localtransfer.Builder
	regions            []regionDraft
	events             []wtoEventDraft
	bodyBoundary       *bodyboundary.Bundle
	allocations        *allocation.Bundle
	exactScalar        *exactscalar.Bundle
	issuance           issuance.Directory
	pointGeometry      map[identity.ContentID]pointDraft
	occurrenceSpans    map[occurrenceLookup]occurrenceSpanGeometry
	pointScratch       []identity.ContentID
	pointScratchSeen   map[identity.ContentID]struct{}
	stages             *stageplan.Builder
	environmentByRoute map[identity.ContentID]environmentRouteIndex
}

func fitsUint32(value int) bool { return value >= 0 && uint64(value) <= uint64(^uint32(0)) }

func artifactFormat() uint64 { return programartifact.ArtifactFormatVersion }

func contentIDBefore(left, right identity.ContentID) bool {
	for index := range left {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return false
}

// valueRowForTerm resolves the canonical Values column by the authored Flow
// ordinal. The compiler retains no nested Values draft or member slice.
func (compiler *compiler) valueRowForTerm(term keyspace.Term) (programschema.Values, bool) {
	if compiler == nil || keyspace.TermFamily(term) != keyspace.FamilyValues || keyspace.TermOrdinal(term) == 0 {
		return programschema.Values{}, false
	}
	index := int(keyspace.TermOrdinal(term)) - 1
	if index < 0 || index >= len(compiler.publication.Values) {
		return programschema.Values{}, false
	}
	row := compiler.publication.Values[index]
	return row, row.Available()
}

// valueMemberAt reads one member directly from the canonical dense member
// column named by a Values row's span.
func (compiler *compiler) valueMemberAt(row programschema.Values, index int) (programschema.ValuesMember, bool) {
	if compiler == nil || !row.Available() || index < 0 {
		return programschema.ValuesMember{}, false
	}
	offset, count, spanOK := row.MemberSpan()
	if !spanOK || index >= int(count) || uint64(offset)+uint64(index) >= uint64(len(compiler.publication.ValuesMembers)) {
		return programschema.ValuesMember{}, false
	}
	member := compiler.publication.ValuesMembers[int(offset)+index]
	return member, member.Available()
}
