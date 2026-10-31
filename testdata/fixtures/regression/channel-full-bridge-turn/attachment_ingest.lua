local M = {}

function M.ingest(attachment)
    if type(attachment) == "table" and type(attachment.upload_id) == "string" and attachment.upload_id ~= "" then
        return { uuid = attachment.upload_id }, nil
    end
    return nil, "attachment ingest disabled in channel bridge tests"
end

return M

