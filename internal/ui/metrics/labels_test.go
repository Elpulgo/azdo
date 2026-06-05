package metrics

import "testing"

func TestLabelFor_OverrideWins(t *testing.T) {
	if got := labelFor("Ready for Test", "rft"); got != "rft" {
		t.Errorf("labelFor with override = %q, want rft", got)
	}
	if got := labelFor("Active", "ACT"); got != "ACT" {
		t.Errorf("override should be preserved as-is, got %q", got)
	}
}

func TestLabelFor_AutoDerive_MultiWord(t *testing.T) {
	cases := map[string]string{
		"Ready for Test": "rft",
		"In Progress":    "ip",
		"Ready For Test": "rft",
	}
	for in, want := range cases {
		if got := labelFor(in, ""); got != want {
			t.Errorf("labelFor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLabelFor_AutoDerive_SingleWord(t *testing.T) {
	cases := map[string]string{
		"Active": "active",
		"Closed": "closed",
		"Done":   "done",
		"RFT":    "rft",
	}
	for in, want := range cases {
		if got := labelFor(in, ""); got != want {
			t.Errorf("labelFor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLabelFor_EmptyName(t *testing.T) {
	if got := labelFor("", ""); got != "" {
		t.Errorf("labelFor(empty) = %q, want empty", got)
	}
	if got := labelFor("   ", ""); got != "" {
		t.Errorf("labelFor(whitespace) = %q, want empty", got)
	}
}

func TestLabelTitle_TitleCasesAutoDerived(t *testing.T) {
	if got := labelTitle("Active", ""); got != "Active" {
		t.Errorf("labelTitle(Active) = %q, want Active", got)
	}
	if got := labelTitle("Ready for Test", ""); got != "Rft" {
		t.Errorf("labelTitle(RFT) = %q, want Rft", got)
	}
}

func TestLabelTitle_OverridePreservedAsIs(t *testing.T) {
	if got := labelTitle("Ready for Test", "RFT"); got != "RFT" {
		t.Errorf("labelTitle with override = %q, want RFT", got)
	}
}
