package main

import (
	"reflect"
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
