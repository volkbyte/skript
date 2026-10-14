package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const validTOML = `
[associations]
"*.lua" = "lua"
"*.apple.lua" = "/opt/luaJIT/lua"

[skripts]
hello = "~/skripts/hello.lua"
world = "projects/world/main.lua"
abs = "C:/fixed/tool.lua"
`

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_MissingFileIsEmptyConfig(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "config.toml"), "/home/kael")
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.Dir == "" {
		t.Error("cfg.Dir is empty, want the config file's directory")
	}
	if len(cfg.Skripts) != 0 || len(cfg.Associations) != 0 {
		t.Errorf("missing config should be empty, got %+v", cfg)
	}
}

func TestLoad(t *testing.T) {
	t.Run("parses tables", func(t *testing.T) {
		cfg, err := Load(writeConfig(t, validTOML), "/home/kael")
		if err != nil {
			t.Fatalf("Load() unexpected error: %v", err)
		}
		if cfg.Associations["*.lua"] != "lua" {
			t.Errorf("associations = %+v, want *.lua entry", cfg.Associations)
		}
		if _, ok := cfg.Skripts["hello"]; !ok {
			t.Errorf("skripts = %+v, want hello entry", cfg.Skripts)
		}
	})

	t.Run("parses exec executor tables", func(t *testing.T) {
		content := `
[associations]
"*.lua" = "lua"

[associations."*.apple.lua"]
type = "exec"
executable = "/opt/luaJIT/lua"
args = ["-J"]
`
		cfg, err := Load(writeConfig(t, content), "/home/kael")
		if err != nil {
			t.Fatalf("Load() unexpected error: %v", err)
		}
		if cfg.Associations["*.apple.lua"] != "/opt/luaJIT/lua" {
			t.Errorf("associations = %+v, want the table's executable", cfg.Associations)
		}
	})
}

func TestLoad_Errors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "unparseable toml",
			content: "associations = { broken",
			want:    "parse config",
		},
		{
			name:    "invalid alias name starts with digit",
			content: `[skripts]\n1bad = "x.lua"`,
			want:    "invalid name",
		},
		{
			name:    "invalid alias name has space",
			content: `[skripts]\n"bad name" = "x.lua"`,
			want:    "invalid name",
		},
		{
			name:    "empty alias target",
			content: `[skripts]\nok1 = ""`,
			want:    "empty target",
		},
		{
			name:    "empty executor",
			content: `[associations]\n"*.lua" = ""`,
			want:    "empty executor",
		},
		{
			name:    "invalid glob pattern",
			content: `[associations]\n"[z" = "lua"`,
			want:    "invalid pattern",
		},
		{
			name:    "association executor with entrypoint",
			content: `[associations]\n[associations."*.lua"]\ntype = "exec"\nexecutable = "lua"\nentrypoint = "main.lua"`,
			want:    "deliberately no entrypoint field",
		},
		{
			name:    "association executor of wrong type",
			content: `[associations]\n[associations."*.lua"]\ntype = "command"\ncommand = "make"`,
			want:    `executor.type must be "exec"`,
		},
		{
			name:    "association executor without executable",
			content: `[associations]\n[associations."*.lua"]\ntype = "exec"`,
			want:    "executable must be a non-empty path",
		},
		{
			name:    "association value of wrong shape",
			content: `[associations]\n"*.lua" = 4`,
			want:    "must be an executable path or an exec table",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, strings.ReplaceAll(tt.content, `\n`, "\n")), "/home/kael")
			if err == nil {
				t.Fatalf("Load() = nil error, want containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load() error = %q, want containing %q", err, tt.want)
			}
		})
	}
}

func TestAliasTarget(t *testing.T) {
	cfg, err := Load(writeConfig(t, validTOML), "/home/kael")
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	dir := cfg.Dir

	// "C:/fixed/tool.lua" is a native absolute path on Windows only.
	absExpected := "C:/fixed/tool.lua"
	if runtime.GOOS != "windows" {
		absExpected = filepath.ToSlash(filepath.Join(dir, "C:/fixed/tool.lua"))
	}

	tests := []struct {
		name     string
		alias    string
		expected string
		found    bool
		wantErr  string
	}{
		{name: "tilde expands to home", alias: "hello", expected: toNative("/home/kael", "/skripts/hello.lua"), found: true},
		{name: "relative resolves against config dir", alias: "world", expected: filepath.ToSlash(filepath.Join(dir, "projects/world/main.lua")), found: true},
		{name: "absolute is kept", alias: "abs", expected: absExpected, found: true},
		{name: "unknown alias", alias: "nope", expected: "", found: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found, err := cfg.AliasTarget(tt.alias)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("AliasTarget(%q) error = %v, want containing %q", tt.alias, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("AliasTarget(%q) unexpected error: %v", tt.alias, err)
			}
			if found != tt.found {
				t.Errorf("AliasTarget(%q) found = %v, want %v", tt.alias, found, tt.found)
			}
			if got != tt.expected {
				t.Errorf("AliasTarget(%q) = %q, want %q", tt.alias, got, tt.expected)
			}
		})
	}
}

// toNative builds a path from home and rest using forward slashes.
func toNative(home, rest string) string {
	return filepath.ToSlash(home + rest)
}
