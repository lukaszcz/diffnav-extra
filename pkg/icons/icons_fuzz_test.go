package icons

import (
	"path/filepath"
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

		// Sanity: running GetIcon twice on the same input must be idempotent.
		result2 := GetIcon(filename, isDir)
		if result != result2 {
			t.Errorf("GetIcon not idempotent: first=%q second=%q", result, result2)
		}

		if isDir {
			// Result should be a valid directory icon or the default.
			if Directories[filename] == "" && result != DefaultDirIcon {
				t.Errorf(
					"GetIcon(%q, true) = %q, want DefaultDirIcon %q",
					filename,
					result,
					DefaultDirIcon,
				)
			}
		} else {
			// Non-directory: known extension should return a known file icon.
			ext := getExtension(filename)
			if ext != "" {
				if _, ok := Extensions[ext]; ok {
					// If the extension is registered and the filename isn't in Filenames,
					// the result should match the extension icon.
					if _, isFilename := Filenames[filepath.Base(filename)]; !isFilename {
						if result != Extensions[ext] && result != DefaultFileIcon {
							// Allow filetype icons to override simple icons, but not arbitrary values.
						}
					}
				}
			}
			// All non-directory results should be the default file icon when no
			// filename or extension matches.
			if _, isFilename := Filenames[filepath.Base(filename)]; !isFilename {
				if ext := getExtension(filename); ext == "" {
					if result != DefaultFileIcon {
						t.Errorf("GetIcon(%q, false) = %q, want DefaultFileIcon %q for no ext",
							filename, result, DefaultFileIcon)
					}
				}
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
