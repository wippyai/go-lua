package service

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/embedding"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

var (
	ErrUnitNotFound   = errors.New("checker service: unit not found")
	ErrResultNotFound = errors.New("checker service: completed result not found")
)

type retainedUnit struct {
	input         UnitInput
	digest        Digest
	sourceDigests map[embedding.DocumentID]Digest
	// generation changes on every accepted upsert. It prevents an outside-lock
	// solve from publishing against a unit snapshot replaced underneath it.
	generation uint64
}

func normalizeUnitInput(input UnitInput) (retainedUnit, error) {
	if input.ID == "" {
		return retainedUnit{}, errors.New("checker service: empty unit id")
	}
	if input.ModulePath == "" {
		input.ModulePath = string(input.ID)
	}
	sources, labels, entry, err := materializeSources(input)
	if err != nil {
		return retainedUnit{}, err
	}
	input.Sources = sources
	input.DocumentLabels = labels
	input.EntryDocument = entry
	// The legacy raw-file fields are accepted only at the boundary. Retained
	// solve inputs are document-keyed and cannot accidentally use labels as
	// semantic source identity.
	input.EntryFile = ""
	input.SourceFiles = nil

	manifests, err := cloneManifests(input.ExternalManifests)
	if err != nil {
		return retainedUnit{}, err
	}
	input.ExternalManifests = manifests
	input.Globals = normalizedStrings(input.Globals)
	input.GlobalTypes = cloneGlobalTypes(input.GlobalTypes)
	// Nil selects the default State lanes; a non-nil empty slice deliberately
	// disables every lane. Preserve that distinction while retaining input.
	input.StateLanes = state.CloneLanes(input.StateLanes)
	input.DiagnosticPolicy = cloneDiagnosticPolicy(input.DiagnosticPolicy)

	sourceDigests := make(map[embedding.DocumentID]Digest, len(input.Sources))
	for document, snapshot := range input.Sources {
		sourceDigests[document] = snapshot.ContentDigest
	}
	digest, err := unitInputDigest(input, sourceDigests)
	if err != nil {
		return retainedUnit{}, err
	}
	return retainedUnit{
		input:         input,
		digest:        digest,
		sourceDigests: sourceDigests,
	}, nil
}

func materializeSources(input UnitInput) (map[embedding.DocumentID]embedding.SourceSnapshot, embedding.StaticLabels, embedding.DocumentID, error) {
	if len(input.Sources) != 0 && len(input.SourceFiles) != 0 {
		return nil, nil, embedding.DocumentID{}, errors.New("checker service: provide Sources or deprecated SourceFiles, not both")
	}
	sources := make(map[embedding.DocumentID]embedding.SourceSnapshot)
	labels := make(embedding.StaticLabels)
	for document, label := range input.DocumentLabels {
		if !document.Valid() {
			return nil, nil, embedding.DocumentID{}, errors.New("checker service: label has invalid document id")
		}
		labels[document] = label
	}
	if len(input.Sources) != 0 {
		for document, snapshot := range input.Sources {
			if !document.Valid() {
				return nil, nil, embedding.DocumentID{}, errors.New("checker service: source has invalid document id")
			}
			if snapshot.Document != document {
				return nil, nil, embedding.DocumentID{}, fmt.Errorf("checker service: source snapshot document %q does not match map key %q", snapshot.Document, document)
			}
			verified, err := snapshot.Verify()
			if err != nil {
				return nil, nil, embedding.DocumentID{}, err
			}
			sources[document] = verified
			if _, ok := labels[document]; !ok {
				labels[document] = embedding.DefaultDocumentLabel(document)
			}
		}
	} else {
		for path, data := range input.SourceFiles {
			document := embedding.FileDocument(path)
			verified, err := (embedding.SourceSnapshot{Document: document, Content: data}).Verify()
			if err != nil {
				return nil, nil, embedding.DocumentID{}, err
			}
			sources[document] = verified
			labels[document] = path
		}
	}
	if len(sources) == 0 {
		return nil, nil, embedding.DocumentID{}, errors.New("checker service: unit has no sources")
	}
	entry := input.EntryDocument
	if !entry.Valid() && input.Plan.Entry.Valid() {
		entry = input.Plan.Entry
	}
	if !entry.Valid() && input.EntryFile != "" {
		entry = embedding.FileDocument(input.EntryFile)
	}
	if !entry.Valid() {
		entry = embedding.SortedDocuments(sources)[0]
	}
	if _, ok := sources[entry]; !ok {
		return nil, nil, embedding.DocumentID{}, fmt.Errorf("checker service: entry document %q is not in sources", entry)
	}
	if input.Plan.ID != "" && input.Plan.ID != embedding.UnitID(input.ID) {
		return nil, nil, embedding.DocumentID{}, fmt.Errorf("checker service: unit plan id %q does not match input id %q", input.Plan.ID, input.ID)
	}
	if input.Plan.Entry.Valid() && input.Plan.Entry != entry {
		return nil, nil, embedding.DocumentID{}, fmt.Errorf("checker service: unit plan entry %q does not match input entry %q", input.Plan.Entry, entry)
	}
	for _, document := range input.Plan.Sources {
		if _, ok := sources[document]; !ok {
			return nil, nil, embedding.DocumentID{}, fmt.Errorf("checker service: unit plan source %q is not materialized", document)
		}
	}
	return sources, labels, entry, nil
}

func unitInputDigest(input UnitInput, sourceDigests map[embedding.DocumentID]Digest) (Digest, error) {
	w := newDigestWriter()
	w.string("checker-service-unit-v2")
	w.string(input.ModulePath)
	writeDocumentID(w, input.EntryDocument)
	for _, document := range sortedDocumentDigests(sourceDigests) {
		writeDocumentID(w, document)
		digest := sourceDigests[document]
		w.bytes(digest[:])
	}
	w.bytes(input.ResolutionDigest[:])
	writeUnitPlan(w, input.Plan)
	for _, path := range sortedKeys(input.ExternalManifests) {
		w.string(path)
		// Unit digests only need canonical content. Avoid paying to indent large
		// recursive manifest graphs for this internal cache key; Encode remains
		// the stable presentation format at the module boundary.
		data, err := manifest.EncodeCompact(input.ExternalManifests[path])
		if err != nil {
			return Digest{}, fmt.Errorf("checker service: digest external manifest %q: %w", path, err)
		}
		w.bytes(data)
	}
	w.bool(input.IncludeStdlib)
	for _, name := range input.Globals {
		w.string(name)
	}
	for _, name := range sortedKeys(input.GlobalTypes) {
		w.string(name)
		t := input.GlobalTypes[name]
		if t == nil {
			w.uint64(0)
			w.string("")
			continue
		}
		w.uint64(typ.EqualityHash(t))
		w.string(t.String())
	}
	for _, lane := range input.StateLanes {
		w.string(string(lane))
	}
	w.uint64(uint64(input.Schedule))
	writeDiagnosticPolicy(w, input.DiagnosticPolicy)
	writeJudgmentPolicy(w, input.JudgmentPolicy)
	return w.sum(), nil
}

func writeDocumentID(w *digestWriter, document embedding.DocumentID) {
	w.string(string(document.Scheme))
	w.string(document.OpaqueKey)
}

func sortedDocumentDigests(items map[embedding.DocumentID]Digest) []embedding.DocumentID {
	keys := make([]embedding.DocumentID, 0, len(items))
	for document := range items {
		keys = append(keys, document)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Scheme != keys[j].Scheme {
			return keys[i].Scheme < keys[j].Scheme
		}
		return keys[i].OpaqueKey < keys[j].OpaqueKey
	})
	return keys
}

func writeUnitPlan(w *digestWriter, plan embedding.UnitPlan) {
	w.string(string(plan.ID))
	w.string(plan.ModulePath)
	writeDocumentID(w, plan.Entry)
	sources := append([]embedding.DocumentID(nil), plan.Sources...)
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Scheme != sources[j].Scheme {
			return sources[i].Scheme < sources[j].Scheme
		}
		return sources[i].OpaqueKey < sources[j].OpaqueKey
	})
	for _, document := range sources {
		writeDocumentID(w, document)
	}
	imports := append([]embedding.UnitImport(nil), plan.Imports...)
	sort.Slice(imports, func(i, j int) bool {
		if imports[i].Alias != imports[j].Alias {
			return imports[i].Alias < imports[j].Alias
		}
		if imports[i].TargetUnit != imports[j].TargetUnit {
			return imports[i].TargetUnit < imports[j].TargetUnit
		}
		return imports[i].ManifestDigest.String() < imports[j].ManifestDigest.String()
	})
	for _, imported := range imports {
		w.string(imported.Alias)
		w.string(string(imported.TargetUnit))
		w.bytes(imported.ManifestDigest[:])
	}
}

func writeDiagnosticPolicy(w *digestWriter, policy diagnostic.Policy) {
	for _, code := range sortedKeys(policy.Rules) {
		rule := policy.Rules[code]
		w.string(string(code))
		w.bool(rule.Disabled)
		w.bool(rule.Enabled)
		w.bool(rule.HasEnabled)
		w.uint64(uint64(rule.Severity))
		w.bool(rule.HasSeverity)
	}
}

func writeJudgmentPolicy(w *digestWriter, config judgment.PolicyConfig) {
	w.string(string(config.Strictness))
	policy := config.Policy
	for _, code := range judgment.DefaultRegistry().Codes() {
		for _, verdict := range []judgment.Verdict{judgment.VerdictUnknown, judgment.VerdictProven, judgment.VerdictRefuted} {
			for _, mode := range []judgment.StrictnessMode{judgment.StrictnessDefault, judgment.StrictnessLenient, judgment.StrictnessStrict} {
				w.string(string(code))
				w.uint64(uint64(verdict))
				w.string(string(mode))
				level, ok := policy.LevelFor(judgment.Judgment{Code: code, Verdict: verdict}, mode)
				w.bool(ok)
				w.uint64(uint64(level))
			}
		}
	}
}

func cloneSourceFiles(in map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(in))
	for path, data := range in {
		out[path] = append([]byte(nil), data...)
	}
	return out
}

func cloneManifests(in map[string]*manifest.Manifest) (map[string]*manifest.Manifest, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make(map[string]*manifest.Manifest, len(in))
	for path, item := range in {
		if path == "" || item == nil {
			return nil, errors.New("checker service: external manifest has empty path or nil value")
		}
		data, err := manifest.Encode(item)
		if err != nil {
			return nil, fmt.Errorf("checker service: encode external manifest %q: %w", path, err)
		}
		cloned, err := manifest.Decode(data)
		if err != nil {
			return nil, fmt.Errorf("checker service: clone external manifest %q: %w", path, err)
		}
		out[path] = cloned
	}
	return out, nil
}

func cloneGlobalTypes(in map[string]typ.Type) map[string]typ.Type {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]typ.Type, len(in))
	for name, t := range in {
		out[name] = t
	}
	return out
}

func cloneDiagnosticPolicy(policy diagnostic.Policy) diagnostic.Policy {
	if len(policy.Rules) == 0 {
		return diagnostic.Policy{}
	}
	out := diagnostic.Policy{Rules: make(map[diagnostic.Code]diagnostic.Rule, len(policy.Rules))}
	for code, rule := range policy.Rules {
		out.Rules[code] = rule
	}
	return out
}

func normalizedStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, item := range in {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func sortedKeys[K ~string, V any](items map[K]V) []K {
	keys := make([]K, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

type digestWriter struct {
	h hash.Hash
}

func newDigestWriter() *digestWriter { return &digestWriter{h: sha256.New()} }

func (w *digestWriter) uint64(value uint64) {
	var data [8]byte
	binary.BigEndian.PutUint64(data[:], value)
	_, _ = w.h.Write(data[:])
}

func (w *digestWriter) bytes(value []byte) {
	w.uint64(uint64(len(value)))
	_, _ = w.h.Write(value)
}

func (w *digestWriter) string(value string) { w.bytes([]byte(value)) }

func (w *digestWriter) bool(value bool) {
	if value {
		w.uint64(1)
		return
	}
	w.uint64(0)
}

func (w *digestWriter) sum() Digest {
	var out Digest
	copy(out[:], w.h.Sum(nil))
	return out
}
