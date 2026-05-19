package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestRegression_HttpTimeoutOptionInference(t *testing.T) {
	source := `
type HttpOptions = {
	headers?: {[string]: string},
	timeout?: number,
	body?: string,
	stream?: boolean
}

local http_client = {
	post = function(url: string, opts: HttpOptions): {status_code: number, body: string}
		return {status_code = 200, body = ""}
	end
}

local function build_context_values(): {[string]: any}
	return {}
end

local options = build_context_values()
local config = build_context_values()

local timeout_val = 600
local timeout_custom = tonumber(options.timeout or config.timeout)
if timeout_custom then
	timeout_val = timeout_custom
end

local headers: {[string]: string} = { ["content-type"] = "application/json" }
local http_options = {
	headers = headers,
	timeout = timeout_val,
}

if true then
	http_options.body = "{}"
	if true then
		http_options.stream = true
	end
end

local _ = http_client.post("https://example.local", http_options)
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected no errors for timeout option inference")
	}
}

func TestRegression_TonumberDefaultLiteralIsNumber(t *testing.T) {
	source := `
local function resolve_string(_key: string, _default_env: string?): string?
	return nil
end

local timeout: number = tonumber(resolve_string("timeout", "HTTP_TIMEOUT")) or 600
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected tonumber default literal to infer number")
	}
}

func TestRegression_HttpTimeoutFromResolvedConfigRemainsNumber(t *testing.T) {
	source := `
type HttpOptions = {
	headers?: {[string]: string},
	timeout?: number,
	body?: string,
	stream?: boolean
}

local http_client = {
	post = function(url: string, opts: HttpOptions): {status_code: number, body: string}
		return {status_code = 200, body = ""}
	end
}

local client = {
	_ctx = {
		all = function(): {[string]: any}
			return {}
		end
	},
	_env = {
		get = function(_key: string): string?
			return nil
		end
	},
}

local function resolve_config()
	local ctx_all = client._ctx.all() or {}

	local function resolve_string(key: string, default_env: string?): string?
		if ctx_all[key] then
			return tostring(ctx_all[key])
		end
		if default_env then
			local val = client._env.get(default_env)
			if val and val ~= "" then return val end
		end
		return nil
	end

	return {
		timeout = tonumber(resolve_string("timeout", "HTTP_TIMEOUT")) or 600,
		headers = ctx_all.headers,
	}
end

function client.request(options)
	options = options or {}
	local config = resolve_config()
	local headers: {[string]: string} = {}
	local http_options = {
		headers = headers,
		timeout = tonumber(options.timeout) or config.timeout,
	}
	http_options.body = "{}"
	http_options.stream = true
	local _ = http_client.post("https://example.local", http_options)
end
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected resolved config timeout to remain number")
	}
}

func TestRegression_HttpOptionsParamProjectionCompatibleWithManifestCall(t *testing.T) {
	source := `
type HttpOptions = {
	headers?: {[string]: string},
	body?: string,
	stream?: boolean,
	stream_buffer_size?: number,
	stream_reply_to?: string,
	stream_topic?: string,
}

local http_client = {
	get = function(url: string, opts: HttpOptions): ({status_code: number, body: string?}?, string?)
		return {status_code = 200, body = ""}, nil
	end,
	post = function(url: string, opts: HttpOptions): ({status_code: number, body: string?}?, string?)
		return {status_code = 200, body = ""}, nil
	end,
}

local client = {
	_http_client = http_client,
}

function client.request(method, url, http_options)
	http_options.headers["Accept"] = "application/json"

	if http_options.stream then
		url = url .. "?alt=sse"
		http_options.headers["Accept"] = "text/event-stream"
	end

	local response = nil
	local err = nil
	if method == "GET" then
		response, err = client._http_client.get(url, http_options)
	else
		http_options.headers["Content-Type"] = "application/json"
		response, err = client._http_client.post(url, http_options)
	end

	return response, err
end

client.request("GET", "https://example.local", { headers = {} })
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected projected http options parameter to remain compatible with manifest call")
	}
}

func TestRegression_HttpOptionsMultipleCallHintsStillUseBodyContract(t *testing.T) {
	source := `
type HttpOptions = {
	headers?: {[string]: string},
	body?: string,
	stream?: boolean,
	stream_buffer_size?: number,
	stream_reply_to?: string,
	stream_topic?: string,
}

local http_client = {
	get = function(url: string, opts: HttpOptions): ({status_code: number, body: string?}?, string?)
		return {status_code = 200, body = ""}, nil
	end,
	post = function(url: string, opts: HttpOptions): ({status_code: number, body: string?}?, string?)
		return {status_code = 200, body = ""}, nil
	end,
}

local client = { _http_client = http_client }

function client.request(method, url, http_options)
	http_options.headers["Accept"] = "application/json"
	if http_options.stream then
		url = url .. "?alt=sse"
		http_options.headers["Accept"] = "text/event-stream"
	end
	local response = nil
	local err = nil
	if method == "GET" then
		response, err = client._http_client.get(url, http_options)
	else
		http_options.headers["Content-Type"] = "application/json"
		response, err = client._http_client.post(url, http_options)
	end
	return response, err
end

local function call_one()
	return client.request("GET", "https://example.local", {
		headers = {},
		stream_buffer_size = 4096,
	})
end

local function call_two()
	return client.request("POST", "https://example.local", {
		headers = {},
		body = "{}",
		stream = true,
		stream_reply_to = "reply",
		stream_topic = "topic",
	})
end

call_one()
call_two()
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected body contract to dominate compatible multi-call http option hints")
	}
}

func TestRegression_HttpTimeoutValueNarrowing(t *testing.T) {
	source := `
local options: {[string]: any} = {}
local config: {[string]: any} = {}

local timeout_val = 600
local timeout_custom = tonumber(options.timeout or config.timeout)
if timeout_custom then
	timeout_val = timeout_custom
end

local must_be_number: number = timeout_val
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected timeout_val to narrow to number after tonumber guard")
	}
}

func TestRegression_RecordMutationPreservesUnchangedFieldType(t *testing.T) {
	source := `
local timeout_val = 600
local opts = {
	timeout = timeout_val,
}

opts.body = "{}"
opts.stream = true

local must_be_number: number = opts.timeout
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected record mutation to preserve timeout:number")
	}
}
