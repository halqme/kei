package acp

import (
	"bufio"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"sync"

	"github.com/halqme/kei/internal/agent"
)

type RuntimeFactory func(id, cwd string) (*agent.Runtime, error)

type Server struct {
	in       io.Reader
	out      io.Writer
	factory  RuntimeFactory
	mu       sync.Mutex
	runtimes map[string]*agent.Runtime
	cancels  map[string]context.CancelFunc
	seq      int
	wg       sync.WaitGroup
}

func NewServer(in io.Reader, out io.Writer, f RuntimeFactory) *Server {
	return &Server{in: in, out: out, factory: f, runtimes: map[string]*agent.Runtime{}, cancels: map[string]context.CancelFunc{}}
}

type request struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id,omitempty"`
	Method  string         `json:"method"`
	Params  jsontext.Value `json:"params,omitempty"`
}
type response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

func (s *Server) Serve(ctx context.Context) error {
	sc := bufio.NewScanner(s.in)
	sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
	for sc.Scan() {
		var r request
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			continue
		}
		s.wg.Add(1)
		go func() { defer s.wg.Done(); s.handle(ctx, r) }()
	}
	s.wg.Wait()
	return sc.Err()
}

func (s *Server) handle(ctx context.Context, r request) {
	switch r.Method {
	case "initialize":
		s.reply(r.ID, map[string]any{
			"protocolVersion":   1,
			"agentCapabilities": map[string]any{"loadSession": false},
			"agentInfo":         map[string]any{"name": "kei", "version": "0.1.0"},
		})
	case "session/new":
		var p struct {
			CWD string `json:"cwd"`
		}
		_ = json.Unmarshal(r.Params, &p)
		s.mu.Lock()
		s.seq++
		id := fmt.Sprintf("kei-%d", s.seq)
		runtime, err := s.factory(id, p.CWD)
		if err != nil {
			s.mu.Unlock()
			s.fail(r.ID, -32603, err.Error())
			return
		}
		s.runtimes[id] = runtime
		s.mu.Unlock()
		s.reply(r.ID, map[string]any{"sessionId": id})
		s.advertiseCommands(id, runtime)
	case "session/prompt":
		var p struct {
			SessionID string `json:"sessionId"`
			Prompt    []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"prompt"`
		}
		if err := json.Unmarshal(r.Params, &p); err != nil {
			s.fail(r.ID, -32602, err.Error())
			return
		}
		s.mu.Lock()
		runtime := s.runtimes[p.SessionID]
		turnCtx, cancel := context.WithCancel(ctx)
		s.cancels[p.SessionID] = cancel
		s.mu.Unlock()
		if runtime == nil {
			s.fail(r.ID, -32602, "unknown session")
			return
		}
		streamed := false
		runtime.OnEvent = func(kind string, payload any) {
			if kind == "assistant_message_chunk" {
				data, ok := payload.(map[string]any)
				if !ok {
					return
				}
				text, _ := data["text"].(string)
				if text == "" {
					return
				}
				streamed = true
				s.notify("session/update", map[string]any{"sessionId": p.SessionID, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": text}}})
				return
			}
			s.notify("session/update", map[string]any{"sessionId": p.SessionID, "update": map[string]any{"sessionUpdate": "tool_call_update", "kind": kind, "payload": payload}})
		}
		text := ""
		for _, b := range p.Prompt {
			if b.Type == "text" || b.Type == "" {
				text += b.Text
			}
		}
		out, err := runtime.Prompt(turnCtx, text)
		s.mu.Lock()
		delete(s.cancels, p.SessionID)
		s.mu.Unlock()
		if err != nil {
			s.notify("session/update", map[string]any{"sessionId": p.SessionID, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "Error: " + err.Error()}}})
			s.reply(r.ID, map[string]any{"stopReason": "end_turn"})
			return
		}
		if !streamed {
			s.notify("session/update", map[string]any{"sessionId": p.SessionID, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": out}}})
		}
		s.reply(r.ID, map[string]any{"stopReason": "end_turn"})
	case "session/cancel":
		var p struct {
			SessionID string `json:"sessionId"`
		}
		_ = json.Unmarshal(r.Params, &p)
		s.mu.Lock()
		cancel := s.cancels[p.SessionID]
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	default:
		if r.ID != nil {
			s.fail(r.ID, -32601, "method not found")
		}
	}
}

func (s *Server) reply(id any, result any) { s.write(response{JSONRPC: "2.0", ID: id, Result: result}) }
func (s *Server) fail(id any, code int, message string) {
	s.write(response{JSONRPC: "2.0", ID: id, Error: map[string]any{"code": code, "message": message}})
}
func (s *Server) notify(method string, params any) {
	s.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}
func (s *Server) write(v any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(v)
	_, _ = fmt.Fprintf(s.out, "%s\n", b)
}

func (s *Server) advertiseCommands(sessionID string, runtime *agent.Runtime) {
	if runtime == nil || runtime.Commands == nil {
		return
	}
	available := make([]map[string]any, 0)
	for _, d := range runtime.Commands.List() {
		c := map[string]any{"name": d.QualifiedName, "description": d.Description}
		if d.InputHint != "" {
			c["input"] = map[string]any{"hint": d.InputHint}
		}
		available = append(available, c)
	}
	s.notify("session/update", map[string]any{
		"sessionId": sessionID,
		"update": map[string]any{
			"sessionUpdate":     "available_commands_update",
			"availableCommands": available,
		},
	})
}
