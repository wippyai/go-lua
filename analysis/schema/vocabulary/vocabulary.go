// Package vocabulary owns the closed, process-independent semantic
// vocabulary used by the analysis engine.  It deliberately contains no
// mounted-program, Link, or runtime identity: those belong to the program and
// binding layers.
package vocabulary

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
)

// SemanticFormat is the version of the global semantic vocabulary.  Changing
// the role list, framing domain, or interpretation of a role requires bumping
// this value.
const SemanticFormat uint64 = 6

// RuleSemantics is the closed identity tuple for one rule: its rule identity,
// operand form, and evidence form.
type RuleSemantics struct {
	Rule     identity.SemanticKey
	Operand  identity.SemanticKey
	Evidence identity.SemanticKey
}

// TransformedRuleSemantics adds the transform form used by rules whose output
// is normalized before admission.
type TransformedRuleSemantics struct {
	RuleSemantics
	Transform identity.SemanticKey
}

// Bundle is the complete global cold vocabulary: factor identities, fixed
// summary identities where applicable, rule identities, the call-activation
// schema, and query families with their result codecs. The fields are
// intentionally explicit so callers cannot silently invent an unregistered
// factor or rule.
//
// The activation family and admission fields are part of the call-activation
// law and remain stable across Links and Programs.
type Bundle struct {
	ValueFactor, ValueSummary, ValueSummaryFold, CallFactor, HeapFactor, PackFactor, EffectFactor identity.SemanticKey
	ValueQuery, ValueCodec, EffectQuery, EffectCodec                                              identity.SemanticKey
	CallActivation, CallActivationFamily, CallActivationAdmission                                 identity.SemanticKey

	ValueSourceRule, PackSourceRule, HeapIngressRule, RawGetRule, RawSetRule, CallDispatchRule                               RuleSemantics
	EffectSelectedRule, EffectOpaqueRule, EffectBodyRule, ValueBootstrapRule, HeapBootstrapRule                              RuleSemantics
	ValueTransferRule, ValueBinaryArithmeticRule, ValueBinaryEqualityRule, ValueBinaryOrderRule, ValuePresenceRefinementRule RuleSemantics
	ValueAllocationRule, HeapEmptyRule, HeapClosedRule                                                                       TransformedRuleSemantics
}

// New returns the canonical global vocabulary and whether every closed role
// is available and distinct.  Construction is pure and replayable.
func New() (Bundle, bool) {
	key := func(role string) identity.SemanticKey {
		value, _ := Key(role)
		return value
	}
	rule := func(role string) RuleSemantics {
		return RuleSemantics{Rule: key("rule/" + role), Operand: key("operand/" + role), Evidence: key("evidence/" + role)}
	}
	transformed := func(role string) TransformedRuleSemantics {
		return TransformedRuleSemantics{RuleSemantics: rule(role), Transform: key("transform/" + role)}
	}
	result := Bundle{
		ValueFactor: key("factor/value"), ValueSummary: key("factor/value/summary-identity"), ValueSummaryFold: key("factor/value/summary-coordinatewise"),
		CallFactor: key("factor/call"), HeapFactor: key("factor/heap"), PackFactor: key("factor/pack"),
		EffectFactor: key("factor/effect"),
		ValueQuery:   key("query/value-summary"), ValueCodec: key("query-result/value-summary"),
		EffectQuery: key("query/effect-exact"), EffectCodec: key("query-result/effect-exact"),
		CallActivation: key("activation/call-body"), CallActivationFamily: key("activation-family/call-body"), CallActivationAdmission: key("activation-admission/call-body"),
		ValueSourceRule: rule("value/source"), PackSourceRule: rule("pack/source"), HeapIngressRule: rule("heap/allocation-ingress"),
		ValueAllocationRule: transformed("value/allocation"), HeapEmptyRule: transformed("heap/allocation-empty"), HeapClosedRule: transformed("heap/allocation-closed"),
		RawGetRule: rule("heap/index-get-raw"), RawSetRule: rule("heap/index-set-raw"), CallDispatchRule: rule("call/dispatch"),
		EffectSelectedRule: rule("effect/callsite-selected"), EffectOpaqueRule: rule("effect/callsite-opaque"), EffectBodyRule: rule("effect/callsite-body"),
		ValueBootstrapRule: rule("value/host-global-bootstrap"), HeapBootstrapRule: rule("heap/host-bootstrap"), ValueTransferRule: rule("value/storage-transfer"),
		ValueBinaryArithmeticRule: rule("value/binary-arithmetic"), ValueBinaryEqualityRule: rule("value/binary-equality"), ValueBinaryOrderRule: rule("value/binary-order"), ValuePresenceRefinementRule: rule("value/presence-refinement"),
	}
	return result, result.Available()
}

// Available reports whether every key in the closed vocabulary is usable and
// no two roles share an identity.
func (bundle Bundle) Available() bool {
	keys := [...]identity.SemanticKey{
		bundle.ValueFactor, bundle.ValueSummary, bundle.ValueSummaryFold, bundle.CallFactor, bundle.HeapFactor, bundle.PackFactor, bundle.EffectFactor,
		bundle.ValueQuery, bundle.ValueCodec, bundle.EffectQuery, bundle.EffectCodec,
		bundle.CallActivation, bundle.CallActivationFamily, bundle.CallActivationAdmission,
		bundle.ValueSourceRule.Rule, bundle.ValueSourceRule.Operand, bundle.ValueSourceRule.Evidence,
		bundle.PackSourceRule.Rule, bundle.PackSourceRule.Operand, bundle.PackSourceRule.Evidence,
		bundle.HeapIngressRule.Rule, bundle.HeapIngressRule.Operand, bundle.HeapIngressRule.Evidence,
		bundle.ValueAllocationRule.Rule, bundle.ValueAllocationRule.Operand, bundle.ValueAllocationRule.Evidence, bundle.ValueAllocationRule.Transform,
		bundle.HeapClosedRule.Rule, bundle.HeapClosedRule.Operand, bundle.HeapClosedRule.Evidence, bundle.HeapClosedRule.Transform,
		bundle.HeapEmptyRule.Rule, bundle.HeapEmptyRule.Operand, bundle.HeapEmptyRule.Evidence, bundle.HeapEmptyRule.Transform,
		bundle.RawGetRule.Rule, bundle.RawGetRule.Operand, bundle.RawGetRule.Evidence,
		bundle.RawSetRule.Rule, bundle.RawSetRule.Operand, bundle.RawSetRule.Evidence,
		bundle.CallDispatchRule.Rule, bundle.CallDispatchRule.Operand, bundle.CallDispatchRule.Evidence,
		bundle.EffectSelectedRule.Rule, bundle.EffectSelectedRule.Operand, bundle.EffectSelectedRule.Evidence,
		bundle.EffectOpaqueRule.Rule, bundle.EffectOpaqueRule.Operand, bundle.EffectOpaqueRule.Evidence,
		bundle.EffectBodyRule.Rule, bundle.EffectBodyRule.Operand, bundle.EffectBodyRule.Evidence,
		bundle.ValueBootstrapRule.Rule, bundle.ValueBootstrapRule.Operand, bundle.ValueBootstrapRule.Evidence,
		bundle.HeapBootstrapRule.Rule, bundle.HeapBootstrapRule.Operand, bundle.HeapBootstrapRule.Evidence,
		bundle.ValueTransferRule.Rule, bundle.ValueTransferRule.Operand, bundle.ValueTransferRule.Evidence,
		bundle.ValueBinaryArithmeticRule.Rule, bundle.ValueBinaryArithmeticRule.Operand, bundle.ValueBinaryArithmeticRule.Evidence,
		bundle.ValueBinaryEqualityRule.Rule, bundle.ValueBinaryEqualityRule.Operand, bundle.ValueBinaryEqualityRule.Evidence,
		bundle.ValueBinaryOrderRule.Rule, bundle.ValueBinaryOrderRule.Operand, bundle.ValueBinaryOrderRule.Evidence,
		bundle.ValuePresenceRefinementRule.Rule, bundle.ValuePresenceRefinementRule.Operand, bundle.ValuePresenceRefinementRule.Evidence,
	}
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

// Key derives one global semantic role.  The framing and domain are part of
// the stable preimage and intentionally match the historical analysis key
// derivation.
func Key(role string) (identity.SemanticKey, bool) {
	if role == "" {
		return identity.SemanticKey{}, false
	}
	hash := sha256.New()
	var version [8]byte
	binary.BigEndian.PutUint64(version[:], SemanticFormat)
	if !writeFramedHash(hash, []byte("analysis/global-schema")) || !writeFramedHash(hash, version[:]) || !writeFramedHash(hash, []byte(role)) {
		return identity.SemanticKey{}, false
	}
	var digest [32]byte
	if sum := hash.Sum(digest[:0]); len(sum) != len(digest) {
		return identity.SemanticKey{}, false
	}
	return identity.NewSemanticKey(digest, SemanticFormat)
}

func writeFramedHash(hash interface{ Write([]byte) (int, error) }, value []byte) bool {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	first, firstErr := hash.Write(size[:])
	second, secondErr := hash.Write(value)
	return firstErr == nil && secondErr == nil && first == len(size) && second == len(value)
}
