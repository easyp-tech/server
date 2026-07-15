package content

import (
	"testing"
)

func TestIsSHA(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{name: "40-char hex", in: "81353411f7b010d5b9ebeb1899066aac18a36701", want: true},
		{name: "64-char hex", in: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", want: true},
		{name: "empty string", in: "", want: false},
		{name: "main", in: "main", want: false},
		{name: "main/v2", in: "main/v2", want: false},
		{name: "32-char hex (buf UUID)", in: "81353411f7b010d5b9ebeb1899066aac", want: false},
		{name: "40-char uppercase hex", in: "81353411F7B010D5B9EBEB1899066AAC18A36701", want: false},
		{name: "40-char hex with non-hex char", in: "81353411f7b010d5b9ebeb1899066aac18a3670g", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsSHA(tc.in); got != tc.want {
				t.Errorf("IsSHA(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsConventionalDefaultName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{name: "main", in: "main", want: true},
		{name: "master", in: "master", want: true},
		{name: "develop", in: "develop", want: true},
		{name: "trunk", in: "trunk", want: true},
		{name: "production", in: "production", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsConventionalDefaultName(tc.in); got != tc.want {
				t.Errorf("IsConventionalDefaultName(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
