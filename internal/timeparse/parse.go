package timeparse

import (
	"TelegramBot/internal/storage"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var months = map[string]time.Month{
	"январ": 1, "феврал": 2, "март": 3, "апрел": 4, "ма": 5, "июн": 6, "июл": 7, "август": 8, "сентябр": 9, "октябр": 10, "ноябр": 11, "декабр": 12,
}

type Parsed struct {
	Title       string
	DueUTC      *time.Time
	LeadMinutes int
	RRULE       *string
}

// ParseRU разбирает русскоязычные фразы про дату/время и lead.
// Поддерживает:
//   - "за 30 минут" / "за 2 часа" (lead; по умолчанию 30 минут);
//   - "сегодня|завтра|послезавтра [HH:MM] <текст>";
//   - "DD <месяц-рус> [в] HH:MM <текст>" (напр. "27 сентября в 22:40 встреча");
//   - "YYYY-MM-DD HH:MM <текст>" (ISO);
//   - "[по|каждый|каждое] <день недели> [HH:MM] <текст>" — разово или еженедельно (RRULE).
func ParseRU(input, tz string, now time.Time) (*Parsed, error) {
	loc := storage.LoadUserLocation(tz)
	low := strings.ToLower(strings.TrimSpace(input))

	// --- lead: "за 30 минут"/"за 2 часа"/"за 5 мин"/"за 1 ч"
	lead := 30 // дефолт — 30 минут
	reLead := regexp.MustCompile(`\bза\s+(\d+)\s*(мин(?:ут[а-я]*)?|м|ч(?:ас(?:а|ов)?)?)?\b`)
	if m := reLead.FindStringSubmatch(low); len(m) >= 2 {
		n := toInt(m[1])
		unit := ""
		if len(m) >= 3 {
			unit = m[2]
		}
		switch {
		case unit == "" || strings.HasPrefix(unit, "м"): // мин, минуты, м
			lead = n
		case strings.HasPrefix(unit, "ч"): // ч, час, часа, часов
			lead = n * 60
		}
		// убрать фрагмент "за N ..." из текста, чтобы не мешал парсингу дат
		low = strings.Replace(low, m[0], "", 1)
		low = strings.TrimSpace(low)
	}

	// --- относительные дни: сегодня/завтра/послезавтра [HH:MM]
	reRel := regexp.MustCompile(`\b(сегодня|завтра|послезавтра)\b(?:[^0-9]{0,10}(\d{1,2})[:.](\d{2}))?`)
	if m := reRel.FindStringSubmatch(low); len(m) >= 2 {
		base := now.In(loc)
		switch m[1] {
		case "завтра":
			base = base.AddDate(0, 0, 1)
		case "послезавтра":
			base = base.AddDate(0, 0, 2)
		}
		hh, mm := 9, 0
		if len(m) == 4 && m[2] != "" && m[3] != "" {
			hh = toInt(m[2])
			mm = toInt(m[3])
		}
		local := time.Date(base.Year(), base.Month(), base.Day(), hh, mm, 0, 0, loc)
		// если получилось в прошлом — сместим на следующий день
		if !local.After(now.In(loc)) {
			local = local.Add(24 * time.Hour)
		}
		utc := local.UTC()
		title := strings.TrimSpace(strings.Replace(low, m[0], "", 1))
		if title == "" {
			title = "дело"
		}
		return &Parsed{Title: title, DueUTC: &utc, LeadMinutes: lead}, nil
	}

	// --- конкретная дата: "DD <месяц-рус> [в] HH:MM"
	// допускаем запятую, разные пробелы и "22.40" вместо "22:40"
	reDate := regexp.MustCompile(`\b(\d{1,2})\s+([а-яё]+)\s*(?:,)?\s*(?:в\s*)?(\d{1,2})[:.](\d{2})\b`)
	if m := reDate.FindStringSubmatch(low); len(m) == 5 {
		day := toInt(m[1])
		mon := detectMonth(m[2])
		hh := toInt(m[3])
		mm := toInt(m[4])
		if mon != 0 {
			y := now.In(loc).Year()
			local := time.Date(y, mon, day, hh, mm, 0, 0, loc)
			utc := local.UTC()
			title := strings.TrimSpace(strings.Replace(low, m[0], "", 1))
			if title == "" {
				title = "дело"
			}
			return &Parsed{Title: title, DueUTC: &utc, LeadMinutes: lead}, nil
		}
	}

	// --- ISO: "YYYY-MM-DD HH:MM"
	reISO := regexp.MustCompile(`\b(\d{4})-(\d{2})-(\d{2})\s+(\d{1,2})[:.](\d{2})\b`)
	if m := reISO.FindStringSubmatch(low); len(m) == 6 {
		y := toInt(m[1])
		mon := toInt(m[2])
		day := toInt(m[3])
		hh := toInt(m[4])
		mm := toInt(m[5])
		local := time.Date(y, time.Month(mon), day, hh, mm, 0, 0, loc)
		utc := local.UTC()
		title := strings.TrimSpace(strings.Replace(low, m[0], "", 1))
		if title == "" {
			title = "дело"
		}
		return &Parsed{Title: title, DueUTC: &utc, LeadMinutes: lead}, nil
	}

	// --- день недели: "[по|каждый||каждую|каждое]? <день недели> [HH:MM]"
	reWD := regexp.MustCompile(`\b(по|каждый|каждое)?\s*(?:в|во)?\s*(понедельник|вторник|среда|среду|четверг|пятница|пятницу|суббота|субботу|воскресенье)\b(?:[^0-9]{0,10}(\d{1,2})[:.](\d{2}))?`)
	if m := reWD.FindStringSubmatch(low); len(m) >= 4 {
		wd := map[string]time.Weekday{
			"понедельник": time.Monday,
			"вторник":     time.Tuesday,
			"среда":       time.Wednesday,
			"среду":       time.Wednesday,
			"четверг":     time.Thursday,
			"пятница":     time.Friday,
			"пятницу":     time.Friday,
			"суббота":     time.Saturday,
			"субботу":     time.Saturday,
			"воскресенье": time.Sunday,
		}[m[2]]

		hh, mm := 9, 0
		if len(m) >= 5 && m[3] != "" {
			hh = toInt(m[3])
			mm = toInt(m[4])
		}

		weekly := false
		if s := strings.TrimSpace(m[1]); s != "" {
			weekly = true
		}

		title := strings.TrimSpace(strings.Replace(low, m[0], "", 1))
		if title == "" {
			title = "дело"
		}

		if weekly {
			byday := map[time.Weekday]string{
				time.Monday:    "MO",
				time.Tuesday:   "TU",
				time.Wednesday: "WE",
				time.Thursday:  "TH",
				time.Friday:    "FR",
				time.Saturday:  "SA",
				time.Sunday:    "SU",
			}[wd]
			r := fmt.Sprintf("FREQ=WEEKLY;BYDAY=%s;BYHOUR=%d;BYMINUTE=%d", byday, hh, mm)
			return &Parsed{Title: title, RRULE: &r, LeadMinutes: lead}, nil
		}

		cur := now.In(loc)
		cand := time.Date(cur.Year(), cur.Month(), cur.Day(), hh, mm, 0, 0, loc)
		for i := 0; i < 7 && cand.Weekday() != wd; i++ {
			cand = cand.Add(24 * time.Hour)
		}
		if !cand.After(cur) {
			cand = cand.Add(7 * 24 * time.Hour)
		}
		utc := cand.UTC()
		return &Parsed{Title: title, DueUTC: &utc, LeadMinutes: lead}, nil
	}

	return nil, fmt.Errorf("не распознал дату/время")
}

func detectMonth(s string) time.Month {
	for k, m := range months {
		if strings.HasPrefix(s, k) {
			return m
		}
	}
	return 0
}
func toInt(s string) int {
	n := 0
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			n = n*10 + int(ch-'0')
		}
	}
	return n
}

var ruWeek = map[string]int{
	"пн": 1, "пон": 1, "понедельник": 1,
	"вт": 2, "втор": 2, "вторник": 2,
	"ср": 3, "среда": 3, "ср.": 3,
	"чт": 4, "чет": 4, "четверг": 4,
	"пт": 5, "пят": 5, "пятница": 5,
	"сб": 6, "суб": 6, "суббота": 6,
	"вс": 7, "воск": 7, "воскресенье": 7,
}

func ParseWeeklyEntries(raw string) ([]storage.WeeklyEntry, error) {
	parts := strings.Split(raw, ";")
	var out []storage.WeeklyEntry
	for _, seg := range parts {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}

		fields := strings.Fields(seg)
		if len(fields) < 3 {
			return nil, fmt.Errorf("bad segment: %q", seg)
		}

		wdRaw := strings.ToLower(strings.Trim(fields[0], ".,"))
		wd, ok := ruWeek[wdRaw]
		if !ok {
			return nil, fmt.Errorf("bad weekday: %q", fields[0])
		}

		timeRaw := fields[1]
		title := strings.TrimSpace(strings.Join(fields[2:], " "))

		st, et, err := ParseTimeRange(timeRaw)
		if err != nil {
			return nil, err
		}

		out = append(out, storage.WeeklyEntry{
			Weekday:   wd,
			StartTime: st,
			EndTime:   et,
			Title:     title,
		})
	}
	return out, nil
}

func ParseTimeRange(s string) (time.Time, *time.Time, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "–", "-")
	if strings.Contains(s, "-") {
		ab := strings.SplitN(s, "-", 2)
		a, b := strings.TrimSpace(ab[0]), strings.TrimSpace(ab[1])
		st, err := ParseHM(a)
		if err != nil {
			return time.Time{}, nil, err
		}
		et, err := ParseHM(b)
		if err != nil {
			return time.Time{}, nil, err
		}
		return st, &et, nil
	}
	st, err := ParseHM(s)
	if err != nil {
		return time.Time{}, nil, err
	}
	return st, nil, nil
}

func ParseHM(s string) (time.Time, error) {
	if strings.Contains(s, ":") {
		t, err := time.Parse("15:04", s)
		if err != nil {
			return time.Time{}, fmt.Errorf("bad time %q", s)
		}
		return t, nil
	}
	h, err := strconv.Atoi(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("bad hour %q", s)
	}
	if h < 0 || h > 23 {
		return time.Time{}, fmt.Errorf("hour out of range")
	}
	return time.Date(0, 1, 1, h, 0, 0, 0, time.UTC), nil
}
