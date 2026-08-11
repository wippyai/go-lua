package verify

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
)

// Verify checks parseability unconditionally, then exactly the requested
// gate dispositions. It intentionally cannot call a semantic verifier.
func Verify(request Request) (Report, error) {
	pre, err := validateSnapshot(request.Before, "pre-cut")
	if err != nil {
		return Report{}, err
	}
	post, err := validateSnapshot(request.After, "post-cut")
	if err != nil {
		return Report{}, err
	}
	postDigest, err := snapshotDigest(post)
	if err != nil {
		return Report{}, err
	}
	// A declared import route is a lock invariant, not semantic evidence. It
	// must describe the complete graph delta even when import-dag was not an
	// explicitly requested reporting gate.
	deltas, graph, err := verifyImports(pre, post, request.Imports)
	if err != nil {
		return Report{}, fmt.Errorf("structural import verification: %w", err)
	}
	report, err := verifyGates(request, post, postDigest)
	if err != nil {
		return Report{}, fmt.Errorf("structural verification: %w", err)
	}
	report.ImportDeltas = deltas
	report.ImportGraph = graph
	report.PostDigest = postDigest
	digest, err := reportDigest(report)
	if err != nil {
		return Report{}, err
	}
	report.Digest = digest
	return report, nil
}

func reportDigest(report Report) (string, error) {
	canonical := struct {
		Executed     []cutplan.Gate
		ImportDeltas []ImportDelta
		ImportGraph  []ImportEdge
		PostDigest   string
		Evidence     []GateEvidence
	}{report.Executed, report.ImportDeltas, report.ImportGraph, report.PostDigest, report.Evidence}
	bytes, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("canonical structural report: %w", err)
	}
	digest := sha256.Sum256(bytes)
	return hex.EncodeToString(digest[:]), nil
}

func snapshotDigest(files map[string]SourceFile) (string, error) {
	type sourceDigest struct {
		Path    string
		Package string
		SHA256  string
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	canonical := make([]sourceDigest, 0, len(paths))
	for _, path := range paths {
		file := files[path]
		sum := sha256.Sum256(file.Source)
		canonical = append(canonical, sourceDigest{Path: path, Package: file.Package, SHA256: hex.EncodeToString(sum[:])})
	}
	bytes, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("canonical post-source digest: %w", err)
	}
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:]), nil
}
