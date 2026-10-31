local test = require("test")
local json = require("json")
local consts = require("consts")
local prompt_builder = require("prompt_builder")

local function define_tests()
    describe("Prompt Builder", function()
        describe("provider_metadata in function calls", function()
            it("should pass provider_metadata to function call when present", function()
                local messages = {
                    {
                        message_id = "msg-1",
                        type = consts.MSG_TYPE.FUNCTION,
                        data = json.encode({ query = "test" }),
                        metadata = {
                            function_name = "search",
                            call_id = "call-1",
                            registry_id = "reg-1",
                            status = consts.FUNC_STATUS.SUCCESS,
                            result = "found it",
                            provider_metadata = {
                                anthropic = { citations = { enabled = true } }
                            }
                        }
                    }
                }

                local builder, err = prompt_builder.build(messages, {}, {}, {
                    include_contexts = false,
                    include_files = false,
                    cache_markers = false
                })

                test.is_nil(err)
                test.not_nil(builder)

                local built = builder:get_messages()
                test.ok(#built >= 2)

                -- First message should be the function call
                local fc_msg = built[1]
                test.eq(fc_msg.role, "function_call")
                test.not_nil(fc_msg.function_call)
                test.eq(fc_msg.function_call.name, "search")
                test.eq(fc_msg.function_call.id, "call-1")
                test.not_nil(fc_msg.function_call.provider_metadata)
                test.not_nil(fc_msg.function_call.provider_metadata.anthropic)
                test.is_true(fc_msg.function_call.provider_metadata.anthropic.citations.enabled)

                -- Second message should be the function result
                local fr_msg = built[2]
                test.eq(fr_msg.role, "function_result")
                test.eq(fr_msg.name, "search")
            end)

            it("should not set provider_metadata when absent", function()
                local messages = {
                    {
                        message_id = "msg-2",
                        type = consts.MSG_TYPE.FUNCTION,
                        data = json.encode({ query = "test" }),
                        metadata = {
                            function_name = "search",
                            call_id = "call-2",
                            registry_id = "reg-1",
                            status = consts.FUNC_STATUS.SUCCESS,
                            result = "found it"
                        }
                    }
                }

                local builder, err = prompt_builder.build(messages, {}, {}, {
                    include_contexts = false,
                    include_files = false,
                    cache_markers = false
                })

                test.is_nil(err)
                test.not_nil(builder)

                local built = builder:get_messages()
                local fc_msg = built[1]
                test.eq(fc_msg.role, "function_call")
                test.is_nil(fc_msg.function_call.provider_metadata)
            end)

            it("should ignore non-table provider_metadata", function()
                local messages = {
                    {
                        message_id = "msg-3",
                        type = consts.MSG_TYPE.FUNCTION,
                        data = json.encode({ action = "run" }),
                        metadata = {
                            function_name = "execute",
                            call_id = "call-3",
                            status = consts.FUNC_STATUS.PENDING,
                            provider_metadata = "not_a_table"
                        }
                    }
                }

                local builder, err = prompt_builder.build(messages, {}, {}, {
                    include_contexts = false,
                    include_files = false,
                    cache_markers = false
                })

                test.is_nil(err)

                local built = builder:get_messages()
                local fc_msg = built[1]
                test.eq(fc_msg.role, "function_call")
                test.is_nil(fc_msg.function_call.provider_metadata)
            end)

            it("should handle provider_metadata for private function type", function()
                local messages = {
                    {
                        message_id = "msg-4",
                        type = consts.MSG_TYPE.PRIVATE_FUNCTION,
                        data = json.encode({ input = "data" }),
                        metadata = {
                            function_name = "internal_tool",
                            call_id = "call-4",
                            status = consts.FUNC_STATUS.SUCCESS,
                            result = "done",
                            provider_metadata = {
                                custom = { key = "value" }
                            }
                        }
                    }
                }

                local builder, err = prompt_builder.build(messages, {}, {}, {
                    include_contexts = false,
                    include_files = false,
                    cache_markers = false
                })

                test.is_nil(err)

                local built = builder:get_messages()
                local fc_msg = built[1]
                test.not_nil(fc_msg.function_call.provider_metadata)
                test.eq(fc_msg.function_call.provider_metadata.custom.key, "value")
            end)

            it("should handle provider_metadata for delegation type", function()
                local messages = {
                    {
                        message_id = "msg-5",
                        type = consts.MSG_TYPE.DELEGATION,
                        data = json.encode({ task = "delegate" }),
                        metadata = {
                            function_name = "delegate_agent",
                            call_id = "call-5",
                            status = consts.FUNC_STATUS.SUCCESS,
                            result = "delegated",
                            provider_metadata = {
                                routing = { priority = 1 }
                            }
                        }
                    }
                }

                local builder, err = prompt_builder.build(messages, {}, {}, {
                    include_contexts = false,
                    include_files = false,
                    cache_markers = false
                })

                test.is_nil(err)

                local built = builder:get_messages()
                local fc_msg = built[1]
                test.not_nil(fc_msg.function_call.provider_metadata)
                test.eq(fc_msg.function_call.provider_metadata.routing.priority, 1)
            end)
        end)

        describe("function call statuses", function()
            it("should add incomplete result for pending status", function()
                local messages = {
                    {
                        message_id = "msg-10",
                        type = consts.MSG_TYPE.FUNCTION,
                        data = json.encode({ q = "test" }),
                        metadata = {
                            function_name = "slow_tool",
                            call_id = "call-10",
                            status = consts.FUNC_STATUS.PENDING
                        }
                    }
                }

                local builder, err = prompt_builder.build(messages, {}, {}, {
                    include_contexts = false,
                    include_files = false,
                    cache_markers = false
                })

                test.is_nil(err)

                local built = builder:get_messages()
                test.eq(#built, 2)

                test.eq(built[1].role, "function_call")
                test.eq(built[2].role, "function_result")
                test.eq(built[2].content[1].text, "incomplete")
            end)

            it("should add result content for success status", function()
                local messages = {
                    {
                        message_id = "msg-11",
                        type = consts.MSG_TYPE.FUNCTION,
                        data = json.encode({ q = "test" }),
                        metadata = {
                            function_name = "fast_tool",
                            call_id = "call-11",
                            status = consts.FUNC_STATUS.SUCCESS,
                            result = "result text"
                        }
                    }
                }

                local builder, err = prompt_builder.build(messages, {}, {}, {
                    include_contexts = false,
                    include_files = false,
                    cache_markers = false
                })

                test.is_nil(err)

                local built = builder:get_messages()
                test.eq(#built, 2)
                test.eq(built[2].role, "function_result")
                test.eq(built[2].content[1].text, "result text")
            end)

            it("should use call_id as function_call_id", function()
                local messages = {
                    {
                        message_id = "msg-12",
                        type = consts.MSG_TYPE.FUNCTION,
                        data = json.encode({}),
                        metadata = {
                            function_name = "tool",
                            call_id = "specific-call-id",
                            status = consts.FUNC_STATUS.SUCCESS,
                            result = "ok"
                        }
                    }
                }

                local builder, err = prompt_builder.build(messages, {}, {}, {
                    include_contexts = false,
                    include_files = false,
                    cache_markers = false
                })

                test.is_nil(err)

                local built = builder:get_messages()
                test.eq(built[1].function_call.id, "specific-call-id")
                test.eq(built[2].function_call_id, "specific-call-id")
            end)

            it("should fallback to message_id when call_id is absent", function()
                local messages = {
                    {
                        message_id = "msg-13",
                        type = consts.MSG_TYPE.FUNCTION,
                        data = json.encode({}),
                        metadata = {
                            function_name = "tool",
                            status = consts.FUNC_STATUS.SUCCESS,
                            result = "ok"
                        }
                    }
                }

                local builder, err = prompt_builder.build(messages, {}, {}, {
                    include_contexts = false,
                    include_files = false,
                    cache_markers = false
                })

                test.is_nil(err)

                local built = builder:get_messages()
                test.eq(built[1].function_call.id, "msg-13")
                test.eq(built[2].function_call_id, "msg-13")
            end)
        end)

        describe("build with nil messages", function()
            it("should return error when messages is nil", function()
                local builder, err = prompt_builder.build(nil, {}, {})
                test.is_nil(builder)
                test.contains(tostring(err), "Messages are required")
            end)
        end)

        describe("file resolution via the file_provider contract", function()
            local original_contract = prompt_builder._contract

            -- Stub the contract seam: impls drives the implementations() check (empty =>
            -- nobody bound it => fall back), get_info returns the upload record.
            local function stub_contract(impls: { any }, get_info: any): any
                return {
                    get = function(_id)
                        return {
                            implementations = function(_self) return impls end,
                            open = function(_self)
                                return { get_info = function(_self2, args) return get_info(args) end }, nil
                            end,
                        }, nil
                    end,
                }
            end

            local function user_with_file(uuid)
                return {
                    { message_id = "m1", type = consts.MSG_TYPE.USER, data = "see attached", metadata = { file_uuids = { uuid } } },
                }
            end

            local function all_text(built)
                local parts = {}
                for _, m in ipairs(built or {}) do
                    local c = m.content or m.text or m.data
                    if type(c) == "string" then
                        parts[#parts + 1] = c
                    elseif type(c) == "table" then
                        parts[#parts + 1] = json.encode(c)
                    end
                end
                return table.concat(parts, "\n")
            end

            it("resolves a file through the contract when an app binds one", function()
                prompt_builder._contract = stub_contract({ "userspace.uploads:file_provider" }, function(args)
                    if args.file_uuid == "file-1" then
                        return { size = 1234, mime_type = "text/plain", metadata = { filename = "notes.txt" } }
                    end
                    return nil
                end)

                local builder, err = prompt_builder.build(user_with_file("file-1"), {}, {}, {})
                prompt_builder._contract = original_contract

                test.is_nil(err)
                test.not_nil(builder)
                test.contains(all_text(builder:get_messages()), "notes.txt")
            end)

            it("falls back to options.upload_repo when nothing binds the contract", function()
                -- Empty implementations => contract path is a no-op => fallback runs.
                prompt_builder._contract = stub_contract({}, function() return nil end)

                local builder, err = prompt_builder.build(user_with_file("file-2"), {}, {}, {
                    upload_repo = {
                        get = function(uuid)
                            if uuid == "file-2" then
                                return { size = 9, mime_type = "image/png", metadata = { filename = "pic.png" } }
                            end
                            return nil
                        end,
                    },
                })
                prompt_builder._contract = original_contract

                test.is_nil(err)
                test.contains(all_text(builder:get_messages()), "pic.png")
            end)

            it("renders Unknown filename when neither the contract nor options resolve", function()
                prompt_builder._contract = stub_contract({}, function() return nil end)

                local builder, err = prompt_builder.build(user_with_file("file-3"), {}, {}, {})
                prompt_builder._contract = original_contract

                test.is_nil(err)
                test.contains(all_text(builder:get_messages()), "Unknown filename")
            end)
        end)
    end)
end

return test.run_cases(define_tests)
