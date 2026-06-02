package canonical_test

import (
	"testing"
)

func TestNilUnionNarrowingRegression(t *testing.T) {
	cases := map[string]struct {
		src          string
		wantClean    bool
		wantContains string
	}{
		"not-ok-then-value-after": {
			wantClean: true,
			src: `
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
		},
		"not-ok-assign-value": {
			wantClean: true,
			src: `
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
		},
		"discriminated-else-field": {
			wantClean: true,
			src: `
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
		},
		"wrong-variant-field-after-return": {
			wantContains: "field 'reservation_token' does not exist",
			src: `
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
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.wantClean {
				requireCanonicalClean(t, tc.src)
				return
			}
			requireCanonicalDiagnosticContains(t, tc.src, tc.wantContains)
		})
	}
}

func TestNilUnionUnrelatedGuardKeepsOptionalGap(t *testing.T) {
	src := `
type Action = {kind: "a", x: string} | {kind: "b", y: string}
type VR = {ok: true, value: Action} | {ok: false, error: string}
local function use(vr: VR, flag: boolean): Action
    if flag then
        return {kind = "a", x = "z"}
    end
    return vr.value
end
return use
`
	requireCanonicalDiagnosticContains(t, src, "cannot return Action?")
}
