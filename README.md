<p align="center">
  <img src=".github/troy-banner.png" alt="Troy: your assistant at USC." width="600">
</p>

Troy is a personal assistant for USC students. Text it on iMessage or chat on the web, and it works across your USC accounts (Brightspace, Web Registration, Leavey Library rooms, mobile ordering and Handshake) to answer questions and get things done.

![The Troy web app](.github/screenshot.jpg)

## Skills

- Academic
- Registration
- Study rooms
- Food
- Career fairs

Troy also texts you on its own: new grades as they post, reminders you set and anything a monitor you asked for turns up. Before it books, orders or registers, it shows you what it's about to do and waits for a yes.

## Stack

- Go
- OpenRouter
- Docker
- [usc-cli](https://github.com/nsigel/usc-cli)
- Sendblue
- React

## Run it

Put `OPENROUTER_API_KEY=...` in `~/.config/troy/env` (mode 0600), then, from the checkout:

```sh
docker compose up -d --build                  # troy serve
docker compose exec troy troy chat alice      # chat as user "alice"
docker compose port troy 8787                 # where the web app is
```

## License

See [LICENSE](LICENSE).
