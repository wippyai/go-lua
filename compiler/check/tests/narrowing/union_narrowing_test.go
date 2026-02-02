package narrowing

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestUnionNarrowing_NestedFieldAccess tests that after narrowing a union,
// nested field access uses the narrowed type's field types.
func TestUnionNarrowing_NestedFieldAccess(t *testing.T) {
	source := `
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
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors after union narrowing")
	}
}

// TestUnionNarrowing_FieldEquality tests narrowing by field value equality (discriminated unions).
func TestUnionNarrowing_FieldEquality(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "discriminated union by literal",
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
			Name: "wrong field access after narrowing should fail",
			Code: `
				type A = {kind: "a", value_a: string}
				type B = {kind: "b", value_b: number}
				type AB = A | B

				function f(x: AB)
					if x.kind == "a" then
						local v: number = x.value_b
					end
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestUnionNarrowing_LocalAssignFromNarrowed tests that assigning a field
// from a narrowed union variable gets the narrowed type.
func TestUnionNarrowing_LocalAssignFromNarrowed(t *testing.T) {
	source := `
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
			-- After the guard, result is narrowed to {channel: EventCh, value: Event, ok: boolean}
			local event = result.value
			-- event should be Event, not Event | Time
			local k: string = event.kind
			if event.error then
				local e: string = event.error
			end
			return true
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
		t.Errorf("expected no errors after channel comparison narrowing")
	}
}

// TestUnionNarrowing_NegatedCondition tests narrowing with ~= (not equals).
func TestUnionNarrowing_NegatedCondition(t *testing.T) {
	source := `
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
			-- Early return on NOT matching events_ch
			if result.channel ~= events_ch then
				-- Inside here, result is narrowed to TimeoutCh variant
				local t: Time = result.value
				return false
			end
			-- After the if, result is narrowed to EventCh variant
			local event: Event = result.value
			return true
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
		t.Errorf("expected no errors with negated condition narrowing")
	}
}

// TestUnionNarrowing_MultipleChannels tests narrowing with 3+ variants.
func TestUnionNarrowing_MultipleChannels(t *testing.T) {
	source := `
		type Message = {_topic: string}
		type Event = {kind: string}
		type Timer = {elapsed: number}

		type MsgCh = {__tag: "msg"}
		type EventCh = {__tag: "event"}
		type TimerCh = {__tag: "timer"}

		type Result = {channel: MsgCh, value: Message, ok: boolean} |
		              {channel: EventCh, value: Event, ok: boolean} |
		              {channel: TimerCh, value: Timer, ok: boolean}

		function do_select(m: MsgCh, e: EventCh, t: TimerCh): Result
			return {channel = m, value = {_topic = "test"}, ok = true}
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

			-- Must be msg_ch
			local msg = result.value
			local topic: string = msg._topic
			return "message", topic
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
		t.Errorf("expected no errors with multi-variant narrowing")
	}
}

// TestUnionNarrowing_TruthyGuard tests narrowing union to members that have a truthy field.
func TestUnionNarrowing_TruthyGuard(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "truthy field narrows to variant with that field",
			Code: `
				type Event = {kind: string, error: string?}
				type Timer = {elapsed: number}
				type SelectResult = Event | Timer

				function get_result(): SelectResult
					return {kind = "exit", error = nil}
				end

				function f()
					local result = get_result()
					-- result.kind only exists on Event, should narrow to Event
					if result.kind then
						local k: string = result.kind
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestUnionNarrowing_MethodCallAfterNarrowing tests that method calls work
// on types with methods after narrowing from a union.
func TestUnionNarrowing_MethodCallAfterNarrowing(t *testing.T) {
	source := `
		type Message = {
			_topic: string,
			topic: (self: Message) -> string
		}

		type Timer = {elapsed: number}

		type MsgCh = {__tag: "msg"}
		type TimerCh = {__tag: "timer"}

		type Result = {channel: MsgCh, value: Message, ok: boolean} |
		              {channel: TimerCh, value: Timer, ok: boolean}

		function select_fn(msg_ch: MsgCh, timer_ch: TimerCh): Result
			return {
				channel = msg_ch,
				value = {
					_topic = "test",
					topic = function(s: Message): string return s._topic end
				},
				ok = true
			}
		end

		function f(msg_ch: MsgCh, timer_ch: TimerCh)
			local result = select_fn(msg_ch, timer_ch)
			if result.channel == timer_ch then
				return nil, "timeout"
			end
			-- result.value should be narrowed to Message
			local msg = result.value
			local topic: string = msg:topic()
			return topic
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
		t.Errorf("expected no errors calling methods after narrowing")
	}
}

// TestUnionNarrowing_TimeoutCheckPattern tests the common pattern of checking
// result.channel == timeout before accessing result.value fields/methods.
func TestUnionNarrowing_TimeoutCheckPattern(t *testing.T) {
	source := `
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

			-- After checking timeout, result.value should be Event
			local event = result.value
			if event.kind ~= "EXIT" then
				return false, "wrong event"
			end
			if event.error then
				return false, "error"
			end
			return true
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
		t.Errorf("expected no errors with timeout check pattern")
	}
}

// TestUnionNarrowing_ElseBranchNarrowsToOther tests that the else branch
// narrows to the remaining variants.
func TestUnionNarrowing_ElseBranchNarrowsToOther(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "else branch gets remaining variant",
			Code: `
				type ChanInt = {__tag: "int"}
				type ChanStr = {__tag: "str"}
				type SelResult =
					{channel: ChanInt, value: number, ok: boolean} |
					{channel: ChanStr, value: string, ok: boolean}

				function get_result(a: ChanInt, b: ChanStr): SelResult
					return {channel = a, value = 42, ok = true}
				end

				function f(ch1: ChanInt, ch2: ChanStr)
					local result = get_result(ch1, ch2)
					if result.channel == ch1 then
						-- Narrowed to first variant, value is number
						local n: number = result.value
					else
						-- Narrowed to second variant, value is string
						local s: string = result.value
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "wrong type in else branch should fail",
			Code: `
				type ChanInt = {__tag: "int"}
				type ChanStr = {__tag: "str"}
				type SelResult =
					{channel: ChanInt, value: number, ok: boolean} |
					{channel: ChanStr, value: string, ok: boolean}

				function get_result(a: ChanInt, b: ChanStr): SelResult
					return {channel = a, value = 42, ok = true}
				end

				function f(ch1: ChanInt, ch2: ChanStr)
					local result = get_result(ch1, ch2)
					if result.channel == ch1 then
						local n: number = result.value
					else
						-- WRONG: else branch has string value, not number
						local n: number = result.value
					end
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestUnionNarrowing_WithoutNarrowingShouldFail tests that accessing
// variant-specific fields without narrowing produces an error.
func TestUnionNarrowing_WithoutNarrowingShouldFail(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "accessing variant field without narrowing fails",
			Code: `
				type Event = {kind: string}
				type Timer = {elapsed: number}
				type Result = Event | Timer

				function get_result(): Result
					return {kind = "exit"}
				end

				function f()
					local result = get_result()
					-- NO narrowing - accessing .kind should fail because Timer has no .kind
					local k: string = result.kind
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}
