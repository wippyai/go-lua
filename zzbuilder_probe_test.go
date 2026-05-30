package lua

import (
	"testing"

	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// TestZZBuilderProbe runs the fluent-builder realworld fixtures in isolation
// through the canonical flow and dumps every diagnostic, to capture the exact
// EXPECTED vs GOT types in the builder field-type-mismatch false positives.
func TestZZBuilderProbe(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	order := []string{
		"realworld/tenant-policy-runtime",
		"realworld/cqrs-order-runtime",
		"realworld/cqrs-order-runtime-soundness",
		"realworld/notification-delivery-runtime-soundness",
		"realworld/agent-workflow-engine",
		"realworld/middleware-session-router",
		"realworld/plugin-supervisor-runtime",
	}
	byName := map[string]namedSuite{}
	for _, s := range suites {
		byName[s.Name] = s
	}
	for _, name := range order {
		s, ok := byName[name]
		if !ok {
			t.Logf("MISSING fixture %s", name)
			continue
		}
		diags, entry := canonicalFixtureDiagnostics(s)
		t.Logf("=== %s (entry=%s) %d diags ===", s.Name, entry, len(diags))
		for _, d := range diags {
			t.Logf("  %s:%d:%d [%s] %s", d.Position.File, d.Position.Line, d.Position.Column, d.Code.Name(), d.Message)
		}
	}
}

// TestZZBuilderWidenRoot builds a self-recursive fluent-builder method type
// (the for_kind shape: a literal-union param, returning the owner family) and
// proves that the convergence/seal widening flattens the param literal-union to
// `string`, which is exactly the EXPECTED side mutation observed in the oracle.
func TestZZBuilderWidenRoot(t *testing.T) {
	kindUnion := typ.NewUnion(
		typ.LiteralString("auth"),
		typ.LiteralString("query"),
		typ.LiteralString("update"),
	)

	// mu RuleBuilder . { for_kind: (self: RuleBuilder, request_kind: "auth"|"query"|"update") -> RuleBuilder }
	rb := typ.NewRecursive("RuleBuilder", func(self typ.Type) typ.Type {
		forKind := typ.Func().
			Param("self", self).
			Param("request_kind", kindUnion).
			Returns(self).
			Build()
		return typ.NewRecord().Field("for_kind", forKind).Build()
	})

	t.Logf("HasHigherOrderGrowthRisk(RuleBuilder) = %v", value.HasHigherOrderGrowthRisk(rb))

	body, ok := rb.Body.(*typ.Record)
	if !ok {
		t.Fatalf("expected record body, got %T", rb.Body)
	}
	forKind := body.GetField("for_kind").Type.(*typ.Function)
	t.Logf("ORIGINAL for_kind        = %s", typ.FormatShort(forKind))
	t.Logf("HasHigherOrderGrowthRisk(for_kind) = %v", value.HasHigherOrderGrowthRisk(forKind))

	// What the seal's join (MergeForConvergence -> widenFunction) does:
	wConv := value.WidenFunctionForConvergence(forKind)
	t.Logf("WidenFunctionForConvergence = %s", typ.FormatShort(wConv))

	// What that path falls through to when growth risk is present:
	wInfer := subtype.WidenForInference(forKind)
	t.Logf("WidenForInference          = %s", typ.FormatShort(wInfer))

	// Build the widened EXPECTED explicitly (string param, family return) and
	// compare exactly as ops.checkTableAsRecord does: Consistent(GOT, EXPECTED).
	widenedExpected := subtype.WidenForInference(forKind)
	t.Logf("Consistent(GOT-literal, EXPECTED-string) = %v (want true; false => false-positive)",
		subtype.Consistent(forKind, widenedExpected))
	t.Logf("IsSubtype(GOT-literal, EXPECTED-string)  = %v", subtype.IsSubtype(forKind, widenedExpected))

	// Param-only contravariance sanity: string vs the literal union.
	t.Logf("IsSubtype(string, literalUnion) = %v (param contravariance check)",
		subtype.IsSubtype(typ.String, kindUnion))
	t.Logf("IsSubtype(literalUnion, string) = %v", subtype.IsSubtype(kindUnion, typ.String))

	// Non-recursive variant: same shape but return a plain record alias of owner
	// fields so growth-risk fires (mirrors how the real family body looks before
	// the mu node folds the self edge).
	plainSelf := typ.NewRecord().Field("name", typ.String).Build()
	forKindPlain := typ.Func().
		Param("self", plainSelf).
		Param("request_kind", kindUnion).
		Returns(plainSelf).
		Build()
	t.Logf("plain for_kind growth-risk = %v", value.HasHigherOrderGrowthRisk(forKindPlain))

	// SECOND-widen path: interner.Widen runs join(existingBody, candidateBody) on
	// the 2nd+ observation. Simulate joining the family body with itself.
	mergedBody := value.MergeForConvergence(rb.Body, rb.Body)
	if mr, ok := mergedBody.(*typ.Record); ok {
		if f := mr.GetField("for_kind"); f != nil {
			t.Logf("MERGE(body,body).for_kind = %s", typ.FormatShort(f.Type))
		}
	} else {
		t.Logf("MERGE(body,body) = %s", typ.FormatShort(mergedBody))
	}
	// Merge the whole recursive family with itself.
	mergedFam := value.MergeForConvergence(rb, rb)
	t.Logf("MERGE(rb,rb) = %s", typ.FormatShort(mergedFam))
	// Direct function merge.
	mf := value.MergeForConvergence(forKind, forKind)
	t.Logf("MERGE(for_kind,for_kind) = %s", typ.FormatShort(mf))

	// PRE-FOLD shape: the builder body BEFORE the self-edge folds to a mu node.
	// for_kind's return is the FULL builder record (callable surface present), so
	// recordHasSelfRecursiveMethod fires and growth-risk is true. This is the
	// shape the seal actually widens.
	var builderRec *typ.Record
	{
		// Two-level structural self-embedding (return is a structural copy of the
		// builder record, not a mu ref).
		inner := func() *typ.Record {
			fk := typ.Func().Param("self", typ.Unknown).Param("request_kind", kindUnion).Returns(typ.Unknown).Build()
			return typ.NewRecord().Field("name", typ.String).Field("for_kind", fk).Build()
		}()
		fk := typ.Func().Param("self", inner).Param("request_kind", kindUnion).Returns(inner).Build()
		builderRec = typ.NewRecord().Field("name", typ.String).Field("for_kind", fk).Build()
	}
	t.Logf("preFold builder growth-risk = %v", value.HasHigherOrderGrowthRisk(builderRec))
	preFoldFK := builderRec.GetField("for_kind").Type.(*typ.Function)
	t.Logf("preFold for_kind ORIG       = %s", typ.FormatShort(preFoldFK))
	t.Logf("preFold for_kind WidenConv  = %s", typ.FormatShort(value.WidenFunctionForConvergence(preFoldFK)))
	merged := value.MergeForConvergence(builderRec, builderRec)
	if mr, ok := merged.(*typ.Record); ok {
		if f := mr.GetField("for_kind"); f != nil {
			t.Logf("MERGE(preFold,preFold).for_kind = %s", typ.FormatShort(f.Type))
		}
	}
}
