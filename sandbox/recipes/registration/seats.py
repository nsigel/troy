"""Seat monitor: seats.py TERM COURSE:SECTION... emits an event while a section has open seats.
State counts each section's full checks, so a seat that reopens later is a new event."""
import json, sys
import usc

term, wanted = sys.argv[1], [w.split(":") for w in sys.argv[2:]]
state = json.load(sys.stdin) or {}
events = []
for course in dict.fromkeys(c for c, _ in wanted):
    ids = {s for c, s in wanted if c == course}
    for s in usc.classes(term, course)["sections"]:
        if s["id"] not in ids:
            continue
        ids.discard(s["id"])
        n = state.get(s["id"], 0)
        if (s.get("open_seats") or 0) > 0 and not s["cancelled"]:
            events.append({"id": f"{s['id']}:{n}",
                           "text": f"{course} {s['type']} {s['id']} has {s['open_seats']} open seats"})
        else:
            state[s["id"]] = n + 1
    if ids:
        sys.exit(f"{course} has no section {', '.join(ids)} in {term}")
json.dump({"state": state, "events": events}, sys.stdout)
