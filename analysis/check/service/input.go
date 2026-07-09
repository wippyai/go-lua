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
	sourceDigests map[string]Digest
}

func normalizeUnitInput(input UnitInput) (retainedUnit, error) {
	if input.ID == "" {
		return retainedUnit{}, errors.New("checker service: empty unit id")
	}
	if input.ModulePath == "" {
		input.ModulePath = string(input.ID)
	}
	if len(input.SourceFiles) == 0 {
		return retainedUnit{}, errors.New("checker service: unit has no source files")
	}
	input.SourceFiles = cloneSourceFiles(input.SourceFiles)
	if input.EntryFile == "" {
		paths := sortedKeys(input.SourceFiles)
		input.EntryFile = paths[0]
	}
	if _, ok := input.SourceFiles[input.EntryFile]; !ok {
		return retainedUnit{}, fmt.Errorf("checker service: entry file %q is not in source files", input.EntryFile)
	}

	manifests, err := cloneManifests(input.ExternalManifests)
	if err != nil {
		return retainedUnit{}, err
	}
	input.ExternalManifests = manifests
	input.Globals = normalizedStrings(input.Globals)
	input.GlobalTypes = cloneGlobalTypes(input.GlobalTypes)
	input.StateLanes = state.NewLaneSet(input.StateLanes...).IDs()
	input.DiagnosticPolicy = cloneDiagnosticPolicy(input.DiagnosticPolicy)

	sourceDigests := make(map[string]Digest, len(input.SourceFiles))
	for path, data := range input.SourceFiles {
		sourceDigests[path] = digestBytes(data)
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

func unitInputDigest(input UnitInput, sourceDigests map[string]Digest) (Digest, error) {
	w := newDigestWriter()
	w.string("checker-service-unit-v1")
	w.string(input.ModulePath)
	w.string(input.EntryFile)
	for _, path := range sortedKeys(sourceDigests) {
		w.string(path)
		digest := sourceDigests[path]
		w.bytes(digest[:])
	}
	for _, path := range sortedKeys(input.ExternalManifests) {
		w.string(path)
		data, err := manifest.Encode(input.ExternalManifests[path])
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
	writeDiagnosticPolicy(w, input.DiagnosticPolicy)
	writeJudgmentPolicy(w, input.JudgmentPolicy)
	return w.sum(), nil
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
