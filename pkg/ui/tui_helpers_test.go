package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/lukaszcz/diffnav-extra/pkg/filenode"
)

// ---------------------------------------------------------------------------
// relativeTime tests
// ---------------------------------------------------------------------------

func TestRelativeTime(t *testing.T) {
	cases := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"now", 0, "now"},
		{"minutes", 5 * time.Minute, "5m"},
		{"minute_boundary", 59 * time.Second, "now"},
		{"hours", 2 * time.Hour, "2h"},
		{"hour_boundary", 59 * time.Minute, "59m"},
		{"days", 5 * 24 * time.Hour, "5d"},
		{"day_boundary", 23 * time.Hour, "23h"},
		{"months", 90 * 24 * time.Hour, "3mo"},
		{"month_boundary", 29 * 24 * time.Hour, "29d"},
		{"years", 400 * 24 * time.Hour, "1y"},
		{"multiple_years", 800 * 24 * time.Hour, "2y"},
		{"year_boundary", 364 * 24 * time.Hour, "12mo"},
		{"exactly_one_hour", 60 * time.Minute, "1h"},
		{"exactly_one_day", 24 * time.Hour, "1d"},
		{"exactly_30_days", 30 * 24 * time.Hour, "1mo"},
		{"future", 5 * time.Minute, "now"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var ts time.Time
			if tc.name == "future" {
				ts = time.Now().Add(tc.ago) // positive duration = future time
			} else {
				ts = time.Now().Add(-tc.ago)
			}
			got := relativeTime(ts)
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// abs tests
// ---------------------------------------------------------------------------

func TestAbs(t *testing.T) {
	cases := []struct {
		name  string
		input int
		want  int
	}{
		{"positive", 5, 5},
		{"negative", -5, 5},
		{"zero", 0, 0},
		{"large_negative", -1000, 1000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := abs(tc.input); got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// sortFiles tests
// ---------------------------------------------------------------------------

func TestSortFiles(t *testing.T) {
	cases := []struct {
		name      string
		files     []*gitdiff.File
		wantNames []string
	}{
		{
			"same_directory_sorted_alphabetically",
			[]*gitdiff.File{{NewName: "dir/z.txt"}, {NewName: "dir/a.txt"}},
			[]string{"dir/a.txt", "dir/z.txt"},
		},
		{
			"top_level_same_directory_alphabetically",
			[]*gitdiff.File{{NewName: "b.txt"}, {NewName: "a.txt"}},
			[]string{"a.txt", "b.txt"},
		},
		{
			"parent_dir_before_child_dir",
			[]*gitdiff.File{{NewName: "parent/file.txt"}, {NewName: "parent/child/file.txt"}},
			[]string{"parent/child/file.txt", "parent/file.txt"},
		},
		{
			"sibling_directories_sorted_by_full_path",
			[]*gitdiff.File{{NewName: "b-dir/file.txt"}, {NewName: "a-dir/file.txt"}},
			[]string{"a-dir/file.txt", "b-dir/file.txt"},
		},
		{
			"top_level_and_nested_different_dirs",
			[]*gitdiff.File{{NewName: "z.txt"}, {NewName: "a-dir/file.txt"}},
			[]string{"a-dir/file.txt", "z.txt"},
		},
		{
			"empty_slice",
			[]*gitdiff.File{},
			[]string{},
		},
		{
			"case_insensitive",
			[]*gitdiff.File{{NewName: "B.txt"}, {NewName: "a.txt"}},
			[]string{"a.txt", "B.txt"},
		},
		{
			"non_root_before_root_name_dir",
			[]*gitdiff.File{{NewName: "/root-file.txt"}, {NewName: "sub/file.txt"}},
			[]string{"sub/file.txt", "/root-file.txt"},
		},
		{
			"root_name_dir_after_non_root",
			[]*gitdiff.File{{NewName: "sub/file.txt"}, {NewName: "/root-file.txt"}},
			[]string{"sub/file.txt", "/root-file.txt"},
		},
		{
			"parent_dir_comes_before_child_dir",
			[]*gitdiff.File{{NewName: "parent/child/file.txt"}, {NewName: "parent/file.txt"}},
			[]string{"parent/child/file.txt", "parent/file.txt"},
		},
		{
			"child_dir_after_parent_same_dir",
			[]*gitdiff.File{{NewName: "sub/z.txt"}, {NewName: "sub/a.txt"}},
			[]string{"sub/a.txt", "sub/z.txt"},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			sortFiles(tc.files)
			var gotNames []string
			for _, f := range tc.files {
				gotNames = append(gotNames, f.NewName)
			}
			if len(gotNames) != len(tc.wantNames) {
				t.Fatalf("expected %d files, got %d", len(tc.wantNames), len(gotNames))
			}
			for i, got := range gotNames {
				if got != tc.wantNames[i] {
					t.Fatalf("position %d: expected %q, got %q", i, tc.wantNames[i], got)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// cycleIconStyle tests
// ---------------------------------------------------------------------------

func TestCycleIconStyle(t *testing.T) {
	t.Run("Steps", func(t *testing.T) {
		cases := []struct {
			name  string
			input string
			want  string
		}{
			{"ascii_to_unicode", filenode.IconsASCII, filenode.IconsUnicode},
			{"unicode_to_nerd_status", filenode.IconsUnicode, filenode.IconsNerdStatus},
			{"nerd_status_to_nerd_simple", filenode.IconsNerdStatus, filenode.IconsNerdSimple},
			{"nerd_simple_to_nerd_filetype", filenode.IconsNerdSimple, filenode.IconsNerdFiletype},
			{"nerd_filetype_to_nerd_full", filenode.IconsNerdFiletype, filenode.IconsNerdFull},
			{"nerd_full_wraps_to_ascii", filenode.IconsNerdFull, filenode.IconsASCII},
			{"unknown_resets_to_ascii", "unknown-style", filenode.IconsASCII},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				m := newTestMainModel(t)
				m.iconStyle = tc.input
				m.cycleIconStyle()
				if m.iconStyle != tc.want {
					t.Fatalf(
						"expected %q after cycling from %q, got %q",
						tc.want,
						tc.input,
						m.iconStyle,
					)
				}
			})
		}
	})

	t.Run("FullCycle", func(t *testing.T) {
		m := newTestMainModel(t)
		styles := []string{
			filenode.IconsASCII,
			filenode.IconsUnicode,
			filenode.IconsNerdStatus,
			filenode.IconsNerdSimple,
			filenode.IconsNerdFiletype,
			filenode.IconsNerdFull,
		}
		m.iconStyle = styles[0]
		for i := 0; i < len(styles); i++ {
			if m.iconStyle != styles[i] {
				t.Fatalf("step %d: expected %q, got %q", i, styles[i], m.iconStyle)
			}
			m.cycleIconStyle()
		}
		// After full cycle, should wrap back to ASCII
		if m.iconStyle != filenode.IconsASCII {
			t.Fatalf("expected wrap back to ASCII, got %q", m.iconStyle)
		}
	})
}

// ---------------------------------------------------------------------------
// parseCommitMeta tests
// ---------------------------------------------------------------------------

func TestParseCommitMeta(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	t.Run("Static", func(t *testing.T) {
		cases := []struct {
			name       string
			preamble   string
			wantHash   *string
			wantAuthor *string
			wantDate   *string
		}{
			{
				"full_preamble",
				"commit abcdef1234567890 (HEAD -> main)\nAuthor: John Doe <john@example.com>\nAuthorDate: Mon Jan 2 15:04:05 2006 -0700",
				strPtr("abcdef1"),
				strPtr("JDoe"),
				nil,
			},
			{"empty_preamble", "", strPtr(""), strPtr(""), strPtr("")},
			{
				"short_hash",
				"commit abc\nAuthor: Test User <test@test.com>",
				strPtr("abc"), nil, nil,
			},
			{
				"hash_truncated",
				"commit abcdefghijklmnop\nAuthor: Test <t@t.com>",
				strPtr("abcdefg"), nil, nil,
			},
			{
				"hash_with_refs_decoration",
				"commit abcdef1234567890 (HEAD -> main, tag: v1.0)\nAuthor: Jane Smith <jane@example.com>",
				strPtr("abcdef1"),
				nil,
				nil,
			},
			{
				"author_with_email_only",
				"commit abc1234567890\nAuthor: <user@example.com>",
				nil, strPtr("user"), nil,
			},
			{
				"author_single_name",
				"commit abc1234567890\nAuthor: SingleName <s@test.com>",
				nil, strPtr("SingleName"), nil,
			},
			{
				"date_invalid_format",
				"commit abc1234567890\nAuthor: Test <t@t.com>\nAuthorDate: not-a-date",
				nil, nil, strPtr(""),
			},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				m := newTestMainModel(t)
				m.preamble = tc.preamble
				meta := m.parseCommitMeta()
				if tc.wantHash != nil && meta.hash != *tc.wantHash {
					t.Fatalf("expected hash %q, got %q", *tc.wantHash, meta.hash)
				}
				if tc.wantAuthor != nil && meta.author != *tc.wantAuthor {
					t.Fatalf("expected author %q, got %q", *tc.wantAuthor, meta.author)
				}
				if tc.wantDate != nil && meta.date != *tc.wantDate {
					t.Fatalf("expected date %q, got %q", *tc.wantDate, meta.date)
				}
			})
		}
	})

	t.Run("DateAuthorDate", func(t *testing.T) {
		m := newTestMainModel(t)
		past := time.Now().Add(-2 * time.Hour)
		m.preamble = fmt.Sprintf(
			"commit abc1234567890\nAuthor: Test <t@t.com>\nAuthorDate: %s",
			past.Format("Mon Jan 2 15:04:05 2006 -0700"),
		)
		meta := m.parseCommitMeta()
		if meta.date == "" {
			t.Fatal("expected non-empty date from AuthorDate")
		}
	})

	t.Run("DateDate", func(t *testing.T) {
		m := newTestMainModel(t)
		past := time.Now().Add(-30 * time.Minute)
		m.preamble = fmt.Sprintf(
			"commit abc1234567890\nAuthor: Test <t@t.com>\nDate: %s",
			past.Format("Mon Jan 2 15:04:05 2006 -0700"),
		)
		meta := m.parseCommitMeta()
		if meta.date == "" {
			t.Fatal("expected non-empty date from Date")
		}
	})

	t.Run("AuthorDatePreferredOverDate", func(t *testing.T) {
		m := newTestMainModel(t)
		past := time.Now().Add(-30 * time.Minute)
		m.preamble = fmt.Sprintf(
			"commit abc1234567890\nAuthor: Test <t@t.com>\nAuthorDate: %s\nDate: Mon Jan 1 00:00:00 2000 +0000",
			past.Format("Mon Jan 2 15:04:05 2006 -0700"),
		)
		meta := m.parseCommitMeta()
		if meta.date == "" {
			t.Fatal("expected date to be set from AuthorDate")
		}
	})

	t.Run("DateWithDatePrefix", func(t *testing.T) {
		m := newTestMainModel(t)
		past := time.Now().Add(-1 * time.Hour)
		m.preamble = "commit abc123\nDate: " + past.Format("Mon Jan 2 15:04:05 2006 -0700")
		meta := m.parseCommitMeta()
		if meta.date == "" {
			t.Fatal("expected date to be parsed from 'Date:' prefix")
		}
	})
}

// ---------------------------------------------------------------------------
// commitSubject tests
// ---------------------------------------------------------------------------

func TestCommitSubject(t *testing.T) {
	cases := []struct {
		name     string
		preamble string
		want     string
	}{
		{"simple", "commit abc123\nAuthor: Test\nDate: now\n\nFix the bug", "Fix the bug"},
		{"empty_preamble", "", ""},
		{
			"skips_metadata_lines",
			"commit abc123\nAuthor: Test\nAuthorDate: now\nDate: now\nCommit: Test\nCommitDate: now\nMerge: abc def\n\nThe actual subject",
			"The actual subject",
		},
		{"no_subject_only_metadata", "commit abc123\nAuthor: Test\nDate: now", ""},
		{"trims_whitespace", "commit abc123\n  \n  Hello world  ", "Hello world"},
		{
			"with_merge",
			"commit abc123\nMerge: def456\nAuthor: Test\n\nMerge branch 'feature'",
			"Merge branch 'feature'",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestMainModel(t)
			m.preamble = tc.preamble
			got := m.commitSubject()
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// resolveBranch tests
// ---------------------------------------------------------------------------

func TestResolveBranch(t *testing.T) {
	cases := []struct {
		name     string
		preamble string
		want     string
	}{
		{"with_head_arrow", "commit abc123 (HEAD -> feature-branch)", "feature-branch"},
		{"with_multiple_refs", "commit abc123 (HEAD -> main, tag: v1.0, origin/main)", "main"},
		{"no_decoration", "commit 0000000000000000000000000000000000000000", ""},
		{"empty_preamble", "", ""},
		{
			"no_head_arrow",
			"commit 0000000000000000000000000000000000000000 (tag: v1.0, origin/main)",
			"",
		},
		{
			"commit_line_with_extra_content",
			"commit abc123 extra content (HEAD -> develop)",
			"develop",
		},
		{"with_commit_line_and_ref", "commit abc (HEAD -> feature)", "feature"},
		{"only_tag", "commit abc (tag: v1.0)", ""},
		{"commit_hash_truncated_by_space", "commit abc123456 (HEAD -> test-branch)", "test-branch"},
		{"empty_hash_single_space", "commit ", ""},
		{"empty_hash_double_space", "commit  ", ""},
		{"commit_with_only_spaces", "commit \t ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveBranch(tc.preamble)
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// KeyGroups tests
// ---------------------------------------------------------------------------

func TestKeyGroups(t *testing.T) {
	t.Run("ThreeGroups", func(t *testing.T) {
		groups := KeyGroups()
		if len(groups) != 3 {
			t.Fatalf("expected 3 key groups, got %d", len(groups))
		}
	})

	t.Run("NonEmptyGroups", func(t *testing.T) {
		groups := KeyGroups()
		for i, g := range groups {
			if len(g) == 0 {
				t.Fatalf("expected group %d to have bindings", i)
			}
		}
	})

	t.Run("AllBindingsHaveHelp", func(t *testing.T) {
		groups := KeyGroups()
		for i, group := range groups {
			for j, binding := range group {
				help := binding.Help()
				if help.Desc == "" {
					t.Fatalf("group %d binding %d: expected non-empty help description", i, j)
				}
			}
		}
	})
}
