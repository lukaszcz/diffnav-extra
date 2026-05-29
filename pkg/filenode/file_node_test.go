package filenode

import (
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

// --- GetFileName ---

func TestGetFileName_NewName(t *testing.T) {
	f := &gitdiff.File{NewName: "added.go", OldName: "old.go"}
	if got := GetFileName(f); got != "added.go" {
		t.Errorf("GetFileName() = %q, want %q", got, "added.go")
	}
}

func TestGetFileName_OldNameFallback(t *testing.T) {
	f := &gitdiff.File{NewName: "", OldName: "old.go"}
	if got := GetFileName(f); got != "old.go" {
		t.Errorf("GetFileName() = %q, want %q", got, "old.go")
	}
}

func TestGetFileName_BothEmpty(t *testing.T) {
	f := &gitdiff.File{}
	if got := GetFileName(f); got != "" {
		t.Errorf("GetFileName() = %q, want empty string", got)
	}
}

// --- FileNode.Path() ---

func TestPath_DelegatesToGetFileName(t *testing.T) {
	fn := makeFileNode(newFile(withIsNew), defaultCfg())
	if got := fn.Path(); got != "new.txt" {
		t.Errorf("Path() = %q, want %q", got, "new.txt")
	}
}

func TestPath_OldNameFallback(t *testing.T) {
	f := &gitdiff.File{OldName: "legacy.go"}
	fn := makeFileNode(f, defaultCfg())
	if got := fn.Path(); got != "legacy.go" {
		t.Errorf("Path() = %q, want %q", got, "legacy.go")
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

func TestStatusColor_New(t *testing.T) {
	fn := makeFileNode(newFile(withIsNew), defaultCfg())
	if fn.StatusColor() != lipgloss.Green {
		t.Error("StatusColor() for new file should be Green")
	}
}

func TestStatusColor_Deleted(t *testing.T) {
	fn := makeFileNode(newFile(withIsDelete), defaultCfg())
	if fn.StatusColor() != lipgloss.Red {
		t.Error("StatusColor() for deleted file should be Red")
	}
}

func TestStatusColor_Modified(t *testing.T) {
	fn := makeFileNode(newFile(), defaultCfg()) // neither new nor delete
	if fn.StatusColor() != lipgloss.Yellow {
		t.Error("StatusColor() for modified file should be Yellow")
	}
}

// --- getIcon: nerd-fonts-status ---

func TestGetIcon_NerdStatus_New(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdStatus
	fn := makeFileNode(newFile(withIsNew), cfg)
	icon := fn.getIcon()
	if icon == "" {
		t.Error("getIcon() nerd-fonts-status new file returned empty")
	}
}

func TestGetIcon_NerdStatus_Deleted(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdStatus
	fn := makeFileNode(newFile(withIsDelete), cfg)
	icon := fn.getIcon()
	if icon == "" {
		t.Error("getIcon() nerd-fonts-status deleted file returned empty")
	}
}

func TestGetIcon_NerdStatus_Modified(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdStatus
	fn := makeFileNode(newFile(), cfg)
	icon := fn.getIcon()
	if icon == "" {
		t.Error("getIcon() nerd-fonts-status modified file returned empty")
	}
}

// --- getIcon: nerd-fonts-simple ---

func TestGetIcon_NerdSimple(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdSimple
	fn := makeFileNode(newFile(withIsNew), cfg)
	icon := fn.getIcon()
	if icon == "" {
		t.Error("getIcon() nerd-fonts-simple returned empty")
	}
}

// --- getIcon: nerd-fonts-filetype ---

func TestGetIcon_NerdFiletype(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdFiletype
	file := newFile(func(f *gitdiff.File) { f.NewName = "main.go" })
	fn := makeFileNode(file, cfg)
	icon := fn.getIcon()
	if icon == "" {
		t.Error("getIcon() nerd-fonts-filetype returned empty")
	}
}

// --- getIcon: unicode ---

func TestGetIcon_Unicode_New(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsUnicode
	fn := makeFileNode(newFile(withIsNew), cfg)
	if got := fn.getIcon(); got != "+" {
		t.Errorf("getIcon() unicode new = %q, want %q", got, "+")
	}
}

func TestGetIcon_Unicode_Deleted(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsUnicode
	fn := makeFileNode(newFile(withIsDelete), cfg)
	if got := fn.getIcon(); got != "⛌" {
		t.Errorf("getIcon() unicode deleted = %q, want %q", got, "⛌")
	}
}

func TestGetIcon_Unicode_Modified(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsUnicode
	fn := makeFileNode(newFile(), cfg)
	if got := fn.getIcon(); got != "●" {
		t.Errorf("getIcon() unicode modified = %q, want %q", got, "●")
	}
}

// --- getIcon: ascii (default) ---

func TestGetIcon_ASCII_New(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsASCII
	fn := makeFileNode(newFile(withIsNew), cfg)
	if got := fn.getIcon(); got != "+" {
		t.Errorf("getIcon() ASCII new = %q, want %q", got, "+")
	}
}

func TestGetIcon_ASCII_Deleted(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsASCII
	fn := makeFileNode(newFile(withIsDelete), cfg)
	if got := fn.getIcon(); got != "x" {
		t.Errorf("getIcon() ASCII deleted = %q, want %q", got, "x")
	}
}

func TestGetIcon_ASCII_Modified(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsASCII
	fn := makeFileNode(newFile(), cfg)
	if got := fn.getIcon(); got != "*" {
		t.Errorf("getIcon() ASCII modified = %q, want %q", got, "*")
	}
}

// --- getIcon: unknown style falls back to ASCII default ---

func TestGetIcon_UnknownStyle_New(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = "unknown-style"
	fn := makeFileNode(newFile(withIsNew), cfg)
	if got := fn.getIcon(); got != "+" {
		t.Errorf("getIcon() unknown style new = %q, want %q", got, "+")
	}
}

func TestGetIcon_UnknownStyle_Deleted(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = "unknown-style"
	fn := makeFileNode(newFile(withIsDelete), cfg)
	if got := fn.getIcon(); got != "x" {
		t.Errorf("getIcon() unknown style deleted = %q, want %q", got, "x")
	}
}

func TestGetIcon_UnknownStyle_Modified(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = "unknown-style"
	fn := makeFileNode(newFile(), cfg)
	if got := fn.getIcon(); got != "*" {
		t.Errorf("getIcon() unknown style modified = %q, want %q", got, "*")
	}
}

// --- getStatusIcon ---

func TestGetStatusIcon_New(t *testing.T) {
	fn := makeFileNode(newFile(withIsNew), defaultCfg())
	icon := fn.getStatusIcon()
	if icon == "" {
		t.Error("getStatusIcon() for new file returned empty")
	}
}

func TestGetStatusIcon_Deleted(t *testing.T) {
	fn := makeFileNode(newFile(withIsDelete), defaultCfg())
	icon := fn.getStatusIcon()
	if icon == "" {
		t.Error("getStatusIcon() for deleted file returned empty")
	}
}

func TestGetStatusIcon_Modified(t *testing.T) {
	fn := makeFileNode(newFile(), defaultCfg())
	icon := fn.getStatusIcon()
	if icon == "" {
		t.Error("getStatusIcon() for modified file returned empty")
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

// --- renderStandardLayout: all branches ---

func TestRenderStandardLayout_NotSelected_Uncolored_NoStats(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsASCII
	cfg.UI.ColorFileNames = false
	cfg.UI.ShowDiffStats = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	fn.Selected = false
	result := fn.renderStandardLayout("test.go")
	if !strings.Contains(result, "test.go") {
		t.Errorf("renderStandardLayout() = %q, should contain filename", result)
	}
}

func TestRenderStandardLayout_NotSelected_Colored_NoStats(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsASCII
	cfg.UI.ColorFileNames = true
	cfg.UI.ShowDiffStats = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	fn.Selected = false
	result := fn.renderStandardLayout("test.go")
	if !strings.Contains(result, "test.go") {
		t.Errorf("renderStandardLayout() = %q, should contain filename", result)
	}
}

func TestRenderStandardLayout_Selected_NoStats(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsASCII
	cfg.UI.ColorFileNames = false
	cfg.UI.ShowDiffStats = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	fn.Selected = true
	result := fn.renderStandardLayout("test.go")
	if !strings.Contains(result, "test.go") {
		t.Errorf("renderStandardLayout() = %q, should contain filename", result)
	}
}

func TestRenderStandardLayout_NotSelected_WithDiffStats(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsASCII
	cfg.UI.ColorFileNames = false
	cfg.UI.ShowDiffStats = true
	file := newFile(withIsNew, withFragments(5, 3))
	fn := makeFileNode(file, cfg)
	result := fn.renderStandardLayout("test.go")
	if !strings.Contains(result, "test.go") {
		t.Errorf("renderStandardLayout() = %q, should contain filename", result)
	}
	if !strings.Contains(result, "+5") {
		t.Errorf("renderStandardLayout() = %q, should contain '+5'", result)
	}
	if !strings.Contains(result, "-3") {
		t.Errorf("renderStandardLayout() = %q, should contain '-3'", result)
	}
}

func TestRenderStandardLayout_NotSelected_WithoutDiffStats(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsASCII
	cfg.UI.ColorFileNames = false
	cfg.UI.ShowDiffStats = false
	file := newFile(withIsNew, withFragments(5, 3))
	fn := makeFileNode(file, cfg)
	result := fn.renderStandardLayout("test.go")
	if strings.Contains(result, "+5") {
		t.Errorf(
			"renderStandardLayout() = %q, should not contain stats when ShowDiffStats is false",
			result,
		)
	}
}

func TestRenderStandardLayout_Selected_WithDiffStats(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsASCII
	cfg.UI.ColorFileNames = false
	cfg.UI.ShowDiffStats = true
	file := newFile(withIsNew, withFragments(10, 2))
	fn := makeFileNode(file, cfg)
	fn.Selected = true
	result := fn.renderStandardLayout("test.go")
	if !strings.Contains(result, "test.go") {
		t.Errorf("renderStandardLayout() = %q, should contain filename", result)
	}
	if !strings.Contains(result, "+10") {
		t.Errorf("renderStandardLayout() = %q, should contain '+10'", result)
	}
	if !strings.Contains(result, "-2") {
		t.Errorf("renderStandardLayout() = %q, should contain '-2'", result)
	}
}

func TestRenderStandardLayout_Colored_Selected(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsASCII
	cfg.UI.ColorFileNames = true
	cfg.UI.ShowDiffStats = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	fn.Selected = true
	result := fn.renderStandardLayout("test.go")
	if !strings.Contains(result, "test.go") {
		t.Errorf("renderStandardLayout() = %q, should contain filename", result)
	}
}

// --- renderFullLayout: all branches ---

func TestRenderFullLayout_NotSelected_Uncolored_NoStats(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdFull
	cfg.UI.ColorFileNames = false
	cfg.UI.ShowDiffStats = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	fn.Selected = false
	result := fn.renderFullLayout("test.go")
	if !strings.Contains(result, "test.go") {
		t.Errorf("renderFullLayout() = %q, should contain filename", result)
	}
}

func TestRenderFullLayout_NotSelected_Colored_NoStats(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdFull
	cfg.UI.ColorFileNames = true
	cfg.UI.ShowDiffStats = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	fn.Selected = false
	result := fn.renderFullLayout("test.go")
	if !strings.Contains(result, "test.go") {
		t.Errorf("renderFullLayout() = %q, should contain filename", result)
	}
}

func TestRenderFullLayout_Selected_NoStats(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdFull
	cfg.UI.ColorFileNames = false
	cfg.UI.ShowDiffStats = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	fn.Selected = true
	result := fn.renderFullLayout("test.go")
	if !strings.Contains(result, "test.go") {
		t.Errorf("renderFullLayout() = %q, should contain filename", result)
	}
}

func TestRenderFullLayout_Colored_Selected_NoStats(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdFull
	cfg.UI.ColorFileNames = true
	cfg.UI.ShowDiffStats = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	fn.Selected = true
	result := fn.renderFullLayout("test.go")
	if !strings.Contains(result, "test.go") {
		t.Errorf("renderFullLayout() = %q, should contain filename", result)
	}
}

func TestRenderFullLayout_WithDiffStats(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdFull
	cfg.UI.ColorFileNames = false
	cfg.UI.ShowDiffStats = true
	file := newFile(withIsNew, withFragments(7, 4))
	fn := makeFileNode(file, cfg)
	result := fn.renderFullLayout("test.go")
	if !strings.Contains(result, "test.go") {
		t.Errorf("renderFullLayout() = %q, should contain filename", result)
	}
	if !strings.Contains(result, "+7") {
		t.Errorf("renderFullLayout() = %q, should contain '+7'", result)
	}
	if !strings.Contains(result, "-4") {
		t.Errorf("renderFullLayout() = %q, should contain '-4'", result)
	}
}

func TestRenderFullLayout_WithoutDiffStats(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdFull
	cfg.UI.ColorFileNames = false
	cfg.UI.ShowDiffStats = false
	file := newFile(withIsNew, withFragments(7, 4))
	fn := makeFileNode(file, cfg)
	result := fn.renderFullLayout("test.go")
	if strings.Contains(result, "+7") {
		t.Errorf(
			"renderFullLayout() = %q, should not contain stats when ShowDiffStats is false",
			result,
		)
	}
}

func TestRenderFullLayout_DeletedFile(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdFull
	cfg.UI.ColorFileNames = false
	cfg.UI.ShowDiffStats = false
	fn := makeFileNode(newFile(withIsDelete), cfg)
	result := fn.renderFullLayout("test.go")
	if !strings.Contains(result, "test.go") {
		t.Errorf("renderFullLayout() for deleted file = %q, should contain filename", result)
	}
}

func TestRenderFullLayout_ModifiedFile(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdFull
	cfg.UI.ColorFileNames = false
	cfg.UI.ShowDiffStats = false
	fn := makeFileNode(newFile(), cfg)
	result := fn.renderFullLayout("test.go")
	if !strings.Contains(result, "test.go") {
		t.Errorf("renderFullLayout() for modified file = %q, should contain filename", result)
	}
}

func TestRenderFullLayout_Selected_WithDiffStats(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdFull
	cfg.UI.ColorFileNames = false
	cfg.UI.ShowDiffStats = true
	file := newFile(withIsNew, withFragments(3, 1))
	fn := makeFileNode(file, cfg)
	fn.Selected = true
	result := fn.renderFullLayout("test.go")
	if !strings.Contains(result, "+3") {
		t.Errorf("renderFullLayout() = %q, should contain '+3'", result)
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
	if fn.Value() == "" {
		t.Fatal("expected non-empty Value() with PanelWidth=0")
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
	if fn.Value() == "" {
		t.Fatal("expected non-empty Value() when depth exceeds panel width")
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

func TestRenderStandardLayout_LargeDepth_Unselected(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsASCII
	cfg.UI.ColorFileNames = false
	cfg.UI.ShowDiffStats = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	fn.Depth = 10
	fn.Selected = false
	result := fn.renderStandardLayout("test.go")
	if !strings.Contains(result, "test.go") {
		t.Errorf("renderStandardLayout() with large depth = %q, should contain filename", result)
	}
}

func TestRenderStandardLayout_LargeDepth_Selected(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsASCII
	cfg.UI.ColorFileNames = false
	cfg.UI.ShowDiffStats = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	fn.Depth = 10
	fn.Selected = true
	result := fn.renderStandardLayout("test.go")
	if !strings.Contains(result, "test.go") {
		t.Errorf(
			"renderStandardLayout() selected with large depth = %q, should contain filename",
			result,
		)
	}
}

func TestRenderFullLayout_LargeDepth_Unselected(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdFull
	cfg.UI.ColorFileNames = false
	cfg.UI.ShowDiffStats = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	fn.Depth = 10
	fn.Selected = false
	result := fn.renderFullLayout("test.go")
	if !strings.Contains(result, "test.go") {
		t.Errorf("renderFullLayout() with large depth = %q, should contain filename", result)
	}
}

func TestRenderFullLayout_LargeDepth_Selected(t *testing.T) {
	cfg := defaultCfg()
	cfg.UI.Icons = IconsNerdFull
	cfg.UI.ColorFileNames = false
	cfg.UI.ShowDiffStats = false
	fn := makeFileNode(newFile(withIsNew), cfg)
	fn.Depth = 10
	fn.Selected = true
	result := fn.renderFullLayout("test.go")
	if !strings.Contains(result, "test.go") {
		t.Errorf(
			"renderFullLayout() selected with large depth = %q, should contain filename",
			result,
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
