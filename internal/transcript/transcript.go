package transcript

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type Entry struct {
	Role       Role
	Content    any
	ToolCalls  []ToolCall
	ToolCallID string
}

type Transcript struct {
	entries []Entry
}

func (t *Transcript) Append(entry Entry) {
	entry.ToolCalls = append([]ToolCall(nil), entry.ToolCalls...)
	t.entries = append(t.entries, entry)
}

func (t *Transcript) AppendUser(content any) {
	t.Append(Entry{Role: RoleUser, Content: content})
}

func (t *Transcript) AppendAssistant(content any, toolCalls []ToolCall) {
	t.Append(Entry{Role: RoleAssistant, Content: content, ToolCalls: toolCalls})
}

func (t *Transcript) AppendTool(toolCallID string, content any) {
	t.Append(Entry{Role: RoleTool, Content: content, ToolCallID: toolCallID})
}

func (t *Transcript) Entries() []Entry {
	entries := make([]Entry, len(t.entries))
	for i, entry := range t.entries {
		entries[i] = entry
		entries[i].ToolCalls = append([]ToolCall(nil), entry.ToolCalls...)
	}
	return entries
}
