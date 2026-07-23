type Request = {
    method: string,
    path: string,
    headers: {[string]: string},
    query: {[string]: string},
    timeout: number,
}

type Builder = {
    method: string,
    path: string,
    headers: {[string]: string},
    query: {[string]: string},
    timeout: number,
    with_method: (self: Builder, method: string) -> Builder,
    with_header: (self: Builder, key: string, value: string) -> Builder,
    with_query: (self: Builder, key: string, value: string?) -> Builder,
    with_timeout: (self: Builder, timeout: number?) -> Builder,
    build: (self: Builder) -> Request,
}

local Builder = {}
Builder.__index = Builder

function Builder:with_method(method: string): Builder
    self.method = method
    return self
end

function Builder:with_header(key: string, value: string): Builder
    self.headers[key] = value
    return self
end

function Builder:with_query(key: string, value: string?): Builder
    if value then
        self.query[key] = value
    end
    return self
end

function Builder:with_timeout(timeout: number?): Builder
    self.timeout = timeout or self.timeout
    return self
end

function Builder:build(): Request
    return {
        method = self.method,
        path = self.path,
        headers = self.headers,
        query = self.query,
        timeout = self.timeout,
    }
end

local M = {}
M.Request = Request
M.Builder = Builder

function M.new(): Builder
    local builder: Builder = {
        method = "GET",
        path = "/",
        headers = {} :: {[string]: string},
        query = {} :: {[string]: string},
        timeout = 30,
        with_method = Builder.with_method,
        with_header = Builder.with_header,
        with_query = Builder.with_query,
        with_timeout = Builder.with_timeout,
        build = Builder.build,
    }
    setmetatable(builder, Builder)
    return builder
end

return M
