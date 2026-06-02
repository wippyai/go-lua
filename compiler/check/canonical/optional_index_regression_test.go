package canonical_test

import "testing"

func TestOptionalIndexNarrowingRegression(t *testing.T) {
	cases := map[string]struct {
		src          string
		wantClean    bool
		wantContains string
	}{
		"map-index-guard-return": {
			wantClean: true,
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
		},
		"two-level-map-index-guard-return": {
			wantClean: true,
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
		},
		"map-index-guard-field-read": {
			wantClean: true,
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
		},
		"two-level-no-guard": {
			wantContains: "cannot return",
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
		},
		"no-guard": {
			wantContains: "cannot return",
			src: `
type OrderAggregate = {id: string, version: number}
type Store = {orders: {[string]: OrderAggregate}}
local function ensure(self: Store, id: string): OrderAggregate
    local current = self.orders[id]
    return current
end
return ensure
`,
		},
		"guard-false-edge-return": {
			wantContains: "cannot return",
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
		},
		"inner-field-nilcheck-then-return": {
			wantClean: true,
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
		},
		"only-field-guard-no-base-narrow": {
			wantContains: "cannot return",
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
		},
		"map-index-nilcompare": {
			wantClean: true,
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
