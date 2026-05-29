package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/bluekeyes/go-gitdiff/gitdiff"

	"github.com/dlvhdr/diffnav/pkg/filenode"
)

func FuzzRelativeTime(f *testing.F) {
	// Seed with past, present, and future times.
	now := time.Now()
	f.Add(now.UnixNano())
	f.Add(now.Add(-time.Hour).UnixNano())
	f.Add(now.Add(time.Hour).UnixNano())
	f.Add(now.Add(-365 * 24 * time.Hour).UnixNano())

	f.Fuzz(func(t *testing.T, nano int64) {
		tt := time.Unix(0, nano)
		result := relativeTime(tt)
		if result == "" {
			t.Errorf("relativeTime(%v) returned empty string", tt)
		}
		// Must be one of the known format suffixes
		validSuffixes := []string{"now", "m", "h", "d", "mo", "y"}
		found := false
		for _, s := range validSuffixes {
			if strings.HasSuffix(result, s) || result == s {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("relativeTime(%v) = %q, unexpected format", tt, result)
		}
	})
}

func FuzzClampToViewportWidth(f *testing.F) {
	f.Add(5, 10)
	f.Add(15, 10)
	f.Add(0, 10)
	f.Add(5, 0)

	f.Fuzz(func(t *testing.T, x, vpWidth int) {
		result := clampToViewportWidth(x, vpWidth)
		if vpWidth > 0 && result >= vpWidth {
			t.Errorf("clampToViewportWidth(%d, %d) = %d, should be < vpWidth", x, vpWidth, result)
		}
		if result < 0 {
			t.Errorf("clampToViewportWidth(%d, %d) = %d, should be >= 0", x, vpWidth, result)
		}
		if vpWidth <= 0 && result != x {
			t.Errorf(
				"clampToViewportWidth(%d, %d) = %d, should be x when vpWidth<=0",
				x,
				vpWidth,
				result,
			)
		}
	})
}

func FuzzGetFileName(f *testing.F) {
	f.Add("old.txt", "new.txt")
	f.Add("", "new.txt")
	f.Add("old.txt", "")
	f.Add("", "")

	f.Fuzz(func(t *testing.T, oldName, newName string) {
		file := &gitdiff.File{OldName: oldName, NewName: newName}
		result := filenode.GetFileName(file)
		// Result must be either the newName (if non-empty) or oldName
		if newName != "" && result != newName {
			t.Errorf(
				"GetFileName({OldName=%q, NewName=%q}) = %q, want %q",
				oldName,
				newName,
				result,
				newName,
			)
		}
		if newName == "" && result != oldName {
			t.Errorf(
				"GetFileName({OldName=%q, NewName=%q}) = %q, want %q",
				oldName,
				newName,
				result,
				oldName,
			)
		}
	})
}
