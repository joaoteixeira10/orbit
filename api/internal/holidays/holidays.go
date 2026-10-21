// Package holidays computes national public holidays from their rules (fixed
// dates, Easter-relative ones, "the last Monday of May", weekend
// substitutes) — no service, no data file. Names are in the country's own
// language. Regional holidays aren't included.
package holidays

import (
	"sort"
	"time"
)

// Holiday is one day off.
type Holiday struct {
	Date string `json:"date"` // YYYY-MM-DD
	Name string `json:"name"`
}

// Country is one country's rules.
type Country struct {
	Code string `json:"code"` // ISO 3166-1 alpha-2
	Name string `json:"name"` // in its own language
	days func(y int) []Holiday
}

// Countries lists what Orbit knows, by code.
var Countries = []Country{
	{"PT", "Portugal", pt},
	{"ES", "España", es},
	{"FR", "France", fr},
	{"IT", "Italia", it},
	{"DE", "Deutschland", de},
	{"GB", "United Kingdom (England and Wales)", gb},
	{"NL", "Nederland", nl},
	{"BE", "België / Belgique", be},
	{"US", "United States", us},
	{"BR", "Brasil", br},
}

// Known reports whether code is a country here.
func Known(code string) bool {
	_, ok := find(code)
	return ok
}

func find(code string) (Country, bool) {
	for _, c := range Countries {
		if c.Code == code {
			return c, true
		}
	}
	return Country{}, false
}

// Between lists a country's holidays from from to to (YYYY-MM-DD, both
// included), by date.
func Between(code, from, to string) []Holiday {
	c, ok := find(code)
	if !ok || from > to {
		return nil
	}
	y0, _ := time.Parse("2006-01-02", from)
	y1, _ := time.Parse("2006-01-02", to)
	var out []Holiday
	for y := y0.Year(); y <= y1.Year(); y++ {
		for _, h := range c.days(y) {
			if h.Date >= from && h.Date <= to {
				out = append(out, h)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

// ---- Date helpers ----

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func h(t time.Time, name string) Holiday { return Holiday{Date: t.Format("2006-01-02"), Name: name} }

func fixed(y int, m time.Month, d int, name string) Holiday { return h(date(y, m, d), name) }

// Easter is Easter Sunday (Gregorian; Meeus/Jones/Butcher).
func Easter(y int) time.Time {
	a := y % 19
	b, c := y/100, y%100
	d, e := b/4, b%4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	hh := (19*a + b - d - g + 15) % 30
	i, k := c/4, c%4
	l := (32 + 2*e + 2*i - hh - k) % 7
	m := (a + 11*hh + 22*l) / 451
	month := (hh + l - 7*m + 114) / 31
	day := (hh+l-7*m+114)%31 + 1
	return date(y, time.Month(month), day)
}

func easter(y, offset int, name string) Holiday { return h(Easter(y).AddDate(0, 0, offset), name) }

// nth is the n-th weekday wd of a month (n = -1: the last).
func nth(y int, m time.Month, wd time.Weekday, n int) time.Time {
	if n < 0 {
		t := date(y, m+1, 1).AddDate(0, 0, -1)
		for t.Weekday() != wd {
			t = t.AddDate(0, 0, -1)
		}
		return t
	}
	t := date(y, m, 1)
	for t.Weekday() != wd {
		t = t.AddDate(0, 0, 1)
	}
	return t.AddDate(0, 0, 7*(n-1))
}

// ---- Countries ----

func pt(y int) []Holiday {
	return []Holiday{
		fixed(y, 1, 1, "Ano Novo"),
		easter(y, -2, "Sexta-feira Santa"),
		easter(y, 0, "Páscoa"),
		fixed(y, 4, 25, "Dia da Liberdade"),
		fixed(y, 5, 1, "Dia do Trabalhador"),
		easter(y, 60, "Corpo de Deus"),
		fixed(y, 6, 10, "Dia de Portugal"),
		fixed(y, 8, 15, "Assunção de Nossa Senhora"),
		fixed(y, 10, 5, "Implantação da República"),
		fixed(y, 11, 1, "Dia de Todos os Santos"),
		fixed(y, 12, 1, "Restauração da Independência"),
		fixed(y, 12, 8, "Imaculada Conceição"),
		fixed(y, 12, 25, "Natal"),
	}
}

func es(y int) []Holiday {
	return []Holiday{
		fixed(y, 1, 1, "Año Nuevo"),
		fixed(y, 1, 6, "Epifanía del Señor"),
		easter(y, -2, "Viernes Santo"),
		fixed(y, 5, 1, "Fiesta del Trabajo"),
		fixed(y, 8, 15, "Asunción de la Virgen"),
		fixed(y, 10, 12, "Fiesta Nacional de España"),
		fixed(y, 11, 1, "Todos los Santos"),
		fixed(y, 12, 6, "Día de la Constitución"),
		fixed(y, 12, 8, "Inmaculada Concepción"),
		fixed(y, 12, 25, "Natividad del Señor"),
	}
}

func fr(y int) []Holiday {
	return []Holiday{
		fixed(y, 1, 1, "Jour de l'an"),
		easter(y, 1, "Lundi de Pâques"),
		fixed(y, 5, 1, "Fête du Travail"),
		fixed(y, 5, 8, "Victoire 1945"),
		easter(y, 39, "Ascension"),
		easter(y, 50, "Lundi de Pentecôte"),
		fixed(y, 7, 14, "Fête nationale"),
		fixed(y, 8, 15, "Assomption"),
		fixed(y, 11, 1, "Toussaint"),
		fixed(y, 11, 11, "Armistice 1918"),
		fixed(y, 12, 25, "Noël"),
	}
}

func it(y int) []Holiday {
	return []Holiday{
		fixed(y, 1, 1, "Capodanno"),
		fixed(y, 1, 6, "Epifania"),
		easter(y, 0, "Pasqua"),
		easter(y, 1, "Lunedì dell'Angelo"),
		fixed(y, 4, 25, "Festa della Liberazione"),
		fixed(y, 5, 1, "Festa del Lavoro"),
		fixed(y, 6, 2, "Festa della Repubblica"),
		fixed(y, 8, 15, "Ferragosto"),
		fixed(y, 11, 1, "Ognissanti"),
		fixed(y, 12, 8, "Immacolata Concezione"),
		fixed(y, 12, 25, "Natale"),
		fixed(y, 12, 26, "Santo Stefano"),
	}
}

func de(y int) []Holiday {
	return []Holiday{
		fixed(y, 1, 1, "Neujahr"),
		easter(y, -2, "Karfreitag"),
		easter(y, 1, "Ostermontag"),
		fixed(y, 5, 1, "Tag der Arbeit"),
		easter(y, 39, "Christi Himmelfahrt"),
		easter(y, 50, "Pfingstmontag"),
		fixed(y, 10, 3, "Tag der Deutschen Einheit"),
		fixed(y, 12, 25, "1. Weihnachtstag"),
		fixed(y, 12, 26, "2. Weihnachtstag"),
	}
}

// gb: England and Wales bank holidays, with the weekend substitutes.
func gb(y int) []Holiday {
	out := []Holiday{}
	ny := date(y, 1, 1)
	switch ny.Weekday() {
	case time.Saturday:
		ny = ny.AddDate(0, 0, 2)
	case time.Sunday:
		ny = ny.AddDate(0, 0, 1)
	}
	out = append(out, h(ny, "New Year's Day"),
		easter(y, -2, "Good Friday"),
		easter(y, 1, "Easter Monday"),
		h(nth(y, 5, time.Monday, 1), "Early May bank holiday"),
		h(nth(y, 5, time.Monday, -1), "Spring bank holiday"),
		h(nth(y, 8, time.Monday, -1), "Summer bank holiday"))
	xmas, boxing := date(y, 12, 25), date(y, 12, 26)
	switch xmas.Weekday() {
	case time.Saturday: // Mon 27, Tue 28
		xmas, boxing = date(y, 12, 27), date(y, 12, 28)
	case time.Sunday: // Boxing Day Mon 26, Christmas Tue 27
		xmas = date(y, 12, 27)
	case time.Friday: // Boxing Day on Saturday: Mon 28
		boxing = date(y, 12, 28)
	}
	return append(out, h(xmas, "Christmas Day"), h(boxing, "Boxing Day"))
}

func nl(y int) []Holiday {
	kings := date(y, 4, 27)
	if kings.Weekday() == time.Sunday {
		kings = kings.AddDate(0, 0, -1)
	}
	out := []Holiday{
		fixed(y, 1, 1, "Nieuwjaarsdag"),
		easter(y, 0, "Eerste Paasdag"),
		easter(y, 1, "Tweede Paasdag"),
		h(kings, "Koningsdag"),
	}
	if y%5 == 0 {
		out = append(out, fixed(y, 5, 5, "Bevrijdingsdag"))
	}
	return append(out,
		easter(y, 39, "Hemelvaartsdag"),
		easter(y, 49, "Eerste Pinksterdag"),
		easter(y, 50, "Tweede Pinksterdag"),
		fixed(y, 12, 25, "Eerste Kerstdag"),
		fixed(y, 12, 26, "Tweede Kerstdag"))
}

func be(y int) []Holiday {
	return []Holiday{
		fixed(y, 1, 1, "Nieuwjaar / Nouvel An"),
		easter(y, 1, "Paasmaandag / Lundi de Pâques"),
		fixed(y, 5, 1, "Dag van de Arbeid / Fête du Travail"),
		easter(y, 39, "O.L.H. Hemelvaart / Ascension"),
		easter(y, 50, "Pinkstermaandag / Lundi de Pentecôte"),
		fixed(y, 7, 21, "Nationale feestdag / Fête nationale"),
		fixed(y, 8, 15, "O.L.V. Hemelvaart / Assomption"),
		fixed(y, 11, 1, "Allerheiligen / Toussaint"),
		fixed(y, 11, 11, "Wapenstilstand / Armistice"),
		fixed(y, 12, 25, "Kerstmis / Noël"),
	}
}

// us: federal holidays, observed on Friday / Monday when on a weekend.
func us(y int) []Holiday {
	observed := func(t time.Time, name string) Holiday {
		switch t.Weekday() {
		case time.Saturday:
			t = t.AddDate(0, 0, -1)
		case time.Sunday:
			t = t.AddDate(0, 0, 1)
		}
		return h(t, name)
	}
	return []Holiday{
		observed(date(y, 1, 1), "New Year's Day"),
		h(nth(y, 1, time.Monday, 3), "Martin Luther King Jr. Day"),
		h(nth(y, 2, time.Monday, 3), "Washington's Birthday"),
		h(nth(y, 5, time.Monday, -1), "Memorial Day"),
		observed(date(y, 6, 19), "Juneteenth"),
		observed(date(y, 7, 4), "Independence Day"),
		h(nth(y, 9, time.Monday, 1), "Labor Day"),
		h(nth(y, 10, time.Monday, 2), "Columbus Day"),
		observed(date(y, 11, 11), "Veterans Day"),
		h(nth(y, 11, time.Thursday, 4), "Thanksgiving Day"),
		observed(date(y, 12, 25), "Christmas Day"),
	}
}

func br(y int) []Holiday {
	out := []Holiday{
		fixed(y, 1, 1, "Confraternização Universal"),
		easter(y, -2, "Paixão de Cristo"),
		fixed(y, 4, 21, "Tiradentes"),
		fixed(y, 5, 1, "Dia do Trabalho"),
		fixed(y, 9, 7, "Independência do Brasil"),
		fixed(y, 10, 12, "Nossa Senhora Aparecida"),
		fixed(y, 11, 2, "Finados"),
		fixed(y, 11, 15, "Proclamação da República"),
	}
	if y >= 2024 {
		out = append(out, fixed(y, 11, 20, "Dia da Consciência Negra"))
	}
	return append(out, fixed(y, 12, 25, "Natal"))
}
