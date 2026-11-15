---
name: registration
description: Watch full USC class sections for open seats and register the student when one opens.
---

Watching uses only the public Schedule of Classes, so it works without a USC sign-in; only registering needs one. Term codes are the year plus 1 for spring, 2 for summer, 3 for fall (Spring 2027 is 20271). Find sections with `usc.classes(TERM, "CSCI-201")`. Each section has `id`, `type`, `open_seats`, `full`, `d_clearance`, `cancelled` and `schedule`. A lecture often needs a lab or quiz too: name every section the student needs. D-clearance sections need department approval, so they can't be registered this way; tell the student.

If a section is open now, offer to register right away instead of watching. Before setting up a watch, confirm the exact sections with the student and whether to register on their own as soon as seats open or text them first.

Then create a monitor with the bundled script, which emits an event while a listed section has open seats:
```
monitor(name="seats-csci201", every_minutes=3,
        command="python3 /usr/local/lib/troy/skills/registration/seats.py 20271 CSCI-201:30381 CSCI-201:30317",
        prompt="Seats opened (above). The student said: <register right away | text first>. ...")
```

Write the student's choice and the full section list into the prompt. To register, add and schedule each section, preview, then submit:
```python
for s in ["30381", "30317"]:
    usc.webreg.add(s, term=20271)
    usc.webreg.schedule(s, term=20271)
print(json.dumps(usc.webreg.checkout(term=20271)))
```

Read the checkout: it must register exactly those sections and drop nothing. Check `usc.webreg.calendar(term=20271)` for `Conflicted` meetings. Only then `usc.webreg.submit(term=20271, confirm=True)`, and read the bin afterwards to confirm the result. If a needed partner section is still full, don't submit; wait for the next event. Once registered, cancel the monitor with `jobs` and text the student what you registered.
