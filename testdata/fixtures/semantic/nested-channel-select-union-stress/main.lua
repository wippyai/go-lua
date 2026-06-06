type AuditLeaf = {
    kind: "audit",
    audit: {
        summary: {
            actor: {
                id: string,
                level: number,
            },
        },
    },
}

type MetricLeaf = {
    kind: "metric",
    metric: {
        sample: {
            bucket: {
                name: string,
                value: number,
            },
        },
    },
}

type FaultLeaf = {
    kind: "fault",
    fault: {
        cause: {
            source: {
                code: string,
                retryable: boolean,
            },
        },
    },
}

type Leaf = AuditLeaf | MetricLeaf | FaultLeaf

type PrimaryPayload = {
    kind: "primary",
    frame: {
        stage: {
            shard: {
                route: {
                    leaf: Leaf,
                },
            },
        },
    },
}

type MirrorPayload = {
    kind: "mirror",
    frame: {
        stage: {
            shard: {
                route: {
                    nested: PrimaryPayload | FaultLeaf,
                },
            },
        },
    },
}

type TombstonePayload = {
    kind: "tombstone",
    tombstone: {
        reason: string,
    },
}

type ControlPayload = {
    kind: "control",
    command: {
        name: string,
        priority: number,
    },
}

type DeadlinePayload = {
    kind: "deadline",
    deadline: {
        tick: number,
    },
}

type RouteA = {
    kind: "route_a",
    ch: Channel<Leaf | ControlPayload>,
}

type RouteB = {
    kind: "route_b",
    ch: Channel<MirrorPayload | DeadlinePayload>,
}

type StreamPayload = {
    kind: "stream",
    router: {
        selected: RouteA | RouteB,
        fallback: Channel<DeadlinePayload | ControlPayload>,
    },
}

type BoxPayload = {
    kind: "box",
    node: {
        left: Channel<RouteA | RouteB>,
        right: Channel<Leaf | DeadlinePayload>,
        next: BoxPayload | StreamPayload | TombstonePayload,
    },
}

type EventPayload = PrimaryPayload | MirrorPayload | TombstonePayload | StreamPayload | BoxPayload

local function read_leaf(leaf: Leaf): string
    if leaf.kind == "audit" then
        local actor: string = leaf.audit.summary.actor.id
        local wrong_actor: number = leaf.audit.summary.actor.id -- expect-error
        return actor
    end

    if leaf.kind == "metric" then
        local bucket: string = leaf.metric.sample.bucket.name
        local value: number = leaf.metric.sample.bucket.value
        local wrong_value: string = leaf.metric.sample.bucket.value -- expect-error
        return bucket .. ":" .. tostring(value)
    end

    local code: string = leaf.fault.cause.source.code
    local retryable: boolean = leaf.fault.cause.source.retryable
    local wrong_retryable: string = leaf.fault.cause.source.retryable -- expect-error
    return code .. ":" .. tostring(retryable)
end

local function analyze(
    events: Channel<EventPayload>,
    controls: Channel<ControlPayload>,
    deadlines: Channel<DeadlinePayload>
): string
    local selected = channel.select {
        events:case_receive(),
        controls:case_receive(),
        deadlines:case_receive(),
    }

    if selected.channel == events then
        local payload = selected.value
        local wrong_control: ControlPayload = selected.value -- expect-error

        if payload.kind == "primary" then
            local leaf = payload.frame.stage.shard.route.leaf
            return "primary:" .. read_leaf(leaf)
        end

        if payload.kind == "mirror" then
            local nested = payload.frame.stage.shard.route.nested
            if nested.kind == "primary" then
                local leaf = nested.frame.stage.shard.route.leaf
                return "mirror-primary:" .. read_leaf(leaf)
            end

            local code: string = nested.fault.cause.source.code
            local wrong_code: number = nested.fault.cause.source.code -- expect-error
            return "mirror-fault:" .. code
        end

        if payload.kind == "stream" then
            local route = payload.router.selected

            if route.kind == "route_a" then
                local routed = channel.select {
                    route.ch:case_receive(),
                    payload.router.fallback:case_receive(),
                }

                if routed.channel == route.ch then
                    local value = routed.value
                    if value.kind == "control" then
                        local name: string = value.command.name
                        local wrong_name: number = value.command.name -- expect-error
                        return "stream-control:" .. name
                    end

                    local leaf_label: string = read_leaf(value)
                    local wrong_leaf: DeadlinePayload = value -- expect-error
                    return "stream-leaf:" .. leaf_label
                end

                local fallback = routed.value
                if fallback.kind == "deadline" then
                    local tick: number = fallback.deadline.tick
                    local wrong_tick: string = fallback.deadline.tick -- expect-error
                    return "stream-fallback-deadline:" .. tostring(tick)
                end

                local fallback_name: string = fallback.command.name
                local wrong_fallback: Leaf = fallback -- expect-error
                return "stream-fallback-control:" .. fallback_name
            end

            local routed = channel.select {
                route.ch:case_receive(),
                payload.router.fallback:case_receive(),
            }

            if routed.channel == route.ch then
                local value = routed.value
                if value.kind == "mirror" then
                    local nested = value.frame.stage.shard.route.nested
                    if nested.kind == "primary" then
                        return "stream-route-b-primary:" .. read_leaf(nested.frame.stage.shard.route.leaf)
                    end
                    return "stream-route-b-fault:" .. nested.fault.cause.source.code
                end

                local tick: number = value.deadline.tick
                local wrong_deadline: ControlPayload = value -- expect-error
                return "stream-route-b-deadline:" .. tostring(tick)
            end

            local fallback = routed.value
            if fallback.kind == "control" then
                local priority: number = fallback.command.priority
                local wrong_priority: string = fallback.command.priority -- expect-error
                return "stream-route-b-fallback-control:" .. tostring(priority)
            end

            return "stream-route-b-fallback-deadline:" .. tostring(fallback.deadline.tick)
        end

        if payload.kind == "box" then
            local boxed = channel.select {
                payload.node.left:case_receive(),
                payload.node.right:case_receive(),
            }

            if boxed.channel == payload.node.left then
                local route = boxed.value
                if route.kind == "route_a" then
                    local nested = channel.select {
                        route.ch:case_receive(),
                        payload.node.right:case_receive(),
                    }

                    if nested.channel == route.ch then
                        local value = nested.value
                        if value.kind == "control" then
                            local priority: number = value.command.priority
                            local wrong_priority: string = value.command.priority -- expect-error
                            return "box-route-a-control:" .. tostring(priority)
                        end

                        local label: string = read_leaf(value)
                        local wrong_label: DeadlinePayload = value -- expect-error
                        return "box-route-a-leaf:" .. label
                    end

                    local value = nested.value
                    if value.kind == "deadline" then
                        local tick: number = value.deadline.tick
                        local wrong_tick: string = value.deadline.tick -- expect-error
                        return "box-route-a-right-deadline:" .. tostring(tick)
                    end

                    return "box-route-a-right-leaf:" .. read_leaf(value)
                end

                local nested = channel.select {
                    route.ch:case_receive(),
                    payload.node.right:case_receive(),
                }

                if nested.channel == route.ch then
                    local value = nested.value
                    if value.kind == "deadline" then
                        local tick: number = value.deadline.tick
                        local wrong_tick: string = value.deadline.tick -- expect-error
                        return "box-route-b-deadline:" .. tostring(tick)
                    end

                    local nested_again = value.frame.stage.shard.route.nested
                    if nested_again.kind == "primary" then
                        return "box-route-b-primary:" .. read_leaf(nested_again.frame.stage.shard.route.leaf)
                    end

                    local code: string = nested_again.fault.cause.source.code
                    local wrong_code: number = nested_again.fault.cause.source.code -- expect-error
                    return "box-route-b-fault:" .. code
                end

                local value = nested.value
                if value.kind == "deadline" then
                    return "box-route-b-right-deadline:" .. tostring(value.deadline.tick)
                end

                local wrong_channel: Channel<Leaf> = route.ch -- expect-error
                return "box-route-b-right-leaf:" .. read_leaf(value)
            end

            local right = boxed.value
            if right.kind == "deadline" then
                local tick: number = right.deadline.tick
                local wrong_right: Leaf = right -- expect-error
                return "box-right-deadline:" .. tostring(tick)
            end

            local next_payload = payload.node.next
            if next_payload.kind == "stream" then
                local selected_route = next_payload.router.selected
                if selected_route.kind == "route_b" then
                    local nested = channel.select {
                        selected_route.ch:case_receive(),
                        next_payload.router.fallback:case_receive(),
                    }

                    if nested.channel == selected_route.ch then
                        local nested_value = nested.value
                        if nested_value.kind == "deadline" then
                            return "box-next-stream-deadline:" .. tostring(nested_value.deadline.tick)
                        end

                        local nested_again = nested_value.frame.stage.shard.route.nested
                        if nested_again.kind == "primary" then
                            return "box-next-stream-primary:" .. read_leaf(nested_again.frame.stage.shard.route.leaf)
                        end
                        return "box-next-stream-fault:" .. nested_again.fault.cause.source.code
                    end

                    local fallback = nested.value
                    if fallback.kind == "control" then
                        return "box-next-stream-control:" .. fallback.command.name
                    end
                    return "box-next-stream-fallback-deadline:" .. tostring(fallback.deadline.tick)
                end
            end

            if next_payload.kind == "box" then
                local wrong_box_reason: string = next_payload.tombstone.reason -- expect-error
                return "box-recursive"
            end

            return "box-tombstone:" .. next_payload.tombstone.reason
        end

        local reason: string = payload.tombstone.reason
        local wrong_reason: number = payload.tombstone.reason -- expect-error
        return "tombstone:" .. reason
    end

    if selected.channel == controls then
        local control = selected.value
        local command: string = control.command.name
        local priority: number = control.command.priority
        local wrong_event: EventPayload = selected.value -- expect-error
        return command .. ":" .. tostring(priority)
    end

    local deadline = selected.value
    local tick: number = deadline.deadline.tick
    local wrong_tick: string = deadline.deadline.tick -- expect-error
    return "deadline:" .. tostring(tick)
end

return analyze
