package service

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/exportmanifest"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/placementplan"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/module/typelookup"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/parse"
)

// EnsureSolved publishes a complete immutable result for the requested unit.
// Concurrent requests for the same unchanged unit use the both-valid policy:
// each may independently solve and publish a distinct sequence-tagged result.
func (s *BatchSession) EnsureSolved(ctx context.Context, req SolveRequest) (ResultTag, error) {
	if err := ctx.Err(); err != nil {
		return ResultTag{}, err
	}

	for {
		unit, profile, documentVersion, tag, cached, err := s.solveInputSnapshot(req)
		if err != nil {
			return ResultTag{}, err
		}
		if cached {
			return tag, nil
		}

		snapshot, err := solveUnit(ctx, unit, profile, documentVersion)
		if err != nil {
			return ResultTag{}, err
		}
		if err := ctx.Err(); err != nil {
			return ResultTag{}, err
		}

		tag, discard, err := s.publishSolved(req.UnitID, unit, profile, snapshot)
		if err != nil {
			return ResultTag{}, err
		}
		if discard {
			// The unit changed while this solve ran. Discard the unpublishable
			// snapshot and retry from a fresh immutable input snapshot. Concurrent
			// EnsureSolved calls are intentionally both-valid: each may publish a
			// complete result for this unchanged generation in sequence order.
			if err := ctx.Err(); err != nil {
				return ResultTag{}, err
			}
			continue
		}
		return tag, nil
	}
}

// publishSolved publishes snapshot only if unit is still the exact retained
// input generation used to produce it. A discard is not an error: EnsureSolved
// retries from the newer immutable unit snapshot.
func (s *BatchSession) publishSolved(unitID UnitID, unit retainedUnit, profile string, snapshot *completedSnapshot) (ResultTag, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, exists := s.units[unitID]
	if !exists {
		return ResultTag{}, false, fmt.Errorf("%w: %s", ErrUnitNotFound, unitID)
	}
	if current.generation != unit.generation || current.digest != unit.digest {
		return ResultTag{}, true, nil
	}
	s.nextSeq++
	snapshot.tag.SolveSeq = s.nextSeq
	key := resultKey{
		unitID:     unitID,
		unitDigest: unit.digest,
		profile:    profile,
		solveSeq:   s.nextSeq,
	}
	s.results[key] = snapshot
	s.latest[unitProfileKey{unitID: unitID, profile: profile}] = key
	s.bySeq[key.solveSeq] = key
	return cloneResultTag(snapshot.tag), false, nil
}

// solveInputSnapshot takes the smallest immutable unit snapshot needed for an
// outside-lock solve. Readers and unrelated writers only hold the lock for the
// map lookup/cache check, never for parsing or fixed-point iteration.
func (s *BatchSession) solveInputSnapshot(req SolveRequest) (retainedUnit, string, int64, ResultTag, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	unit, ok := s.units[req.UnitID]
	if !ok {
		return retainedUnit{}, "", 0, ResultTag{}, false, fmt.Errorf("%w: %s", ErrUnitNotFound, req.UnitID)
	}
	profile := req.Profile
	if profile == "" {
		profile = unit.input.Profile
	}
	profile = effectiveProfile(profile)
	documentVersion := req.DocumentVersion
	if documentVersion == 0 {
		documentVersion = unit.input.DocumentVersion
	}
	if req.Freshness != FreshnessRequireNew {
		latestKey := unitProfileKey{unitID: req.UnitID, profile: profile}
		if key, exists := s.latest[latestKey]; exists && key.unitDigest == unit.digest {
			if snapshot := s.results[key]; snapshot != nil && snapshot.tag.DocumentVersion == documentVersion {
				return retainedUnit{}, "", 0, cloneResultTag(snapshot.tag), true, nil
			}
		}
	}
	return unit, profile, documentVersion, ResultTag{}, false, nil
}

func solveUnit(ctx context.Context, unit retainedUnit, profile string, documentVersion int64) (*completedSnapshot, error) {
	input := unit.input
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stmts, err := parse.ParseString(string(input.SourceFiles[input.EntryFile]), input.EntryFile)
	if err != nil {
		return nil, fmt.Errorf("checker service: parse %s: %w", input.EntryFile, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	manifests := orderedManifests(input.ExternalManifests)
	globals := append([]string(nil), input.Globals...)
	globalTypes := manifestGlobalTypes(manifests)
	for name, valueType := range input.GlobalTypes {
		globalTypes[name] = valueType
	}
	for _, item := range manifests {
		globals = append(globals, item.Globals...)
	}
	globals = normalizedStrings(globals)
	checked, err := program.RunChunk(stmts, program.Config{Context: ctx, Check: body.Config{
		Registry:      checkerRegistry,
		Globals:       globals,
		GlobalTypes:   globalTypes,
		StateLanes:    input.StateLanes,
		Signatures:    signaturelookup.Source{Manifests: manifests, IncludeStdlib: input.IncludeStdlib},
		ModuleExports: importlookup.Source{Manifests: manifests},
		ModuleTypes:   typelookup.Source{Manifests: manifests},
	}})
	if err != nil {
		return nil, fmt.Errorf("checker service: solve %s: %w", input.ID, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	exported := exportmanifest.FromProgramResult(input.ModulePath, checked)
	manifestData, err := manifest.Encode(exported)
	if err != nil {
		return nil, fmt.Errorf("checker service: encode manifest %s: %w", input.ID, err)
	}
	items := diagnostics.ProduceJudgments(checked.RootResult(), input.EntryFile)
	diagnosticConfig := diagnostics.Config{
		Policy:   cloneDiagnosticPolicy(input.DiagnosticPolicy),
		Judgment: input.JudgmentPolicy,
	}
	rendered := diagnostics.RenderJudgments(items, diagnosticConfig)
	placement := placementplan.FromProgramResult(checked)
	bodies, bodyVersions := collectBodyResults(checked.RootResult())
	snapshot := checked.Snapshot()
	summaryDigests := digestSummaries(checked.RootResult(), snapshot)

	return &completedSnapshot{
		tag: ResultTag{
			UnitID:          input.ID,
			UnitDigest:      unit.digest,
			ManifestDigest:  digestBytes(manifestData),
			SourceDigests:   cloneMap(unit.sourceDigests),
			BodyVersions:    bodyVersions,
			Profile:         profile,
			DocumentVersion: documentVersion,
		},
		bodies:           bodies,
		judgments:        cloneJudgments(items),
		diagnostics:      cloneDiagnostics(rendered),
		manifestPath:     exported.Path,
		manifestData:     append([]byte(nil), manifestData...),
		placement:        clonePlacementPlan(placement),
		summaries:        snapshot,
		summaryDigests:   summaryDigests,
		diagnosticConfig: diagnosticConfig,
	}, nil
}

func orderedManifests(items map[string]*manifest.Manifest) []*manifest.Manifest {
	keys := sortedKeys(items)
	out := make([]*manifest.Manifest, 0, len(keys))
	for _, key := range keys {
		if items[key] != nil {
			out = append(out, items[key])
		}
	}
	return out
}

func manifestGlobalTypes(manifests []*manifest.Manifest) map[string]typ.Type {
	out := make(map[string]typ.Type)
	for _, item := range manifests {
		if item == nil {
			continue
		}
		for name, valueType := range item.GlobalTypes {
			if name != "" && valueType != nil {
				out[name] = valueType
			}
		}
	}
	return out
}

func collectBodyResults(root *body.Result) ([]BodyResultRef, map[BodyID]uint64) {
	var bodies []BodyResultRef
	versions := make(map[BodyID]uint64)
	var visit func(*body.Result, BodyID)
	visit = func(result *body.Result, id BodyID) {
		if result == nil {
			return
		}
		version := result.ResultVersion()
		bodies = append(bodies, BodyResultRef{ID: id, ResultVersion: version})
		versions[id] = version
		for index, child := range result.FunctionResults() {
			visit(child, BodyID(fmt.Sprintf("%s/%d", id, index)))
		}
	}
	visit(root, BodyID("root"))
	return bodies, versions
}

func digestSummaries(root *body.Result, snapshot summary.Snapshot) map[summary.SummaryKey]summary.Digest {
	if root == nil || root.Registry() == nil {
		return nil
	}
	entries := snapshot.Entries()
	if len(entries) == 0 {
		return nil
	}
	out := make(map[summary.SummaryKey]summary.Digest, len(entries))
	for _, entry := range entries {
		out[entry.Key] = summary.NormalizedPayloadDigest(root.Registry(), entry.Summary)
	}
	return out
}
