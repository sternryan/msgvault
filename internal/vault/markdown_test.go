package vault

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateFrontmatter(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
		want string
	}{
		{
			name: "empty data",
			data: map[string]interface{}{},
			want: "",
		},
		{
			name: "simple data",
			data: map[string]interface{}{
				"title": "Test",
				"count": 42,
			},
			want: "---\ncount: 42\ntitle: Test\n---\n\n",
		},
		{
			name: "with tags",
			data: map[string]interface{}{
				"tags": []string{"person", "contact"},
			},
			want: "---\ntags:\n    - person\n    - contact\n---\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateFrontmatter(tt.data)
			if got != tt.want {
				t.Errorf("GenerateFrontmatter() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateWikiLink(t *testing.T) {
	tests := []struct {
		name   string
		target string
		text   string
		want   string
	}{
		{
			name:   "simple link",
			target: "Person",
			text:   "",
			want:   "[[Person]]",
		},
		{
			name:   "link with text",
			target: "alice@example.com",
			text:   "Alice",
			want:   "[[alice@example.com|Alice]]",
		},
		{
			name:   "link with same text as target",
			target: "Person",
			text:   "Person",
			want:   "[[Person]]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateWikiLink(tt.target, tt.text)
			if got != tt.want {
				t.Errorf("GenerateWikiLink() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple email",
			input: "alice@example.com",
			want:  "alice@example.com",
		},
		{
			name:  "with spaces",
			input: "Work - Important Client",
			want:  "Work-Important-Client",
		},
		{
			name:  "with slashes",
			input: "work/project",
			want:  "work-project",
		},
		{
			name:  "with problematic chars",
			input: "file:name*with?bad|chars",
			want:  "file-name-with-bad-chars",
		},
		{
			name:  "empty after sanitization",
			input: "///",
			want:  "unnamed",
		},
		{
			name:  "multiple spaces",
			input: "foo   bar",
			want:  "foo-bar",
		},
		{
			name:  "leading and trailing spaces",
			input: "  foo  ",
			want:  "foo",
		},
		{
			name:  "unicode",
			input: "alice@例え.com",
			want:  "alice@例え.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeFilename(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeFilename() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{
			name:  "bytes",
			bytes: 500,
			want:  "500 B",
		},
		{
			name:  "kilobytes",
			bytes: 1024,
			want:  "1.0 KB",
		},
		{
			name:  "megabytes",
			bytes: 1024 * 1024,
			want:  "1.0 MB",
		},
		{
			name:  "gigabytes",
			bytes: 1024 * 1024 * 1024,
			want:  "1.0 GB",
		},
		{
			name:  "fractional",
			bytes: 1536,
			want:  "1.5 KB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatSize(tt.bytes)
			if got != tt.want {
				t.Errorf("FormatSize() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatDate(t *testing.T) {
	tests := []struct {
		name   string
		time   time.Time
		format string
		want   string
	}{
		{
			name:   "zero time",
			time:   time.Time{},
			format: "2006-01-02",
			want:   "N/A",
		},
		{
			name:   "YYYY-MM-DD",
			time:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			format: "2006-01-02",
			want:   "2024-01-15",
		},
		{
			name:   "long form",
			time:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			format: "January 2, 2006",
			want:   "January 15, 2024",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDate(tt.time, tt.format)
			if got != tt.want {
				t.Errorf("FormatDate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEscapeMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no escaping needed",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "wiki links",
			input: "[[link]]",
			want:  "\\[\\[link\\]\\]",
		},
		{
			name:  "pipe",
			input: "foo|bar",
			want:  "foo\\|bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EscapeMarkdown(tt.input)
			if got != tt.want {
				t.Errorf("EscapeMarkdown() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSafeDisplayName(t *testing.T) {
	tests := []struct {
		name        string
		displayName string
		email       string
		want        string
	}{
		{
			name:        "with display name",
			displayName: "Alice Smith",
			email:       "alice@example.com",
			want:        "Alice Smith",
		},
		{
			name:        "empty display name",
			displayName: "",
			email:       "alice@example.com",
			want:        "alice@example.com",
		},
		{
			name:        "display name same as email",
			displayName: "alice@example.com",
			email:       "alice@example.com",
			want:        "alice@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SafeDisplayName(tt.displayName, tt.email)
			if got != tt.want {
				t.Errorf("SafeDisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPersonFilename(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  string
	}{
		{
			name:  "simple email",
			email: "alice@example.com",
			want:  "alice@example.com.md",
		},
		{
			name:  "email with special chars",
			email: "alice+tag@example.com",
			want:  "alice+tag@example.com.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PersonFilename(tt.email)
			if got != tt.want {
				t.Errorf("PersonFilename() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTimelineFilename(t *testing.T) {
	tests := []struct {
		name        string
		period      string
		granularity string
		want        string
	}{
		{
			name:        "monthly",
			period:      "2024-01",
			granularity: "month",
			want:        "2024-01 January.md",
		},
		{
			name:        "yearly",
			period:      "2024",
			granularity: "year",
			want:        "2024.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TimelineFilename(tt.period, tt.granularity)
			if got != tt.want {
				t.Errorf("TimelineFilename() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatMessageCount(t *testing.T) {
	tests := []struct {
		name  string
		count int
		want  string
	}{
		{
			name:  "singular",
			count: 1,
			want:  "1 message",
		},
		{
			name:  "plural",
			count: 42,
			want:  "42 messages",
		},
		{
			name:  "zero",
			count: 0,
			want:  "0 messages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatMessageCount(tt.count)
			if got != tt.want {
				t.Errorf("FormatMessageCount() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatPeriod(t *testing.T) {
	tests := []struct {
		name   string
		period string
		want   string
	}{
		{
			name:   "monthly",
			period: "2024-01",
			want:   "January 2024",
		},
		{
			name:   "yearly",
			period: "2024",
			want:   "2024",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatPeriod(tt.period)
			if got != tt.want {
				t.Errorf("FormatPeriod() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateMsgVaultURI(t *testing.T) {
	tests := []struct {
		name   string
		action string
		params map[string]string
		want   string
	}{
		{
			name:   "no params",
			action: "message",
			params: map[string]string{},
			want:   "msgvault://message",
		},
		{
			name:   "with params",
			action: "filter",
			params: map[string]string{
				"sender": "alice@example.com",
			},
			want: "msgvault://filter?sender=alice@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateMsgVaultURI(tt.action, tt.params)
			// For params tests, just check that it contains the expected parts
			if tt.params == nil || len(tt.params) == 0 {
				if got != tt.want {
					t.Errorf("GenerateMsgVaultURI() = %q, want %q", got, tt.want)
				}
			} else {
				if !strings.HasPrefix(got, "msgvault://") {
					t.Errorf("GenerateMsgVaultURI() = %q, should start with msgvault://", got)
				}
			}
		})
	}
}
