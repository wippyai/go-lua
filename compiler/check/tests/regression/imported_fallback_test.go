package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestImportedUntypedRepositoryFallbackEliminatesNil(t *testing.T) {
	sessionSource := `
local session = {}

local executor = {}
function executor:query(): any
	return nil
end

local repo = {}
function repo.list_by_type(_session_id, _context_type)
	local contexts, err = executor:query()
	if err then
		return nil, err
	end
	return contexts
end

local context_query = {
	_session_id = nil :: string?,
	_type_filter = nil :: string?,
	_error = nil :: string?,
}
context_query.__index = context_query

local session_reader = {
	session_id = nil :: string?,
}
session_reader.__index = session_reader

function session.open(session_id)
	return setmetatable({ session_id = session_id }, session_reader), nil
end

function session_reader:contexts()
	local query = setmetatable({}, context_query)
	query._session_id = self.session_id
	query._type_filter = nil
	query._error = nil
	return query
end

function context_query:type(context_type)
	if not context_type then
		self._error = "Context type is required"
		return self
	end
	self._type_filter = context_type
	return self
end

function context_query:all()
	if self._error then
		return nil, self._error
	end
	local contexts, err = repo.list_by_type(self._session_id, self._type_filter)
	if err then
		return nil, err
	end
	return contexts or {}, nil
end

return session
`
	sessionModule := testutil.CheckAndExport(sessionSource, "session", testutil.WithStdlib())
	if sessionModule.HasError() {
		for _, e := range sessionModule.Errors {
			t.Logf("session error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("session module errors: %v", testutil.ErrorMessages(sessionModule.Errors))
	}

	source := `
local session = require("session")

local function handle(args)
if not args.session_id then
	return nil, "session_id is required"
end

local session_reader, session_err = session.open(args.session_id)
if not session_reader then
	return nil, session_err
end

local existing_summaries, ctx_err = session_reader:contexts():type("conversation_summary"):all()
if ctx_err then
	existing_summaries = {}
end

if existing_summaries and #existing_summaries > 0 then
	table.sort(existing_summaries, function(a, b)
		return (a.time or a.created_at or "") > (b.time or b.created_at or "")
	end)
	local first = existing_summaries[1].text
end

return true
end

return handle
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("session", sessionModule))
	if result.HasError() {
		t.Fatalf("producer+consumer errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
