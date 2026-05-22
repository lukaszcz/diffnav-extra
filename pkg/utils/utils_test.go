package utils

import "testing"

// ansiReset returns the ANSI reset escape sequence that RemoveReset strips.
func ansiReset() string {
	return string(rune(0x1b)) + "[m"
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		max   int
		want  string
	}{
		{
			name:  "string width less than max no truncation",
			input: "hi",
			max:   10,
			want:  "hi",
		},
		{
			name:  "string width equal to max no truncation",
			input: "hello",
			max:   5,
			want:  "hello",
		},
		{
			name:  "string width greater than max truncated",
			input: "hello world",
			max:   5,
			want:  "hell…",
		},
		{
			name:  "max negative treated as zero",
			input: "hello",
			max:   -1,
			want:  "…",
		},
		{
			name:  "max zero",
			input: "hello",
			max:   0,
			want:  "…",
		},
		{
			name:  "empty string no truncation",
			input: "",
			max:   5,
			want:  "",
		},
		{
			name:  "wide characters width within max no truncation",
			input: "你好", // display width = 4
			max:   5,
			want:  "你好",
		},
		{
			name:  "wide characters width exceeds max truncated",
			input: "你好世界", // display width = 8
			max:   5,
			want:  "你好…",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateString(tt.input, tt.max)
			if got != tt.want {
				t.Errorf("TruncateString(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
			}
		})
	}
}

func TestRemoveReset(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "string with reset codes",
			input: "hello" + ansiReset() + " world",
			want:  "hello world",
		},
		{
			name:  "string without reset codes",
			input: "plain text",
			want:  "plain text",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "multiple reset codes",
			input: "a" + ansiReset() + "b" + ansiReset() + "c",
			want:  "abc",
		},
		{
			name:  "only reset code",
			input: ansiReset(),
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RemoveReset(tt.input)
			if got != tt.want {
				t.Errorf("RemoveReset(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
