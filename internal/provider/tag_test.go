package provider_test

import (
	"errors"
	"testing"

	"github.com/Elpulgo/azdo/internal/provider"
)

func TestNormalizeTag_ValidTagsPassUnchangedKeepingCase(t *testing.T) {
	for _, kind := range []provider.Kind{provider.KindAzure, provider.KindGitHub} {
		for _, raw := range []string{"Bug-fix_2", "åäö", "ÅÄÖ1", "v2"} {
			got, err := provider.NormalizeTag(raw, kind)
			if err != nil {
				t.Errorf("NormalizeTag(%q, %v) error = %v, want nil", raw, kind, err)
				continue
			}
			if got != raw {
				t.Errorf("NormalizeTag(%q, %v) = %q, want %q", raw, kind, got, raw)
			}
		}
	}
}

func TestNormalizeTag_TrimsSurroundingWhitespace(t *testing.T) {
	got, err := provider.NormalizeTag("  abc  ", provider.KindGitHub)
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if got != "abc" {
		t.Errorf("got %q, want %q", got, "abc")
	}
}

func TestNormalizeTag_RejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"whitespace only", " \t "},
		{"exclamation mark", "my tag!"},
		{"colon", "a:b"},
		{"slash", "a/b"},
		{"leading separators", "--error"},
		{"trailing hyphen", "a-"},
		{"leading underscore", "_a"},
		{"doubled hyphen", "a--b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := provider.NormalizeTag(tc.raw, provider.KindAzure)
			if !errors.Is(err, provider.ErrInvalidTag) {
				t.Errorf("NormalizeTag(%q) = (%q, %v), want ErrInvalidTag", tc.raw, got, err)
			}
		})
	}
}

func TestNormalizeTag_InnerSpaceAllowedOnlyForAzure(t *testing.T) {
	cases := []struct {
		raw    string
		kind   provider.Kind
		wantOK bool
	}{
		{"a b", provider.KindAzure, true},
		{"a b", provider.KindGitHub, false},
		{"a  b", provider.KindAzure, false},
		{"a  b", provider.KindGitHub, false},
	}
	for _, tc := range cases {
		got, err := provider.NormalizeTag(tc.raw, tc.kind)
		if tc.wantOK {
			if err != nil || got != tc.raw {
				t.Errorf("NormalizeTag(%q, %v) = (%q, %v), want (%q, nil)", tc.raw, tc.kind, got, err, tc.raw)
			}
			continue
		}
		if !errors.Is(err, provider.ErrInvalidTag) {
			t.Errorf("NormalizeTag(%q, %v) = (%q, %v), want ErrInvalidTag", tc.raw, tc.kind, got, err)
		}
	}
}
