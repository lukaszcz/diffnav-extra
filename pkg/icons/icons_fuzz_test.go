package icons

import (
	"strings"
	"testing"
)

func FuzzGetIcon(f *testing.F) {
	f.Add("main.go", false)
	f.Add("README.md", false)
	f.Add(".gitignore", false)
	f.Add("src", true)
	f.Add("", false)
	f.Add("noextension", false)
	f.Add("archive.tar.gz", false)

	f.Fuzz(func(t *testing.T, filename string, isDir bool) {
		// GetIcon must never panic, even with unusual inputs.
		result := GetIcon(filename, isDir)

		// Result must be non-empty for all inputs.
		if result == "" {
			t.Errorf("GetIcon(%q, %v) returned empty string", filename, isDir)
		}

		if isDir {
			// Result should be a valid directory icon or the default.
			if Directories[filename] == "" && result != DefaultDirIcon {
				t.Errorf("GetIcon(%q, true) = %q, want DefaultDirIcon %q", filename, result, DefaultDirIcon)
			}
		}
	})
}

func FuzzGetExtension(f *testing.F) {
	f.Add("file.txt")
	f.Add(".dotfile")
	f.Add("archive.tar.gz")
	f.Add("noextension")
	f.Add("")

	f.Fuzz(func(t *testing.T, filename string) {
		// getExtension must never panic.
		result := getExtension(filename)

		// Result must not contain a leading dot.
		if strings.HasPrefix(result, ".") {
			t.Errorf("getExtension(%q) = %q, should not start with '.'", filename, result)
		}

		// Result must be a suffix of the filename.
		if result != "" && !strings.HasSuffix(filename, result) {
			t.Errorf("getExtension(%q) = %q, should be suffix of filename", filename, result)
		}
	})
}
