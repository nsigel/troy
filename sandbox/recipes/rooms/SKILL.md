---
name: rooms
description: Find available Leavey rooms or pods and reserve an explicitly confirmed space and time.
---

For a specific time window, query the CLI directly:
```python
result = usc.libcal.availability(
    start="2026-11-16T14:00:00-08:00", end="2026-11-16T15:00:00-08:00",
    category="rooms", min_capacity=2)
print(json.dumps(result))
```

Replace the timestamps and capacity with the request. Times require an explicit UTC offset; use America/Los_Angeles. Duration is at most two hours.

Returns `{"start":"...-08:00","end":"...-08:00","timezone":"America/Los_Angeles","matches":[{"id":123,"name":"Room 302C","capacity":4,"category_slug":"lvl3","kind":"room","url":"..."}],"window_end":{"lvl3":false}}`. Each match covers the full requested interval without gaps and on bookable slot boundaries. This is a read, not a hold or reservation. Matches are not ranked; choose according to the user's needs. Empty matches do not prove a booking horizon; `window_end` preserves the server's category flags.

For flexible times, use `usc.libcal.schedule(date="tomorrow", category="rooms", min_capacity=2)`. It returns `categories[].spaces[]` containing `space` and sorted `intervals` with `start`, `end`, `available`, and `status`. Missing intervals are not available. Categories: `rooms`, `pods`, `all`, `lvl1`, `lvl2`, `lvl3`. `end_date` is exclusive. Batch independent windows with `parallel`.

Before a confirmed booking, refresh availability. Call `usc.libcal.book(space=ID, start=START, end=END, accept_terms=True)` with the selected ID and returned timestamps. Read checkout questions/terms and command help for additional fields; don't fabricate answers. After an ambiguous result, check the confirmation before retrying. `usc.libcal.release` releases unfinished checkout state, not a confirmed reservation.
