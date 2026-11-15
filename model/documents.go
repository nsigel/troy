package model

// Document is an immutable workspace snapshot. Data is hydrated for a model request,
// never written into the conversation log or archived transcript.
type Document struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Data   []byte `json:"-"`
}

const Default = "anthropic/claude-haiku-5.5"

// Suggest predicts a user's next message: one short call, so fast and cheap beats smart.
const Suggest = "anthropic/claude-haiku-5.5"
