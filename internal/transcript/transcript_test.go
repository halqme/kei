package transcript

import "testing"

func TestTranscriptRecordsLogicalConversationInOrder(t *testing.T) {
	var transcript Transcript
	transcript.AppendUser("hello")
	transcript.AppendAssistant("checking", []ToolCall{{ID: "call-1", Name: "search", Arguments: `{"query":"hello"}`}})
	transcript.AppendTool("call-1", "result")

	got := transcript.Entries()
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3", len(got))
	}
	if got[0].Role != RoleUser || got[0].Content != "hello" {
		t.Fatalf("unexpected user entry: %+v", got[0])
	}
	if got[1].Role != RoleAssistant || len(got[1].ToolCalls) != 1 || got[1].ToolCalls[0].Name != "search" {
		t.Fatalf("unexpected assistant entry: %+v", got[1])
	}
	if got[2].Role != RoleTool || got[2].ToolCallID != "call-1" || got[2].Content != "result" {
		t.Fatalf("unexpected tool entry: %+v", got[2])
	}
}

func TestEntriesDoNotExposeTranscriptSlices(t *testing.T) {
	var transcript Transcript
	transcript.AppendAssistant("", []ToolCall{{ID: "call-1", Name: "search"}})

	got := transcript.Entries()
	got[0].Role = RoleUser
	got[0].ToolCalls[0].Name = "changed"

	fresh := transcript.Entries()
	if fresh[0].Role != RoleAssistant || fresh[0].ToolCalls[0].Name != "search" {
		t.Fatalf("Entries exposed mutable transcript storage: %+v", fresh[0])
	}
}
