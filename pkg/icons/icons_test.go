package icons

import "testing"

func TestGetIcon_KnownDirectory(t *testing.T) {
	got := GetIcon(".git", true)
	// Independent expected value — not read from the Directories map.
	if got != "\uf02a2" {
		t.Errorf("GetIcon(%q, true) = %q, want %q", ".git", got, "\uf02a2")
	}
}

func TestGetIcon_UnknownDirectory(t *testing.T) {
	got := GetIcon("nonexistent-dir", true)
	if got != DefaultDirIcon {
		t.Errorf(
			"GetIcon(%q, true) = %q, want DefaultDirIcon %q",
			"nonexistent-dir",
			got,
			DefaultDirIcon,
		)
	}
}

func TestGetIcon_KnownFilename(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"Dockerfile", "\ue650"},
		{"go.mod", "\ue65e"},
		{"go.sum", "\ue65e"},
	}
	for _, tt := range tests {
		got := GetIcon(tt.filename, false)
		if got != tt.want {
			t.Errorf("GetIcon(%q, false) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}

func TestGetIcon_KnownExtension(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"main.go", "\ue65e"},
		{"app.ts", "\ue628"},
	}
	for _, tt := range tests {
		got := GetIcon(tt.filename, false)
		if got != tt.want {
			t.Errorf("GetIcon(%q, false) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}

func TestGetIcon_UnknownFilenameAndExtension(t *testing.T) {
	got := GetIcon("weird.xyz123", false)
	if got != DefaultFileIcon {
		t.Errorf(
			"GetIcon(%q, false) = %q, want DefaultFileIcon %q",
			"weird.xyz123",
			got,
			DefaultFileIcon,
		)
	}
}

func TestGetIcon_FilenameBeatsExtension(t *testing.T) {
	// Dockerfile has an entry in the Filenames map; it should be returned
	// even though it has no extension (no fallback to the Extensions map).
	filenameResult := GetIcon("Dockerfile", false)
	if filenameResult == "" || filenameResult == DefaultFileIcon {
		t.Errorf("expected a specific icon for 'Dockerfile', not the default")
	}
}

func TestGetIcon_NoExtension(t *testing.T) {
	// "Makefile" should match via Filenames map; use something that won't
	// match either Filenames or Extensions.
	got := GetIcon("noext", false)
	if got != DefaultFileIcon {
		t.Errorf("GetIcon(%q, false) = %q, want DefaultFileIcon %q", "noext", got, DefaultFileIcon)
	}
}

func TestGetIcon_PathWithSlash(t *testing.T) {
	// Ensure the function extracts extension from a path-containing filename
	ext := "go"
	want, ok := Extensions[ext]
	if !ok {
		t.Fatalf("no icon registered for extension %q", ext)
	}
	got := GetIcon("dir/main.go", false)
	if got != want {
		t.Errorf("GetIcon(%q, false) = %q, want %q", "dir/main.go", got, want)
	}
}

func TestGetExtension_Simple(t *testing.T) {
	got := getExtension("main.go")
	if got != "go" {
		t.Errorf("getExtension(%q) = %q, want %q", "main.go", got, "go")
	}
}

func TestGetExtension_MultipleDots(t *testing.T) {
	got := getExtension("test.test.go")
	if got != "go" {
		t.Errorf("getExtension(%q) = %q, want %q", "test.test.go", got, "go")
	}
}

func TestGetExtension_NoExtension(t *testing.T) {
	got := getExtension("Makefile")
	if got != "" {
		t.Errorf("getExtension(%q) = %q, want %q", "Makefile", got, "")
	}
}

func TestGetExtension_PathWithSlash(t *testing.T) {
	got := getExtension("dir/file.go")
	if got != "go" {
		t.Errorf("getExtension(%q) = %q, want %q", "dir/file.go", got, "go")
	}
}

func TestGetExtension_PathWithSlashNoExtension(t *testing.T) {
	// A filename with a slash but no dot after it should break at the slash
	// and return empty string.
	got := getExtension("dir/nofileext")
	if got != "" {
		t.Errorf("getExtension(%q) = %q, want %q", "dir/nofileext", got, "")
	}
}
