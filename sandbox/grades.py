"""The built-in grades monitor: prints every graded Brightspace item in the student's current
courses as a monitor event. The id includes the score, so a changed score is a new event."""
import json, sys
import usc

events, failed = [], []
courses = usc.brightspace.courses()
for course in courses:
    try:
        grades = usc.brightspace.grades(course["id"], graded_only=True)["grades"]
    except usc.Error as e:  # e.g. a course that hides its gradebook (403)
        print(e, file=sys.stderr)
        failed.append(course)
        continue
    for g in grades:
        score = f"{g['score']:g}/{g['score_max']:g}" if g.get("score") is not None and g.get("score_max") else ""
        shown = g.get("displayed_grade") or ""
        if shown.replace(" ", "") != score:
            score = f"{score} ({shown})" if score else shown
        events.append({"id": f"{course['id']}:{g['id']}:{g.get('score')}",
                       "text": f"{course['code']} {g['name']}: {score}"})
if courses and len(failed) == len(courses):
    sys.exit("couldn't read grades in any course")
json.dump({"state": None, "events": events}, sys.stdout)
