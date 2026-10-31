-- Reduced from spiralscout.quantum:aggregate, lines 72-83 and 241-245.
local function establish(idea_id: string, injected: any?): (any?, string?)
    if idea_id == "" then return nil, "missing idea" end
    return { exec = {}, actor_id = "actor" }, nil
end
local idea_thread = {
    resolve = function(exec: any, actor_id: string, idea_id: string): (string?, string?)
        if idea_id == "missing" then return nil, "missing thread" end
        return "thread", nil
    end,
    append = function(exec: any, thread_id: string, event: string, payload: any): string?
        return nil
    end,
}
local repo = {
    get_idea = function(idea_id: string): (any?, string?)
        if idea_id == "error" then return nil, "read failed" end
        return { source_text = "source" }, nil
    end,
}
local function resolve(idea_id: string, injected: any?): (any?, string?, any?, string?)
    local est, err = establish(idea_id, injected)
    if not est then return nil, nil, nil, err end
    local e: any = est
    local thread_id, terr = idea_thread.resolve(e.exec, e.actor_id, idea_id)
    if not thread_id then return nil, nil, nil, terr end
    local idea, gerr = repo.get_idea(idea_id)
    if gerr then return nil, nil, nil, gerr end
    if not idea then return nil, nil, nil, "idea not found" end
    return e, thread_id, idea, nil
end
local function generate_angles(input: any): (any, string?)
    local idea_id = input.idea_id
    local e, thread_id, idea, err = resolve(idea_id)
    if not e then return { ok = false }, err end
    local aerr = idea_thread.append((e :: any).exec, thread_id, "angles", { idea_id = idea_id })
    if aerr then return { ok = false }, aerr end
    return { ok = true }, nil
end
return generate_angles
