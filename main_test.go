package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseEmailRecipients(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "single recipient",
			in:   "one@example.com",
			want: []string{"one@example.com"},
		},
		{
			name: "comma separated recipients",
			in:   "one@example.com, two@example.com",
			want: []string{"one@example.com", "two@example.com"},
		},
		{
			name: "semicolon separated recipients",
			in:   "one@example.com;two@example.com",
			want: []string{"one@example.com", "two@example.com"},
		},
		{
			name: "mixed separators and whitespace",
			in:   " one@example.com ; two@example.com, three@example.com ",
			want: []string{"one@example.com", "two@example.com", "three@example.com"},
		},
		{
			name: "empty recipients ignored",
			in:   ", ; ,",
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEmailRecipients(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseEmailRecipients(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestLoadConfigRequiresEmailTo(t *testing.T) {
	t.Setenv("FEED_URL", "https://example.com/feed")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("GCP_PROJECT", "")
	t.Setenv("FIREBASE_DATABASE_ID", "")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_USER", "sender@example.com")
	t.Setenv("SMTP_PASS", "secret")
	t.Setenv("EMAIL_TO", "")

	_, err := loadConfig()
	if err == nil {
		t.Fatal("loadConfig() succeeded without EMAIL_TO")
	}
	if !strings.Contains(err.Error(), "EMAIL_TO must contain at least one recipient") {
		t.Fatalf("loadConfig() error = %q, want EMAIL_TO validation error", err)
	}
}
