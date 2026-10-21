package holidays

import "testing"

func has(t *testing.T, code, date, name string) {
	t.Helper()
	for _, h := range Between(code, date, date) {
		if h.Name == name {
			return
		}
	}
	t.Errorf("%s %s: no %q (have %v)", code, date, name, Between(code, date, date))
}

// TestRules: Easter and the days that follow it, fixed dates, "the last
// Monday", and the UK and US weekend substitutes, against known calendars.
func TestRules(t *testing.T) {
	for y, want := range map[int]string{2024: "2024-03-31", 2025: "2025-04-20", 2026: "2026-04-05", 2027: "2027-03-28", 2038: "2038-04-25"} {
		if got := Easter(y).Format("2006-01-02"); got != want {
			t.Errorf("Easter %d = %s, want %s", y, got, want)
		}
	}
	has(t, "PT", "2026-04-03", "Sexta-feira Santa")
	has(t, "PT", "2026-06-04", "Corpo de Deus")
	has(t, "PT", "2026-12-01", "Restauração da Independência")
	if n := len(Between("PT", "2026-01-01", "2026-12-31")); n != 13 {
		t.Errorf("PT 2026: %d holidays", n)
	}
	has(t, "FR", "2026-05-14", "Ascension")
	has(t, "DE", "2026-05-25", "Pfingstmontag")
	// UK: 2022's Christmas on a Sunday → Tuesday 27; 2021's on a Saturday → Mon 27, Tue 28.
	has(t, "GB", "2022-12-27", "Christmas Day")
	has(t, "GB", "2022-12-26", "Boxing Day")
	has(t, "GB", "2021-12-28", "Boxing Day")
	has(t, "GB", "2026-05-25", "Spring bank holiday")
	// US: 2027's Independence Day (a Sunday) is observed on Monday 5 July.
	has(t, "US", "2027-07-05", "Independence Day")
	has(t, "US", "2026-11-26", "Thanksgiving Day")
	has(t, "NL", "2025-05-05", "Bevrijdingsdag")
	if len(Between("NL", "2026-05-05", "2026-05-05")) != 0 {
		t.Error("NL Bevrijdingsdag is a day off every five years")
	}
	if Between("XX", "2026-01-01", "2026-12-31") != nil || Known("XX") || !Known("BR") {
		t.Error("unknown country")
	}
	// Across a new year.
	if n := len(Between("PT", "2026-12-20", "2027-01-05")); n != 2 {
		t.Errorf("across the year: %d", n)
	}
}
