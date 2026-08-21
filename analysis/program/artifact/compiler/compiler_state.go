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
	"github.com/wippyai/go-lua/analysis/schema/program/calltarget"
	"github.com/wippyai/go-lua/analysis/schema/program/heapindex"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	programdiagnostic "github.com/wippyai/go-lua/analysis/schema/program/programdiagnostic"
	staticnode "github.com/wippyai/go-lua/analysis/schema/program/staticnode"
)

// compiler is the private one-shot assembly state used by the artifact
// compiler. It exists only during construction; the sealed Artifact is the
// sole retained result.
type compiler struct {
	input                                    *program.Program
	key                                      programartifact.CompileKey
	counts                                   denominator.CountRows
	environment                              []environmentEdgeDraft
	localTransfer                            *localtransfer.Builder
	regions                                  []regionDraft
	events                                   []wtoEventDraft
	values                                   []programschema.Values
	valuesMembers                            []programschema.ValuesMember
	calls                                    []programschema.Call
	callResults                              []programschema.CallResult
	callResultSlots                          []programschema.CallResultSlot
	callOperands                             []programschema.CallOperand
	callArguments                            []programschema.CallArgument
	callTypeArguments                        []programschema.CallTypeArgument
	moduleImports                            []programschema.ModuleImport
	moduleRequests                           []programschema.ModuleRequest
	moduleEntries                            []programschema.ModuleEntry
	moduleEntryRootCells                     []programschema.ModuleEntryRootCell
	moduleEntryRootFunctions                 []programschema.ModuleEntryRootFunction
	moduleEntryMembers                       []programschema.ModuleEntryMember
	bodyBoundary                             *bodyboundary.Bundle
	storageCellLifetimes                     []lifecycle.StorageCellLifetime
	subjectLifetimes                         []lifecycle.SubjectLiveness
	subjectEvents                            []lifecycle.SubjectEvent
	aliasRouteScopes                         []lifecycle.SubjectAliasRouteScope
	aliasRouteScopeMembers                   []lifecycle.SubjectAliasRouteScopeMember
	aliasCandidates                          []lifecycle.SubjectAliasCandidate
	callTargets                              []calltarget.Target
	allocations                              *allocation.Bundle
	heapIndexes                              []heapindex.Index
	occurrences                              []programschema.Occurrence
	occurrencePoints                         []programschema.OccurrencePoint
	occurrenceInputs                         []programschema.OccurrenceInput
	exactScalar                              *exactscalar.Bundle
	arithmeticSummaries                      []programschema.ArithmeticSummary
	unarySummaries                           []programschema.UnarySummary
	ruleOccurrences                          []programschema.RuleOccurrence
	issuance                                 issuance.Directory
	diagnostic                               programdiagnostic.Publication
	staticTypeValues                         []programschema.StaticTypeValue
	staticTypeNodes                          []staticnode.StaticTypeNode
	staticTypeNodeUnionMembers               []staticnode.StaticTypeNodeUnionMember
	staticTypeNodeIntersectionMembers        []staticnode.StaticTypeNodeIntersectionMember
	staticTypeNodeGenericArguments           []staticnode.StaticTypeNodeGenericArgument
	staticTypeNodeAliasParameters            []staticnode.StaticTypeNodeAliasParameter
	staticTypeNodeInterfaceExtends           []staticnode.StaticTypeNodeInterfaceExtend
	staticTypeNodeInterfaceMembers           []staticnode.StaticTypeNodeInterfaceMember
	staticTypeNodeTypeFunctionTypeParameters []staticnode.StaticTypeNodeTypeFunctionTypeParameter
	staticTypeNodeTypeFunctionParameters     []staticnode.StaticTypeNodeTypeFunctionParameter
	staticTypeNodeTypeFunctionReturns        []staticnode.StaticTypeNodeTypeFunctionReturn
	staticTypeNodeRecordFields               []staticnode.StaticTypeNodeRecordField
	staticTypeNodeReferenceSourceKeys        []staticnode.StaticTypeNodeReferenceSourceKey
	staticTypeNodeReferenceCanonicalKeys     []staticnode.StaticTypeNodeReferenceCanonicalKey
	staticExpressions                        []programschema.StaticExpression
	staticInputs                             []programschema.StaticInput
	pointGeometry                            map[identity.ContentID]pointDraft
	occurrenceSpans                          map[occurrenceLookup]occurrenceSpanGeometry
	stages                                   *stageplan.Builder
	pointIDsBySite                           map[identity.ContentID][]identity.ContentID
	environmentByRoute                       map[identity.ContentID]environmentRouteIndex
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
	if index < 0 || index >= len(compiler.values) {
		return programschema.Values{}, false
	}
	row := compiler.values[index]
	return row, row.Available()
}

// valueMemberAt reads one member directly from the canonical dense member
// column named by a Values row's span.
func (compiler *compiler) valueMemberAt(row programschema.Values, index int) (programschema.ValuesMember, bool) {
	if compiler == nil || !row.Available() || index < 0 {
		return programschema.ValuesMember{}, false
	}
	offset, count, spanOK := row.MemberSpan()
	if !spanOK || index >= int(count) || uint64(offset)+uint64(index) >= uint64(len(compiler.valuesMembers)) {
		return programschema.ValuesMember{}, false
	}
	member := compiler.valuesMembers[int(offset)+index]
	return member, member.Available()
}
