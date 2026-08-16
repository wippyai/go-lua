// Package analysis owns the sole production composition and Program-body
// compiler for the symbolic analyzer.
package analysis

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/internal/semanticvocabulary"
	"github.com/wippyai/go-lua/program/keyspace"
)

// newSemanticBundle preserves the construction seam used by analysis while
// returning the canonical global vocabulary directly.  The vocabulary
// package owns the complete factor/rule/query inventory and its availability
// check; analysis only retains its content-ID helpers below.
func newSemanticBundle() (semanticvocabulary.Bundle, bool) {
	return semanticvocabulary.New()
}

// classifyDiagnosticRule projects one existing Engine Rule key into the
// closed analyzer vocabulary. It does not retain or decode the key and is
// used only while returning a same-run diagnostic snapshot.
func classifyDiagnosticRule(bundle semanticvocabulary.Bundle, key engine.SemanticKey) AnalyzeDiagnosticRule {
	for _, candidate := range [...]struct {
		key  engine.SemanticKey
		rule AnalyzeDiagnosticRule
	}{
		{bundle.ValueSourceRule.Rule, AnalyzeDiagnosticRuleValueSource},
		{bundle.PackSourceRule.Rule, AnalyzeDiagnosticRulePackSource},
		{bundle.HeapIngressRule.Rule, AnalyzeDiagnosticRuleHeapIngress},
		{bundle.ValueAllocationRule.Rule, AnalyzeDiagnosticRuleValueAllocation},
		{bundle.HeapEmptyRule.Rule, AnalyzeDiagnosticRuleHeapEmpty},
		{bundle.HeapClosedRule.Rule, AnalyzeDiagnosticRuleHeapClosed},
		{bundle.RawGetRule.Rule, AnalyzeDiagnosticRuleRawGet},
		{bundle.RawSetRule.Rule, AnalyzeDiagnosticRuleRawSet},
		{bundle.CallDispatchRule.Rule, AnalyzeDiagnosticRuleCallDispatch},
		{bundle.EffectSelectedRule.Rule, AnalyzeDiagnosticRuleEffectSelected},
		{bundle.EffectOpaqueRule.Rule, AnalyzeDiagnosticRuleEffectOpaque},
		{bundle.EffectBodyRule.Rule, AnalyzeDiagnosticRuleEffectBody},
		{bundle.CallActivation, AnalyzeDiagnosticRuleCallActivation},
		{bundle.ValueBootstrapRule.Rule, AnalyzeDiagnosticRuleValueBootstrap},
		{bundle.HeapBootstrapRule.Rule, AnalyzeDiagnosticRuleHeapBootstrap},
		{bundle.ValueTransferRule.Rule, AnalyzeDiagnosticRuleValueTransfer},
		{bundle.ValueBinaryArithmeticRule.Rule, AnalyzeDiagnosticRuleValueBinaryArithmetic},
		{bundle.ValueBinaryEqualityRule.Rule, AnalyzeDiagnosticRuleValueBinaryEquality},
		{bundle.ValueBinaryOrderRule.Rule, AnalyzeDiagnosticRuleValueBinaryOrder},
		{bundle.ValuePresenceRefinementRule.Rule, AnalyzeDiagnosticRuleValuePresenceRefinement},
	} {
		if candidate.key == key {
			return candidate.rule
		}
	}
	return AnalyzeDiagnosticRuleUnknown
}

func analysisSemanticKey(role string) (engine.SemanticKey, bool) {
	return semanticvocabulary.Key(role)
}

func analysisContentID(role string, parts ...[]byte) (keyspace.ContentID, bool) {
	if role == "" {
		return keyspace.ContentID{}, false
	}
	hash := sha256.New()
	if !writeFramedHash(hash, []byte(role)) {
		return keyspace.ContentID{}, false
	}
	for _, part := range parts {
		if !writeFramedHash(hash, part) {
			return keyspace.ContentID{}, false
		}
	}
	var id keyspace.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

func writeFramedHash(hash interface{ Write([]byte) (int, error) }, value []byte) bool {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	first, firstErr := hash.Write(size[:])
	second, secondErr := hash.Write(value)
	return firstErr == nil && secondErr == nil && first == len(size) && second == len(value)
}
