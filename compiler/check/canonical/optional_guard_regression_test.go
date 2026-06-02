package canonical_test

import "testing"

func TestOptionalGuardNarrowingRegression(t *testing.T) {
	cases := map[string]struct {
		src          string
		wantClean    bool
		wantContains string
	}{
		"if-call": {
			wantClean: true,
			src: `
type Cb = fun(): nil
local function run(f: Cb?)
    if f then
        f()
    end
end
return run
`,
		},
		"if-method": {
			wantClean: true,
			src: `
type Svc = { go: fun(self: Svc) }
local function run(s: Svc?)
    if s then
        s:go()
    end
end
return run
`,
		},
		"and-method": {
			wantClean: true,
			src: `
type Svc = { go: fun(self: Svc): boolean }
local function run(s: Svc?)
    local ok = s and s:go()
end
return run
`,
		},
		"if-and-index": {
			wantClean: true,
			src: `
type Row = { exists: boolean }
type QR = { [number]: Row }
local function run(result: QR?)
    if result and result[1] then
        local r = result[1].exists
    end
end
return run
`,
		},
		"if-and-index-array": {
			wantClean: true,
			src: `
type QueryResult = {[string]: any}
local function run(result: {QueryResult}?)
    if result and result[1] then
        local r = result[1].exists
    end
end
return run
`,
		},
		"index-guard-does-not-leak-sibling": {
			wantContains: "cannot index optional value",
			src: `
type QueryResult = {[string]: any}
local function run(result: {QueryResult})
    if result[1] then
        local a = result[1]["k"]
        local b = result[3]["k"]
    end
end
return run
`,
		},
		"if-field-then-call": {
			wantClean: true,
			src: `
type Cb = fun(): nil
type Obj = { cb: Cb? }
local function run(o: Obj)
    if o.cb then
        o.cb()
    end
end
return run
`,
		},
		"local-from-field-if-method": {
			wantClean: true,
			src: `
type Svc = { go: fun(self: Svc) }
type Holder = { store: Svc? }
local function run(h: Holder)
    local store = h.store
    if store then
        store:go()
    end
end
return run
`,
		},
		"upvalue-if-method": {
			wantClean: true,
			src: `
type Svc = { lookup: fun(self: Svc, k: string): boolean }
type Holder = { store: Svc? }
local function build(h: Holder)
    local store = h.store
    return function(token: string)
        if store then
            local snap = store:lookup(token)
        end
    end
end
return build
`,
		},
		"upvalue-if-call": {
			wantClean: true,
			src: `
type Cb = fun(): nil
type Holder = { cb: Cb? }
local function build(h: Holder)
    local cb = h.cb
    return function()
        if cb then
            cb()
        end
    end
end
return build
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
