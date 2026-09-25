-- Source: app:automation_test, lines 2660-2677 and 2691-2695.
local rows: any = {}
                local workflow_versions = type((rows :: any)["__workflow_versions"]) == "table" and (rows :: any)["__workflow_versions"] or {
                    ["wf-1"] = {
                        [3] = {
                            version = 3,
                            document = '{"nodes":[{"id":"start","kind":"start","config":{"input_schema":{"type":"object","required":["contact_id"],"properties":{"contact_id":{"type":"string"},"requested_by":{"type":"string"}},"additionalProperties":true}}}]}',
                        },
                        [4] = {
                            version = 4,
                            document = '{"nodes":[{"id":"start","kind":"start","config":{"input_schema":{"type":"object","required":["contact_id"],"properties":{"contact_id":{"type":"string"},"requested_by":{"type":"string"}},"additionalProperties":true}}}]}',
                        },
                    },
                    ["wf-legacy"] = {
                        [1] = {
                            version = 1,
                            document = '{"nodes":[{"id":"start","kind":"start","config":{"input_schema":{"type":"object","additionalProperties":true}}}]}',
                        },
                    },
                }
local workflow_id = "wf-1"
local requested_version = tonumber("4")
                            local versions = type(workflow_versions[workflow_id]) == "table" and workflow_versions[workflow_id] or {}
                            if requested_version then
                                local row = versions[requested_version]
                                return row and { row } or {}, nil
                            end
