# channel-identity-reassigned-ref-no-narrow

Pending regression fixture for channel identity narrowing invalidation.

The checker currently narrows `result.value` through `result.channel == e` even
after `e` has been reassigned from `events_ch` to `ticks_ch`. The expected
behavior is an error on `result.value.id`, because the guard no longer proves
the selected value is the `Event` payload.

When fixed, remove both manifest skip flags and keep the structured diagnostic
expectation pinned to the missing `id` field.
