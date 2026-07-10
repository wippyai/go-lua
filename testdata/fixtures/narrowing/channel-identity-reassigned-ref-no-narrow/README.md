# channel-identity-reassigned-ref-no-narrow

Regression fixture for channel identity narrowing invalidation.

After `e` is reassigned from `events_ch` to `ticks_ch`, `result.channel == e`
uses the current alias proof only. The branch no longer proves the selected
value is the `Event` payload, so `result.value.id` reports a missing field.
