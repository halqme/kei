package session

import "github.com/halqme/kei/internal/transcript"

// State is the logical conversation state that can outlive one agent runtime.
type State struct {
	ID         string
	Workspace  string
	Transcript transcript.Transcript
}
