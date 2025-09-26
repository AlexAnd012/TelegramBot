package timeparse

import (
	"TelegramBot/internal/storage"
	"fmt"
	"strconv"
	"strings"
	"time"
)

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
