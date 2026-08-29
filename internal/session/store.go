package session

import (
	"bufio"
	"bytes"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/halqme/kei/internal/transcript"
)

const storeVersion = 1

var (
	ErrNotFound = errors.New("session not found")
	ErrExists   = errors.New("session already exists")
)

type Store interface {
	Create(*State) error
	Load(id string) (*State, error)
	Append(id string, entry transcript.Entry) error
}

type FileStore struct {
	root string
}

type fileRecord struct {
	Type       string         `json:"type"`
	Version    int            `json:"version,omitempty"`
	ID         string         `json:"id,omitempty"`
	Workspace  string         `json:"workspace,omitempty"`
	Role       string         `json:"role,omitempty"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []fileToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type fileToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func NewFileStore(root string) *FileStore {
	return &FileStore{root: root}
}

func DefaultFileStore() (*FileStore, error) {
	root, err := DefaultStoreDir()
	if err != nil {
		return nil, err
	}
	return NewFileStore(root), nil
}

func DefaultStoreDir() (string, error) {
	if stateHome := os.Getenv("XDG_STATE_HOME"); filepath.IsAbs(stateHome) {
		return filepath.Join(stateHome, "kei", "sessions"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", errors.New("could not determine a session state directory")
	}
	return filepath.Join(home, ".local", "state", "kei", "sessions"), nil
}

func (s *FileStore) Create(state *State) error {
	if state == nil {
		return errors.New("session state is nil")
	}
	path, err := s.path(state.ID)
	if err != nil {
		return err
	}

	records := []fileRecord{{Type: "session", Version: storeVersion, ID: state.ID, Workspace: state.Workspace}}
	for _, entry := range state.Transcript.Entries() {
		record, err := entryRecord(entry)
		if err != nil {
			return err
		}
		records = append(records, record)
	}

	var data bytes.Buffer
	for _, record := range records {
		b, err := json.Marshal(record)
		if err != nil {
			return err
		}
		data.Write(b)
		data.WriteByte('\n')
	}

	if err := os.MkdirAll(s.root, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%w: %s", ErrExists, state.ID)
	}
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if _, err := f.Write(data.Bytes()); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

func (s *FileStore) Load(id string) (*State, error) {
	path, err := s.path(id)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	header, err := readFileRecord(reader, 1)
	if err != nil {
		return nil, err
	}
	if header.Type != "session" || header.Version != storeVersion {
		return nil, fmt.Errorf("session %q has unsupported header type=%q version=%d", id, header.Type, header.Version)
	}
	if header.ID != id {
		return nil, fmt.Errorf("session %q file contains id %q", id, header.ID)
	}

	state := &State{ID: header.ID, Workspace: header.Workspace}
	for line := 2; ; line++ {
		record, err := readFileRecord(reader, line)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		entry, err := recordEntry(record)
		if err != nil {
			return nil, fmt.Errorf("session %q line %d: %w", id, line, err)
		}
		state.Transcript.Append(entry)
	}
	return state, nil
}

func (s *FileStore) Append(id string, entry transcript.Entry) error {
	path, err := s.path(id)
	if err != nil {
		return err
	}
	record, err := entryRecord(entry)
	if err != nil {
		return err
	}
	b, err := json.Marshal(record)
	if err != nil {
		return err
	}
	b = append(b, '\n')

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(b); err != nil {
		return err
	}
	return f.Sync()
}

func (s *FileStore) path(id string) (string, error) {
	if s == nil || s.root == "" {
		return "", errors.New("session store directory is empty")
	}
	if err := validateID(id); err != nil {
		return "", err
	}
	return filepath.Join(s.root, id+".jsonl"), nil
}

func validateID(id string) error {
	if id == "" || len(id) > 128 || id == "." || id == ".." {
		return fmt.Errorf("invalid session id %q", id)
	}
	for _, r := range id {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			continue
		}
		return fmt.Errorf("invalid session id %q", id)
	}
	return nil
}

func entryRecord(entry transcript.Entry) (fileRecord, error) {
	content, err := durableContent(entry.Content)
	if err != nil {
		return fileRecord{}, err
	}
	switch entry.Role {
	case transcript.RoleUser, transcript.RoleAssistant, transcript.RoleTool:
	default:
		return fileRecord{}, fmt.Errorf("unsupported transcript role %q", entry.Role)
	}

	record := fileRecord{
		Type:       "entry",
		Role:       string(entry.Role),
		Content:    content,
		ToolCallID: entry.ToolCallID,
	}
	for _, call := range entry.ToolCalls {
		record.ToolCalls = append(record.ToolCalls, fileToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
		})
	}
	return record, nil
}

func recordEntry(record fileRecord) (transcript.Entry, error) {
	if record.Type != "entry" {
		return transcript.Entry{}, fmt.Errorf("unexpected record type %q", record.Type)
	}
	role := transcript.Role(record.Role)
	switch role {
	case transcript.RoleUser, transcript.RoleAssistant, transcript.RoleTool:
	default:
		return transcript.Entry{}, fmt.Errorf("unsupported transcript role %q", record.Role)
	}
	entry := transcript.Entry{Role: role, Content: record.Content, ToolCallID: record.ToolCallID}
	for _, call := range record.ToolCalls {
		entry.ToolCalls = append(entry.ToolCalls, transcript.ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
		})
	}
	return entry, nil
}

func durableContent(content any) (string, error) {
	switch content := content.(type) {
	case nil:
		return "", nil
	case string:
		return content, nil
	default:
		return "", fmt.Errorf("transcript content type %T is not durable; session storage currently supports text content only", content)
	}
}

func readFileRecord(reader *bufio.Reader, line int) (fileRecord, error) {
	for {
		b, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) && len(b) == 0 {
			return fileRecord{}, io.EOF
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return fileRecord{}, err
		}
		b = bytes.TrimSpace(b)
		if len(b) == 0 {
			if errors.Is(err, io.EOF) {
				return fileRecord{}, io.EOF
			}
			continue
		}
		var record fileRecord
		if unmarshalErr := json.Unmarshal(b, &record); unmarshalErr != nil {
			return fileRecord{}, fmt.Errorf("invalid session record on line %d: %w", line, unmarshalErr)
		}
		return record, nil
	}
}
