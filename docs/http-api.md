# HTTP API

JSON over HTTP under `/api`, same origin as the web app (`TROY_URL`, e.g.
`https://troy.usc.sh`). Keys are `snake_case`. The browser is authenticated by
the `troy_web_session` cookie (HttpOnly, Secure, SameSite=Lax, 30 days).

Errors are `{"error": {"code": "<stable_snake_case>", "message": "<for people>"}}`.
`401 not_signed_in` means there is no valid web session; send the person to sign in.
A 4xx means retrying the same request cannot help; a 5xx may.

## Signing in

A student is the address they text from (a phone number, or an Apple ID email, which `phone_number` in `/api/me` then holds), and proves it by texting Troy:

1. The sign-in page calls `GET /api/sign-in` and shows a "Sign in with iMessage"
   button linking to `sms:<troy_phone_number>&body=<sign_in_phrase>` (URL-encode the body).
2. The student sends it. Troy texts back a link `TROY_URL/s/<code>`.
3. Opening the link sets the cookie and redirects (`303`) to `/`. The link works
   for 10 minutes, any number of times, in any browser that opens it, and only
   for the number that texted. An expired or altered link answers `400` with a
   plain-text explanation.

The phrase matches ignoring case, surrounding whitespace, and trailing
punctuation; any other text goes to Troy as a normal message. Nothing is ever
typed into the web app, and no browser is signed in except by opening the link.

### `GET /api/sign-in`

No session needed.

```json
{ "troy_phone_number": "+13105550123", "sign_in_phrase": "Sign in to Troy" }
```

### `DELETE /api/web-session`

Signs out this browser (clears the cookie). No session needed. `204`.

## The signed-in user

### `GET /api/me`

```json
{
  "phone_number": "+13105550123",
  "connections": [ Connection ]
}
```

## Connections

**Connection**

```json
{ "kind": "usc_account", "status": "connected", "net_id": "ttrojan", "first_name": "Tommy" }
```

Every kind Troy supports is listed, connected or not. `status` is `connected`
or `not_connected`. Each kind adds its own fields when connected; `usc_account`
adds `net_id`, and `first_name` when Brightspace told Troy who they are. Expect more kinds later and ignore unknown ones.

### `GET /api/connections`

Array of **Connection**, the same as in `GET /api/me`.

### `PUT /api/connections/usc-account`

```json
{ "net_id": "ttrojan", "password": "…", "duo_bypass_code": "123456789" }
```

Signs in to USC with these and saves them only if that works, replacing any
earlier USC account, along with the student's name from Brightspace. Takes several seconds (up to 90). Returns the
**Connection**. Troy then texts the student about it.

- `400 invalid_request`: a field is missing.
- `422 password_rejected`: USC rejected the NetID or password.
- `422 duo_bypass_code_rejected`: Duo rejected the bypass code; get a new one.
- `502 usc_unavailable`: USC sign-in could not be reached or completed; try again.

### `DELETE /api/connections/usc-account`

Deletes the saved USC credentials and sign-in cookies. `204`, also when nothing
was connected.

## Web chats

A student can have any number of web chats, each its own conversation with
Troy, separate from their iMessage thread. A chat id is 10 characters
(`a`-`z`, `2`-`7`). Every chat endpoint answers `404 not_found` for an id the
student doesn't have.

**Message**

```json
{ "role": "user", "text": "When is my next midterm?" }
```

```json
{
  "role": "assistant",
  "parts": [
    { "type": "text", "text": "Let me check Brightspace." },
    { "type": "tool", "id": "call_1", "name": "python", "args": "{…}", "output": "…", "failed": false, "status": "done" }
  ]
}
```

One assistant message holds a whole reply in order: the model's text as `text`
parts (Markdown), each tool call a `tool` part (`status` `running` until it
finishes, then `done` with its `output`, and `failed` when it failed). A field
that is empty or false may be left out.

### `GET /api/chats`

The student's chats, most recently active first. `title` is the start of the
first message (`""` before there is one); `preview` is the start of Troy's
latest text, as plain words; `busy` is whether Troy is replying.

```json
[{ "id": "k3m9q2x7ab", "title": "When is my next midterm?", "preview": "Your BISC 120 midterm is Thursday at 10.", "updated_at": "2026-11-15T10:04:05-08:00", "busy": false }]
```

### `POST /api/chats`

Starts an empty chat. `201 {"id": "k3m9q2x7ab"}`.

### `DELETE /api/chats/{id}`

Deletes the chat. `204`.

- `409 busy`: Troy is replying in it, or a message is waiting; try again when it's done.

### `POST /api/chats/{id}/messages`

```json
{ "text": "When is my next midterm?" }
```

Sends a message to Troy in this chat. `202`; the message and Troy's reply
arrive as events. The message is pending until Troy starts the turn that
answers it; messages sent while Troy is replying wait for the next turn, which
answers them all at once.

- `400 invalid_request`: the text is empty or longer than 4000 characters.
- `503 unavailable`: Troy couldn't take the message; try again.

### `GET /api/chats/{id}/events`

[Server-Sent Events](https://html.spec.whatwg.org/multipage/server-sent-events.html):
one JSON object per `data:` line, unnamed, so `EventSource.onmessage` gets
them all. The first is the chat as it is right now, including a reply in
progress:

```json
{ "type": "snapshot", "busy": true, "messages": [ Message ], "pending": [ "and my final?" ], "suggestion": "", "error": false }
```

`pending` holds the texts sent but not yet taken by a turn, oldest first; show
them after `messages`. `suggestion` is Troy's guess at the student's next
message, `""` when there is none; offer it in the empty message box. `error` is
true only on a snapshot sent because a reply failed. Then each change, never repeating or skipping anything
in the snapshot. Apply them in order:

| Event | Change |
|---|---|
| `{"type": "user", "text"}` | append `text` to `pending` |
| `{"type": "turn_start", "taken"}` | Troy started replying to the first `taken` pending texts: remove them from `pending` and append them as one user Message, joined by `"\n"`; `busy` is true. The turn's reply starts a new assistant Message, even with no texts taken (a reminder set in this chat firing) |
| `{"type": "text", "text"}` | the next piece of Troy's text: if the last Message is the assistant's and its last part is `text`, append `"\n\n"` and `text` to that part; otherwise append a new `text` part to the last Message if it is the assistant's and not from an earlier turn, else to a new assistant Message |
| `{"type": "tool_start", "id", "name", "args"}` | append a `running` `tool` part to the last Message if it is the assistant's, else to a new one; text after it starts a new `text` part |
| `{"type": "tool_end", "id", "output", "failed"}` | that tool part is `done` with this `output` and `failed` |
| `{"type": "turn_end"}` | Troy finished: `busy` is false |
| `{"type": "suggestion", "text"}` | `suggestion` is `text`; it comes a little after `turn_end`, and a `user` event clears it |
| `{"type": "snapshot", …}` | replaces the whole state; sent instead of `turn_end` when the reply failed, since some streamed text may not have been kept |

Lines starting with `:` are a heartbeat about every 25 seconds. If the stream
closes (a browser that falls far behind is disconnected), `EventSource`
reconnects and starts again from a new snapshot, which replaces the old state.
