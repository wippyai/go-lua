local funcs = require("funcs")

local sunpack, sbyte, ssub = string.unpack, string.byte, string.sub

local function u(fmt, s, pos)
	local v, np = sunpack(fmt, s, math.tointeger(pos) or 1)
	return v, math.tointeger(np) or 0
end

-- R-wire decoder: wire bytes -> native Lua objects (length-1 vectors -> scalars)
local dec
dec = function(s, pos)
	pos = math.tointeger(pos) or 1
	local tag = math.tointeger(sbyte(s, pos)) or -1
	pos = pos + 1
	if tag == 0 then
		return nil, pos
	end

	local n
	n, pos = u("<I4", s, pos)
	n = math.tointeger(n) or 0
	if tag == 1 then
		if n == 1 then
			local v
			v, pos = u("<d", s, pos)
			return v, pos
		end

		local t = {}
		for i = 1, n do
			t[i], pos = u("<d", s, pos)
		end
		return t, pos
	elseif tag == 2 then
		if n == 1 then
			local v
			v, pos = u("<i4", s, pos)
			return v, pos
		end

		local t = {}
		for i = 1, n do
			t[i], pos = u("<i4", s, pos)
		end
		return t, pos
	elseif tag == 3 then
		local function rd()
			local b
			b, pos = u("<B", s, pos)
			b = math.tointeger(b) or 0
			if b == 0x80 then
				return nil
			end
			return b ~= 0
		end

		if n == 1 then
			return rd(), pos
		end

		local t = {}
		for i = 1, n do
			t[i] = rd()
		end
		return t, pos
	elseif tag == 4 then
		local function rd()
			local l
			l, pos = u("<I4", s, pos)
			l = math.tointeger(l) or 0
			if l == 0xFFFFFFFF then
				return nil
			end
			local v = ssub(s, pos, pos + l - 1)
			pos = pos + l
			return v
		end

		if n == 1 then
			return rd(), pos
		end

		local t = {}
		for i = 1, n do
			t[i] = rd()
		end
		return t, pos
	elseif tag == 5 then
		local t = {}
		for i = 1, n do
			t[i], pos = dec(s, pos)
		end
		return t, pos
	elseif tag == 7 then
		local v = ssub(s, pos, pos + n - 1)
		pos = pos + n
		return v, pos
	elseif tag == 6 then
		local t = {}
		for i = 1, n do
			local l
			l, pos = u("<I4", s, pos)
			l = math.tointeger(l) or 0
			local k = ssub(s, pos, pos + l - 1)
			pos = pos + l
			t[k], pos = dec(s, pos)
		end
		return t, pos
	end

	error("bad wire tag " .. tostring(tag))
end

-- r_lit: a Lua value -> a self-contained R literal expression. Used to bind the
-- positional args of r.eval(code, ...) into an R list `argv` WITHOUT string
-- interpolation of code: every value is emitted as a data literal (strings fully
-- escaped), so a malicious string can never break out and become R code.
local function r_lit(v)
	local t = type(v)
	if v == nil then
		return "NULL"
	elseif t == "boolean" then
		return v and "TRUE" or "FALSE"
	elseif t == "number" then
		local x = tonumber(v) or 0
		if x ~= x then return "NaN" end
		if x == math.huge then return "Inf" end
		if x == -math.huge then return "-Inf" end
		if x == math.floor(x) and x <= 2147483647 and x >= -2147483648 then
			return string.format("%d", x)
		end
		return string.format("%.17g", x)
	elseif t == "string" then
		local e = v:gsub("\\", "\\\\"):gsub('"', '\\"'):gsub("\n", "\\n"):gsub("\r", "\\r"):gsub("\t", "\\t")
		return '"' .. e .. '"'
	elseif t == "table" then
		local n = #v
		local isArr = true
		for k in pairs(v) do
			if type(k) ~= "number" then isArr = false break end
		end
		if isArr and n > 0 then
			local simple = true
			for i = 1, n do
				local et = type(v[i])
				if et == "table" or et == "nil" then simple = false end
			end
			local parts = {}
			for i = 1, n do parts[i] = r_lit(v[i]) end
			return (simple and "c(" or "list(") .. table.concat(parts, ", ") .. ")"
		end
		local parts = {}
		for k, val in pairs(v) do
			if type(k) == "string" then
				parts[#parts + 1] = "`" .. k .. "` = " .. r_lit(val)
			else
				parts[#parts + 1] = r_lit(val)
			end
		end
		return "list(" .. table.concat(parts, ", ") .. ")"
	end

	error("r.eval: unsupported arg type " .. t)
end

-- named_bindings: a table with only string keys -> "name <- <literal>" lines, one
-- per entry, so r.eval("x + y", { x = 1, y = 3 }) sees x and y as R variables.
-- Names are interpolated, so they must be plain R identifiers; values go through
-- the injection-safe r_lit serializer.
local function named_bindings(t)
	local lines = {}
	for k, v in pairs(t) do
		if not k:match("^[a-zA-Z.][a-zA-Z0-9._]*$") then
			error("r.eval: invalid R variable name " .. string.format("%q", k))
		end
		lines[#lines + 1] = k .. " <- " .. r_lit(v)
	end
	table.sort(lines)
	return table.concat(lines, "\n")
end

local function is_named_table(v)
	if type(v) ~= "table" then
		return false
	end
	local any = false
	for k in pairs(v) do
		if type(k) ~= "string" then
			return false
		end
		any = true
	end
	return any
end

-- the SDK: r.eval(code, ...) -> { stdout, stderr, error, result } with result native
-- Lua. error is nil, or the R error message when the code raised (stop(...)).
-- stderr is R's fd-2 exactly as a REPL would show it (warnings, messages, and R's
-- own "Error: ..." line), untouched by the SDK. Binding of extra args, both forms
-- injection-safe:
--   r.eval(code, {x = 1, y = 3})  -- exactly one string-keyed table: each key
--                                 -- becomes an R variable (x, y)
--   r.eval(code, a, b, ...)       -- anything else: an R list `argv`, referenced
--                                 -- as argv[[1]], argv[[2]], ...
-- R flushes an open graphics device when the session ends; this session is resident and
-- never ends, so a bare plot() would leave its device open and its file empty on disk
-- forever. Closing whatever the code left open makes each call behave like a script run:
-- what was drawn is complete and readable once the call returns. It runs as its own call,
-- after the caller's code and on the error path alike, so the code, its error message and
-- its value all come back untouched.
local close_devices = "while (length(dev.list())) dev.off()"

local r = {}
function r.eval(code, ...)
	local n = select("#", ...)
	if n == 1 and is_named_table((select(1, ...))) then
		code = named_bindings((select(1, ...))) .. "\n" .. code
	elseif n > 0 then
		local parts = {}
		for i = 1, n do parts[i] = r_lit((select(i, ...))) end
		code = "argv <- list(" .. table.concat(parts, ", ") .. ")\n" .. code
	end

	local wire, err = funcs.call("r:run", code)
	if err then
		error("funcs.call(r:run) failed: " .. tostring(err))
	end

	local _, ferr = funcs.call("r:run", close_devices)
	if ferr then
		error("funcs.call(r:run) failed to close the graphics devices: " .. tostring(ferr))
	end

	return (dec(tostring(wire), 1))
end

-- r.plot(code, opts) -> { stdout, stderr, error, result } with result = the
-- rendered image bytes (a Lua string). The code draws on an already-open device;
-- opts: width (px, 800), height (px, 600), dpi (96; res accepted as an alias;
-- scales text/lines on png and the px-to-inch conversion on svg/pdf), format
-- ("png" | "svg" | "pdf"). opts are validated numbers/enum - never spliced as
-- code - and the same named/argv binding rules as r.eval apply to extra args.
local plot_formats = { png = true, svg = true, pdf = true }

function r.plot(code, opts, ...)
	opts = opts or {}
	local format = opts.format or "png"
	if not plot_formats[format] then
		error("r.plot: unsupported format " .. tostring(format))
	end
	local width = math.tointeger(opts.width) or 800
	local height = math.tointeger(opts.height) or 600
	local dpi = math.tointeger(opts.dpi) or math.tointeger(opts.res) or 96
	local open_dev
	if format == "png" then
		open_dev = string.format('png(.plotfile, width = %d, height = %d, res = %d)', width, height, dpi)
	elseif format == "svg" then
		open_dev = string.format('svg(.plotfile, width = %d / %d, height = %d / %d)', width, dpi, height, dpi)
	else
		open_dev = string.format('pdf(.plotfile, width = %d / %d, height = %d / %d)', width, dpi, height, dpi)
	end
	local wrapped = table.concat({
		'.plotfile <- tempfile(fileext = ".' .. format .. '")',
		open_dev,
		'.plotok <- tryCatch({',
		code,
		'; TRUE }, finally = dev.off())',
		'.plotbytes <- readBin(.plotfile, "raw", file.size(.plotfile))',
		'unlink(.plotfile); rm(.plotfile, .plotok)',
		'.plotbytes',
	}, "\n")
	return r.eval(wrapped, ...)
end

return r
