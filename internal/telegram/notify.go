package telegram

import (
	"TelegramBot/internal/storage"
	"TelegramBot/internal/timeparse"
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func loadUserLocation(tz string) *time.Location {
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	return time.UTC
}

func HandleList(bot *tgbotapi.BotAPI, store *storage.Storage, chatID int64, arg string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cs, _ := store.ChatSettings().Get(ctx, chatID)
	loc := loadUserLocation(cs.TimeZone)
	now := time.Now().In(loc)

	var fromUTC, toUTC *time.Time

	switch strings.ToLower(arg) {
	case "today", "сегодня":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		end := start.Add(24 * time.Hour)
		f, t := start.UTC(), end.UTC()
		fromUTC, toUTC = &f, &t
	case "week", "неделя":
		dow := int(now.Weekday())
		if dow == 0 {
			dow = 7
		} // 1..7
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -(dow - 1))
		end := start.AddDate(0, 0, 7)
		f, t := start.UTC(), end.UTC()
		fromUTC, toUTC = &f, &t
	case "all", "все":
		f := time.Now().UTC()
		fromUTC = &f
	default:
		reply(bot, chatID, "Использование: /list today | week | all")
		return
	}

	list, err := store.Reminders().GetUpcoming(ctx, chatID, *fromUTC, toUTC, 50)
	if err != nil {
		reply(bot, chatID, "Не удалось получить список")
		return
	}
	if len(list) == 0 {
		reply(bot, chatID, "Пусто в выбранном диапазоне")
		return
	}

	var b strings.Builder
	for _, r := range list {
		when := "—"
		if r.EventTime != nil {
			when = r.EventTime.In(loc).Format("Mon, 02 Jan 15:04")
		} else if r.NextReport != nil {
			when = r.NextReport.In(loc).Format("Mon, 02 Jan 15:04")
		}
		fmt.Fprintf(&b, "• %s — %s\n", when, r.Message)
	}
	reply(bot, chatID, b.String())
}

func HandleTimetable(bot *tgbotapi.BotAPI, store *storage.Storage, chatID int64, rest string) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	parts := strings.Fields(rest)
	if len(parts) == 0 {
		reply(bot, chatID, "Использование:\n/timetable show\n/timetable clear\n/timetable set Пн 10-18 Работа; Ср 19:00 Английский")
		return
	}
	sub := strings.ToLower(parts[0])

	switch sub {
	case "show", "показать":
		// выведем весь недельный план
		var b strings.Builder
		names := []string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}
		for wd := 1; wd <= 7; wd++ {
			entries, err := store.Schedule().ListForWeekday(ctx, chatID, wd)
			if err != nil {
				reply(bot, chatID, "Ошибка чтения расписания")
				return
			}
			if len(entries) == 0 {
				continue
			}
			fmt.Fprintf(&b, "%s:\n", names[wd-1])
			for _, e := range entries {
				st := e.StartTime.Format("15:04")
				et := ""
				if e.EndTime != nil {
					et = "–" + e.EndTime.Format("15:04")
				}
				fmt.Fprintf(&b, "  %s%s — %s\n", st, et, e.Title)
			}
		}
		if b.Len() == 0 {
			reply(bot, chatID, "Расписание пусто")
			return
		}
		reply(bot, chatID, b.String())

	case "clear", "очистить":
		if err := store.Schedule().Clear(ctx, chatID); err != nil {
			reply(bot, chatID, "Не удалось очистить")
			return
		}
		reply(bot, chatID, "Расписание очищено")

	case "set", "задать":
		// всё после "set" — это описание
		raw := strings.TrimSpace(strings.TrimPrefix(rest, parts[0]))
		if raw == "" {
			reply(bot, chatID, "Пример: /timetable set Пн 10-18 Работа; Ср 19:00 Английский")
			return
		}
		entries, err := timeparse.ParseWeeklyEntries(raw)
		if err != nil {
			reply(bot, chatID, "Не понял формат. Пример: /timetable set Пн 10-18 Работа; Ср 19:00 Английский")
			return
		}
		if err := store.Schedule().Set(ctx, chatID, entries); err != nil {
			reply(bot, chatID, "Не удалось сохранить расписание")
			return
		}
		reply(bot, chatID, "Расписание обновлено")

	default:
		reply(bot, chatID, "Неизвестная подкоманда. Использование:\n/timetable show | clear | set ...")
	}
}
