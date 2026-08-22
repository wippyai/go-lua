-- Contract: the Wippy v1 process module as the runtime declares it. Every
-- declared member of the module, of the registry sub-table, of the spawn
-- builder, and of the Message and Event handles the actor mailboxes carry is
-- called, and every member whose final result is the module error is read on
-- both of its arms.

local process = require("process")

local scopes: number = process.registry.LOCAL + process.registry.EVENTUAL +
    process.registry.CONSISTENT + process.registry.STRONG
local event_kinds: string = process.event.CANCEL .. process.event.EXIT ..
    process.event.LINK_DOWN .. process.event.OUTDATED

local id, id_err = process.id()
if id_err ~= nil then
    return id_err:kind()
end

local self_pid, pid_err = process.pid()
if pid_err ~= nil then
    return pid_err:message()
end

local registered, register_err = process.registry.register("worker")
if register_err ~= nil then
    return register_err:message()
end
local scoped, scoped_register_err = process.registry.register("worker", self_pid, process.registry.STRONG)
if scoped_register_err ~= nil then
    return scoped_register_err:message()
end

local worker, lookup_err = process.registry.lookup("worker")
if lookup_err ~= nil then
    return lookup_err:message()
end

local unregistered, unregister_err = process.registry.unregister("worker")
if unregister_err ~= nil then
    return unregister_err:message()
end
local scoped_unregistered, scoped_unregister_err = process.registry.unregister("worker", process.registry.LOCAL)
if scoped_unregister_err ~= nil then
    return scoped_unregister_err:message()
end

local delivered, send_err = process.send(worker, "job", { key = "alpha" }, 7)
if send_err ~= nil then
    return send_err:message()
end

local child, spawn_err = process.spawn("app:worker", "run", "alpha")
if spawn_err ~= nil then
    return spawn_err:message()
end
local monitored, monitored_err = process.spawn_monitored("app:worker", "run")
if monitored_err ~= nil then
    return monitored_err:message()
end
local linked, linked_err = process.spawn_linked("app:worker", "run")
if linked_err ~= nil then
    return linked_err:message()
end
local both, both_err = process.spawn_linked_monitored("app:worker", "run")
if both_err ~= nil then
    return both_err:message()
end

local monitor_ok, monitor_err = process.monitor(child)
if monitor_err ~= nil then
    return monitor_err:message()
end
local unmonitor_ok, unmonitor_err = process.unmonitor(monitored)
if unmonitor_err ~= nil then
    return unmonitor_err:message()
end
local link_ok, link_err = process.link(child)
if link_err ~= nil then
    return link_err:message()
end
local unlink_ok, unlink_err = process.unlink(linked)
if unlink_err ~= nil then
    return unlink_err:message()
end

local cancelled, cancel_err = process.cancel(both)
if cancel_err ~= nil then
    return cancel_err:message()
end
local reasoned, reasoned_cancel_err = process.cancel(both, "shutdown")
if reasoned_cancel_err ~= nil then
    return reasoned_cancel_err:message()
end
local terminated, terminate_err = process.terminate(child)
if terminate_err ~= nil then
    return terminate_err:message()
end

local options = process.get_options()
local trapping: boolean = options.trap_links
local upgradable: boolean = options.upgradable
local options_set, set_options_err = process.set_options({ trap_links = true })
if set_options_err ~= nil then
    return set_options_err:message()
end

local upgraded, upgrade_err = process.upgrade()
if upgrade_err ~= nil then
    return upgrade_err:message()
end
local upgraded_to, upgrade_to_err = process.upgrade("app:worker.v2", "alpha")
if upgrade_to_err ~= nil then
    return upgrade_to_err:message()
end

local executed, exec_err = process.exec("app:worker", "run", "alpha")
if exec_err ~= nil then
    return exec_err:message()
end

local topic_channel, listen_err = process.listen("jobs")
if listen_err ~= nil then
    return listen_err:message()
end
local message_channel, message_listen_err = process.listen("jobs", { message = true })
if message_listen_err ~= nil then
    return message_listen_err:message()
end
local unlistened, unlisten_err = process.unlisten(topic_channel)
if unlisten_err ~= nil then
    return unlisten_err:message()
end

-- The builder answers itself from every with_*, so the whole chain is one
-- expression that ends in a spawn variant.
local builder = process.with_context({ tenant = "acme" })
local configured = process.with_options({ trap_links = true })

local built, built_err = builder
    :with_context({ tenant = "acme" })
    :with_options({ upgradable = true })
    :with_actor("supervisor")
    :with_scope(process.registry.LOCAL)
    :with_name("worker-1")
    :with_message("boot", "alpha")
    :spawn("app:worker", "run", "alpha")
if built_err ~= nil then
    return built_err:message()
end
local built_monitored, built_monitored_err = configured:spawn_monitored("app:worker", "run")
if built_monitored_err ~= nil then
    return built_monitored_err:message()
end
local built_linked, built_linked_err = configured:spawn_linked("app:worker", "run")
if built_linked_err ~= nil then
    return built_linked_err:message()
end
local built_both, built_both_err = configured:spawn_linked_monitored("app:worker", "run")
if built_both_err ~= nil then
    return built_both_err:message()
end
local built_exec, built_exec_err = configured:exec("app:worker", "run")
if built_exec_err ~= nil then
    return built_exec_err:message()
end

local inbox = process.inbox()
local message, message_ok = inbox:receive()
local sender: string = ""
local topic: string = ""
if message_ok then
    sender = message:from()
    topic = message:topic()
    local payload = message:payload()
    if payload ~= nil then
        topic = topic .. tostring(payload)
    end
end

local events = process.events()
local event, event_ok = events:receive()
local event_kind: string = ""
if event_ok then
    event_kind = event.kind .. event.from
    if event.reason ~= nil then
        event_kind = event_kind .. event.reason
    end
    if event.result ~= nil then
        event_kind = event_kind .. tostring(event.result)
    end
    if event.error ~= nil then
        event_kind = event_kind .. tostring(event.error)
    end
    -- Event.payload answers an optional value, so the nil is tested before it
    -- reaches a consumer.
    local event_payload = event:payload()
    if event_payload ~= nil then
        event_kind = event_kind .. tostring(event_payload)
    end
end

if not (registered and scoped and unregistered and scoped_unregistered and delivered and
    monitor_ok and unmonitor_ok and link_ok and unlink_ok and cancelled and reasoned and
    terminated and options_set and upgraded and upgraded_to and unlistened and
    trapping and upgradable) then
    return "incomplete"
end
return id .. self_pid .. worker .. child .. monitored .. linked .. both ..
    built .. built_monitored .. built_linked .. built_both ..
    sender .. topic .. event_kind .. event_kinds .. tostring(scopes) ..
    tostring(executed) .. tostring(built_exec) ..
    tostring(message_channel) .. tostring(topic_channel)
