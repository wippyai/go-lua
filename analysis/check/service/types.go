package service

import (
	"context"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/placementplan"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// UnitID is the session-stable identity of one independently solved source
// unit.
type UnitID string

// BodyID is a deterministic path within a materialized program result. The
// chunk root is "root" and nested bodies use slash-separated child ordinals.
type BodyID string

// UnitInput is the complete batch-check input for one source unit. The
// reference implementation checks EntryFile as the program chunk; every source
// snapshot participates in UnitDigest so changes cannot reuse a stale result.
type UnitInput struct {
	ID                UnitID
	ModulePath        string
	EntryFile         string
	SourceFiles       map[string][]byte
	ExternalManifests map[string]*manifest.Manifest
	IncludeStdlib     bool
	Globals           []string
	GlobalTypes       map[string]typ.Type
	StateLanes        []state.LaneID
	DiagnosticPolicy  diagnostic.Policy
	JudgmentPolicy    judgment.PolicyConfig
	Profile           string
	DocumentVersion   int64
}

// UnitState describes the normalized source snapshot retained by the session.
type UnitState struct {
	UnitID        UnitID
	UnitDigest    Digest
	SourceDigests map[string]Digest
	Profile       string
	Changed       bool
}

// SolveTrigger records why an adapter requested a solve. It is observational;
// checker semantics do not depend on the trigger.
type SolveTrigger string

const (
	TriggerKeystroke SolveTrigger = "keystroke"
	TriggerSave      SolveTrigger = "save"
	TriggerAdmission SolveTrigger = "admission"
	TriggerBatch     SolveTrigger = "batch"
)

// Freshness controls whether an identical completed result may be reused.
type Freshness uint8

const (
	FreshnessAllowCached Freshness = iota
	FreshnessRequireNew
)

type SolveRequest struct {
	UnitID          UnitID
	Profile         string
	Trigger         SolveTrigger
	Freshness       Freshness
	DocumentVersion int64
}

// ResultTag is the public version key for one immutable completed result.
type ResultTag struct {
	UnitID          UnitID
	SolveSeq        uint64
	UnitDigest      Digest
	ManifestDigest  Digest
	SourceDigests   map[string]Digest
	BodyVersions    map[BodyID]uint64
	Profile         string
	DocumentVersion int64
}

type BodyResultRef struct {
	ID            BodyID
	ResultVersion uint64
}

// CompletedResult is an immutable handle to a published result snapshot. Its
// zero value is invalid. Snapshot accessors never expose the session's mutable
// maps or slices.
type CompletedResult struct {
	snapshot *completedSnapshot
}

func (r CompletedResult) Valid() bool { return r.snapshot != nil }

func (r CompletedResult) Tag() ResultTag {
	if r.snapshot == nil {
		return ResultTag{}
	}
	return cloneResultTag(r.snapshot.tag)
}

func (r CompletedResult) Bodies() []BodyResultRef {
	if r.snapshot == nil {
		return nil
	}
	return append([]BodyResultRef(nil), r.snapshot.bodies...)
}

func (r CompletedResult) Judgments() []judgment.Judgment {
	if r.snapshot == nil {
		return nil
	}
	return cloneJudgments(r.snapshot.judgments)
}

func (r CompletedResult) RenderedDiagnostics() []diagnostic.Diagnostic {
	if r.snapshot == nil {
		return nil
	}
	return cloneDiagnostics(r.snapshot.diagnostics)
}

func (r CompletedResult) ManifestBytes() (path string, digest Digest, data []byte) {
	if r.snapshot == nil {
		return "", Digest{}, nil
	}
	return r.snapshot.manifestPath, r.snapshot.tag.ManifestDigest, append([]byte(nil), r.snapshot.manifestData...)
}

func (r CompletedResult) PlacementPlan() placementplan.Plan {
	if r.snapshot == nil {
		return placementplan.Plan{}
	}
	return clonePlacementPlan(r.snapshot.placement)
}

// SummarySnapshot returns a read-only view of the exact-key fixed-point
// snapshot. The view only exposes the cloning accessors of summary.Snapshot;
// it cannot reach ReadOwnedNormalized or EntriesOwnedNormalized, so a client
// can never observe or mutate the storage backing the published result.
func (r CompletedResult) SummarySnapshot() SummaryView {
	if r.snapshot == nil {
		return SummaryView{}
	}
	return SummaryView{snapshot: r.snapshot.summaries}
}

// SummaryView is a read-only projection of summary.Snapshot for published
// results. It forwards only Read and Entries, both of which clone every
// summary they return.
type SummaryView struct {
	snapshot summary.Snapshot
}

// Read returns a clone of the summary for k.
func (v SummaryView) Read(k summary.SummaryKey) (summary.Summary, bool) {
	return v.snapshot.Read(k)
}

// Entries returns clones of every exact-key summary in deterministic key
// order.
func (v SummaryView) Entries() []summary.EntrySummary {
	return v.snapshot.Entries()
}

type ResultSelector struct {
	UnitID        UnitID
	ResultVersion uint64 // SolveSeq; zero selects the latest complete result.
	Profile       string
}

type ResultRequest struct {
	Selector ResultSelector
}

type QueryMeta struct {
	Tag   ResultTag
	Stale bool
}

type SourceRange struct {
	File      string
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

type ListJudgmentsRequest struct {
	Selector ResultSelector
	Codes    []judgment.Code
	Verdicts []judgment.Verdict
	Anchors  []judgment.SubjectAnchor
	Range    *SourceRange
}

type ListJudgmentsResponse struct {
	Meta      QueryMeta
	Judgments []judgment.Judgment
	CodeSpecs []judgment.CodeSpec
}

type JudgmentsByAnchorRequest struct {
	Selector  ResultSelector
	AnchorKey string
	Codes     []judgment.Code
}

type JudgmentsByAnchorResponse struct {
	Meta      QueryMeta
	Anchor    judgment.SubjectAnchor
	Judgments []judgment.Judgment
}

type ListDiagnosticsRequest struct {
	Selector         ResultSelector
	JudgmentPolicy   judgment.PolicyConfig
	DiagnosticPolicy diagnostic.Policy
	Range            *SourceRange
}

// ListDiagnosticsResponse carries the rendered projection and the exact raw
// judgments from which it was produced.
type ListDiagnosticsResponse struct {
	Meta     QueryMeta
	Rendered []diagnostic.Diagnostic
	Raw      []judgment.Judgment
}

type ExportManifestRequest struct {
	Selector ResultSelector
}

type ExportManifestResponse struct {
	Meta   QueryMeta
	Path   string
	Digest Digest
	Data   []byte
}

type PlacementPlanRequest struct {
	Selector ResultSelector
}

type PlacementPlanResponse struct {
	Meta QueryMeta
	Plan placementplan.Plan
}

type SummarySnapshotRequest struct {
	Selector ResultSelector
	Keys     []summary.SummaryKey
}

type SummarySnapshotResponse struct {
	Meta      QueryMeta
	Summaries []summary.EntrySummary
	Digests   map[summary.SummaryKey]summary.Digest
}

type BodyResultVersionsRequest struct {
	Selector ResultSelector
}

type BodyResultVersionsResponse struct {
	Meta     QueryMeta
	Versions map[BodyID]uint64
}

// WorkspaceSession is the core checker service surface. Implementations safely
// serve concurrent readers and writers while publishing immutable
// CompletedResult snapshots.
type WorkspaceSession interface {
	UpsertUnit(context.Context, UnitInput) (UnitState, error)
	RemoveUnit(context.Context, UnitID) error
	EnsureSolved(context.Context, SolveRequest) (ResultTag, error)
	LastComplete(context.Context, ResultRequest) (CompletedResult, bool)

	ListJudgments(context.Context, ListJudgmentsRequest) (ListJudgmentsResponse, error)
	JudgmentsByAnchor(context.Context, JudgmentsByAnchorRequest) (JudgmentsByAnchorResponse, error)
	Diagnostics(context.Context, ListDiagnosticsRequest) (ListDiagnosticsResponse, error)
	ManifestBytes(context.Context, ExportManifestRequest) (ExportManifestResponse, error)
	PlacementPlan(context.Context, PlacementPlanRequest) (PlacementPlanResponse, error)
	SummarySnapshot(context.Context, SummarySnapshotRequest) (SummarySnapshotResponse, error)
	BodyResultVersions(context.Context, BodyResultVersionsRequest) (BodyResultVersionsResponse, error)
}
