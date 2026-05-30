package subtype

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
)

// TestZZGenVarProbe reproduces gradual-typing-adversarial root 3a: an
// Instantiated generic whose type param is used covariantly should accept a
// subtype type-arg, but checkInstantiated treats args invariantly. Read-only.
func TestZZGenVarProbe(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	okVariant := typ.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", tp).Build()
	errVariant := typ.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).Build()
	body := typ.NewUnion(okVariant, errVariant)
	validation := typ.NewGeneric("Validation", []*typ.TypeParam{tp}, body)

	config := typ.NewAlias("Config", typ.NewRecord().
		Field("id", typ.String).
		Field("retries", typ.Number).Build())
	inferred := typ.NewRecord().
		Field("id", typ.String).
		Field("retries", typ.Integer).Build()

	subInst := typ.Instantiate(validation, inferred)
	superInst := typ.Instantiate(validation, config)

	t.Logf("inferred <: Config = %v", IsSubtype(inferred, config))
	t.Logf("Config <: inferred = %v", IsSubtype(config, inferred))
	t.Logf("Validation<inferred> <: Validation<Config> [invariant args] = %v", IsSubtype(subInst, superInst))

	// Body-expansion comparison (what variance-aware/covariant would do).
	subBody := subst.ExpandInstantiated(subInst)
	superBody := subst.ExpandInstantiated(superInst)
	t.Logf("expand sub=%s", fmtT(subBody))
	t.Logf("expand super=%s", fmtT(superBody))
	t.Logf("expandedSubBody <: expandedSuperBody = %v", IsSubtype(subBody, superBody))

	// Consistent (gradual) admission.
	t.Logf("Consistent(Validation<inferred>, Validation<Config>) = %v", Consistent(subInst, superInst))
}

func fmtT(t typ.Type) string {
	if t == nil {
		return "<nil>"
	}
	return t.String()
}
