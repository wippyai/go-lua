package service

import (
	"context"
	"sync"

	"github.com/wippyai/go-lua/analysis/embedding"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

type resultKey struct {
	unitID     UnitID
	unitDigest Digest
	profile    string
	solveSeq   embedding.SolveSeq
}

type unitProfileKey struct {
	unitID  UnitID
	profile string
}

// analysisCacheKey identifies every input that can affect a completed analysis
// result. UnitNamespace fences stable lexical ownership to the exact logical
// (unit ID, module path, entry document) triple. UnitDigest fences source,
// resolution, manifests, and checker policy; entryLabel is deliberately
// separate because labels are presentation-only for a UnitInput but are
// embedded in rendered diagnostics produced by the parser.
type analysisCacheKey struct {
	unitNamespace lexicalidentity.UnitNamespace
	unitDigest    Digest
	profile       string
	entryLabel    string
}

// BatchSession is the reference whole-unit implementation of
// WorkspaceSession. Its mutex protects unit/result maps and immutable result
// publication. Parsing and solving happen against a retained input snapshot
// outside the lock, then publish only if that unit generation still matches.
type BatchSession struct {
	mu sync.RWMutex

	units              map[UnitID]retainedUnit
	results            map[resultKey]*completedSnapshot
	latest             map[unitProfileKey]resultKey
	bySeq              map[embedding.SolveSeq]resultKey
	analysisCache      map[analysisCacheKey]*completedSnapshot
	semanticProjection bool
	nextSeq            embedding.SolveSeq
	nextUnitGeneration uint64
}

var _ WorkspaceSession = (*BatchSession)(nil)

type batchSessionOptions struct {
	semanticProjection bool
}

// BatchSessionOption configures a BatchSession at construction time. Session
// options are immutable after construction so cached completed results always
// have one consistent projection shape.
type BatchSessionOption func(*batchSessionOptions)

// WithoutSemanticProjection omits the semantic-query snapshot used by LSP and
// readmodel queries. Core completed-result projections such as diagnostics,
// judgments, manifests, placement, summaries, and debug maps remain available.
// Semantic query methods return empty responses with their normal query meta.
func WithoutSemanticProjection() BatchSessionOption {
	return func(options *batchSessionOptions) {
		options.semanticProjection = false
	}
}

func NewBatchSession(options ...BatchSessionOption) *BatchSession {
	selected := batchSessionOptions{semanticProjection: true}
	for _, option := range options {
		if option != nil {
			option(&selected)
		}
	}
	return &BatchSession{
		units:              make(map[UnitID]retainedUnit),
		results:            make(map[resultKey]*completedSnapshot),
		latest:             make(map[unitProfileKey]resultKey),
		bySeq:              make(map[embedding.SolveSeq]resultKey),
		analysisCache:      make(map[analysisCacheKey]*completedSnapshot),
		semanticProjection: selected.semanticProjection,
	}
}

func (s *BatchSession) UpsertUnit(ctx context.Context, input UnitInput) (UnitState, error) {
	if err := ctx.Err(); err != nil {
		return UnitState{}, err
	}
	unit, err := normalizeUnitInput(input)
	if err != nil {
		return UnitState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return UnitState{}, err
	}
	previous, exists := s.units[input.ID]
	changed := !exists || previous.digest != unit.digest || previous.input.Profile != unit.input.Profile
	s.nextUnitGeneration++
	unit.generation = s.nextUnitGeneration
	s.units[input.ID] = unit
	return UnitState{
		UnitID:        input.ID,
		UnitDigest:    unit.digest,
		SourceDigests: cloneMap(unit.sourceDigests),
		Profile:       unit.input.Profile,
		Changed:       changed,
	}, nil
}

func (s *BatchSession) RemoveUnit(ctx context.Context, id UnitID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.units, id)
	for key := range s.results {
		if key.unitID != id {
			continue
		}
		delete(s.results, key)
		delete(s.bySeq, key.solveSeq)
	}
	for key := range s.latest {
		if key.unitID == id {
			delete(s.latest, key)
		}
	}
	for key, snapshot := range s.analysisCache {
		if snapshot != nil && snapshot.tag.UnitID == id {
			delete(s.analysisCache, key)
		}
	}
	return nil
}

func analysisKey(unit retainedUnit, profile string) analysisCacheKey {
	return analysisCacheKey{
		unitNamespace: unitLexicalNamespace(unit.input),
		unitDigest:    unit.digest,
		profile:       profile,
		entryLabel:    documentLabel(unit.input, unit.input.EntryDocument),
	}
}

// cachedAnalysis returns a new publication wrapper around immutable completed
// analysis. The wrapper's tag is the only mutable part of publication, so a
// forced solve can still receive a fresh SolveSeq without rerunning the
// analysis. The key includes the exact logical lexical namespace as
// well as content/version digests: content-identical logical units may have
// different stable body ownership and therefore must never share completed
// artifacts.
func (s *BatchSession) cachedAnalysis(unit retainedUnit, profile string, documentVersion int64) *completedSnapshot {
	s.mu.RLock()
	cached := s.analysisCache[analysisKey(unit, profile)]
	s.mu.RUnlock()
	if cached == nil {
		return nil
	}
	copy := *cached
	copy.tag = cloneResultTag(cached.tag)
	copy.tag.UnitID = unit.input.ID
	copy.tag.UnitDigest = unit.digest
	copy.tag.Profile = profile
	copy.tag.DocumentVersion = documentVersion
	copy.tag.SourceDigests = cloneMap(unit.sourceDigests)
	return &copy
}

func (s *BatchSession) LastComplete(ctx context.Context, req ResultRequest) (CompletedResult, bool) {
	if err := ctx.Err(); err != nil {
		return CompletedResult{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot, _, ok := s.resultForSelectorLocked(req.Selector)
	if !ok {
		return CompletedResult{}, false
	}
	return CompletedResult{snapshot: snapshot}, true
}

func (s *BatchSession) resultForSelectorLocked(selector ResultSelector) (*completedSnapshot, QueryMeta, bool) {
	profile := selector.Profile
	if profile == "" {
		if unit, ok := s.units[selector.UnitID]; ok {
			profile = effectiveProfile(unit.input.Profile)
		}
	}
	var key resultKey
	var ok bool
	if selector.SolveSeq != 0 {
		key, ok = s.bySeq[selector.SolveSeq]
		ok = ok && key.unitID == selector.UnitID && (profile == "" || key.profile == profile)
	} else {
		key, ok = s.latest[unitProfileKey{unitID: selector.UnitID, profile: profile}]
	}
	if !ok {
		return nil, QueryMeta{}, false
	}
	snapshot, ok := s.results[key]
	if !ok || snapshot == nil {
		return nil, QueryMeta{}, false
	}
	stale := true
	if unit, exists := s.units[key.unitID]; exists {
		stale = unit.digest != key.unitDigest || snapshot.tag.DocumentVersion != unit.input.DocumentVersion
	}
	meta := QueryMeta{
		Tag:   cloneResultTag(snapshot.tag),
		Stale: stale,
	}
	return snapshot, meta, true
}

func effectiveProfile(profile string) string {
	if profile == "" {
		return "default"
	}
	return profile
}
