package canonical_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestZZOptIdxProbe reproduces the optional-index + truthiness-guard narrowing
// patterns of the cqrs/engine fixtures in single files so ZNARROW traces the
// guard edge on a LOCAL bound from a map/array index read. Debug probe.
func TestZZOptIdxProbe(t *testing.T) {
	cases := map[string]struct {
		src       string
		wantClean bool // expect no diagnostics (guard soundly narrows)
	}{
		// A local bound from a map index read is OrderAggregate?; the `if current`
		// guard must narrow it to non-nil so the return matches OrderAggregate.
		"map-index-guard-return": {
			src: `
type OrderAggregate = {id: string, version: number}
type Store = {orders: {[string]: OrderAggregate}}
local function ensure(self: Store, id: string): OrderAggregate
    local current = self.orders[id]
    if current then
        return current
    end
    return {id = id, version = 0}
end
return ensure
`,
			wantClean: true,
		},
		// Faithful cqrs shape: a two-level field path (self.state.orders) whose map
		// value is a named alias; the local is OrderAggregate?, guarded by truthiness.
		"two-level-map-index-guard-return": {
			src: `
type OrderAggregate = {id: string, version: number}
type StoreState = {orders: {[string]: OrderAggregate}}
type Store = {state: StoreState}
local function ensure(self: Store, id: string): OrderAggregate
    local current = self.state.orders[id]
    if current then
        if current.version == 0 then
            current.version = 1
        end
        return current
    end
    return {id = id, version = 0}
end
return ensure
`,
			wantClean: true,
		},
		// Same shape via a method-self split (engine.lua:138 SummaryResult?).
		"map-index-guard-field-read": {
			src: `
type Summary = {total: number}
type Store = {cache: {[string]: Summary}}
local function get(self: Store, key: string): number
    local current = self.cache[key]
    if current then
        return current.total
    end
    return 0
end
return get
`,
			wantClean: true,
		},
		// SOUNDNESS: in-module two-level map index read with NO guard must error
		// (proves the in-module read is genuinely optional, matching cross-module).
		"soundness-two-level-no-guard": {
			src: `
type OrderAggregate = {id: string, version: number}
type StoreState = {orders: {[string]: OrderAggregate}}
type Store = {state: StoreState}
local function ensure(self: Store, id: string): OrderAggregate
    local current = self.state.orders[id]
    return current
end
return ensure
`,
			wantClean: false,
		},
		// SOUNDNESS: no guard at all -> the map index read stays optional and the
		// return must still error.
		"soundness-no-guard": {
			src: `
type OrderAggregate = {id: string, version: number}
type Store = {orders: {[string]: OrderAggregate}}
local function ensure(self: Store, id: string): OrderAggregate
    local current = self.orders[id]
    return current
end
return ensure
`,
			wantClean: false,
		},
		// SOUNDNESS: the guard narrows on the FALSE edge (current is nil there); the
		// return on the post-guard path must still error.
		"soundness-guard-false-edge-return": {
			src: `
type OrderAggregate = {id: string, version: number}
type Store = {orders: {[string]: OrderAggregate}}
local function ensure(self: Store, id: string): OrderAggregate
    local current = self.orders[id]
    if current then
    end
    return current
end
return ensure
`,
			wantClean: false,
		},
		// The cqrs ensure_order shape: a truthiness guard on the optional local, with
		// a NESTED field-path nil-check (`if current.updated_at == nil`) inside the
		// guarded block, then `return current`. The inner guard's ScopeExit carries
		// only the ROOT symbol + a nil check, dropping the `.updated_at` field path; a
		// bare-symbol re-narrowing would pin `current` to nil and re-widen the return
		// to OrderAggregate?. The exit-guard must recover the field path so the inner
		// guard narrows the FIELD, leaving `current` non-nil for the return.
		"inner-field-nilcheck-then-return": {
			src: `
type OrderAggregate = {id: string, version: number, updated_at: number?}
type StoreState = {orders: {[string]: OrderAggregate}}
type Store = {state: StoreState}
local function ensure(self: Store, id: string, at: number): OrderAggregate
    local current = self.state.orders[id]
    if current then
        if current.updated_at == nil then
            current.updated_at = at
        end
        return current
    end
    return {id = id, version = 0, updated_at = at}
end
return ensure
`,
			wantClean: true,
		},
		// SOUNDNESS: the OUTER guard is itself a field-path nil-check on the base; the
		// post-block read of the base must NOT be narrowed by the field guard. Here
		// `current` is required non-optional but is only ever guarded on its field, so
		// the return of the optional base must still error.
		"soundness-only-field-guard-no-base-narrow": {
			src: `
type OrderAggregate = {id: string, version: number, updated_at: number?}
type StoreState = {orders: {[string]: OrderAggregate}}
type Store = {state: StoreState}
local function ensure(self: Store, id: string): OrderAggregate
    local current = self.state.orders[id]
    if current and current.updated_at == nil then
    end
    return current
end
return ensure
`,
			wantClean: false,
		},
		// A bare local bound from a map index, guarded by an explicit nil compare.
		"map-index-nilcompare": {
			src: `
type Summary = {total: number}
type Store = {cache: {[string]: Summary}}
local function get(self: Store, key: string): Summary
    local current = self.cache[key]
    if current ~= nil then
        return current
    end
    return {total = 0}
end
return get
`,
			wantClean: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			res := testutil.Check(tc.src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
			msgs := testutil.ErrorMessages(res.Diagnostics)
			for _, m := range msgs {
				t.Logf("DIAG: %s", m)
			}
			if tc.wantClean && len(msgs) != 0 {
				t.Errorf("expected clean, got %d diagnostics", len(msgs))
			}
			if !tc.wantClean && len(msgs) == 0 {
				t.Errorf("expected a diagnostic (soundness), got none")
			}
		})
	}
}

func TestZZStderrSanity(t *testing.T) {
	fmt.Fprintln(os.Stderr, "[ZZSANITY] stderr reaches here")
}
