-- An `or` default whose left operand is a field read off an untyped source
-- carries gradual `any`, not the right literal: `args` is unannotated (dynamic),
-- so `args.url` is `any` and `(args and args.url) or default` is `any`. Passing
-- that to a `string` parameter is an untyped-to-typed flow the string-proof rule
-- rejects: at runtime `args.url` can be a non-string.
local http = {
	get = function(url: string, options: table)
		return { url = url, options = options }, nil
	end,
}

local function main(args)
	local url = (args and args.url) or "http://localhost:8085/hello"
	return http.get(url, { timeout = "2s" }) -- expect-error: not string
end

return main
