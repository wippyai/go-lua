local http_client = require("http_client")
local json = require("json")
local types = require("types")

local M = {}

local function headers(api_key: string): { [string]: string }
    return {
        ["Authorization"] = "Bearer " .. api_key,
        ["Accept"] = "application/json",
        ["Content-Type"] = "application/json",
    }
end
M.headers = headers

-- The GraphQL request body. `variables` is omitted entirely when empty: an empty
-- Lua table encodes as a JSON array ([]), which Fireflies rejects ("`variables`
-- must be an object"); a non-empty table has string keys and encodes as an object.
function M.build_body(query: string, variables: any?): string
    local payload: { [string]: any } = { query = query }
    if type(variables) == "table" and next(variables :: { [string]: any }) ~= nil then
        payload.variables = variables
    end
    return json.encode(payload) :: string
end

function M.extract_http_error(response: any): (string, integer)
    local status_code = (response and response.status_code or 0) :: integer
    if status_code == 401 then
        return "unauthorized (check the Fireflies API key)", status_code
    end
    if status_code == 403 then
        return "forbidden", status_code
    end
    if status_code == 404 then
        return "not found", status_code
    end
    if status_code == 429 then
        return "rate limited", status_code
    end
    local message = "request failed"
    if response and response.body and response.body ~= "" then
        local body = response.body :: string
        message = #body > 300 and (body:sub(1, 300) .. "...") or body
    end
    return message, status_code
end

-- A GraphQL `errors[0].message`, if the response carried one. Fireflies returns
-- HTTP 200 with an `errors` array for query-level failures (e.g. a bad key), so
-- this is checked independently of the status code.
function M.graphql_error(decoded: any): string?
    if type(decoded) ~= "table" then
        return nil
    end
    local errs: any = (decoded :: any).errors
    if type(errs) == "table" and errs[1] then
        local first: any = errs[1]
        return tostring((type(first) == "table" and first.message) or first)
    end
    return nil
end

-- graphql(conn, query, variables) -> Result. POSTs to the Fireflies GraphQL
-- endpoint; on success data is the GraphQL `data` object.
function M.graphql(conn: types.Conn, query: string, variables: any?): types.Result
    local response, err = http_client.request("POST", types.API_URL, {
        headers = headers(conn.api_key),
        body = M.build_body(query, variables),
    })
    if err then
        return { success = false, error = tostring(err), status_code = 0 } :: types.Result
    end

    local decoded: any = nil
    if response and response.body and response.body ~= "" then
        decoded = json.decode(response.body :: string)
    end

    local gql = M.graphql_error(decoded)
    if gql then
        return {
            success = false,
            error = gql,
            status_code = (response and response.status_code or 0) :: integer,
        } :: types.Result
    end

    if response and response.status_code >= 200 and response.status_code < 300 then
        local data: any = type(decoded) == "table" and (decoded :: any).data or nil
        return { success = true, data = data, status_code = response.status_code :: integer } :: types.Result
    end

    local message, code = M.extract_http_error(response)
    return { success = false, error = message, status_code = code } :: types.Result
end

-- ── Typed queries ──────────────────────────────────────────────────────
function M.get_user(conn: types.Conn): types.Result
    return M.graphql(conn, "{ user { name email user_id num_transcripts } }", nil)
end

function M.test_connection(conn: types.Conn): types.Result
    return M.get_user(conn)
end

local LIST_QUERY =
    "query L($limit: Int, $skip: Int) { transcripts(limit: $limit, skip: $skip) { id title date dateString duration host_email organizer_email participants is_live speakers { name } meeting_attendees { displayName email } } }"
function M.list_transcripts(conn: types.Conn, variables: any?): types.Result
    return M.graphql(conn, LIST_QUERY, variables or {})
end

local GET_QUERY =
    "query T($id: String!) { transcript(id: $id) { id title date dateString duration host_email organizer_email transcript_url meeting_attendees { displayName email } summary { overview keywords action_items outline topics_discussed } } }"
function M.get_transcript(conn: types.Conn, id: string): types.Result
    return M.graphql(conn, GET_QUERY, { id = id })
end

local TEXT_QUERY =
    "query S($id: String!) { transcript(id: $id) { id title sentences { index speaker_name text start_time } } }"
function M.get_transcript_sentences(conn: types.Conn, id: string): types.Result
    return M.graphql(conn, TEXT_QUERY, { id = id })
end

-- ── The one write ──────────────────────────────────────────────────────
-- Fireflies exposes exactly one mutation that changes what a transcript SAYS
-- about itself: its title. There is no mutation for speaker names and none for
-- transcript text, so a rename is the whole of the write-back surface and
-- anything else a caller might want to push has to stay a local override.
--
-- The API refuses the rename for a non-admin key and for a transcript whose
-- owner is outside the key's team; both come back as a GraphQL error on an HTTP
-- 200, which graphql() already turns into an unsuccessful result.
local UPDATE_TITLE_MUTATION =
    "mutation UpdateMeetingTitle($input: UpdateMeetingTitleInput!) { updateMeetingTitle(input: $input) { title } }"
M.UPDATE_TITLE_MUTATION = UPDATE_TITLE_MUTATION

function M.update_meeting_title(conn: types.Conn, id: string, title: string): types.Result
    return M.graphql(conn, UPDATE_TITLE_MUTATION, { input = { id = id, title = title } })
end

return M
