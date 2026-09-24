local json = require("json")
local consts = require("consts")
local contract = require("contract")

type BuildOptions = {
    include_contexts: boolean?,
    include_files: boolean?,
    cache_markers: boolean?,
}

local prompt_builder = {
    _prompt = require("prompt")
}

-- The session-owned, optional file-provider contract. An application that stores
-- uploads binds it (e.g. an uploads module) so the session can resolve a file_uuid to
-- its metadata WITHOUT the session depending on any concrete uploads module. Modeled on
-- wippy.agent:resolver: consumed only when something binds it; otherwise the caller
-- falls back to the injected options below, so apps that never bound it keep working.
local FILE_PROVIDER_CONTRACT = "wippy.session:file_provider"

-- resolve_via_contract returns the upload record for file_uuid through the file_provider
-- contract, or nil when no application binds it (the optional-contract pattern: inspect
-- implementations() first, fall back when none). Swappable for tests via the seam below.
prompt_builder._contract = contract
local function resolve_via_contract(file_uuid: string): any
    local def, err = prompt_builder._contract.get(FILE_PROVIDER_CONTRACT)
    if err or not def then
        return nil
    end
    local impls, impl_err = (def :: any):implementations()
    if impl_err or type(impls) ~= "table" or #(impls :: { any }) == 0 then
        -- Contract defined but unbound: this app provides no uploads. Fall back.
        return nil
    end
    local inst, open_err = (def :: any):open()
    if open_err or not inst then
        return nil
    end
    local ok, info = pcall(function() return (inst :: any):get_info({ file_uuid = file_uuid }) end)
    if not ok or type(info) ~= "table" then
        return nil
    end
    return info
end

local function resolve_file(file_uuid: string, options: table)
    -- 1. Canonical: the session's file_provider contract, when an app binds one.
    local via_contract = resolve_via_contract(file_uuid)
    if via_contract ~= nil then
        return via_contract
    end

    -- 2. Fallback (preserves prior behavior for apps that bind no contract): an
    -- explicitly injected resolver function or upload_repo passed through options.
    local resolver = options.file_resolver or options.file_lookup
    if type(resolver) == "function" then
        local ok, upload_or_err, err = pcall(resolver, file_uuid)
        if ok and not err and upload_or_err then
            return upload_or_err
        end
    end

    local upload_repo = options.upload_repo
    if type(upload_repo) == "table" and type(upload_repo.get) == "function" then
        local ok, upload, err = pcall(upload_repo.get, file_uuid)
        if ok and not err and upload then
            return upload
        end
    end

    return nil
end

function prompt_builder.build(messages, contexts, session_meta, options)
    if not messages then
        return nil, "Messages are required"
    end

    options = options or {}
    local include_contexts = options.include_contexts ~= false
    local include_files = options.include_files ~= false
    local cache_markers = options.cache_markers ~= false

    local builder = prompt_builder._prompt.new()

    if include_contexts and contexts and #contexts > 0 then
        local memory_text = "Session context memory:\n\n"
        for _, context in ipairs(contexts) do
            memory_text = memory_text .. "## " .. context.type .. "\n" .. context.text .. "\n\n"
        end
        builder:add_system(memory_text)

        if cache_markers then
            builder:add_cache_marker("context_memories")
        end
    end

    for i, msg in ipairs(messages) do
        local metadata: table = msg.metadata or {}

        if msg.type == consts.MSG_TYPE.SYSTEM then
            -- for internal use only, use developer role for ongoing system messages
        elseif msg.type == consts.MSG_TYPE.USER then
            builder:add_user(msg.data :: string)

            if include_files and metadata.file_uuids and #metadata.file_uuids > 0 then
                local file_info = {}
                for _, file_uuid in ipairs(metadata.file_uuids) do
                    if type(file_uuid) == "string" then
                        local upload = resolve_file(file_uuid, options)
                        table.insert(file_info, {
                            filename = upload and upload.metadata and upload.metadata.filename or "Unknown filename",
                            size = upload and upload.size or 0,
                            type = upload and upload.mime_type or "Unknown type",
                            uuid = file_uuid
                        })
                    end
                end

                if #file_info > 0 then
                    local files_text = "User attached the following files:\n"
                    for _, file in ipairs(file_info) do
                        files_text = files_text .. string.format(
                            "- %s (Type: %s, Size: %d bytes, ID: %s)\n",
                            file.filename, file.type, file.size, file.uuid
                        )
                    end
                    builder:add_developer(files_text)
                end
            end

            if cache_markers and metadata.last_checkpoint then
                builder:add_cache_marker("checkpoint_" .. msg.message_id)
            end
        elseif msg.type == consts.MSG_TYPE.ASSISTANT then
            -- Always use add_assistant and let prompt library handle thinking blocks internally
            builder:add_assistant(msg.data :: string, metadata)
        elseif msg.type == consts.MSG_TYPE.DEVELOPER then
            builder:add_developer(msg.data :: string, metadata)
        elseif
            msg.type == consts.MSG_TYPE.FUNCTION
            or msg.type == consts.MSG_TYPE.PRIVATE_FUNCTION
            or msg.type == consts.MSG_TYPE.DELEGATION
        then
            local func_name = tostring(metadata.function_name)
            if func_name ~= "" and metadata.status then
                local args = msg.data
                if type(args) == "string" then
                    local parsed, parse_err = json.decode(args)
                    if not parse_err then
                        args = parsed
                    end
                end

                local llm_call_id = tostring(metadata.call_id or msg.message_id)
                local opts: {provider_metadata: table?}? = nil
                if type(metadata.provider_metadata) == "table" then
                    opts = { provider_metadata = metadata.provider_metadata }
                end
                builder:add_function_call(func_name, args :: string, llm_call_id, opts)

                if metadata.status == consts.FUNC_STATUS.PENDING then
                    builder:add_function_result(func_name, "incomplete", llm_call_id)
                elseif metadata.status == consts.FUNC_STATUS.SUCCESS or
                    metadata.status == consts.FUNC_STATUS.ERROR then
                    local result_content = metadata.result
                    if type(result_content) == "table" then
                        result_content = json.encode(result_content)
                    elseif result_content == nil then
                        result_content = "nil"
                    else
                        result_content = tostring(result_content)
                    end
                    builder:add_function_result(func_name, tostring(result_content), llm_call_id)
                end
            end
        elseif msg.type == consts.MSG_TYPE.ARTIFACT then
            if msg.data and msg.data ~= "" then
                builder:add_developer("Artifact: " .. msg.data, metadata)
            end
        end
    end

    return builder, nil
end

function prompt_builder.from_session(session, options)
    if not session then
        return nil, "Session reader is required"
    end

    local messages, err = session:messages():from_checkpoint():all()
    if err then
        return nil, "Failed to load messages: " .. err
    end

    local contexts, err = session:contexts():all()
    if err then
        return nil, "Failed to load contexts: " .. err
    end

    local session_meta = session:state()

    return prompt_builder.build(messages, contexts, session_meta, options)
end

return prompt_builder
