package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestZZNilUnion2Probe reproduces the target-fixture narrowing patterns in a
// single file so ZNARROW traces the discriminant/sibling edge. Debug probe.
func TestZZNilUnion2Probe(t *testing.T) {
	cases := map[string]string{
		"not-ok-then-value-after": `
type Action = {kind: "a", x: string} | {kind: "b", y: string}
type VR = {ok: true, value: Action} | {ok: false, error: string}
local function use(vr: VR): Action
    if not vr.ok then
        error("bad")
    end
    return vr.value
end
return use
`,
		"not-ok-assign-value": `
type Action = {kind: "a", x: string} | {kind: "b", y: string}
type VR = {ok: true, value: Action} | {ok: false, error: string}
local function use(vr: VR)
    local current: Action
    if not vr.ok then
        return
    end
    current = vr.value
end
return use
`,
		"compensation-else-field": `
type Rel = {kind: "release", reservation_token: string}
type Ref = {kind: "refund", payment_id: string}
type Comp = Rel | Ref
local function use(comp: Comp): string
    if comp.kind == "release" then
        return comp.reservation_token
    else
        return comp.payment_id
    end
end
return use
`,
		// SOUNDNESS: the then-arm narrows on the "release" variant and returns; the
		// surviving exit edge has comp = Ref, which has no reservation_token, so the
		// post-merge read must still error (the exit-guard re-narrowing must drop the
		// release variant, not over-widen back to the full union and hide the gap).
		"soundness-wrong-variant-field-after-return": `
type Rel = {kind: "release", reservation_token: string}
type Ref = {kind: "refund", payment_id: string}
type Comp = Rel | Ref
local function use(comp: Comp): string
    if comp.kind == "release" then
        return comp.reservation_token
    end
    return comp.reservation_token
end
return use
`,
		// SOUNDNESS: a guard on an UNRELATED value must not narrow vr; reading the
		// union's per-variant field on the surviving edge stays optional and must
		// still error.
		"soundness-unrelated-guard-keeps-optional": `
type Action = {kind: "a", x: string} | {kind: "b", y: string}
type VR = {ok: true, value: Action} | {ok: false, error: string}
local function use(vr: VR, flag: boolean): Action
    if flag then
        return {kind = "a", x = "z"}
    end
    return vr.value
end
return use
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
			for _, m := range testutil.ErrorMessages(res.Diagnostics) {
				t.Logf("DIAG: %s", m)
			}
		})
	}
}
