package channel

import "context"

type Message struct {
	UserID string
	Text   string
}

// Channel connects users to Troy. Inbound messages are pushed to handle as they arrive;
// replies go out through Send. Each channel owns one shape of user id, so a user is only
// ever reached through one channel.
type Channel interface {
	Listen(ctx context.Context, handle func(Message)) error
	Send(ctx context.Context, userID, text string) error
}

// Suggester is implemented by a Channel that can offer a user a predicted next message to
// send with one key, like a TUI or web chat. Texting channels don't: it would be a message.
type Suggester interface {
	Suggest(ctx context.Context, userID, text string) error
}

// Typer is implemented by a Channel that can show a user that a reply is being worked on.
type Typer interface {
	Typing(ctx context.Context, userID string) error
}
