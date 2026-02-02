package narrowing

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestOptionalNarrowing_NestedIf tests that optional types are narrowed correctly in nested if blocks.
func TestOptionalNarrowing_NestedIf(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "simple if narrows optional record",
			Code: `
				type Error = {kind: string, message: string}
				local function test(): Error?
					return {kind = "test", message = "msg"}
				end
				local err = test()
				if err then
					local msg = err.message
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nested if preserves narrowing record",
			Code: `
				type Error = {kind: string, message: string}
				local function test(): Error?
					return {kind = "test", message = "msg"}
				end
				local err = test()
				local flag = true
				if err then
					if flag then
						local msg = err.message
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "simple if narrows optional for method call",
			Code: `
				type Error = {kind: (self: Error) -> string, message: (self: Error) -> string}
				local function test(): Error?
					return nil
				end
				local err = test()
				if err then
					local msg = err:message()
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nested if preserves narrowing for method call",
			Code: `
				type Error = {kind: (self: Error) -> string, message: (self: Error) -> string}
				local function test(): Error?
					return nil
				end
				local err = test()
				local flag = true
				if err then
					if flag then
						local msg = err:message()
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "deeply nested if preserves narrowing for method call",
			Code: `
				type Error = {kind: (self: Error) -> string, message: (self: Error) -> string, retryable: (self: Error) -> boolean}
				local function test(): Error?
					return nil
				end
				local err = test()
				local a, b, c = true, true, true
				if err then
					if a then
						if b then
							if c then
								local k = err:kind()
								local m = err:message()
								local r = err:retryable()
							end
						end
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "multiple method calls after nil check",
			Code: `
				type Error = {kind: (self: Error) -> string, message: (self: Error) -> string, retryable: (self: Error) -> boolean}
				local function test(): Error?
					return nil
				end
				local err = test()
				if err then
					local kind = err:kind()
					if kind == "network" then
						local retryable = err:retryable()
						local message = err:message()
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestUnionNarrowingE2E tests union narrowing patterns.
func TestUnionNarrowingE2E(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "nested field access after union narrowing",
			Code: `
				type ChanInt = {__tag: "int"}
				type ChanStr = {__tag: "str"}
				type SelResult =
					{channel: ChanInt, value: {error: string}, ok: boolean} |
					{channel: ChanStr, value: {data: number}, ok: boolean}

				function get_result(a: ChanInt, b: ChanStr): SelResult
					return {channel = a, value = {error = "oops"}, ok = true}
				end

				function f(ch1: ChanInt, ch2: ChanStr)
					local result = get_result(ch1, ch2)
					if result.channel == ch1 then
						local e: string = result.value.error
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "field equality narrows union",
			Code: `
				type A = {kind: "a", value_a: string}
				type B = {kind: "b", value_b: number}
				type AB = A | B

				function f(x: AB)
					if x.kind == "a" then
						local v: string = x.value_a
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "local assign from narrowed union",
			Code: `
				type EventCh = {__tag: "event"}
				type TimeoutCh = {__tag: "timeout"}
				type Event = {kind: string, error: string?}
				type Time = {sec: number}

				type Result = {channel: EventCh, value: Event, ok: boolean} |
				              {channel: TimeoutCh, value: Time, ok: boolean}

				function get_result(ch: EventCh, timeout: TimeoutCh): Result
					return {channel = ch, value = {kind = "exit", error = nil}, ok = true}
				end

				function f(events_ch: EventCh, timeout_ch: TimeoutCh)
					local result = get_result(events_ch, timeout_ch)
					if result.channel ~= events_ch then
						return false, "timeout"
					end
					local event = result.value
					local k: string = event.kind
					if event.error then
						local e: string = event.error
					end
					return true
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "method call after narrowing from union",
			Code: `
				type Message = {
					_topic: string,
					_data: any,
					topic: (self: Message) -> string,
					payload: (self: Message) -> any
				}

				type Timer = {elapsed: number}

				type MsgCh = {__tag: "msg"}
				type TimerCh = {__tag: "timer"}

				type Result = {channel: MsgCh, value: Message, ok: boolean} |
				              {channel: TimerCh, value: Timer, ok: boolean}

				function select_fn(msg_ch: MsgCh, timer_ch: TimerCh): Result
					return {channel = msg_ch, value = {_topic = "test", _data = nil, topic = function(s) return s._topic end, payload = function(s) return s._data end}, ok = true}
				end

				function f(msg_ch: MsgCh, timer_ch: TimerCh)
					local result = select_fn(msg_ch, timer_ch)
					if result.channel == timer_ch then
						return nil, "timeout"
					end
					local msg = result.value
					local topic: string = msg:topic()
					local data = msg:payload()
					return topic, data
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "timeout check pattern",
			Code: `
				type Event = {kind: string, from: string, result: any?, error: any?}
				type Timer = {elapsed: number}

				type EventCh = {__tag: "event"}
				type TimerCh = {__tag: "timer"}

				type SelectResult = {channel: EventCh, value: Event, ok: boolean} |
				                    {channel: TimerCh, value: Timer, ok: boolean}

				function do_select(events: EventCh, timeout: TimerCh): SelectResult
					return {channel = events, value = {kind = "EXIT", from = "test", result = nil, error = nil}, ok = true}
				end

				function f(events_ch: EventCh)
					local timeout: TimerCh = {__tag = "timer"}
					local result = do_select(events_ch, timeout)

					if result.channel == timeout then
						return false, "timeout"
					end

					local event = result.value
					if event.kind ~= "EXIT" then
						return false, "wrong event"
					end
					if event.error then
						return false, "error: " .. event.error
					end
					return true
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "multiple channels select",
			Code: `
				type Message = {
					_topic: string,
					topic: (self: Message) -> string
				}

				type Event = {kind: string, from: string}
				type Timer = {elapsed: number}

				type MsgCh = {__tag: "msg"}
				type EventCh = {__tag: "event"}
				type TimerCh = {__tag: "timer"}

				type Result = {channel: MsgCh, value: Message, ok: boolean} |
				              {channel: EventCh, value: Event, ok: boolean} |
				              {channel: TimerCh, value: Timer, ok: boolean}

				function do_select(m: MsgCh, e: EventCh, t: TimerCh): Result
					return {channel = m, value = {_topic = "test", topic = function(self) return self._topic end}, ok = true}
				end

				function f(msg_ch: MsgCh, events_ch: EventCh, timeout: TimerCh)
					local result = do_select(msg_ch, events_ch, timeout)

					if result.channel == timeout then
						return nil, "timeout"
					end

					if result.channel == events_ch then
						local event = result.value
						local k: string = event.kind
						return "event", k
					end

					local msg = result.value
					local topic: string = msg:topic()
					return "message", topic
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "negated condition narrowing",
			Code: `
				type EventCh = {__tag: "event"}
				type TimeoutCh = {__tag: "timeout"}
				type Event = {kind: string}
				type Time = {sec: number}

				type Result = {channel: EventCh, value: Event, ok: boolean} |
				              {channel: TimeoutCh, value: Time, ok: boolean}

				function get_result(ch: EventCh, timeout: TimeoutCh): Result
					return {channel = ch, value = {kind = "exit"}, ok = true}
				end

				function f(events_ch: EventCh, timeout_ch: TimeoutCh)
					local result = get_result(events_ch, timeout_ch)
					if result.channel ~= events_ch then
						local t: Time = result.value
						return false
					end
					local event: Event = result.value
					return true
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}
