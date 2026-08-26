package extension

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSearchRootsUsesWorkspaceAndXDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/kei-data-home")
	t.Setenv("XDG_DATA_DIRS", "/opt/share:/usr/share")

	got := SearchRoots("/work/project", []string{"./extra", "/custom/extensions"})
	want := []string{
		"/work/project/.kei/extensions",
		"/tmp/kei-data-home/kei/extensions",
		"/opt/share/kei/extensions",
		"/usr/share/kei/extensions",
		"/work/project/extra",
		"/custom/extensions",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SearchRoots() = %#v, want %#v", got, want)
	}
}

func TestDiscoverShadowsWholeExtension(t *testing.T) {
	root := t.TempDir()
	high := filepath.Join(root, "high")
	low := filepath.Join(root, "low")
	write(t, filepath.Join(high, "demo", "tools.json"), `{"tools":[{"name":"high","description":"high","command":"true"}]}`)
	write(t, filepath.Join(low, "demo", "tools.json"), `{"tools":[{"name":"low","description":"low","command":"true"}]}`)
	write(t, filepath.Join(low, "demo", "commands.json"), `{"commands":[{"name":"low","description":"low","command":"true"}]}`)

	r, err := Discover([]string{high, low})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Tools.Get("demo.high"); !ok {
		t.Fatal("high-precedence extension tool missing")
	}
	if _, ok := r.Tools.Get("demo.low"); ok {
		t.Fatal("lower-precedence extension leaked through shadow")
	}
	if len(r.Commands.List()) != 0 {
		t.Fatal("commands from shadowed extension leaked through")
	}
}

func TestDiscoverNamespacesToolsAndCommands(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "astrolabe", "tools.json"), `{"tools":[{"name":"symbol","description":"Find a symbol","command":"true"}]}`)
	write(t, filepath.Join(root, "astrolabe", "commands.json"), `{"commands":[{"name":"inspect","description":"Inspect symbol","command":"true"}]}`)

	r, err := Discover([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	d, ok := r.Tools.Get("astrolabe.symbol")
	if !ok || d.ModelName != "astrolabe_symbol" {
		t.Fatalf("tool = %#v, %v", d, ok)
	}
	c, ok := r.Commands.Get("astrolabe:inspect")
	if !ok || c.QualifiedName != "astrolabe:inspect" {
		t.Fatalf("command = %#v, %v", c, ok)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
