---
name: academic
description: Find coursework, quiz or exam dates, and course policies using Brightspace data and original documents.
---

Use course IDs from student context and choose the sources needed for the question. A policy question usually needs the syllabus; a daily or weekly plan may need assignments, schedules and recent announcements. Use the Python tool's `usc` bridge and `parallel`.

Calls and representative return shapes (other fields omitted):

| Python call | Result |
| --- | --- |
| `usc.brightspace.content(123, flat=True)` | `[{"id":5,"title":"Syllabus","module":"Course information","url":"/content/syllabus.pdf","due_date":null,"last_modified":"2026-08-20T12:00:00Z"}]` |
| `usc.brightspace.assignments(123)` | `[{"id":9,"name":"Homework 3","due_date":"2026-11-17T07:59:00Z","points":10}]` |
| `usc.brightspace.announcements(123, since="2026-11-01T00:00:00-07:00")` | `{"announcements":[{"id":8,"course_id":123,"title":"Schedule change","body":"Plain text…","start_date":"2026-11-12T18:00:00Z"}]}` |
| `usc.brightspace.download(123, 5, output="notes/course-123/topic-5")` | `{"path":"notes/course-123/topic-5/Syllabus.pdf"}` |

`flat=True` includes nested topics with their module paths. Announcements without a course ID fetch every enrolled course and may include an `errors` list; scope to the relevant course when possible. Assignment dates and content dates can be null. Assignments are dropbox folders, not a complete list of quizzes or coursework, and don't show submission status.

For example, inspect likely policy sources:
```python
topics = usc.brightspace.content(123, flat=True)
print(json.dumps([t for t in topics if 'syllab' in t['title'].lower()]))
```

After selecting topic IDs, download independent documents together:
```python
files = parallel(
    lambda: usc.brightspace.download(123, 5, output="notes/course-123/topic-5"),
    lambda: usc.brightspace.download(456, 8, output="notes/course-456/topic-8"),
)
for result in files:
    print(result)
```

Replace the example IDs with actual IDs. Downloads create the output directory and refuse to overwrite an existing file. Reuse a saved file when current; use a new directory when refreshing. Variables and imports persist across Python calls within this task. Inspect saved results directly; save files under `notes/` for reuse beyond this task.

Pass relevant PDF paths together to `read_documents`. Already attached PDFs remain in context. Use `text(path)` for DOCX/PPTX. Source links use `https://brightspace.usc.edu/d2l/le/content/COURSE_ID/viewContent/TOPIC_ID/View`.

Convert timestamped deadlines to America/Los_Angeles before filtering dates. Read schedule column headings: a class date is not necessarily a due date. New announcements can supersede a syllabus. A registered recurring quiz block does not confirm an actual quiz that week. Answer within the requested window, distinguish missing coverage from no work, and don't invent a clock time when the source only gives a date.
