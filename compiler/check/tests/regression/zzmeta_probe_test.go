package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// zzProbe reveals the inferred chain type by forcing a typed-assignment error.
func zzProbe(t *testing.T, label, src string) {
	t.Helper()
	r := testutil.Check(src, testutil.WithStdlib())
	if !r.HasError() {
		t.Logf("[%s] NO ERROR (chain resolved)", label)
		return
	}
	for _, e := range r.Errors {
		t.Logf("[%s] err: %s @ %d:%d", label, e.Message, e.Position.Line, e.Position.Column)
	}
}

// V1: full chain, single module, with repo/executor.
func TestZZMeta_V1_Full(t *testing.T) {
	zzProbe(t, "V1", `
local session = {}
local executor = {}
function executor:query(): any return nil end
local repo = {}
function repo.list_by_type(_a, _b)
	local c, err = executor:query()
	if err then return nil, err end
	return c
end
local context_query = { _session_id = nil :: string?, _type_filter = nil :: string?, _error = nil :: string? }
context_query.__index = context_query
local session_reader = { session_id = nil :: string? }
session_reader.__index = session_reader
function session.open(session_id)
	return setmetatable({ session_id = session_id }, session_reader), nil
end
function session_reader:contexts()
	local query = setmetatable({}, context_query)
	query._session_id = self.session_id
	return query
end
function context_query:type(context_type)
	if not context_type then self._error = "x"; return self end
	self._type_filter = context_type
	return self
end
function context_query:all()
	if self._error then return nil, self._error end
	local c, err = repo.list_by_type(self._session_id, self._type_filter)
	if err then return nil, err end
	return c or {}, nil
end
local probe: number = session_reader.contexts
`)
}

// V2: drop repo/executor; all returns a literal sequence.
func TestZZMeta_V2_NoRepo(t *testing.T) {
	zzProbe(t, "V2", `
local context_query = {}
context_query.__index = context_query
local session_reader = {}
session_reader.__index = session_reader
local session = {}
function session.open(id)
	return setmetatable({ session_id = id }, session_reader), nil
end
function session_reader:contexts()
	return setmetatable({}, context_query)
end
function context_query:type(t) return self end
function context_query:all() return {1, 2, 3} end
local probe: number = session_reader.contexts
`)
}

// V3: no session.open; reader:contexts():type():all() directly.
func TestZZMeta_V3_NoOpen(t *testing.T) {
	zzProbe(t, "V3", `
local context_query = {}
context_query.__index = context_query
local session_reader = {}
session_reader.__index = session_reader
function session_reader:contexts()
	return setmetatable({}, context_query)
end
function context_query:type(t) return self end
function context_query:all() return {1, 2, 3} end
local r = setmetatable({}, session_reader)
local out = r:contexts():type("x"):all()
local probe: number = out
`)
}

// V4: like V2 but check the chain result type at use site.
func TestZZMeta_V4_ChainUse(t *testing.T) {
	zzProbe(t, "V4", `
local context_query = {}
context_query.__index = context_query
local session_reader = {}
session_reader.__index = session_reader
local session = {}
function session.open(id)
	return setmetatable({ session_id = id }, session_reader), nil
end
function session_reader:contexts()
	return setmetatable({}, context_query)
end
function context_query:type(t) return self end
function context_query:all() return {1, 2, 3} end
local rd = session.open("x")
local out = rd:contexts():type("y"):all()
local probe: number = out
`)
}

// V5: probe session.open's own return type (is session_reader mt also dropped?).
func TestZZMeta_V5_OpenReturn(t *testing.T) {
	zzProbe(t, "V5", `
local context_query = {}
context_query.__index = context_query
local session_reader = {}
session_reader.__index = session_reader
local session = {}
function session.open(id)
	return setmetatable({ session_id = id }, session_reader), nil
end
function session_reader:contexts()
	return setmetatable({}, context_query)
end
function context_query:type(t) return self end
function context_query:all() return {1, 2, 3} end
local probe: number = session.open
`)
}

// V6: V4 dependency order (define session LAST, others first). Source-order test.
func TestZZMeta_V6_OrderedSession(t *testing.T) {
	zzProbe(t, "V6", `
local context_query = {}
context_query.__index = context_query
function context_query:type(t) return self end
function context_query:all() return {1, 2, 3} end
local session_reader = {}
session_reader.__index = session_reader
function session_reader:contexts()
	return setmetatable({}, context_query)
end
local session = {}
function session.open(id)
	return setmetatable({ session_id = id }, session_reader), nil
end
local rd = session.open("x")
local out = rd:contexts():type("y"):all()
local probe: number = out
`)
}

// V7: probe the session_reader VARIABLE type (does it carry __index + contexts?).
func TestZZMeta_V7_ReaderVar(t *testing.T) {
	zzProbe(t, "V7", `
local context_query = {}
context_query.__index = context_query
local session_reader = {}
session_reader.__index = session_reader
local session = {}
function session.open(id)
	return setmetatable({ session_id = id }, session_reader), nil
end
function session_reader:contexts()
	return setmetatable({}, context_query)
end
function context_query:type(t) return self end
function context_query:all() return {1, 2, 3} end
local probe: number = session_reader
`)
}

// V8: session.open returns a PLAIN table (no setmetatable). Does contexts still break?
func TestZZMeta_V8_OpenPlain(t *testing.T) {
	zzProbe(t, "V8", `
local context_query = {}
context_query.__index = context_query
local session_reader = {}
session_reader.__index = session_reader
local session = {}
function session.open(id)
	return { session_id = id }, nil
end
function session_reader:contexts()
	return setmetatable({}, context_query)
end
function context_query:type(t) return self end
function context_query:all() return {1, 2, 3} end
local r = setmetatable({}, session_reader)
local out = r:contexts():type("y"):all()
local probe: number = out
`)
}

// V9: session.open setmetatables a THIRD unrelated class, not session_reader.
func TestZZMeta_V9_ThirdClass(t *testing.T) {
	zzProbe(t, "V9", `
local context_query = {}
context_query.__index = context_query
local session_reader = {}
session_reader.__index = session_reader
local other = {}
other.__index = other
local session = {}
function session.open(id)
	return setmetatable({ session_id = id }, other), nil
end
function session_reader:contexts()
	return setmetatable({}, context_query)
end
function context_query:type(t) return self end
function context_query:all() return {1, 2, 3} end
local r = setmetatable({}, session_reader)
local out = r:contexts():type("y"):all()
local probe: number = out
`)
}

const zzChainPrelude = `
local context_query = {}
context_query.__index = context_query
local session_reader = {}
session_reader.__index = session_reader
local session = {}
function session.open(id)
	return setmetatable({ session_id = id }, session_reader), nil
end
function session_reader:contexts()
	return setmetatable({}, context_query)
end
function context_query:type(t) return self end
function context_query:all() return {1, 2, 3} end
`

// V10: what is rd (session.open's return)?
func TestZZMeta_V10_Rd(t *testing.T) {
	zzProbe(t, "V10", zzChainPrelude+`
local rd = session.open("x")
local probe: number = rd
`)
}

// V11: what is rd:contexts()?
func TestZZMeta_V11_Contexts(t *testing.T) {
	zzProbe(t, "V11", zzChainPrelude+`
local rd = session.open("x")
local c = rd:contexts()
local probe: number = c
`)
}

// V12: what is rd:contexts():type()?
func TestZZMeta_V12_Type(t *testing.T) {
	zzProbe(t, "V12", zzChainPrelude+`
local rd = session.open("x")
local d = rd:contexts():type("y")
local probe: number = d
`)
}

// V13: single-file analog of the external-lint chain; probe all() element.
func TestZZMeta_V13_AllElem(t *testing.T) {
	zzProbe(t, "V13", `
type Context = { text: string, created_at: string }
local repo = {}
function repo.list_by_type(a: string?, b: string?): ({Context}?, string?)
	return { { text = "x", created_at = "now" } }, nil
end
local context_query = { _session_id = nil :: string?, _type_filter = nil :: string?, _error = nil :: string? }
context_query.__index = context_query
local session_reader = { session_id = nil :: string? }
session_reader.__index = session_reader
local session = {}
function session.open(id)
	return setmetatable({ session_id = id }, session_reader), nil
end
function session_reader:contexts()
	local query = setmetatable({}, context_query)
	query._session_id = self.session_id
	return query
end
function context_query:type(ct)
	if not ct then self._error = "x"; return self end
	self._type_filter = ct
	return self
end
function context_query:all()
	if self._error then return nil, self._error end
	local contexts, err = repo.list_by_type(self._session_id, self._type_filter)
	if err then return nil, err end
	return contexts or {}, nil
end
local rd = session.open("x")
local out = rd:contexts():type("y"):all()
local probe: number = out
`)
}

// V14: probe element of out (out[1]).
func TestZZMeta_V14_AllIndex(t *testing.T) {
	zzProbe(t, "V14", `
type Context = { text: string, created_at: string }
local repo = {}
function repo.list_by_type(a: string?, b: string?): ({Context}?, string?)
	return { { text = "x", created_at = "now" } }, nil
end
local context_query = { _session_id = nil :: string?, _type_filter = nil :: string?, _error = nil :: string? }
context_query.__index = context_query
local session_reader = { session_id = nil :: string? }
session_reader.__index = session_reader
local session = {}
function session.open(id)
	return setmetatable({ session_id = id }, session_reader), nil
end
function session_reader:contexts()
	local query = setmetatable({}, context_query)
	query._session_id = self.session_id
	return query
end
function context_query:type(ct)
	if not ct then self._error = "x"; return self end
	self._type_filter = ct
	return self
end
function context_query:all()
	if self._error then return nil, self._error end
	local contexts, err = repo.list_by_type(self._session_id, self._type_filter)
	if err then return nil, err end
	return contexts or {}, nil
end
local rd = session.open("x")
local out = rd:contexts():type("y"):all()
if out and #out > 0 then
	local probe: number = out[1].text
end
`)
}
