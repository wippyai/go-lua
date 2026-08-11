package semantic

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
)

// VerifyExpected compares a replay load to the exact ObjectEvidence committed
// in a lock. Requests only name objects and roles; positions and site sets are
// generated once by preflight and verified here without inference.
func VerifyExpected(expected, actual []cutplan.ObjectEvidence) error {
	want, err := exactEvidence(expected)
	if err != nil {
		return fmt.Errorf("expected evidence: %w", err)
	}
	got, err := exactEvidence(actual)
	if err != nil {
		return fmt.Errorf("actual evidence: %w", err)
	}
	if len(want) != len(got) {
		return fmt.Errorf("object evidence denominator changed: expected=%d actual=%d", len(want), len(got))
	}
	for key, evidence := range want {
		current, found := got[key]
		if !found {
			return fmt.Errorf("expected object evidence missing: %s", key)
		}
		if evidence.Package != current.Package || !samePosition(evidence.Definition, current.Definition) || !samePositionSet(evidence.References, current.References) {
			return fmt.Errorf("object evidence changed: %s", key)
		}
	}
	return nil
}

func exactEvidence(values []cutplan.ObjectEvidence) (map[string]cutplan.ObjectEvidence, error) {
	result := make(map[string]cutplan.ObjectEvidence, len(values))
	for _, value := range values {
		if value.Object.Object == "" || (value.Role != cutplan.ObjectSource && value.Role != cutplan.ObjectTarget) || value.Package == "" || value.Definition.Path == "" || len(value.Definition.PackageIDs) == 0 || value.Definition.Offset < 0 || value.Definition.Line < 1 || value.Definition.Column < 1 || value.Definition.Role != cutplan.SiteDeclaration {
			return nil, fmt.Errorf("invalid object evidence")
		}
		for _, reference := range value.References {
			if len(reference.PackageIDs) == 0 || reference.Path == "" || reference.Offset < 0 || reference.Line < 1 || reference.Column < 1 || reference.Role == cutplan.SiteDeclaration {
				return nil, fmt.Errorf("invalid object reference: %s", value.Object.Object)
			}
		}
		if _, err := uniquePositions(value.References); err != nil {
			return nil, err
		}
		key := string(value.Role) + "\x00" + value.Object.Object
		if _, duplicate := result[key]; duplicate {
			return nil, fmt.Errorf("duplicate object evidence: %s", key)
		}
		result[key] = value
	}
	return result, nil
}

func samePosition(left, right cutplan.Position) bool { return positionKey(left) == positionKey(right) }

func samePositionSet(left, right []cutplan.Position) bool {
	if len(left) != len(right) {
		return false
	}
	seen := positionSet(left)
	if len(seen) != len(left) {
		return false
	}
	for _, value := range right {
		if !seen[positionKey(value)] {
			return false
		}
	}
	return true
}

// VerifyDiagnosticDelta calculates the normalized post-state difference and
// rejects any change not explicitly approved. Passing nil allowances means
// diagnostics must be byte-for-byte stable at their semantic source sites.
func VerifyDiagnosticDelta(before, after []Diagnostic, allowAdded, allowRemoved []Diagnostic) (DiagnosticDelta, error) {
	beforeSet, err := diagnosticSet(before)
	if err != nil {
		return DiagnosticDelta{}, fmt.Errorf("baseline diagnostics: %w", err)
	}
	afterSet, err := diagnosticSet(after)
	if err != nil {
		return DiagnosticDelta{}, fmt.Errorf("post diagnostics: %w", err)
	}
	allowedAdded, err := diagnosticSet(allowAdded)
	if err != nil {
		return DiagnosticDelta{}, fmt.Errorf("allowed added diagnostics: %w", err)
	}
	allowedRemoved, err := diagnosticSet(allowRemoved)
	if err != nil {
		return DiagnosticDelta{}, fmt.Errorf("allowed removed diagnostics: %w", err)
	}
	delta := DiagnosticDelta{}
	for key, diagnostic := range afterSet {
		if _, existed := beforeSet[key]; !existed {
			delta.Added = append(delta.Added, diagnostic)
			if _, allowed := allowedAdded[key]; !allowed {
				return DiagnosticDelta{}, fmt.Errorf("unapproved added diagnostic: %s", diagnosticKey(diagnostic))
			}
		}
	}
	for key, diagnostic := range beforeSet {
		if _, survived := afterSet[key]; !survived {
			delta.Removed = append(delta.Removed, diagnostic)
			if _, allowed := allowedRemoved[key]; !allowed {
				return DiagnosticDelta{}, fmt.Errorf("unapproved removed diagnostic: %s", diagnosticKey(diagnostic))
			}
		}
	}
	delta.Added = canonicalDiagnostics(delta.Added)
	delta.Removed = canonicalDiagnostics(delta.Removed)
	return delta, nil
}

func positionSet(positions []cutplan.Position) map[string]bool {
	result := make(map[string]bool, len(positions))
	for _, position := range positions {
		result[positionKey(position)] = true
	}
	return result
}

func diagnosticSet(values []Diagnostic) (map[string]Diagnostic, error) {
	result := make(map[string]Diagnostic, len(values))
	for _, value := range values {
		if value.Position.Path == "" || value.Position.Line < 1 || value.Position.Column < 1 || value.Message == "" || value.Kind == "" {
			return nil, fmt.Errorf("invalid diagnostic")
		}
		key := diagnosticKey(value)
		if _, duplicate := result[key]; duplicate {
			return nil, fmt.Errorf("duplicate diagnostic: %s", key)
		}
		result[key] = value
	}
	return result, nil
}

func diagnosticKey(value Diagnostic) string {
	return positionKey(value.Position) + "\x00" + value.Kind + "\x00" + value.Message
}

// SortedDiagnosticDelta is a display helper that keeps report formatting out
// of the verifier itself.
func SortedDiagnosticDelta(delta DiagnosticDelta) DiagnosticDelta {
	delta.Added = append([]Diagnostic(nil), delta.Added...)
	delta.Removed = append([]Diagnostic(nil), delta.Removed...)
	sort.Slice(delta.Added, func(i, j int) bool { return diagnosticKey(delta.Added[i]) < diagnosticKey(delta.Added[j]) })
	sort.Slice(delta.Removed, func(i, j int) bool { return diagnosticKey(delta.Removed[i]) < diagnosticKey(delta.Removed[j]) })
	return delta
}

// Residues evaluates an exact batch of post-cut absence assertions through the
// same structured workspace that resolved the lock evidence.
func (snapshot Snapshot) Residues(queries []ResidueQuery) ([]ObjectResidue, error) {
	if snapshot.Workspace == nil {
		return nil, fmt.Errorf("residue query requires semantic workspace")
	}
	result := make([]ObjectResidue, 0, len(queries))
	seen := map[string]bool{}
	for _, query := range queries {
		if query.Object.Object == "" {
			return nil, fmt.Errorf("residue object is empty")
		}
		paths := append([]string(nil), query.Paths...)
		sort.Strings(paths)
		key := query.Object.Object + "\x00" + strings.Join(paths, "\x00")
		if seen[key] {
			return nil, fmt.Errorf("duplicate residue query: %s", query.Object.Object)
		}
		seen[key] = true
		residue, err := snapshot.Workspace.ObjectResidue(query.Object, paths)
		if err != nil {
			return nil, err
		}
		result = append(result, residue)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Object.Object < result[j].Object.Object })
	return result, nil
}
