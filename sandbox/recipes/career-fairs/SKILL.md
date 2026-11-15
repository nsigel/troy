---
name: career-fairs
description: Find upcoming USC career fairs on Handshake, show their details and employers, and register for an explicitly confirmed fair session.
---

Upcoming fairs, soonest first:
```python
fairs = usc.handshake.career_fairs(sort="date", limit=10)
for f in fairs['items']:
    print(f['id'], f['start_date'], f['medium'], f['registered'], f['name'])
```

Each item is `{"id":"69940","name":"...","start_date":"2027-01-25T09:00:00-08:00","end_date":"...","medium":"physical","registered":false,"registration_start":"...","registration_end":"...","employers":[{"name":"Protiviti","industry":"Management Consulting"}],"url":"https://usc.joinhandshake.com/stu/career_fairs/69940"}`. Narrow with `keyword="engineering"`, `medium="virtual"` or `date="next-30"`. "Next" means the soonest that fits the student: a Viterbi or health fair isn't the next fair for every major, and a resume review or job fair for on-campus jobs is a fair too, so say what kind it is.

Details, including sessions and the registration window:
```python
fair = usc.handshake.career_fair(69940)
print({k: fair[k] for k in ['name', 'start_date', 'end_date', 'medium', 'location', 'registration_open', 'registration_start', 'registration_end', 'url']})
print(json.dumps(fair['sessions']))
```

Sessions are `[{"id":80258,"name":"Monday, Jan 25, 9:00 am - 12:00 pm PST","start_date":"...","registration_open":true,"available_space":null,"waitlist_enabled":false}]`. `description` is plain text with the room and who it's open to.

Registering is per session. Offer it when registration is open; when it isn't, say when it opens. A request to sign up for a specific fair is the go-ahead: register right away, without asking them to confirm again. Ask only when it's unclear which fair, or which of several sessions, they mean. Then:
```python
print(usc.handshake.career_fair_register(80258))
```

It returns `{"attendee_id":86589943,"session_id":80258,"session_name":"...","registered":true,"waitlisted":false}`. Tell them if they were waitlisted instead. If it fails with a form to fill in, send the fair's `url` to finish there. When they ask to cancel, `usc.handshake.career_fair_cancel(80258)` does it.
