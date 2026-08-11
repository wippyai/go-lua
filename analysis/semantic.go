// Package analysis owns the sole production composition and Program-body
// compiler for the symbolic analyzer.
package analysis

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
)

const semanticFormat uint64 = 1

// semanticBundle is the one Link-scoped cold vocabulary: five Factors,
// sixteen Rule schemas (fifteen factor-output rules plus one structural
// activation), and two Query schemas. Occurrence identities are issued
// directly while the canonical source traversal prepares their instances.
type semanticBundle struct {
	valueFactor, valueSummary, callFactor, heapFactor, packFactor, effectFactor, effectSummary engine.SemanticKey
	valueQuery, valueCodec, effectQuery, effectCodec                                           engine.SemanticKey
	callActivation, callActivationFamily, callActivationAdmission                              engine.SemanticKey

	valueSourceRule, packSourceRule, heapIngressRule, rawGetRule, rawSetRule, callDispatchRule  ruleSemantics
	effectSelectedRule, effectOpaqueRule, effectBodyRule, valueBootstrapRule, heapBootstrapRule ruleSemantics
	valueTransferRule                                                                           ruleSemantics
	valueAllocationRule, heapEmptyRule, heapClosedRule                                          transformedRuleSemantics
}

type ruleSemantics struct {
	rule, operand, evidence engine.SemanticKey
}

type transformedRuleSemantics struct {
	ruleSemantics
	transform engine.SemanticKey
}

func newSemanticBundle(linkID keyspace.ContentID) (semanticBundle, bool) {
	if !linkID.Available() {
		return semanticBundle{}, false
	}
	key := func(role string) engine.SemanticKey { value, _ := analysisSemanticKey(linkID, role); return value }
	rule := func(role string) ruleSemantics {
		return ruleSemantics{rule: key("rule/" + role), operand: key("operand/" + role), evidence: key("evidence/" + role)}
	}
	transformed := func(role string) transformedRuleSemantics {
		return transformedRuleSemantics{ruleSemantics: rule(role), transform: key("transform/" + role)}
	}
	result := semanticBundle{
		valueFactor: key("factor/value"), valueSummary: key("factor/value/summary-identity"),
		callFactor: key("factor/call"), heapFactor: key("factor/heap"), packFactor: key("factor/pack"),
		effectFactor: key("factor/effect"), effectSummary: key("factor/effect/summary-identity"),
		valueQuery: key("query/value-summary"), valueCodec: key("query-result/value-summary"),
		effectQuery: key("query/effect-exact"), effectCodec: key("query-result/effect-exact"),
		callActivation: key("activation/call-body"), callActivationFamily: key("activation-family/call-body"), callActivationAdmission: key("activation-admission/call-body"),
		valueSourceRule: rule("value/source"), packSourceRule: rule("pack/source"), heapIngressRule: rule("heap/allocation-ingress"),
		valueAllocationRule: transformed("value/allocation"), heapEmptyRule: transformed("heap/allocation-empty"), heapClosedRule: transformed("heap/allocation-closed"),
		rawGetRule: rule("heap/index-get-raw"), rawSetRule: rule("heap/index-set-raw"), callDispatchRule: rule("call/dispatch"),
		effectSelectedRule: rule("effect/callsite-selected"), effectOpaqueRule: rule("effect/callsite-opaque"), effectBodyRule: rule("effect/callsite-body"),
		valueBootstrapRule: rule("value/host-global-bootstrap"), heapBootstrapRule: rule("heap/host-bootstrap"), valueTransferRule: rule("value/storage-transfer"),
	}
	return result, result.available()
}

func (bundle semanticBundle) available() bool {
	keys := [...]engine.SemanticKey{
		bundle.valueFactor, bundle.valueSummary, bundle.callFactor, bundle.heapFactor, bundle.packFactor, bundle.effectFactor, bundle.effectSummary,
		bundle.valueQuery, bundle.valueCodec, bundle.effectQuery, bundle.effectCodec,
		bundle.callActivation, bundle.callActivationFamily, bundle.callActivationAdmission,
		bundle.valueSourceRule.rule, bundle.valueSourceRule.operand, bundle.valueSourceRule.evidence,
		bundle.packSourceRule.rule, bundle.packSourceRule.operand, bundle.packSourceRule.evidence,
		bundle.heapIngressRule.rule, bundle.heapIngressRule.operand, bundle.heapIngressRule.evidence,
		bundle.valueAllocationRule.rule, bundle.valueAllocationRule.operand, bundle.valueAllocationRule.evidence, bundle.valueAllocationRule.transform,
		bundle.heapClosedRule.rule, bundle.heapClosedRule.operand, bundle.heapClosedRule.evidence, bundle.heapClosedRule.transform,
		bundle.heapEmptyRule.rule, bundle.heapEmptyRule.operand, bundle.heapEmptyRule.evidence, bundle.heapEmptyRule.transform,
		bundle.rawGetRule.rule, bundle.rawGetRule.operand, bundle.rawGetRule.evidence,
		bundle.rawSetRule.rule, bundle.rawSetRule.operand, bundle.rawSetRule.evidence,
		bundle.callDispatchRule.rule, bundle.callDispatchRule.operand, bundle.callDispatchRule.evidence,
		bundle.effectSelectedRule.rule, bundle.effectSelectedRule.operand, bundle.effectSelectedRule.evidence,
		bundle.effectOpaqueRule.rule, bundle.effectOpaqueRule.operand, bundle.effectOpaqueRule.evidence,
		bundle.effectBodyRule.rule, bundle.effectBodyRule.operand, bundle.effectBodyRule.evidence,
		bundle.valueBootstrapRule.rule, bundle.valueBootstrapRule.operand, bundle.valueBootstrapRule.evidence,
		bundle.heapBootstrapRule.rule, bundle.heapBootstrapRule.operand, bundle.heapBootstrapRule.evidence,
		bundle.valueTransferRule.rule, bundle.valueTransferRule.operand, bundle.valueTransferRule.evidence,
	}
	return distinctSemanticKeys(keys[:])
}

func distinctSemanticKeys(keys []engine.SemanticKey) bool {
	for index, key := range keys {
		if !key.Available() {
			return false
		}
		for prior := 0; prior < index; prior++ {
			if keys[prior].Digest() == key.Digest() && keys[prior].Version() == key.Version() {
				return false
			}
		}
	}
	return true
}

func analysisSemanticKey(linkID keyspace.ContentID, role string) (engine.SemanticKey, bool) {
	return analysisSemanticKeyParts(linkID, role)
}

func analysisSemanticKeyParts(linkID keyspace.ContentID, role string, extra ...[]byte) (engine.SemanticKey, bool) {
	if !linkID.Available() || role == "" {
		return engine.SemanticKey{}, false
	}
	hash := sha256.New()
	var version [8]byte
	binary.BigEndian.PutUint64(version[:], semanticFormat)
	if !writeFramedHash(hash, []byte("analysis/semantic")) || !writeFramedHash(hash, version[:]) || !writeFramedHash(hash, linkID[:]) || !writeFramedHash(hash, []byte(role)) {
		return engine.SemanticKey{}, false
	}
	for _, value := range extra {
		if !writeFramedHash(hash, value) {
			return engine.SemanticKey{}, false
		}
	}
	var digest [32]byte
	if sum := hash.Sum(digest[:0]); len(sum) != len(digest) {
		return engine.SemanticKey{}, false
	}
	return engine.NewSemanticKey(digest, semanticFormat)
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
