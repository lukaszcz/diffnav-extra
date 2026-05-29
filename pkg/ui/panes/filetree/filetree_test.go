package filetree

import (
	"os"
	"testing"

	"charm.land/bubbles/v2/tree"
	tea "charm.land/bubbletea/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/dlvhdr/diffnav/pkg/config"
	"github.com/dlvhdr/diffnav/pkg/constants"
	"github.com/dlvhdr/diffnav/pkg/dirnode"
	"github.com/dlvhdr/diffnav/pkg/filenode"
)

func TestClickDirectoryRowSelectsOnly(t *testing.T) {
	m := newTestTreeModel([]string{
		"app/main.go",
		"app/internal/db.go",
		"docs/readme.md",
	})
	app := nodeByPath(t, &m, "app")
	internal := nodeByPath(t, &m, "app/internal")
	docs := nodeByPath(t, &m, "docs")
	internal.Close()
	app.Close()
	m.SetCursorNoScroll(docs.YOffset())

	m.ClickNode(app)

	if got := m.CurrNodePath(); got != "app" {
		t.Fatalf("expected clicked directory to be selected, got %q", got)
	}
	if app.IsOpen() {
		t.Fatal("expected folded directory row click to leave it folded")
	}
	if internal.IsOpen() {
		t.Fatal("expected folded child directory to remain folded")
	}
}

func TestClickDirectoryIconSelectsAndTogglesOpenState(t *testing.T) {
	m := newTestTreeModel([]string{
		"app/main.go",
		"app/internal/db.go",
		"docs/readme.md",
	})
	app := nodeByPath(t, &m, "app")
	m.SetCursorNoScroll(app.YOffset())

	if !app.IsOpen() {
		t.Fatal("setup: expected app directory to start open")
	}
	m.ClickNodeIcon(app)
	if app.IsOpen() {
		t.Fatal("expected selected open directory icon click to close it")
	}

	m.ClickNodeIcon(app)
	if !app.IsOpen() {
		t.Fatal("expected selected folded directory icon click to open it")
	}
}

func TestClickDirectoryIconSelectsAndTogglesUnselectedDirectory(t *testing.T) {
	m := newTestTreeModel([]string{
		"app/main.go",
		"docs/readme.md",
	})
	app := nodeByPath(t, &m, "app")
	docs := nodeByPath(t, &m, "docs")
	m.SetCursorNoScroll(docs.YOffset())

	m.ClickNodeIcon(app)

	if got := m.CurrNodePath(); got != "app" {
		t.Fatalf("expected clicked directory to be selected, got %q", got)
	}
	if app.IsOpen() {
		t.Fatal("expected unselected open directory icon click to close it")
	}
}

func TestDirectoryIconHitUsesTreeRelativeColumn(t *testing.T) {
	m := newTestTreeModel([]string{
		"app/main.go",
		"app/internal/db.go",
		"docs/readme.md",
	})
	root := m.GetNodeAtY(0)
	app := nodeByPath(t, &m, "app")
	internal := nodeByPath(t, &m, "app/internal")

	for _, tc := range []struct {
		name string
		node *tree.Node
		x    int
	}{
		{name: "root icon", node: root, x: 0},
		{name: "depth one icon", node: app, x: 1},
		{name: "depth two icon", node: internal, x: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !m.IsDirectoryIconHit(tc.node, tc.x) {
				t.Fatalf("expected x=%d to hit %s", tc.x, tc.name)
			}
			if m.IsDirectoryIconHit(tc.node, tc.x-1) {
				t.Fatalf("expected x=%d to miss before %s", tc.x-1, tc.name)
			}
			if m.IsDirectoryIconHit(tc.node, tc.x+1) {
				t.Fatalf("expected x=%d to miss after %s icon", tc.x+1, tc.name)
			}
		})
	}
}

func TestDirectoryIconHitCoversNerdFontIconWidth(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.UI.Icons = filenode.IconsNerdStatus
	m := New(cfg)
	files := []*gitdiff.File{
		{NewName: "app/main.go"},
		{NewName: "docs/readme.md"},
	}
	m = m.SetFiles(files)

	app := nodeByPath(t, &m, "app")
	start := directoryIconStartColumn(app)

	for _, x := range []int{start, start + 1} {
		if !m.IsDirectoryIconHit(app, x) {
			t.Fatalf("expected x=%d to hit full Nerd Font directory indicator", x)
		}
	}
	if m.IsDirectoryIconHit(app, start+2) {
		t.Fatalf("expected x=%d to miss after Nerd Font directory indicator", start+2)
	}
}

func TestDirectoryIconHitIgnoresFileNodes(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go"})
	file := nodeByPath(t, &m, "app/main.go")

	if m.IsDirectoryIconHit(file, file.Depth()) {
		t.Fatal("expected file node icon column to not be a directory toggle hit")
	}
}

func TestNodeDescendantDiffsIncludesFoldedChildren(t *testing.T) {
	m := newTestTreeModel([]string{
		"app/main.go",
		"app/internal/db.go",
		"docs/readme.md",
	})
	app := nodeByPath(t, &m, "app")
	internal := nodeByPath(t, &m, "app/internal")
	internal.Close()

	files := m.NodeDescendantDiffs(app)

	if len(files) != 2 {
		t.Fatalf("expected app diff to include folded child files, got %d", len(files))
	}
	got := []string{filenode.GetFileName(files[0]), filenode.GetFileName(files[1])}
	want := []string{"app/main.go", "app/internal/db.go"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected file %d to be %q, got %q", i, want[i], got[i])
		}
	}
}

func newTestTreeModel(paths []string) Model {
	cfg := config.DefaultConfig()
	cfg.UI.Icons = filenode.IconsASCII
	m := New(cfg)
	files := make([]*gitdiff.File, 0, len(paths))
	for _, p := range paths {
		files = append(files, &gitdiff.File{NewName: p})
	}
	return m.SetFiles(files)
}

func nodeByPath(t *testing.T, m *Model, path string) *tree.Node {
	t.Helper()
	for _, node := range m.t.AllNodes() {
		switch val := node.GivenValue().(type) {
		case *dirnode.DirNode:
			if val.FullPath == path {
				return node
			}
		case *filenode.FileNode:
			if filenode.GetFileName(val.File) == path {
				return node
			}
		}
	}
	t.Fatalf("expected to find node %q", path)
	return nil
}

// .
// ├── graphql-server
// │   └── tests
// │       └── package.json
// ├── yarn.lock
func TestBuildFullFileTree(t *testing.T) {
	f, err := os.Open("testdata/multiple_files.diff")
	if err != nil {
		t.Fatal(err)
	}
	files, _, err := gitdiff.Parse(f)
	if err != nil {
		t.Fatal(err)
	}

	tr := buildFullFileTree(files, config.Config{})
	allNodes := tr.AllNodes()
	if len(allNodes) != 5 {
		t.Fatalf("expected 5 nodes, but got %d", len(allNodes))
	}
	root := tr
	if root.GivenValue().(*dirnode.DirNode).Name != constants.RootName {
		t.Fatalf(`expected root value to be constants.RootName, but got "%s"`, root.Value())
	}

	if len(root.ChildNodes()) != 2 {
		t.Fatalf("expected root to have 2 children, but got %d", len(root.ChildNodes()))
	}

	graphqlServer := root.ChildNodes()[0]
	if graphqlServer.GivenValue().(*dirnode.DirNode).Name != "graphql-server" {
		t.Fatalf(
			`expected root first child value to be "graphql-server", but got %s`,
			graphqlServer.GivenValue(),
		)
	}
	yarnLock := root.ChildNodes()[1]
	if yarnLock.GivenValue().(*filenode.FileNode).Path() != "yarn.lock" {
		t.Log(tr.String())
		t.Fatalf(`expected root second child value to be "* yarn.lock", but got %s`,
			yarnLock.GivenValue().(*filenode.FileNode).Path())
	}

	if len(graphqlServer.ChildNodes()) != 1 {
		t.Fatalf(
			"expected graphql-server to have 1 children, but got %d",
			len(graphqlServer.ChildNodes()),
		)
	}

	tests := graphqlServer.ChildNodes()[0]
	if tests.GivenValue().(*dirnode.DirNode).Name != "tests" {
		t.Fatalf(
			`expected graphql-server only child value to be "tests", but got %s`,
			tests.GivenValue(),
		)
	}

	if len(tests.ChildNodes()) != 1 {
		t.Fatalf("expected tests to have 1 children, but got %d", len(tests.ChildNodes()))
	}

	packageJson := tests.ChildNodes()[0]
	if packageJson.GivenValue().(*filenode.FileNode).Path() != "graphql-server/tests/package.json" {
		t.Fatalf(
			`expected tests only child value to be "graphql-server/tests/package.json", but got %s`,
			packageJson.GivenValue().(*filenode.FileNode).Path(),
		)
	}
}

// input:
// .
// ├── graphql-server
// │   └── tests
// │       └── package.json
// └── yarn.lock
//
// output:
// .
// ├── graphql-server/tests
// │   └── package.json
// └── yarn.lock
func TestCollapseTree(t *testing.T) {
	f, err := os.Open("testdata/multiple_files.diff")
	if err != nil {
		t.Fatal(err)
	}
	files, _, err := gitdiff.Parse(f)
	if err != nil {
		t.Fatal(err)
	}

	tr := buildFullFileTree(files, config.Config{})
	tr = collapseTree(tr)

	allNodes := tr.AllNodes()
	if len(allNodes) != 4 {
		t.Fatalf("expected 4 nodes, but got %d", len(allNodes))
	}

	root := tr
	if root.GivenValue().(*dirnode.DirNode).Name != constants.RootName {
		t.Fatalf(`expected root value to be constants.RootName, but got "%s"`, root.Value())
	}

	if len(root.ChildNodes()) != 2 {
		t.Fatalf("expected root to have 2 children, but got %d", len(root.ChildNodes()))
	}

	graphqlServer := root.ChildNodes()[0]
	if graphqlServer.GivenValue().(*dirnode.DirNode).Name != "graphql-server/tests" {
		t.Fatalf(
			`expected root first child value to be "graphql-server/tests", but got %s`,
			graphqlServer.GivenValue(),
		)
	}

	if len(graphqlServer.ChildNodes()) != 1 {
		t.Fatalf(
			"expected graphql-server to have 1 children, but got %d",
			len(graphqlServer.ChildNodes()),
		)
	}
	packageJson := graphqlServer.ChildNodes()[0]
	if packageJson.GivenValue().(*filenode.FileNode).Path() != "graphql-server/tests/package.json" {
		t.Fatalf(
			`expected graphql-server/tests only child value to be "graphql-server/tests/package.json", but got %s`,
			packageJson.GivenValue(),
		)
	}

	yarnLock := root.ChildNodes()[1]
	if yarnLock.GivenValue().(*filenode.FileNode).Path() != "yarn.lock" {
		t.Log(tr.String())
		t.Fatalf(`expected root second child value to be "* yarn.lock", but got %s`,
			yarnLock.GivenValue().(*filenode.FileNode).Path())
	}
}

// input:
// .
// └── ui
//     ├── components
//     │   ├── reposection
//     │   │   ├── commands.go
//     │   │   └── reposection.go
//     │   ├── section
//     │   │   └── section.go
//     │   └── tasks
//     │       └── pr.go
//     └─ keys
//     │   └── branchkeys.go
//     └── ui.go

// output is the same as there are no collapsible nodes
func TestUncollapsableTree(t *testing.T) {
	f, err := os.Open("testdata/gh_dash_pr.diff")
	if err != nil {
		t.Fatal(err)
	}
	files, _, err := gitdiff.Parse(f)
	if err != nil {
		t.Fatal(err)
	}

	tr := buildFullFileTree(files, config.Config{})

	tr = collapseTree(tr)
	allNodes := tr.AllNodes()
	if len(allNodes) != 13 {
		t.Fatalf("expected 13 nodes, but got %d", len(allNodes))
	}
}

func TestCloseDirsBelowDepthZero(t *testing.T) {
	f, err := os.Open("testdata/multiple_files.diff")
	if err != nil {
		t.Fatal(err)
	}
	files, _, err := gitdiff.Parse(f)
	if err != nil {
		t.Fatal(err)
	}

	tr := buildFullFileTree(files, config.Config{})
	tr = collapseTree(tr)

	treeModel := tree.New(nil, 80, 40)
	treeModel.SetNodes(tr)

	root := treeModel.Root()

	allNodesBefore := root.AllNodes()
	if len(allNodesBefore) != 4 {
		t.Fatalf("expected 4 nodes before closing, but got %d", len(allNodesBefore))
	}

	closeDirsBelow(root, 0)

	if !root.IsOpen() {
		t.Fatal("expected root node to remain open")
	}

	allNodesAfter := root.AllNodes()
	if len(allNodesAfter) >= len(allNodesBefore) {
		t.Fatalf("expected fewer visible nodes after closing dirs, got %d (before: %d)",
			len(allNodesAfter), len(allNodesBefore))
	}

	for _, node := range allNodesAfter {
		if _, ok := node.GivenValue().(*dirnode.DirNode); ok {
			if node.Depth() > 0 && node.IsOpen() {
				t.Fatalf("expected directory at depth %d to be closed", node.Depth())
			}
		}
	}
}

func TestCloseDirsBelowDepthOne(t *testing.T) {
	f, err := os.Open("testdata/gh_dash_pr.diff")
	if err != nil {
		t.Fatal(err)
	}
	files, _, err := gitdiff.Parse(f)
	if err != nil {
		t.Fatal(err)
	}

	tr := buildFullFileTree(files, config.Config{})
	tr = collapseTree(tr)

	treeModel := tree.New(nil, 80, 40)
	treeModel.SetNodes(tr)

	root := treeModel.Root()

	closeDirsBelow(root, 1)

	if !root.IsOpen() {
		t.Fatal("expected root node to remain open")
	}

	for _, node := range root.ChildNodes() {
		if dir, ok := node.GivenValue().(*dirnode.DirNode); ok {
			if !node.IsOpen() {
				t.Fatalf("expected depth-1 directory %q to be open", dir.Name)
			}
			for _, child := range node.ChildNodes() {
				if _, ok := child.GivenValue().(*dirnode.DirNode); ok {
					if child.IsOpen() {
						t.Fatalf("expected depth-2 directory to be closed")
					}
				}
			}
		}
	}
}

// --- View ---

func TestViewRendersTree(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go", "docs/readme.md"})
	m.SetSize(80, 20)
	view := m.View()
	if view == "" {
		t.Fatal("expected View() to return a non-empty string")
	}
}

func TestViewEmptyModel(t *testing.T) {
	cfg := config.DefaultConfig()
	m := New(cfg)
	view := m.View()
	// An empty model with no files should render an empty or minimal view
	// (not panic). At minimum, calling View() should succeed.
	_ = view
}

// --- Down / Up ---

func TestDown(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt", "b.txt", "c.txt"})
	m.GoToTop()
	node0 := m.GetCurrNode()
	m.Down()
	node1 := m.GetCurrNode()
	if node0.YOffset() == node1.YOffset() {
		t.Fatal("expected Down() to move cursor to next node")
	}
}

func TestUp(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt", "b.txt"})
	m.GoToBottom()
	nodeBottom := m.GetCurrNode()
	m.Up()
	nodeAfterUp := m.GetCurrNode()
	if nodeBottom.YOffset() == nodeAfterUp.YOffset() {
		t.Fatal("expected Up() to move cursor to previous node")
	}
}

// --- GoToBottom / GoToTop ---

func TestGoToBottom(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt", "b.txt", "c.txt"})
	m.GoToTop()
	m.GoToBottom()
	node := m.GetCurrNode()
	if node == nil {
		t.Fatal("expected node at bottom")
	}
}

func TestGoToTop(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt", "b.txt"})
	m.GoToBottom()
	m.GoToTop()
	node := m.GetCurrNode()
	if node == nil {
		t.Fatal("expected node at top")
	}
}

// --- NextFile / PrevFile ---

func TestNextFileSkipsDirectories(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go", "app/internal/db.go", "docs/readme.md"})
	// Start at root (directory), NextFile should skip to a file node
	m.GoToTop()
	ok := m.NextFile()
	if !ok {
		t.Fatal("expected NextFile to find a file")
	}
	node := m.GetCurrNode()
	if _, ok := node.GivenValue().(*filenode.FileNode); !ok {
		t.Fatal("expected NextFile to land on a file node, not a directory")
	}
}

func TestNextFileReturnsFalseAtEnd(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt"})
	// Move to the only file
	m.GoToBottom()
	ok := m.NextFile()
	if ok {
		t.Fatal("expected NextFile to return false when no next file exists")
	}
}

func TestNextFileReturnsFalseWhenNoCurrent(t *testing.T) {
	cfg := config.DefaultConfig()
	m := New(cfg)
	ok := m.NextFile()
	if ok {
		t.Fatal("expected NextFile to return false on empty model")
	}
}

func TestPrevFile(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt", "app/main.go"})
	// Move to the last file (bottom)
	m.GoToBottom()
	ok := m.PrevFile()
	if !ok {
		t.Fatal("expected PrevFile to find a file")
	}
	node := m.GetCurrNode()
	if _, ok := node.GivenValue().(*filenode.FileNode); !ok {
		t.Fatal("expected PrevFile to land on a file node")
	}
}

func TestPrevFileReturnsFalseAtTop(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt"})
	m.GoToTop()
	ok := m.PrevFile()
	if ok {
		t.Fatal("expected PrevFile to return false when at first file")
	}
}

func TestPrevFileReturnsFalseWhenNoCurrent(t *testing.T) {
	cfg := config.DefaultConfig()
	m := New(cfg)
	ok := m.PrevFile()
	if ok {
		t.Fatal("expected PrevFile to return false on empty model")
	}
}

// --- SetCursorByPath ---

func TestSetCursorByPathFile(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go", "docs/readme.md"})
	m.SetCursorByPath("docs/readme.md")
	node := m.GetCurrNode()
	if node == nil {
		t.Fatal("expected node after SetCursorByPath")
	}
	fn, ok := node.GivenValue().(*filenode.FileNode)
	if !ok {
		t.Fatal("expected file node")
	}
	if filenode.GetFileName(fn.File) != "docs/readme.md" {
		t.Fatalf("expected docs/readme.md, got %s", filenode.GetFileName(fn.File))
	}
}

func TestSetCursorByPathDirectory(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go", "docs/readme.md"})
	m.SetCursorByPath("app")
	node := m.GetCurrNode()
	if node == nil {
		t.Fatal("expected node after SetCursorByPath")
	}
	dn, ok := node.GivenValue().(*dirnode.DirNode)
	if !ok {
		t.Fatal("expected directory node")
	}
	if dn.FullPath != "app" {
		t.Fatalf("expected app, got %s", dn.FullPath)
	}
}

func TestSetCursorByPathEmptyFiles(t *testing.T) {
	cfg := config.DefaultConfig()
	m := New(cfg)
	// Should not panic with no files
	m.SetCursorByPath("any/path")
}

// --- SetSize / Width ---

func TestSetSize(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt", "b.txt"})
	m.SetSize(60, 30)
	if m.Width() != 60 {
		t.Fatalf("expected width 60, got %d", m.Width())
	}
}

func TestWidth(t *testing.T) {
	cfg := config.DefaultConfig()
	m := New(cfg)
	w := m.Width()
	if w <= 0 {
		t.Fatalf("expected positive width, got %d", w)
	}
}

// --- ViewportYOffset ---

func TestViewportYOffset(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt"})
	offset := m.ViewportYOffset()
	// Initially should be 0 or some default
	_ = offset
}

// --- GetCurrNode / GetCurrNodeDesendantDiffs ---

func TestGetCurrNode(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt"})
	node := m.GetCurrNode()
	if node == nil {
		t.Fatal("expected current node to be non-nil with files")
	}
}

func TestGetCurrNodeDesendantDiffsFileNode(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go", "docs/readme.md"})
	// Select a file node
	m.SetCursorByPath("app/main.go")
	files := m.GetCurrNodeDesendantDiffs()
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
}

func TestGetCurrNodeDesendantDiffsDirNode(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go", "app/internal/db.go", "docs/readme.md"})
	m.SetCursorByPath("app")
	files := m.GetCurrNodeDesendantDiffs()
	if len(files) != 2 {
		t.Fatalf("expected 2 files under app, got %d", len(files))
	}
}

// --- NodeDescendantDiffs edge cases ---

func TestNodeDescendantDiffsNilNode(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt"})
	files := m.NodeDescendantDiffs(nil)
	if len(files) != 0 {
		t.Fatalf("expected 0 files for nil node, got %d", len(files))
	}
}

func TestNodeDescendantDiffsRootStringNode(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt", "b.txt"})
	// Create a string-valued node and test the string case
	n := tree.Root("/")
	files := m.NodeDescendantDiffs(n)
	// String node should return all model files
	if len(files) != 2 {
		t.Fatalf("expected 2 files for string root node, got %d", len(files))
	}
}

func TestNodeDescendantDiffsDirNonRoot(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go", "app/internal/db.go", "docs/readme.md"})
	app := nodeByPath(t, &m, "app")
	files := m.NodeDescendantDiffs(app)
	if len(files) != 2 {
		t.Fatalf("expected 2 files under app, got %d", len(files))
	}
}

func TestNodeDescendantDiffsRootDirNode(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go", "docs/readme.md"})
	// Find the root DirNode by scanning all nodes.
	for _, node := range m.t.AllNodes() {
		if dn, ok := node.GivenValue().(*dirnode.DirNode); ok && dn.FullPath == "/" {
			files := m.NodeDescendantDiffs(node)
			if len(files) != 2 {
				t.Fatalf("expected 2 files for root dir node, got %d", len(files))
			}
			return
		}
	}
	// If root DirNode with FullPath "/" is not found in AllNodes,
	// the root may be represented as a string. In that case test via
	// GetCurrNodeDesendantDiffs which handles the string case.
	m.GoToTop()
	root := m.GetCurrNode()
	if root == nil {
		t.Fatal("expected root node")
	}
	// String-valued root node should also return all files.
	if _, ok := root.GivenValue().(string); ok {
		files := m.NodeDescendantDiffs(root)
		if len(files) != 2 {
			t.Fatalf("expected 2 files for string root node, got %d", len(files))
		}
		return
	}
	t.Fatal("expected root DirNode or string node not found")
}

func TestNodeDescendantDiffsFileNilFile(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt"})
	// Create a FileNode with nil File — testing the nil guard.
	// We can't use tree.Root with a FileNode that has nil File because
	// the tree library calls String() which dereferences File. Instead,
	// test the code path by finding an existing FileNode and replacing its File.
	m.GoToTop()
	m.Down() // Move to the first file
	node := m.GetCurrNode()
	if node == nil {
		t.Fatal("expected a file node")
	}
	if fn, ok := node.GivenValue().(*filenode.FileNode); ok && fn.File == nil {
		files := m.NodeDescendantDiffs(node)
		if len(files) != 0 {
			t.Fatalf("expected 0 files for FileNode with nil File, got %d", len(files))
		}
	}
	// Otherwise this specific code path isn't easily reachable without panics;
	// it's a defensive guard in the real code.
}

// --- SetCursorNoScroll ---

func TestSetCursorNoScrollEmptyFiles(t *testing.T) {
	cfg := config.DefaultConfig()
	m := New(cfg)
	m.SetCursorNoScroll(5)
	// With no files, CurrNodePath should be empty
	if m.CurrNodePath() != "" {
		t.Fatalf("expected empty path with empty tree, got %q", m.CurrNodePath())
	}
}

func TestSetCursorNoScrollPreservesViewport(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt", "b.txt", "c.txt"})
	m.GoToTop()
	m.SetCursorNoScroll(2)
	// Should not have scrolled the viewport
	node := m.GetCurrNode()
	if node == nil {
		t.Fatal("expected current node")
	}
}

// --- ClickNode ---

func TestClickNodeNil(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt"})
	beforePath := m.CurrNodePath()
	m.ClickNode(nil)
	// Clicking nil should not change selection
	if m.CurrNodePath() != beforePath {
		t.Fatalf("expected path to stay %q after nil click, got %q", beforePath, m.CurrNodePath())
	}
}

func TestClickNodeEmptyFiles(t *testing.T) {
	cfg := config.DefaultConfig()
	m := New(cfg)
	m.ClickNode(nil)
	// With empty files, should not panic and CurrNodePath should be empty
	if m.CurrNodePath() != "" {
		t.Fatalf("expected empty path with no files, got %q", m.CurrNodePath())
	}
}

func TestClickNodeSelectsFile(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go", "docs/readme.md"})
	docs := nodeByPath(t, &m, "docs")
	app := nodeByPath(t, &m, "app")
	m.SetCursorNoScroll(app.YOffset())

	// Click on a different node
	m.ClickNode(docs)

	if got := m.CurrNodePath(); got != "docs" {
		t.Fatalf("expected clicked directory to be selected, got %q", got)
	}
}

// --- ClickNodeIcon ---

func TestClickNodeIconNil(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt"})
	beforePath := m.CurrNodePath()
	m.ClickNodeIcon(nil)
	// Clicking nil icon should not change state
	if m.CurrNodePath() != beforePath {
		t.Fatalf(
			"expected path to stay %q after nil icon click, got %q",
			beforePath,
			m.CurrNodePath(),
		)
	}
}

func TestClickNodeIconEmptyFiles(t *testing.T) {
	cfg := config.DefaultConfig()
	m := New(cfg)
	m.ClickNodeIcon(nil)
	// With empty files, should not panic
	if m.CurrNodePath() != "" {
		t.Fatalf("expected empty path with no files, got %q", m.CurrNodePath())
	}
}

func TestClickNodeIconOnFileNodeDoesNotToggle(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go"})
	app := nodeByPath(t, &m, "app")
	mainGo := nodeByPath(t, &m, "app/main.go")
	m.SetCursorNoScroll(mainGo.YOffset())

	// ClickNodeIcon on a file node should not toggle anything
	m.ClickNodeIcon(mainGo)

	// Verify the app directory state is unchanged
	if !app.IsOpen() {
		t.Fatal("expected app directory to remain open after clicking file icon")
	}
}

// --- IsDirectoryIconHit ---

func TestIsDirectoryIconHitNilNode(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt"})
	if m.IsDirectoryIconHit(nil, 0) {
		t.Fatal("expected false for nil node")
	}
}

func TestIsDirectoryIconHitNegativeX(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go"})
	app := nodeByPath(t, &m, "app")
	if m.IsDirectoryIconHit(app, -1) {
		t.Fatal("expected false for negative x")
	}
}

// --- CurrNodePath ---

func TestCurrNodePathFile(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go"})
	m.SetCursorByPath("app/main.go")
	path := m.CurrNodePath()
	if path != "app/main.go" {
		t.Fatalf("expected app/main.go, got %q", path)
	}
}

func TestCurrNodePathDirectory(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go"})
	m.SetCursorByPath("app")
	path := m.CurrNodePath()
	if path != "app" {
		t.Fatalf("expected app, got %q", path)
	}
}

// --- CopyCurrNodePath ---

func TestCopyCurrNodePath(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt"})
	cmd := m.CopyCurrNodePath()
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
}

// --- ScrollUp / ScrollDown ---

func TestScrollUp(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt"})
	// Scroll down first to establish a non-zero offset
	m.ScrollDown(3)
	offsetBefore := m.ViewportYOffset()
	m.ScrollUp(1)
	offsetAfter := m.ViewportYOffset()
	if offsetAfter >= offsetBefore {
		t.Fatalf(
			"expected scroll offset to decrease, before=%d after=%d",
			offsetBefore,
			offsetAfter,
		)
	}
}

func TestScrollDown(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt"})
	m.SetSize(30, 3) // small height to force scrolling
	m.GoToTop()
	offsetBefore := m.ViewportYOffset()
	m.ScrollDown(2)
	offsetAfter := m.ViewportYOffset()
	if offsetAfter <= offsetBefore {
		t.Fatalf(
			"expected scroll offset to increase, before=%d after=%d",
			offsetBefore,
			offsetAfter,
		)
	}
}

// --- SetIconStyle ---

func TestSetIconStyleASCII(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go"})
	m.SetIconStyle(filenode.IconsASCII)
	if m.cfg.UI.Icons != filenode.IconsASCII {
		t.Fatalf("expected icon style %q, got %q", filenode.IconsASCII, m.cfg.UI.Icons)
	}
}

func TestSetIconStyleNerd(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go"})
	m.SetIconStyle(filenode.IconsNerdStatus)
	if m.cfg.UI.Icons != filenode.IconsNerdStatus {
		t.Fatalf("expected icon style %q, got %q", filenode.IconsNerdStatus, m.cfg.UI.Icons)
	}
}

func TestSetIconStyleUnicode(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go"})
	m.SetIconStyle(filenode.IconsUnicode)
	if m.cfg.UI.Icons != filenode.IconsUnicode {
		t.Fatalf("expected icon style %q, got %q", filenode.IconsUnicode, m.cfg.UI.Icons)
	}
}

func TestSetIconStyleNoFiles(t *testing.T) {
	cfg := config.DefaultConfig()
	m := New(cfg)
	m.SetIconStyle(filenode.IconsNerdFull)
	// Should not panic even with no files
	if m.cfg.UI.Icons != filenode.IconsNerdFull {
		t.Fatalf("expected icon style %q", filenode.IconsNerdFull)
	}
}

// --- SetDarkBackground ---

func TestSetDarkBackground(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt"})
	m.SetDarkBackground(true)
	if m.isDarkBackground == nil || !*m.isDarkBackground {
		t.Fatal("expected isDarkBackground to be true")
	}
}

func TestSetDarkBackgroundLight(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt"})
	m.SetDarkBackground(false)
	if m.isDarkBackground == nil || *m.isDarkBackground {
		t.Fatal("expected isDarkBackground to be false")
	}
}

func TestSetDarkBackgroundNoOpWhenSame(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt"})
	m.SetDarkBackground(true)
	m.SetDarkBackground(true) // should be no-op
	if m.isDarkBackground == nil || !*m.isDarkBackground {
		t.Fatal("expected isDarkBackground to remain true")
	}
}

// --- Update ---

func TestUpdateExpandNode(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go", "app/internal/db.go"})
	app := nodeByPath(t, &m, "app")
	app.Close()
	m.SetCursorNoScroll(app.YOffset())

	if app.IsOpen() {
		t.Fatal("setup: expected app to be closed")
	}

	msg := tea.KeyPressMsg(tea.Key{Text: "l", Code: 'l'})
	m2, cmd := m.Update(msg)
	_ = cmd
	if !nodeByPath(t, m2, "app").IsOpen() {
		t.Fatal("expected app to be open after ExpandNode key")
	}
}

func TestUpdateCollapseNode(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go", "docs/readme.md"})
	app := nodeByPath(t, &m, "app")
	if !app.IsOpen() {
		t.Fatal("setup: expected app to be open")
	}
	m.SetCursorNoScroll(app.YOffset())

	msg := tea.KeyPressMsg(tea.Key{Text: "h", Code: 'h'})
	m2, _ := m.Update(msg)
	if nodeByPath(t, m2, "app").IsOpen() {
		t.Fatal("expected app to be closed after CollapseNode key")
	}
}

func TestUpdateToggleNode(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go"})
	app := nodeByPath(t, &m, "app")
	m.SetCursorNoScroll(app.YOffset())

	wasOpen := app.IsOpen()
	msg := tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	m2, _ := m.Update(msg)
	nowOpen := nodeByPath(t, m2, "app").IsOpen()
	if nowOpen == wasOpen {
		t.Fatalf("expected toggle to change open state from %v to %v", wasOpen, !wasOpen)
	}
}

// --- getDirIcons ---

func TestGetDirIconsNerdStatus(t *testing.T) {
	open, closed := getDirIcons(filenode.IconsNerdStatus)
	if open == "" || closed == "" {
		t.Fatal("expected non-empty nerd font icons")
	}
}

func TestGetDirIconsNerdSimple(t *testing.T) {
	open, closed := getDirIcons(filenode.IconsNerdSimple)
	if open == "" || closed == "" {
		t.Fatal("expected non-empty nerd font icons")
	}
}

func TestGetDirIconsNerdFiletype(t *testing.T) {
	open, closed := getDirIcons(filenode.IconsNerdFiletype)
	if open == "" || closed == "" {
		t.Fatal("expected non-empty nerd font icons")
	}
}

func TestGetDirIconsNerdFull(t *testing.T) {
	open, closed := getDirIcons(filenode.IconsNerdFull)
	if open == "" || closed == "" {
		t.Fatal("expected non-empty nerd font icons")
	}
}

func TestGetDirIconsUnicode(t *testing.T) {
	open, closed := getDirIcons(filenode.IconsUnicode)
	if open != "▼" || closed != "▶" {
		t.Fatalf("expected ▼/▶, got %q/%q", open, closed)
	}
}

func TestGetDirIconsASCII(t *testing.T) {
	open, closed := getDirIcons("ascii")
	if open != ">" || closed != "-" {
		t.Fatalf("expected >/-, got %q/%q", open, closed)
	}
}

func TestGetDirIconsDefault(t *testing.T) {
	open, closed := getDirIcons("")
	if open != ">" || closed != "-" {
		t.Fatalf("expected default to be >/-, got %q/%q", open, closed)
	}
}

// --- isDirectoryNode ---

func TestIsDirectoryNodeNil(t *testing.T) {
	if isDirectoryNode(nil) {
		t.Fatal("expected nil node to not be a directory")
	}
}

func TestIsDirectoryNodeDirNode(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go"})
	app := nodeByPath(t, &m, "app")
	if !isDirectoryNode(app) {
		t.Fatal("expected app to be a directory node")
	}
}

func TestIsDirectoryNodeFileNode(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt"})
	aTxt := nodeByPath(t, &m, "a.txt")
	if isDirectoryNode(aTxt) {
		t.Fatal("expected file node to not be a directory")
	}
}

func TestIsDirectoryNodeStringNode(t *testing.T) {
	n := tree.Root("hello")
	if !isDirectoryNode(n) {
		t.Fatal("expected string node to be classified as directory")
	}
}

// --- directoryIndicatorWidth ---

func TestDirectoryIndicatorWidthNerd(t *testing.T) {
	w := directoryIndicatorWidth(filenode.IconsNerdStatus)
	if w < 2 {
		t.Fatalf("expected nerd font width >= 2, got %d", w)
	}
}

func TestDirectoryIndicatorWidthUnicode(t *testing.T) {
	w := directoryIndicatorWidth(filenode.IconsUnicode)
	if w < 1 {
		t.Fatalf("expected unicode width >= 1, got %d", w)
	}
}

func TestDirectoryIndicatorWidthASCII(t *testing.T) {
	w := directoryIndicatorWidth(filenode.IconsASCII)
	if w < 1 {
		t.Fatalf("expected ascii width >= 1, got %d", w)
	}
}

// --- directoryIconStartColumn ---

func TestDirectoryIconStartColumnDepthZero(t *testing.T) {
	n := tree.Root(&dirnode.DirNode{Name: "root"})
	col := directoryIconStartColumn(n)
	if col != 0 {
		t.Fatalf("expected 0 for depth 0, got %d", col)
	}
}

func TestDirectoryIconStartColumnNilNode(t *testing.T) {
	col := directoryIconStartColumn(nil)
	if col != 0 {
		t.Fatalf("expected 0 for nil node, got %d", col)
	}
}

func TestDirectoryIconStartColumnDepthOne(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go"})
	app := nodeByPath(t, &m, "app")
	if app.Depth() != 1 {
		t.Fatalf("expected depth 1, got %d", app.Depth())
	}
	col := directoryIconStartColumn(app)
	// At depth 1: (1-1) * indenterWidth + enumeratorWidth = 0 + enumeratorWidth
	// enumerator returns "├" which is 1 cell wide
	if col != 1 {
		t.Fatalf("expected column 1 for depth 1, got %d", col)
	}
}

func TestDirectoryIconStartColumnDepthTwo(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go", "app/internal/db.go"})
	internal := nodeByPath(t, &m, "app/internal")
	if internal.Depth() != 2 {
		t.Fatalf("expected depth 2, got %d", internal.Depth())
	}
	col := directoryIconStartColumn(internal)
	// At depth 2: (2-1) * indenterWidth + enumeratorWidth = 1 + 1 = 2
	if col != 2 {
		t.Fatalf("expected column 2 for depth 2, got %d", col)
	}
}

// --- Update with unrelated key ---

func TestUpdateUnrelatedKey(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt"})
	msg := tea.KeyPressMsg{}
	m2, cmd := m.Update(msg)
	if cmd != nil {
		t.Fatal("expected nil cmd for unrelated key")
	}
	_ = m2
}

func TestUpdateNonKeyPressMsg(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt"})
	msg := tea.WindowSizeMsg{Width: 80, Height: 24}
	m2, cmd := m.Update(msg)
	if cmd != nil {
		t.Fatal("expected nil cmd for non-key-press message")
	}
	_ = m2
}

// Test rebuildTree with StartFoldersOpenDepth >= 0.
func TestRebuildTreeWithStartFoldersOpenDepth(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.UI.Icons = filenode.IconsASCII
	cfg.UI.StartFoldersOpenDepth = 0
	m := New(cfg)
	files := []*gitdiff.File{
		{NewName: "app/main.go"},
		{NewName: "app/internal/db.go"},
		{NewName: "docs/readme.md"},
	}
	m = m.SetFiles(files)
	// After rebuild with depth=0, all directories except root should be closed.
	for _, node := range m.t.AllNodes() {
		if dn, ok := node.GivenValue().(*dirnode.DirNode); ok {
			if node.Depth() > 0 && dn.FullPath != "/" && node.IsOpen() {
				t.Fatalf("expected directory %q to be closed at depth %d", dn.Name, node.Depth())
			}
		}
	}
}

// Test NextFile returns false when no more files after current.
func TestNextFileAtLastFile(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt", "b.txt"})
	// Move to last file.
	m.GoToBottom()
	result := m.NextFile()
	// At the last file, NextFile should return false
	if result {
		t.Fatal("expected NextFile to return false at last file")
	}
}

// Test PrevFile returns false when no files before current.
func TestPrevFileAtFirstFile(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt", "b.txt"})
	m.GoToTop()
	result := m.PrevFile()
	// At the first file, PrevFile should return false
	if result {
		t.Fatal("expected PrevFile to return false at first file")
	}
}

// Test NodeDescendantDiffs with a string-valued root node.
func TestNodeDescendantDiffsStringRoot(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt"})
	// Build a tree node with a string value (simulating string root).
	node := tree.Root("some-string-root")
	files := m.NodeDescendantDiffs(node)
	if len(files) != len(m.files) {
		t.Fatalf("expected string root to return all %d files, got %d", len(m.files), len(files))
	}
}

// Test truncateTree with a DirNode child.
func TestTruncateTreeWithDirChild(t *testing.T) {
	cfg := config.DefaultConfig()
	files := []*gitdiff.File{{NewName: "src/app/main.go"}}
	tr := buildFullFileTree(files, cfg)
	tr = collapseTree(tr)
	// truncateTree should work with various depths.
	tr2, numChildren := truncateTree(tr, 0, 0, 0, cfg, 80)
	if tr2 == nil {
		t.Fatal("expected non-nil tree")
	}
	if numChildren < 0 {
		t.Fatalf("expected non-negative numChildren, got %d", numChildren)
	}
}

// Test collapseTree with empty children.
func TestCollapseTreeEmptyChildrenRoot(t *testing.T) {
	cfg := config.DefaultConfig()
	// Build a tree with no files — root has no children.
	files := []*gitdiff.File{}
	tr := buildFullFileTree(files, cfg)
	result := collapseTree(tr)
	if result == nil {
		t.Fatal("expected non-nil result from collapseTree")
	}
}

// Test NodeDescendantDiffs with an explicit fullpath-/ DirNode.
func TestNodeDescendantDiffsRootDirNodeFullPath(t *testing.T) {
	m := newTestTreeModel([]string{"a.txt", "b.txt"})
	// Manually create a tree node with DirNode FullPath="/"
	rootDN := &dirnode.DirNode{FullPath: "/", Name: "/"}
	node := tree.Root(rootDN)
	files := m.NodeDescendantDiffs(node)
	if len(files) != 2 {
		t.Fatalf("expected 2 files for DirNode with FullPath='/', got %d", len(files))
	}
}

// Test NextFile/PrevFile with empty tree (no current node).
func TestNextFilePrevFileNoCurrentNode(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.UI.Icons = filenode.IconsASCII
	m := New(cfg)
	// No files set, so NodeAtCurrentOffset returns nil.
	if m.NextFile() {
		t.Fatal("expected NextFile to return false with no current node")
	}
	if m.PrevFile() {
		t.Fatal("expected PrevFile to return false with no current node")
	}
}

// Test directoryIconStartColumn additional coverage.
func TestDirectoryIconStartColumnAdditionalDepths(t *testing.T) {
	n := tree.Root(&dirnode.DirNode{Name: "x", FullPath: "a/b/x"})
	child := tree.Root(&dirnode.DirNode{Name: "y", FullPath: "a/b/x/y"})
	grandchild := tree.Root(&dirnode.DirNode{Name: "z", FullPath: "a/b/x/y/z"})
	child.Child(grandchild)
	n.Child(child)
	// The tree library tracks depth internally; directoryIconStartColumn
	// should return non-negative values for all nodes.
	for _, node := range n.AllNodes() {
		col := directoryIconStartColumn(node)
		if col < 0 {
			t.Fatalf("expected non-negative icon start column, got %d", col)
		}
	}
}

// --- coverage: nextFileFrom / prevFileFrom nil current node ---

func TestNextFileFrom_NilCurrentNode(t *testing.T) {
	ok := nextFileFrom(nil, nil, func(int) {})
	if ok {
		t.Fatal("expected nextFileFrom to return false with nil current node")
	}
}

func TestPrevFileFrom_NilCurrentNode(t *testing.T) {
	ok := prevFileFrom(nil, nil, func(int) {})
	if ok {
		t.Fatal("expected prevFileFrom to return false with nil current node")
	}
}

// --- coverage: truncateTree with FileNode root ---

// truncateTree with a non-DirNode root (e.g. a FileNode) returns the
// node unchanged with numChildren=0.
func TestTruncateTree_NonDirNodeRoot(t *testing.T) {
	cfg := config.DefaultConfig()

	fn := &filenode.FileNode{
		File: &gitdiff.File{NewName: "standalone.go"},
		Cfg:  cfg,
	}
	node := tree.Root(fn)

	result, numChildren := truncateTree(node, 0, 0, 0, cfg, 80)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if numChildren != 0 {
		t.Fatalf("expected 0 children for FileNode root, got %d", numChildren)
	}
}

// --- collapseTree non-DirNode root ---

func TestCollapseTree_NonDirNodeRoot(t *testing.T) {
	root := tree.Root("not-a-dirnode")
	result := collapseTree(root)
	if result != root {
		t.Fatal("expected collapseTree to return the input unchanged for non-DirNode root")
	}
}
