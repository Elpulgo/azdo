package metrics

import "testing"

func TestStateConfig_IsActive_CaseInsensitive(t *testing.T) {
	sc := StateConfig{Active: "Active", ReadyForTest: "Ready for Test", Closed: "Closed"}
	cases := []string{"Active", "active", "ACTIVE", "  Active  "}
	for _, s := range cases {
		if !sc.IsActive(s) {
			t.Errorf("IsActive(%q) = false, want true", s)
		}
	}
	if sc.IsActive("Ready for Test") {
		t.Error("IsActive matched RFT; want false")
	}
}

func TestStateConfig_IsRFT_DualCasing(t *testing.T) {
	// The user's actual data has both spellings; case-insensitive matching
	// must bucket them together.
	sc := StateConfig{Active: "Active", ReadyForTest: "Ready for Test", Closed: "Closed"}
	for _, s := range []string{"Ready for Test", "Ready For Test", "ready for test", "READY FOR TEST"} {
		if !sc.IsRFT(s) {
			t.Errorf("IsRFT(%q) = false, want true", s)
		}
	}
}

func TestStateConfig_IsClosed(t *testing.T) {
	sc := StateConfig{Active: "Active", ReadyForTest: "Ready for Test", Closed: "Done"}
	if !sc.IsClosed("Done") {
		t.Error("IsClosed(Done) = false, want true")
	}
	if sc.IsClosed("Closed") {
		t.Error("IsClosed(Closed) = true; want false (canonical name is Done)")
	}
}

func TestStateConfig_Order(t *testing.T) {
	sc := StateConfig{Active: "Active", ReadyForTest: "RFT", Closed: "Done"}
	order := sc.Order()
	want := []string{"Active", "RFT", "Done"}
	if len(order) != len(want) {
		t.Fatalf("Order() length = %d, want %d", len(order), len(want))
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("Order()[%d] = %q, want %q", i, order[i], want[i])
		}
	}
}

func TestStateConfig_Index_CaseInsensitive(t *testing.T) {
	sc := StateConfig{Active: "Active", ReadyForTest: "Ready for Test", Closed: "Closed"}
	cases := []struct {
		state string
		want  int
		ok    bool
	}{
		{"Active", 0, true},
		{"active", 0, true},
		{"Ready for Test", 1, true},
		{"ready for test", 1, true},
		{"READY FOR TEST", 1, true},
		{"Closed", 2, true},
		{"New", -1, false},
		{"", -1, false},
	}
	for _, c := range cases {
		got, ok := sc.Index(c.state)
		if ok != c.ok {
			t.Errorf("Index(%q) ok = %v, want %v", c.state, ok, c.ok)
		}
		if got != c.want {
			t.Errorf("Index(%q) = %d, want %d", c.state, got, c.want)
		}
	}
}

func TestDefaultStates_Values(t *testing.T) {
	sc := DefaultStates()
	if sc.Active != "Active" || sc.ReadyForTest != "Ready for Test" || sc.Closed != "Closed" {
		t.Errorf("DefaultStates = %+v, want {Active, Ready for Test, Closed}", sc)
	}
}
