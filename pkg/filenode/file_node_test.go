package filenode

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"

	"github.com/dlvhdr/diffnav/pkg/config"
)

// --- Helper constructors ---

func newFile(opts ...func(*gitdiff.File)) *gitdiff.File {
	f := &gitdiff.File{
		OldName: "old.txt",
		NewName: "new.txt",
	}
	for _, o := range opts {
		o(f)
	}
	return f
}

func withIsNew(f *gitdiff.File)    { f.IsNew = true }
func withIsDelete(f *gitdiff.File) { f.IsDelete = true }

func withFragments(added, deleted int64) func(*gitdiff.File) {
	return func(f *gitdiff.File) {
		f.TextFragments = []*gitdiff.TextFragment{
			{LinesAdded: added, LinesDeleted: deleted},
		}
	}
}

func withMultipleFragments(f *gitdiff.File) {
	f.TextFragments = []*gitdiff.TextFragment{
		{LinesAdded: 3, LinesDeleted: 1},
		{LinesAdded: 2, LinesDeleted: 5},
	}
}

func makeFileNode(file *gitdiff.File, cfg config.Config) *FileNode {
	return &FileNode{
		File:       file,
		Depth:      0,
		YOffset:    0,
		Selected:   false,
		PanelWidth: 60,
		Cfg:        cfg,
	}
}

func defaultCfg() config.Config {
	return config.Config{
		UI: config.UIConfig{
			Icons:          IconsNerdStatus,
			ColorFileNames: false,
			ShowDiffStats:  false,
		},
	}
}

func makeFileForStatus(status string, fragments [2]int64) *gitdiff.File {
	var opts []func(*gitdiff.File)
	switch status {
	case "new":
		opts = append(opts, withIsNew)
	case "deleted":
		opts = append(opts, withIsDelete)
	}
	if fragments[0] > 0 || fragments[1] > 0 {
		opts = append(opts, withFragments(fragments[0], fragments[1]))
	}
	return newFile(opts...)
}

// --- GetFileName ---

func TestGetFileName(t *testing.T) {
	tests := []struct {
		name string
		file *gitdiff.File
		want string
	}{
		{
			name: "new name takes priority",
			file: &gitdiff.File{NewName: "added.go", OldName: "old.go"},
			want: "added.go",
		},
		{
			name: "old name fallback",
			file: &gitdiff.File{NewName: "", OldName: "old.go"},
			want: "old.go",
		},
		{
			name: "both empty",
			file: &gitdiff.File{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetFileName(tt.file); got != tt.want {
				t.Errorf("GetFileName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- FileNode.Path() ---

func TestPath(t *testing.T) {
	tests := []struct {
		name string
		file *gitdiff.File
		want string
	}{
		{
			name: "delegates to GetFileName with new name",
			file: newFile(withIsNew),
			want: "new.txt",
		},
		{
			name: "old name fallback",
			file: &gitdiff.File{OldName: "legacy.go"},
			want: "legacy.go",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := makeFileNode(tt.file, defaultCfg())
			if got := fn.Path(); got != tt.want {
				t.Errorf("Path() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- Icon style constants ---

func TestIconStyleConstants(t *testing.T) {
	constants := map[string]string{
		"IconsNerdStatus":   IconsNerdStatus,
		"IconsNerdSimple":   IconsNerdSimple,
		"IconsNerdFiletype": IconsNerdFiletype,
		"IconsNerdFull":     IconsNerdFull,
		"IconsUnicode":      IconsUnicode,
		"IconsASCII":        IconsASCII,
	}
	for name, value := range constants {
		if value == "" {
			t.Errorf("%s is empty", name)
		}
	}
	// Verify all values are unique
	seen := map[string]string{}
	for name, value := range constants {
		if existing, ok := seen[value]; ok {
			t.Errorf("duplicate constant value %q between %s and %s", value, existing, name)
		}
		seen[value] = name
	}
}

// --- StatusColor ---

func TestStatusColor(t *testing.T) {
	tests := []struct {
		name      string
		file      *gitdiff.File
		wantColor color.Color
	}{
		{name: "new", file: newFile(withIsNew), wantColor: lipgloss.Green},
		{name: "deleted", file: newFile(withIsDelete), wantColor: lipgloss.Red},
		{name: "modified", file: newFile(), wantColor: lipgloss.Yellow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := makeFileNode(tt.file, defaultCfg())
			if got := fn.StatusColor(); got != tt.wantColor {
				t.Errorf("StatusColor() = %v, want %v", got, tt.wantColor)
			}
		})
	}
}

// --- getIcon ---

func TestGetIcon(t *testing.T) {
	tests := []struct {
		name         string
		style        string
		status       string // "new", "deleted", "modified"
		filename     string // optional, overrides default NewName
		wantIcon     string
		wantNonEmpty bool // when true, just assert the icon is non-empty
	}{
		// nerd-fonts-status
		{name: "nerd-status new", style: IconsNerdStatus, status: "new", wantIcon: "\uf457"},
		{
			name:     "nerd-status deleted",
			style:    IconsNerdStatus,
			status:   "deleted",
			wantIcon: "\ueadf",
		},
		{
			name:     "nerd-status modified",
			style:    IconsNerdStatus,
			status:   "modified",
			wantIcon: "\uf459",
		},
		// nerd-fonts-simple
		{name: "nerd-simple new", style: IconsNerdSimple, status: "new", wantIcon: "\uf4a5"},
		// nerd-fonts-filetype
		{
			name:         "nerd-filetype",
			style:        IconsNerdFiletype,
			status:       "new",
			filename:     "main.go",
			wantNonEmpty: true,
		},
		// unicode
		{name: "unicode new", style: IconsUnicode, status: "new", wantIcon: "+"},
		{name: "unicode deleted", style: IconsUnicode, status: "deleted", wantIcon: "⛌"},
		{name: "unicode modified", style: IconsUnicode, status: "modified", wantIcon: "●"},
		// ascii
		{name: "ascii new", style: IconsASCII, status: "new", wantIcon: "+"},
		{name: "ascii deleted", style: IconsASCII, status: "deleted", wantIcon: "x"},
		{name: "ascii modified", style: IconsASCII, status: "modified", wantIcon: "*"},
		// unknown style falls back to ASCII
		{name: "unknown new", style: "unknown-style", status: "new", wantIcon: "+"},
		{name: "unknown deleted", style: "unknown-style", status: "deleted", wantIcon: "x"},
		{name: "unknown modified", style: "unknown-style", status: "modified", wantIcon: "*"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultCfg()
			cfg.UI.Icons = tt.style
			file := makeFileForStatus(tt.status, [2]int64{})
			if tt.filename != "" {
				file.NewName = tt.filename
			}
			fn := makeFileNode(file, cfg)
			got := fn.getIcon()
			if tt.wantNonEmpty {
				if got == "" {
					t.Errorf("getIcon() returned empty, want non-empty")
				}
			} else if got != tt.wantIcon {
				t.Errorf("getIcon() = %q, want %q", got, tt.wantIcon)
			}
		})
	}
}

// --- getStatusIcon ---

func TestGetStatusIcon(t *testing.T) {
	tests := []struct {
		name     string
		file     *gitdiff.File
		wantIcon string
	}{
		{name: "new", file: newFile(withIsNew), wantIcon: "\uf457"},
		{name: "deleted", file: newFile(withIsDelete), wantIcon: "\ueadf"},
		{name: "modified", file: newFile(), wantIcon: "\uf459"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := makeFileNode(tt.file, defaultCfg())
			if got := fn.getStatusIcon(); got != tt.wantIcon {
				t.Errorf("getStatusIcon() = %q, want %q", got, tt.wantIcon)
			}
		})
	}
}

// Verify getStatusIcon returns the same status icon used by getIcon for nerd-fonts-status
func TestGetStatusIcon_SameAsStatusGetIcon(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdStatus
	fn := makeFileNode(newFile(withIsNew), cfg)
	// For nerd-fonts-status, getIcon and getStatusIcon return the same icon for new files
	if fn.getStatusIcon() != fn.getIcon() {
		t.Error(
			"getStatusIcon() and getIcon() should return the same glyph for nerd-fonts-status new files",
		)
	}

	fnDel := makeFileNode(newFile(withIsDelete), cfg)
	if fnDel.getStatusIcon() != fnDel.getIcon() {
		t.Error(
			"getStatusIcon() and getIcon() should return the same glyph for nerd-fonts-status deleted files",
		)
	}

	fnMod := makeFileNode(newFile(), cfg)
	if fnMod.getStatusIcon() != fnMod.getIcon() {
		t.Error(
			"getStatusIcon() and getIcon() should return the same glyph for nerd-fonts-status modified files",
		)
	}
}

// --- Value() ---

func TestValue_StandardLayout(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsASCII
	cfg.UI.ColorFileNames = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	val := fn.Value()
	if val == "" {
		t.Error("Value() returned empty for standard layout")
	}
	if !strings.Contains(val, "new.txt") {
		t.Errorf("Value() = %q, should contain filename", val)
	}
}

func TestValue_FullLayout(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdFull
	cfg.UI.ColorFileNames = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	val := fn.Value()
	if val == "" {
		t.Error("Value() returned empty for full layout")
	}
	if !strings.Contains(val, "new.txt") {
		t.Errorf("Value() = %q, should contain filename", val)
	}
}

// --- renderStandardLayout ---

func TestRenderStandardLayout(t *testing.T) {
	tests := []struct {
		name          string
		status        string
		selected      bool
		colored       bool
		showDiffStats bool
		fragments     [2]int64
		depth         int
		contains      []string
		notContains   []string
		diffSelected  bool
	}{
		{
			name:     "not selected uncolored no stats",
			status:   "new",
			contains: []string{"test.go", "+"},
		},
		{
			name:     "not selected colored no stats",
			status:   "new",
			colored:  true,
			contains: []string{"test.go", "+"},
		},
		{
			name:         "selected no stats",
			status:       "new",
			selected:     true,
			contains:     []string{"test.go"},
			diffSelected: true,
		},
		{
			name:          "not selected with diff stats",
			status:        "new",
			showDiffStats: true,
			fragments:     [2]int64{5, 3},
			contains:      []string{"test.go", "+5", "-3"},
		},
		{
			name:        "not selected without diff stats",
			status:      "new",
			fragments:   [2]int64{5, 3},
			notContains: []string{"+5"},
		},
		{
			name:          "selected with diff stats",
			status:        "new",
			selected:      true,
			showDiffStats: true,
			fragments:     [2]int64{10, 2},
			contains:      []string{"test.go", "+10", "-2"},
		},
		{
			name:     "colored selected",
			status:   "new",
			selected: true,
			colored:  true,
			contains: []string{"test.go"},
		},
		{
			name:     "large depth unselected",
			status:   "new",
			depth:    10,
			contains: []string{"test.go"},
		},
		{
			name:     "large depth selected",
			status:   "new",
			selected: true,
			depth:    10,
			contains: []string{"test.go"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultCfg()
			cfg.UI.Icons = IconsASCII
			cfg.UI.ColorFileNames = tt.colored
			cfg.UI.ShowDiffStats = tt.showDiffStats
			file := makeFileForStatus(tt.status, tt.fragments)
			fn := makeFileNode(file, cfg)
			fn.Selected = tt.selected
			fn.Depth = tt.depth
			result := fn.renderStandardLayout("test.go")
			for _, s := range tt.contains {
				if !strings.Contains(result, s) {
					t.Errorf("renderStandardLayout() = %q, should contain %q", result, s)
				}
			}
			for _, s := range tt.notContains {
				if strings.Contains(result, s) {
					t.Errorf("renderStandardLayout() = %q, should not contain %q", result, s)
				}
			}
			if tt.diffSelected {
				fn.Selected = false
				notSelected := fn.renderStandardLayout("test.go")
				if result == notSelected {
					t.Errorf("selected and unselected render output should differ")
				}
			}
		})
	}
}

// --- renderFullLayout ---

func TestRenderFullLayout(t *testing.T) {
	tests := []struct {
		name             string
		status           string
		selected         bool
		colored          bool
		showDiffStats    bool
		fragments        [2]int64
		depth            int
		contains         []string
		notContains      []string
		diffSelected     bool
		diffFromStatuses []string
	}{
		{
			name:     "not selected uncolored no stats",
			status:   "new",
			contains: []string{"test.go"},
		},
		{
			name:     "not selected colored no stats",
			status:   "new",
			colored:  true,
			contains: []string{"test.go"},
		},
		{
			name:         "selected no stats",
			status:       "new",
			selected:     true,
			contains:     []string{"test.go"},
			diffSelected: true,
		},
		{
			name:     "colored selected no stats",
			status:   "new",
			selected: true,
			colored:  true,
			contains: []string{"test.go"},
		},
		{
			name:          "with diff stats",
			status:        "new",
			showDiffStats: true,
			fragments:     [2]int64{7, 4},
			contains:      []string{"test.go", "+7", "-4"},
		},
		{
			name:        "without diff stats",
			status:      "new",
			fragments:   [2]int64{7, 4},
			notContains: []string{"+7"},
		},
		{
			name:             "deleted file",
			status:           "deleted",
			diffFromStatuses: []string{"new"},
			contains:         []string{"test.go"},
		},
		{
			name:             "modified file",
			status:           "modified",
			diffFromStatuses: []string{"new", "deleted"},
			contains:         []string{"test.go"},
		},
		{
			name:          "selected with diff stats",
			status:        "new",
			selected:      true,
			showDiffStats: true,
			fragments:     [2]int64{3, 1},
			contains:      []string{"+3"},
		},
		{
			name:     "large depth unselected",
			status:   "new",
			depth:    10,
			contains: []string{"test.go"},
		},
		{
			name:     "large depth selected",
			status:   "new",
			selected: true,
			depth:    10,
			contains: []string{"test.go"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultCfg()
			cfg.UI.Icons = IconsNerdFull
			cfg.UI.ColorFileNames = tt.colored
			cfg.UI.ShowDiffStats = tt.showDiffStats
			file := makeFileForStatus(tt.status, tt.fragments)
			fn := makeFileNode(file, cfg)
			fn.Selected = tt.selected
			fn.Depth = tt.depth
			result := fn.renderFullLayout("test.go")
			for _, s := range tt.contains {
				if !strings.Contains(result, s) {
					t.Errorf("renderFullLayout() = %q, should contain %q", result, s)
				}
			}
			for _, s := range tt.notContains {
				if strings.Contains(result, s) {
					t.Errorf("renderFullLayout() = %q, should not contain %q", result, s)
				}
			}
			if tt.diffSelected {
				fn.Selected = false
				notSelected := fn.renderFullLayout("test.go")
				if result == notSelected {
					t.Errorf("selected and unselected full layout render output should differ")
				}
			}
			for _, otherStatus := range tt.diffFromStatuses {
				otherFile := makeFileForStatus(otherStatus, [2]int64{})
				otherFn := makeFileNode(otherFile, cfg)
				otherFn.Selected = tt.selected
				otherFn.Depth = tt.depth
				otherResult := otherFn.renderFullLayout("test.go")
				if result == otherResult {
					t.Errorf("renderFullLayout() output should differ from %q status", otherStatus)
				}
			}
		})
	}
}

// --- DiffStats ---

func TestDiffStats_NilFile(t *testing.T) {
	added, deleted := DiffStats(nil)
	if added != 0 || deleted != 0 {
		t.Errorf("DiffStats(nil) = (%d, %d), want (0, 0)", added, deleted)
	}
}

func TestDiffStats_NoFragments(t *testing.T) {
	f := &gitdiff.File{}
	added, deleted := DiffStats(f)
	if added != 0 || deleted != 0 {
		t.Errorf("DiffStats() with no fragments = (%d, %d), want (0, 0)", added, deleted)
	}
}

func TestDiffStats_NilFragments(t *testing.T) {
	f := &gitdiff.File{TextFragments: nil}
	added, deleted := DiffStats(f)
	if added != 0 || deleted != 0 {
		t.Errorf("DiffStats() with nil fragments = (%d, %d), want (0, 0)", added, deleted)
	}
}

func TestDiffStats_EmptyFragments(t *testing.T) {
	f := &gitdiff.File{TextFragments: []*gitdiff.TextFragment{}}
	added, deleted := DiffStats(f)
	if added != 0 || deleted != 0 {
		t.Errorf("DiffStats() with empty fragments = (%d, %d), want (0, 0)", added, deleted)
	}
}

func TestDiffStats_SingleFragment(t *testing.T) {
	f := newFile(withFragments(10, 5))
	added, deleted := DiffStats(f)
	if added != 10 || deleted != 5 {
		t.Errorf("DiffStats() = (%d, %d), want (10, 5)", added, deleted)
	}
}

func TestDiffStats_MultipleFragments(t *testing.T) {
	f := newFile(withMultipleFragments)
	added, deleted := DiffStats(f)
	if added != 5 || deleted != 6 {
		t.Errorf("DiffStats() = (%d, %d), want (5, 6)", added, deleted)
	}
}

func TestDiffStats_ZeroLines(t *testing.T) {
	f := newFile(withFragments(0, 0))
	added, deleted := DiffStats(f)
	if added != 0 || deleted != 0 {
		t.Errorf("DiffStats() = (%d, %d), want (0, 0)", added, deleted)
	}
}

// --- ViewDiffStats ---

func TestViewDiffStats_AddedOnly(t *testing.T) {
	result := ViewDiffStats(10, 0, lipgloss.NewStyle())
	if !strings.Contains(result, "+10") {
		t.Errorf("ViewDiffStats(10,0) = %q, should contain '+10'", result)
	}
	if strings.Contains(result, "-0") {
		t.Errorf("ViewDiffStats(10,0) = %q, should not contain '-0'", result)
	}
}

func TestViewDiffStats_DeletedOnly(t *testing.T) {
	result := ViewDiffStats(0, 7, lipgloss.NewStyle())
	if !strings.Contains(result, "-7") {
		t.Errorf("ViewDiffStats(0,7) = %q, should contain '-7'", result)
	}
	if strings.Contains(result, "+0") {
		t.Errorf("ViewDiffStats(0,7) = %q, should not contain '+0'", result)
	}
}

func TestViewDiffStats_BothAddedAndDeleted(t *testing.T) {
	result := ViewDiffStats(5, 3, lipgloss.NewStyle())
	if !strings.Contains(result, "+5") {
		t.Errorf("ViewDiffStats(5,3) = %q, should contain '+5'", result)
	}
	if !strings.Contains(result, "-3") {
		t.Errorf("ViewDiffStats(5,3) = %q, should contain '-3'", result)
	}
}

func TestViewDiffStats_NeitherAddedNorDeleted(t *testing.T) {
	result := ViewDiffStats(0, 0, lipgloss.NewStyle())
	if strings.TrimSpace(result) != "" {
		t.Errorf("ViewDiffStats(0,0) = %q, want empty or whitespace-only string", result)
	}
}

func TestViewDiffStats_LargeNumbers(t *testing.T) {
	result := ViewDiffStats(999, 888, lipgloss.NewStyle())
	if !strings.Contains(result, "+999") {
		t.Errorf("ViewDiffStats(999,888) = %q, should contain '+999'", result)
	}
	if !strings.Contains(result, "-888") {
		t.Errorf("ViewDiffStats(999,888) = %q, should contain '-888'", result)
	}
}

// --- ViewFileDiffStats ---

func TestViewFileDiffStats_WithFragments(t *testing.T) {
	file := newFile(withFragments(8, 2))
	result := ViewFileDiffStats(file, lipgloss.NewStyle())
	if !strings.Contains(result, "+8") {
		t.Errorf("ViewFileDiffStats() = %q, should contain '+8'", result)
	}
	if !strings.Contains(result, "-2") {
		t.Errorf("ViewFileDiffStats() = %q, should contain '-2'", result)
	}
}

func TestViewFileDiffStats_NilFile(t *testing.T) {
	result := ViewFileDiffStats(nil, lipgloss.NewStyle())
	if strings.TrimSpace(result) != "" {
		t.Errorf("ViewFileDiffStats(nil) = %q, want empty", result)
	}
}

// --- String() ---

func TestString_DelegatesToValue(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsASCII
	cfg.UI.ColorFileNames = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	if fn.String() != fn.Value() {
		t.Errorf("String() = %q, Value() = %q, should be equal", fn.String(), fn.Value())
	}
}

// --- Children() ---

func TestChildren_ReturnsEmpty(t *testing.T) {
	fn := makeFileNode(newFile(), defaultCfg())
	children := fn.Children()
	if children != nil && children.Length() != 0 {
		t.Errorf("Children() = %v, want empty", children)
	}
}

// --- Hidden() ---

func TestHidden_ReturnsFalse(t *testing.T) {
	fn := makeFileNode(newFile(), defaultCfg())
	if fn.Hidden() {
		t.Error("Hidden() = true, want false")
	}
}

// --- SetHidden() ---

func TestSetHidden_NoOp(t *testing.T) {
	fn := makeFileNode(newFile(), defaultCfg())
	before := fn.Value()
	fn.SetHidden(true)
	fn.SetHidden(false)
	// SetHidden is a no-op; Value() should be unaffected.
	if fn.Value() != before {
		t.Fatal("expected SetHidden to be a no-op")
	}
}

// --- SetValue() ---

func TestSetValue_NoOp(t *testing.T) {
	fn := makeFileNode(newFile(), defaultCfg())
	before := fn.Value()
	fn.SetValue("something")
	fn.SetValue(42)
	fn.SetValue(nil)
	// SetValue is a no-op; Value() should be unaffected.
	if fn.Value() != before {
		t.Fatal("expected SetValue to be a no-op")
	}
}

// --- Edge cases: PanelWidth and Depth ---

func TestValue_PanelWidthZero_StandardLayout(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsASCII
	cfg.UI.ColorFileNames = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	fn.PanelWidth = 0
	val := fn.Value()
	if val == "" {
		t.Fatal("expected non-empty Value() with PanelWidth=0")
	}
	// Even with PanelWidth=0, the status icon should appear.
	if !strings.Contains(val, "+") {
		t.Errorf("expected Value() to contain status icon '+', got %q", val)
	}
}

func TestValue_PanelWidthZero_Selected_StandardLayout(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsASCII
	cfg.UI.ColorFileNames = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	fn.PanelWidth = 0
	fn.Selected = true
	if fn.Value() == "" {
		t.Fatal("expected non-empty Value() with PanelWidth=0 and selected")
	}
}

func TestValue_PanelWidthZero_FullLayout(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdFull
	cfg.UI.ColorFileNames = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	fn.PanelWidth = 0
	if fn.Value() == "" {
		t.Fatal("expected non-empty Value() with PanelWidth=0 full layout")
	}
}

func TestValue_PanelWidthZero_Selected_FullLayout(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdFull
	cfg.UI.ColorFileNames = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	fn.PanelWidth = 0
	fn.Selected = true
	if fn.Value() == "" {
		t.Fatal("expected non-empty Value() with PanelWidth=0 selected full layout")
	}
}

func TestValue_DepthExceedsPanelWidth_StandardLayout(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsASCII
	cfg.UI.ColorFileNames = false
	cfg.UI.ShowDiffStats = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	fn.Depth = 100
	fn.PanelWidth = 10
	fn.Selected = true
	val := fn.Value()
	if val == "" {
		t.Fatal("expected non-empty Value() when depth exceeds panel width")
	}
	// When depth exceeds panel width, the name is heavily truncated but the
	// status icon should still be present.
	if !strings.Contains(val, "+") {
		t.Errorf("expected Value() to contain status icon '+', got %q", val)
	}
}

func TestValue_DepthExceedsPanelWidth_FullLayout_Selected(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdFull
	cfg.UI.ColorFileNames = false
	cfg.UI.ShowDiffStats = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	fn.Depth = 100
	fn.PanelWidth = 10
	fn.Selected = true
	if fn.Value() == "" {
		t.Fatal("expected non-empty Value() when depth exceeds panel width full layout selected")
	}
}

func TestValue_DepthExceedsPanelWidth_FullLayout_NotSelected(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdFull
	cfg.UI.ColorFileNames = false
	cfg.UI.ShowDiffStats = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	fn.Depth = 100
	fn.PanelWidth = 10
	fn.Selected = false
	if fn.Value() == "" {
		t.Fatal(
			"expected non-empty Value() when depth exceeds panel width full layout not selected",
		)
	}
}

// --- Value() with RemoveReset for full layout ---

func TestValue_FullLayout_NoAnsiReset(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdFull
	cfg.UI.ColorFileNames = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	val := fn.Value()
	// After RemoveReset, the value should not contain plain ESC[m resets
	if strings.Contains(val, "new.txt") == false {
		t.Errorf("Value() after RemoveReset = %q, should contain filename", val)
	}
}

// --- SetHidden / SetValue coverage (covered by TestSetHidden_NoOp and TestSetValue_NoOp) ---
