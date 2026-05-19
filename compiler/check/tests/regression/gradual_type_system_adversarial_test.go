package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestGradualTyping_DecodesDynamicPayloadAfterStructuralProof(t *testing.T) {
	source := `
type Address = {city: string, zip: number?}
type User = {id: string, name: string, active: boolean, tags: {string}, address: Address?}
type DecodeError = {kind: string, message: string}

local function err(kind: string, message: string): DecodeError
	return { kind = kind, message = message }
end

local function read_string(value, field: string): (string?, DecodeError?)
	if type(value) == "string" then
		return value, nil
	end
	return nil, err("shape", field)
end

local function read_boolean(value, field: string): (boolean?, DecodeError?)
	if type(value) == "boolean" then
		return value, nil
	end
	return nil, err("shape", field)
end

local function read_tags(value): ({string}?, DecodeError?)
	if value == nil then
		return {}, nil
	end
	if type(value) ~= "table" then
		return nil, err("shape", "tags")
	end
	local tags: {string} = {}
	for _, item in ipairs(value) do
		if type(item) ~= "string" then
			return nil, err("shape", "tag")
		end
		table.insert(tags, item)
	end
	return tags, nil
end

local function read_address(value): (Address?, DecodeError?)
	if value == nil then
		return nil, nil
	end
	if type(value) ~= "table" then
		return nil, err("shape", "address")
	end
	local city, city_err = read_string(value.city, "city")
	if city_err then
		return nil, city_err
	end
	local zip: number? = nil
	if type(value.zip) == "number" then
		zip = value.zip
	end
	return { city = city, zip = zip }, nil
end

local function decode_user(raw: any): (User?, DecodeError?)
	if type(raw) ~= "table" then
		return nil, err("shape", "root")
	end

	local id, id_err = read_string(raw.id, "id")
	if id_err then
		return nil, id_err
	end
	local name, name_err = read_string(raw.name, "name")
	if name_err then
		return nil, name_err
	end
	local active, active_err = read_boolean(raw.active, "active")
	if active_err then
		return nil, active_err
	end
	local tags, tags_err = read_tags(raw.tags)
	if tags_err then
		return nil, tags_err
	end
	local address, address_err = read_address(raw.address)
	if address_err then
		return nil, address_err
	end

	return { id = id, name = name, active = active, tags = tags, address = address }, nil
end

local user, decode_err = decode_user({
	id = "u1",
	name = "Ada",
	active = true,
	tags = { "admin", "founder" },
	address = { city = "London", zip = 12345 },
})

if decode_err then
	return decode_err.message
end
if user.address then
	return user.name .. ":" .. user.address.city .. ":" .. user.tags[1]
end
return user.id
`
	assertNoGradualTypingErrors(t, source)
}

func TestGradualTyping_DispatchesGuardedUnionThroughTypedRegistry(t *testing.T) {
	source := `
type EmailCommand = {kind: "email", to: string, body: string}
type SmsCommand = {kind: "sms", phone: string, text: string}
type Command = EmailCommand | SmsCommand
type DispatchResult = {ok: true, id: string} | {ok: false, error: string}
type Handler = (Command) -> DispatchResult

local handlers: {[string]: Handler} = {}

handlers.email = function(command: Command): DispatchResult
	if command.kind == "email" then
		return { ok = true, id = command.to .. ":" .. command.body }
	end
	return { ok = false, error = "wrong handler" }
end

handlers.sms = function(command: Command): DispatchResult
	if command.kind == "sms" then
		return { ok = true, id = command.phone .. ":" .. command.text }
	end
	return { ok = false, error = "wrong handler" }
end

local function decode(raw: any): Command?
	if type(raw) ~= "table" then
		return nil
	end
	if raw.kind == "email" and type(raw.to) == "string" and type(raw.body) == "string" then
		return { kind = "email", to = raw.to, body = raw.body }
	end
	if raw.kind == "sms" and type(raw.phone) == "string" and type(raw.text) == "string" then
		return { kind = "sms", phone = raw.phone, text = raw.text }
	end
	return nil
end

local function dispatch(raw: any): string
	local command = decode(raw)
	if not command then
		return "bad"
	end
	local handler = handlers[command.kind]
	if not handler then
		return "missing"
	end
	local result = handler(command)
	if result.ok then
		return result.id
	end
	return result.error
end

return dispatch({ kind = "email", to = "ops@example.com", body = "ready" })
`
	assertNoGradualTypingErrors(t, source)
}

func TestGradualTyping_GenericValidatedCollectionPreservesElementType(t *testing.T) {
	source := `
type Validation<T> = {ok: true, value: T} | {ok: false, error: string}
type Point = {x: number, y: number}

local function ok<T>(value: T): Validation<T>
	return { ok = true, value = value }
end

local function invalid<T>(message: string): Validation<T>
	return { ok = false, error = message }
end

local function traverse<T, U>(items: {T}, fn: (T) -> Validation<U>): Validation<{U}>
	local out: {U} = {}
	for _, item in ipairs(items) do
		local next = fn(item)
		if not next.ok then
			return invalid(next.error)
		end
		table.insert(out, next.value)
	end
	return ok(out)
end

local function parse_point(raw: any): Validation<Point>
	if type(raw) ~= "table" then
		return invalid("point")
	end
	if type(raw.x) == "number" and type(raw.y) == "number" then
		return ok({ x = raw.x, y = raw.y })
	end
	return invalid("coords")
end

local parsed = traverse(({ { x = 1, y = 2 }, { x = 3, y = 4 } } :: {any}), parse_point)
if parsed.ok then
	local first = parsed.value[1]
	if first then
		local total: number = first.x + first.y
		return tostring(total)
	end
	return "empty"
end
return parsed.error
`
	assertNoGradualTypingErrors(t, source)
}

func TestGradualTyping_ExplicitBoundaryCastProvidesPreciseLocalType(t *testing.T) {
	source := `
type Metric = {name: string, count: number, tags: {[string]: string}}

local raw: any = {
	name = "requests",
	count = 10,
	tags = { route = "/v1" },
}

local metric = raw :: Metric
local next_count: number = metric.count + 1
local route = metric.tags.route
if not route then
	return "missing"
end
local route_name: string = route
return metric.name .. ":" .. route_name .. ":" .. tostring(next_count)
`
	assertNoGradualTypingErrors(t, source)
}

func TestGradualTyping_LoopRefinesDynamicRecordsIntoTypedArray(t *testing.T) {
	source := `
type Item = {id: string, score: number, tags: {string}}

local function read_tags(value): {string}?
	if type(value) ~= "table" then
		return nil
	end
	local tags: {string} = {}
	for _, tag in ipairs(value) do
		if type(tag) ~= "string" then
			return nil
		end
		table.insert(tags, tag)
	end
	return tags
end

local function collect(raw_items: any): {Item}
	local out: {Item} = {}
	if type(raw_items) ~= "table" then
		return out
	end
	for _, raw in ipairs(raw_items) do
		if type(raw) == "table" then
			local id = raw.id
			if type(id) == "string" then
				local score = raw.score
				if type(score) == "number" then
					local tags = read_tags(raw.tags)
					if tags then
						table.insert(out, { id = id, score = score, tags = tags })
					end
				end
			end
		end
	end
	return out
end

local items = collect({
	{ id = "a", score = 10, tags = { "hot", "new" } },
	{ id = false, score = "bad", tags = { 1 } },
	{ id = "b", score = 20, tags = { "ok" } },
})

local first = items[1]
if not first then
	return "empty"
end
local label: string = first.id .. ":" .. first.tags[1]
local score: number = first.score + 1
return label .. ":" .. tostring(score)
`
	assertNoGradualTypingErrors(t, source)
}

func TestGradualTyping_PairsLoopRefinesDynamicMapValuesInStages(t *testing.T) {
	source := `
type Endpoint = {url: string, weight: number, headers: {[string]: string}}

local function collect(raw: any): {[string]: Endpoint}
	local endpoints: {[string]: Endpoint} = {}
	if type(raw) ~= "table" then
		return endpoints
	end
	for key, value in pairs(raw) do
		if type(key) == "string" and type(value) == "table" then
			local url = value.url
			if type(url) == "string" then
				local weight = value.weight
				if type(weight) == "number" then
					local headers: {[string]: string} = {}
					if type(value.headers) == "table" then
						for header_name, header_value in pairs(value.headers) do
							if type(header_name) == "string" and type(header_value) == "string" then
								headers[header_name] = header_value
							end
						end
					end
					endpoints[key] = { url = url, weight = weight, headers = headers }
				end
			end
		end
	end
	return endpoints
end

local endpoints = collect({
	primary = { url = "https://example.test", weight = 1, headers = { Accept = "application/json" } },
	secondary = { url = false, weight = "heavy" },
})

local primary = endpoints.primary
if not primary then
	return "missing"
end
local accept = primary.headers.Accept
if not accept then
	return primary.url
end
local url: string = primary.url
local weight: number = primary.weight + 1
return url .. ":" .. accept .. ":" .. tostring(weight)
`
	assertNoGradualTypingErrors(t, source)
}

func TestGradualTyping_WhileLoopCarriesOptionalRefinementThroughState(t *testing.T) {
	source := `
type Event = {kind: "name", value: string} | {kind: "count", value: number}
type State = {name: string?, total: number}

local events: {Event} = {
	{ kind = "count", value = 2 },
	{ kind = "name", value = "worker" },
	{ kind = "count", value = 3 },
}

local state: State = { total = 0 }
local i = 1
while i <= #events do
	local event = events[i]
	if event.kind == "name" then
		state.name = event.value
	else
		state.total = state.total + event.value
	end
	i = i + 1
end

local name = state.name
if not name then
	return "missing"
end
local final_name: string = name
local final_total: number = state.total
return final_name .. ":" .. tostring(final_total)
`
	assertNoGradualTypingErrors(t, source)
}

func TestGradualTyping_NestedLoopsRefineMatrixCellsBeforeAggregation(t *testing.T) {
	source := `
type Cell = {row: number, col: number, value: string}

local function cells(raw_rows: any): {Cell}
	local out: {Cell} = {}
	if type(raw_rows) ~= "table" then
		return out
	end
	for row_index, row in ipairs(raw_rows) do
		if type(row) == "table" then
			for col_index, value in ipairs(row) do
				if type(value) == "string" then
					table.insert(out, { row = row_index, col = col_index, value = value })
				end
			end
		end
	end
	return out
end

local out = cells({
	{ "a", false },
	{ "b", "c" },
})

local first = out[1]
if not first then
	return "empty"
end
local pos: number = first.row + first.col
local value: string = first.value
return value .. ":" .. tostring(pos)
`
	assertNoGradualTypingErrors(t, source)
}

func TestGradualTyping_RejectsExistentialLoopProofAsSpecificElementProof(t *testing.T) {
	source := `
local raw: any = { items = { 42, "safe" } }
local saw_string = false

if type(raw.items) == "table" then
	for _, item in ipairs(raw.items) do
		if type(item) == "string" then
			saw_string = true
		end
	end
end

if saw_string then
	local first: string = raw.items[1]
	return first
end
return "missing"
`
	assertGradualTypingErrorContains(t, source, "cannot assign")
}

func TestGradualTyping_RejectsUncheckedAnyRecordAssignment(t *testing.T) {
	source := `
type User = {id: string, name: string}

local raw: any = { id = "u1", name = "Ada" }
local user: User = raw
return user.id
`
	assertGradualTypingErrorContains(t, source, "cannot assign")
}

func TestGradualTyping_RejectsTruthyGuardAsStructuralProof(t *testing.T) {
	source := `
local raw: any = { profile = "not a table" }

if raw.profile then
	local city: string = raw.profile.city
	return city
end
return "missing"
`
	assertGradualTypingErrorContains(t, source, "cannot assign")
}

func TestGradualTyping_RejectsPartiallyCheckedCollectionAsTypedArray(t *testing.T) {
	source := `
local raw: any = { items = { "safe", 42 } }

if type(raw.items) == "table" and type(raw.items[1]) == "string" then
	local items: {string} = raw.items
	return items[1]
end
return "missing"
`
	assertGradualTypingErrorContains(t, source, "cannot assign")
}

func TestGradualTyping_RejectsDynamicCallbackAtTypedFunctionBoundary(t *testing.T) {
	source := `
type User = {id: string}

local callback: any = function(user)
	return 42
end

local typed: (User) -> string = callback
return typed({ id = "u1" })
`
	assertGradualTypingErrorContains(t, source, "cannot assign")
}

func TestGradualTyping_RejectsExtraFieldsAfterNarrowBoundaryCast(t *testing.T) {
	source := `
type Metric = {name: string, count: number}

local raw: any = { name = "requests", count = 10, extra = true }
local metric = raw :: Metric
local extra: boolean = metric.extra
return tostring(extra)
`
	assertGradualTypingErrorContains(t, source, "cannot assign")
}

func assertNoGradualTypingErrors(t *testing.T, source string) {
	t.Helper()
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected gradual-typing adversarial case to type-check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func assertGradualTypingErrorContains(t *testing.T, source, want string) {
	t.Helper()
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected diagnostic containing %q, got no errors", want)
	}
	messages := strings.Join(testutil.ErrorMessages(result.Diagnostics), " | ")
	if !strings.Contains(messages, want) {
		t.Fatalf("expected diagnostic containing %q, got: %s", want, messages)
	}
}
