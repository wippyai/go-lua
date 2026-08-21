-- Distilled from notify_cache.lua:15-56 (kickside placement benchmark).
-- A TTL registry: cache table and entries are actor-local; lookups are frame-local borrows.
type Entry = { value: string, expires: number }

local registry: {[string]: Entry} = {}   -- OwnedHeap module cache

local function put(key: string, value: string, ttl: number)
    local entry: Entry = { value = value, expires = ttl }  -- stored into cache -> OwnedHeap
    registry[key] = entry
end

local function get(key: string, now: number): string?
    local hit: Entry? = registry[key]     -- Stack borrow: dies at return
    if hit ~= nil and hit.expires > now then
        return hit.value                  -- returns a scalar string; the entry stays actor-local
    end
    return nil
end

local M = { put = put, get = get }
return M
