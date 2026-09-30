package regression

import "testing"

func TestGateCollectionEstimateRetainsAppendedEntries(t *testing.T) {
	checkBothModes(t, `
local M = {}
local function entry_of(comp: {id: string})
 if not comp then return nil end
 return {id = comp.id}
end
function M.list(items: {{id: string}})
 local out = {}
 while true do
  for _, comp in ipairs(items) do
   if comp.id ~= "" then out[#out + 1] = entry_of(comp) end
  end
  break
 end
 return out
end
function M.walk(items: {{id: string}})
 local children = M.list(items)
 for _, child in ipairs(children or {}) do
  print(child.id)
 end
end
return M`, "")
}

func TestGateCollectionEstimateRejectsWrongAppendedEntry(t *testing.T) {
	checkBothModes(t, `
local function consume(id: string) end
local function run(items: {{id: number}})
 local out = {}
 for _, item in ipairs(items) do out[#out+1] = item end
 for _, item in ipairs(out) do consume(item.id) end
end
return run`, "expected string")
}

func TestGateDynamicListEstimateRetainsAppendedEntries(t *testing.T) {
	checkBothModes(t, `
local M = {}
local function entry_of(comp, kind)
 if not comp then return nil end
 return {id = comp.id, name = kind.name}
end
function M.list(service: any)
 local out = {}
 while true do
  local result, err = service:list_children({})
  if err then return nil, tostring(err) end
  local rows = (result and (result.children or result.components or result.items)) or result or {}
  for _, comp in ipairs(rows) do
   local kind = service:kind(comp.impl_id)
   if kind then out[#out+1] = entry_of(comp, kind) end
  end
  if #rows < 100 then break end
 end
 return out
end
function M.walk(service: any)
 local function walk(id): string?
  local children, err = M.list(service)
  if err then return tostring(err) end
  for _, child in ipairs(children or {}) do
   local value, gerr = service:get({id = child.id})
   if gerr or not value then return tostring(child.id) end
   local werr = walk(child.id)
   if werr then return werr end
  end
 end
 return walk(1)
end
return M`, "")
}

func TestGateImportedListEstimateRetainsAppendedEntries(t *testing.T) {
	checkBothModes(t, `
local component = require("component")
local kinds = require("kinds")
local M = {}
local function svc()
 local s, err = component.get_service()
 if err or not s then return nil, tostring(err) end
 return s
end
local function entry_of(comp, km)
 if not comp then return nil end
 local meta = comp.meta or {}
 return {id = comp.component_id or comp.id, name = meta.title or "Untitled", renderer = km and km.renderer or nil}
end
function M.list(parent_id)
 local s, err = svc()
 if not s then return nil, err end
 local out = {}
 local offset = 0
 while true do
  local req = {pagination = {limit = 100, offset = offset}}
  if parent_id and parent_id ~= "" then req.parent_id = parent_id end
  local result, lerr = s:list_children(req)
  if lerr then return nil, tostring(lerr) end
  local rows = (result and (result.children or result.components or result.items)) or result or {}
  for _, comp in ipairs(rows) do
   local km = kinds.kind_by_impl(comp.impl_id)
   if km then out[#out+1] = entry_of(comp, km) end
  end
  if #rows < 100 then break end
  offset = offset + 100
 end
 return out
end
function M.delete(id)
 local s, serr = svc()
 if not s then return nil, serr end
 local order = {}
 local function walk(node_id): string?
  local children, lerr = M.list(node_id)
  if lerr then return tostring(lerr) end
  for _, k in ipairs(children or {}) do
   local child, gerr = s:get({component_id = k.id})
   if gerr or not child then return "failed " .. tostring(k.id) end
   local werr = walk(k.id)
   if werr then return werr end
  end
  order[#order+1] = node_id
 end
 local werr = walk(id)
 if werr then return nil, werr end
 return order
end
return M`, "")
}
