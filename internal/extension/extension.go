package extension

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	keicommand "github.com/halqme/kei/internal/command"
	"github.com/halqme/kei/internal/tool"
)

type Extension struct {
	ID       string
	Root     string
	Tools    []tool.Descriptor
	Commands []keicommand.Descriptor
}

type Registry struct {
	Extensions []Extension
	Tools      *tool.Registry
	Commands   *keicommand.Registry
}

// SearchRoots returns extension roots in precedence order. An extension ID is
// shadowed as a whole by the first root that provides it.
func SearchRoots(workdir string, additional []string) []string {
	var roots []string
	if workdir != "" {
		roots = append(roots, filepath.Join(workdir, ".kei", "extensions"))
	}

	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dataHome = filepath.Join(home, ".local", "share")
		}
	}
	if dataHome != "" {
		roots = append(roots, filepath.Join(dataHome, "kei", "extensions"))
	}

	dataDirs := os.Getenv("XDG_DATA_DIRS")
	if dataDirs == "" {
		dataDirs = "/usr/local/share:/usr/share"
	}
	for _, base := range filepath.SplitList(dataDirs) {
		if base != "" {
			roots = append(roots, filepath.Join(base, "kei", "extensions"))
		}
	}

	for _, dir := range additional {
		dir = expandHome(dir)
		if dir == "" {
			continue
		}
		if !filepath.IsAbs(dir) && workdir != "" {
			dir = filepath.Join(workdir, dir)
		}
		roots = append(roots, dir)
	}
	return uniqueDirs(roots)
}

func Discover(roots []string) (*Registry, error) {
	seen := map[string]bool{}
	var extensions []Extension
	var tools []tool.Descriptor
	var commands []keicommand.Descriptor

	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			id := entry.Name()
			if seen[id] {
				continue
			}
			seen[id] = true
			ext, err := load(filepath.Join(root, id), id)
			if err != nil {
				return nil, err
			}
			extensions = append(extensions, ext)
			tools = append(tools, ext.Tools...)
			commands = append(commands, ext.Commands...)
		}
	}

	tr, err := tool.NewRegistry(tools)
	if err != nil {
		return nil, err
	}
	cr, err := keicommand.NewRegistry(commands)
	if err != nil {
		return nil, err
	}
	sort.Slice(extensions, func(i, j int) bool { return extensions[i].ID < extensions[j].ID })
	return &Registry{Extensions: extensions, Tools: tr, Commands: cr}, nil
}

func load(root, id string) (Extension, error) {
	ext := Extension{ID: id, Root: root}

	if b, err := os.ReadFile(filepath.Join(root, "tools.json")); err == nil {
		var f struct {
			Tools []tool.Descriptor `json:"tools"`
		}
		if err := json.Unmarshal(b, &f); err != nil {
			return Extension{}, fmt.Errorf("%s/tools.json: %w", root, err)
		}
		seen := map[string]bool{}
		for i := range f.Tools {
			d := &f.Tools[i]
			if d.Name == "" || d.Command == "" {
				return Extension{}, fmt.Errorf("%s/tools.json: each tool requires name and command", root)
			}
			if seen[d.Name] {
				return Extension{}, fmt.Errorf("%s/tools.json: duplicate tool %q", root, d.Name)
			}
			seen[d.Name] = true
			d.ExtensionID = id
			d.QualifiedName = id + "." + d.Name
			d.ModelName = modelName(id, d.Name)
			d.BaseDir = root
		}
		ext.Tools = f.Tools
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Extension{}, err
	}

	if b, err := os.ReadFile(filepath.Join(root, "commands.json")); err == nil {
		var f struct {
			Commands []keicommand.Descriptor `json:"commands"`
		}
		if err := json.Unmarshal(b, &f); err != nil {
			return Extension{}, fmt.Errorf("%s/commands.json: %w", root, err)
		}
		seen := map[string]bool{}
		for i := range f.Commands {
			d := &f.Commands[i]
			if d.Name == "" || d.Command == "" {
				return Extension{}, fmt.Errorf("%s/commands.json: each command requires name and command", root)
			}
			if seen[d.Name] {
				return Extension{}, fmt.Errorf("%s/commands.json: duplicate command %q", root, d.Name)
			}
			seen[d.Name] = true
			d.ExtensionID = id
			d.QualifiedName = id + ":" + d.Name
			d.BaseDir = root
		}
		ext.Commands = f.Commands
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Extension{}, err
	}

	return ext, nil
}

func modelName(extensionID, localName string) string {
	return sanitize(extensionID) + "_" + sanitize(localName)
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func uniqueDirs(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, dir := range in {
		dir = filepath.Clean(dir)
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		out = append(out, dir)
	}
	return out
}
