-- A mixed literal publishes array capacity and static-string-key capacity
-- separately, and commits each entry's destination: integer array index or
-- record-field offset. The integer index comes from the producer, never from
-- source order.
local mixed = {x = 1, 2, 3}

return mixed
