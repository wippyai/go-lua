package checktest

import "testing"

func TestSameNamedModuleExportsDoNotLeakDeclaredReturnShape(t *testing.T) {
	first := CheckFileAndExport(`
local http_client = {}

type StreamReader = {
    read: (self: any) -> (string?, string?),
}

type Response = {
    status_code: number,
    body: string?,
    stream: StreamReader?,
    headers: {[string]: string}?,
}

function http_client.get(url: string, options: any?): (Response?, string?)
    return nil, "not implemented"
end

return http_client
`, "http_client", "http_client.lua")
	if len(first.Errors) != 0 {
		t.Fatalf("first http_client diagnostics = %#v, want clean", first.Errors)
	}

	second := CheckFileAndExport(`
local http_client = {}

type Stream = {
    read: (self: Stream, n: number?) -> (string?, string?),
}

type Response = {
    status_code: number,
    body: string?,
    stream: Stream?,
}

function http_client.get(url: string): (Response?, string?)
    local stream: Stream = {
        read = function(self: Stream, n: number?)
            return "chunk", nil
        end,
    }
    return { status_code = 500, stream = stream }, nil
end

return http_client
`, "http_client", "http_client.lua")
	if len(second.Errors) != 0 {
		t.Fatalf("second http_client diagnostics = %#v, want clean", second.Errors)
	}

	result := CheckFile(`
local http_client = require("http_client")

local response, err = http_client.get("https://example.test")
if err or not response then
    return nil, err
end
if response.status_code >= 300 and response.stream and not response.body then
    local body_data = response.stream:read()
    response.body = body_data
end
return response
`, "main.lua", WithModule("http_client", second))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want second http_client declared return shape to include optional body", result.Diagnostics)
	}
}
