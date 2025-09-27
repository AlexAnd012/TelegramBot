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
var weekdays = map[string]time.Weekday{
	"пн": time.Monday, "вт": time.Tuesday, "ср": time.Wednesday, "чт": time.Thursday, "пт": time.Friday, "сб": time.Saturday, "вс": time.Sunday,
}

type Parsed struct {
	Title       string
	DueUTC      *time.Time
	LeadMinutes int
	RRULE       *string
}

func ParseRU(input, tz string, now time.Time) (*Parsed, error) {
	loc := storage.LoadUserLocation(tz)
	low := strings.ToLower(strings.TrimSpace(input))
	lead := 15

	reDate := regexp.MustCompile(`(?i)\b(\d{1,2})\s+([а-я]+)\s+(\d{1,2}):(\d{2})\b`)
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

	reWD := regexp.MustCompile(`(?i)(пн|вт|ср|чт|пт|сб|вс).{0,10}(\d{1,2}):(\d{2})`)
	if m := reWD.FindStringSubmatch(low); len(m) == 4 {
		wd := weekdays[m[1]]
		hh := toInt(m[2])
		mm := toInt(m[3])
		byday := map[time.Weekday]string{
			time.Monday: "ПН", time.Tuesday: "ВТ", time.Wednesday: "СР", time.Thursday: "ЧТ",
			time.Friday: "ПТ", time.Saturday: "СБ", time.Sunday: "ВС",
		}[wd]
		r := fmt.Sprintf("FREQ=WEEKLY;BYDAY=%s;BYHOUR=%d;BYMINUTE=%d", byday, hh, mm)
		title := strings.TrimSpace(strings.Replace(low, m[0], "", 1))
		if title == "" {
			title = "дело"
		}
		return &Parsed{Title: title, RRULE: &r, LeadMinutes: lead}, nil
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
