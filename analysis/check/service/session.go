package service

import (
	"context"
	"sync"
)

type resultKey struct {
	unitID     UnitID
	unitDigest Digest
	profile    string
	solveSeq   uint64
}

type unitProfileKey struct {
	unitID  UnitID
	profile string
}

// BatchSession is the reference whole-unit implementation of
// WorkspaceSession. Its mutex enforces the session's single-writer/multi-reader
// contract; a solve holds the writer lock through immutable publication.
type BatchSession struct {
	mu sync.RWMutex

	units   map[UnitID]retainedUnit
	results map[resultKey]*completedSnapshot
	latest  map[unitProfileKey]resultKey
	bySeq   map[uint64]resultKey
	nextSeq uint64
}

var _ WorkspaceSession = (*BatchSession)(nil)

func NewBatchSession() *BatchSession {
	return &BatchSession{
		units:   make(map[UnitID]retainedUnit),
		results: make(map[resultKey]*completedSnapshot),
		latest:  make(map[unitProfileKey]resultKey),
		bySeq:   make(map[uint64]resultKey),
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
	return nil
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
	if selector.ResultVersion != 0 {
		key, ok = s.bySeq[selector.ResultVersion]
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
		stale = unit.digest != key.unitDigest
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
