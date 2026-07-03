package lua

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	diag "github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestAssignmentJudgmentFixtureShadowAudit(t *testing.T) {
	if os.Getenv("ASSIGNMENT_JUDGMENT_SHADOW") == "" {
		t.Skip("set ASSIGNMENT_JUDGMENT_SHADOW=1 to compare legacy assignment diagnostics with assignment judgments")
	}
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	judgmentOpt := checktest.WithDiagnosticsConfig(diagnostics.Config{UseAssignmentJudgments: true})
	var diffs []assignmentShadowDiff
	for _, s := range suites {
		skip, _ := shouldSkipOracleSuite(s)
		if skip {
			continue
		}
		legacy, _ := fixtureDiagnostics(s)
		judgment, _ := fixtureDiagnosticsWithOptions(s, judgmentOpt)
		diffs = append(diffs, compareAssignmentDiagnostics(s.Name, legacy, judgment)...)
	}
	if len(diffs) == 0 {
		t.Log("assignment judgment shadow audit: no assignment-family deltas")
		return
	}
	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].Kind != diffs[j].Kind {
			return diffs[i].Kind < diffs[j].Kind
		}
		if diffs[i].Signature != diffs[j].Signature {
			return diffs[i].Signature < diffs[j].Signature
		}
		return diffs[i].Suite < diffs[j].Suite
	})
	if os.Getenv("ASSIGNMENT_JUDGMENT_SHADOW_DUMP") != "" {
		for _, diff := range diffs {
			t.Logf("delta kind=%s suite=%s signature=%s sample=%s", diff.Kind, diff.Suite, diff.Signature, diff.Sample)
		}
	}
	clusters := assignmentShadowClusters(diffs)
	t.Logf("assignment judgment shadow audit: %d deltas, %d clusters", len(diffs), len(clusters))
	for i, cluster := range clusters {
		if i >= 20 {
			t.Logf("... %d more clusters", len(clusters)-i)
			break
		}
		t.Logf("[%s] count=%d signature=%s sample=%s", cluster.Kind, cluster.Count, cluster.Signature, cluster.Sample)
	}
}

type assignmentShadowDiff struct {
	Kind      string
	Suite     string
	Signature string
	Sample    string
}

type assignmentShadowCluster struct {
	Kind      string
	Signature string
	Count     int
	Sample    string
}

func compareAssignmentDiagnostics(suite string, legacy, judgment []diag.Diagnostic) []assignmentShadowDiff {
	left := assignmentDiagnosticGroups(legacy)
	right := assignmentDiagnosticGroups(judgment)
	var out []assignmentShadowDiff
	for key, leftItems := range left {
		rightItems := right[key]
		paired := len(leftItems)
		if len(rightItems) < paired {
			paired = len(rightItems)
		}
		for i := 0; i < paired; i++ {
			if leftItems[i].Message == rightItems[i].Message {
				continue
			}
			out = append(out, assignmentShadowDiff{
				Kind:      "changed",
				Suite:     suite,
				Signature: assignmentShadowChangedClusterSignature(leftItems[i], rightItems[i]),
				Sample:    assignmentShadowChangedSample(key, leftItems[i], rightItems[i]),
			})
		}
		if len(leftItems) <= len(rightItems) {
			continue
		}
		for _, item := range leftItems[len(rightItems):] {
			out = append(out, assignmentShadowDiff{
				Kind:      "legacy_only",
				Suite:     suite,
				Signature: assignmentShadowMessageKind(item.Message),
				Sample:    key + "|" + item.Message,
			})
		}
	}
	for key, rightItems := range right {
		leftItems := left[key]
		if len(rightItems) <= len(leftItems) {
			continue
		}
		for _, item := range rightItems[len(leftItems):] {
			out = append(out, assignmentShadowDiff{
				Kind:      "judgment_only",
				Suite:     suite,
				Signature: assignmentShadowMessageKind(item.Message),
				Sample:    key + "|" + item.Message,
			})
		}
	}
	return out
}

func assignmentDiagnosticGroups(diags []diag.Diagnostic) map[string][]diag.Diagnostic {
	out := make(map[string][]diag.Diagnostic)
	for _, d := range diags {
		if d.Code != diagnostics.CodeAssignmentType && d.Code != diagnostics.CodeOptionalAssignmentTarget {
			continue
		}
		key := assignmentShadowLocationKey(d)
		out[key] = append(out[key], d)
	}
	for key := range out {
		sort.Slice(out[key], func(i, j int) bool {
			return out[key][i].Message < out[key][j].Message
		})
	}
	return out
}

func assignmentShadowLocationKey(d diag.Diagnostic) string {
	return fmt.Sprintf("%s|%s|%d:%d-%d:%d|%s",
		d.Code,
		d.Severity,
		d.Span.StartLine,
		d.Span.StartCol,
		d.Span.EndLine,
		d.Span.EndCol,
		d.Position.File,
	)
}

func assignmentShadowChangedSample(key string, legacy, judgment diag.Diagnostic) string {
	return fmt.Sprintf("%s|legacy=%s|judgment=%s", key, legacy.Message, judgment.Message)
}

func assignmentShadowChangedClusterSignature(legacy, judgment diag.Diagnostic) string {
	return assignmentShadowMessageKind(legacy.Message) + "->" + assignmentShadowMessageKind(judgment.Message)
}

func assignmentShadowMessageKind(message string) string {
	if strings.Contains(message, "may be nil") {
		return "may-be-nil"
	}
	if strings.Contains(message, "any") {
		return "any-boundary"
	}
	if strings.Contains(message, "missing required field") || strings.Contains(message, "does not provide") {
		return "missing-required-field"
	}
	if strings.Contains(message, "cannot assign") {
		return "type-mismatch"
	}
	return message
}

func assignmentShadowClusters(diffs []assignmentShadowDiff) []assignmentShadowCluster {
	byKey := make(map[string]*assignmentShadowCluster)
	for _, diff := range diffs {
		key := diff.Kind + "|" + diff.Signature
		cluster := byKey[key]
		if cluster == nil {
			cluster = &assignmentShadowCluster{Kind: diff.Kind, Signature: diff.Signature, Sample: diff.Suite + ": " + diff.Sample}
			byKey[key] = cluster
		}
		cluster.Count++
	}
	out := make([]assignmentShadowCluster, 0, len(byKey))
	for _, cluster := range byKey {
		out = append(out, *cluster)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Signature < out[j].Signature
	})
	return out
}
