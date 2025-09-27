package telegram

import (
	"TelegramBot/internal/storage"
	"TelegramBot/internal/timeparse"
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func Reply(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := bot.Send(msg); err != nil {
		log.Printf("reply send error (chatID=%d): %v", chatID, err)
	}
}

func HandleMessage(bot *tgbotapi.BotAPI, store *storage.Storage, message *tgbotapi.Message) {
	chatId := message.Chat.ID
	text := strings.TrimSpace(message.Text)

	_ = store.ChatSettings().UpsertTZ(context.Background(), chatId, "UTC")

	switch {
	case strings.HasPrefix(text, "/start"):
		Reply(bot, chatId, "Привет! Я — твой персональный помощник и ассистент от Александра.\nУ меня есть несколько команд, которые я могу выполнить:\n• /timezone — установить часовой пояс\n• /report 20:00 — включить ежедневный отчёт \n• /list today | week | all — показать запланированные дела\n• /timetable — задать расписание\nА ещё можно просто написать: «во вторник в 14:00 встреча» и я напомню тебе о ней")

	case strings.HasPrefix(text, "/timezone"):
		timezone := strings.TrimSpace(strings.TrimPrefix(text, "/timezone"))
		if timezone == "" {
			Reply(bot, chatId, "Пример: \n /timezone Europe/Moscow \n /timezone Asia/Krasnoyarsk ")
			return
		}
		if err := store.ChatSettings().UpsertTZ(context.Background(), chatId, timezone); err != nil {
			Reply(bot, chatId, "Не смог сохранить timezone")
		} else {
			Reply(bot, chatId, "Часовой пояс обновлён: "+timezone)
		}

	case strings.HasPrefix(text, "/report"):
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/report"))
		if strings.ToLower(arg) == "off" {
			_ = store.ChatSettings().UpsertDigest(context.Background(), chatId, nil)
			Reply(bot, chatId, "Ежевечерний отчёт выключен")
			return
		}
		t, err := time.Parse("15:04", arg)
		if err != nil {
			Reply(bot, chatId, "Пример:\n /report 20:00 (Ежевечерний отчёт будет приходить в указанное время, обязательно указывать в формате HH:MM)\n /report off (Выключение ежевечернего отчёта)")
			return
		}
		_ = store.ChatSettings().UpsertDigest(context.Background(), chatId, &t)
		Reply(bot, chatId, "Ок, буду слать отчёт в "+arg)

	case strings.HasPrefix(text, "/list"):
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/list"))
		if arg == "" {
			arg = "today"
		}
		HandleList(bot, store, chatId, arg)

	case strings.HasPrefix(text, "/timetable"):
		rest := strings.TrimSpace(strings.TrimPrefix(text, "/timetable"))
		HandleTimetable(bot, store, chatId, rest)
	default:
		HandleNaturalReminder(bot, store, message)
	}
}

func HandleList(bot *tgbotapi.BotAPI, store *storage.Storage, chatID int64, arg string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cs, _ := store.ChatSettings().Get(ctx, chatID)
	tz := cs.TimeZone
	if tz == "" {
		tz = "UTC"
	}

	loc := storage.LoadUserLocation(tz)
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
		Reply(bot, chatID, "Использование: /list today | week | all")
		return
	}

	log.Printf("[/list] chat=%d tz=%s arg=%s from=%v to=%v", chatID, tz, arg, fromUTC, toUTC)

	items, err := store.Reminders().GetUpcoming(ctx, chatID, *fromUTC, toUTC, 50)
	if err != nil {
		Reply(bot, chatID, "Не удалось получить список")
		log.Printf("[/list] GetUpcoming error: %v", err)
		return
	}
	log.Printf("[/list] items=%d", len(items))

	if len(items) == 0 {
		Reply(bot, chatID, "Пусто в выбранном диапазоне")
		return
	}

	var b strings.Builder
	for _, r := range items {
		when := "—"
		if r.EventTime != nil {
			when = r.EventTime.In(loc).Format("Mon, 02 Jan 15:04")
		} else if r.NextReport != nil {
			when = r.NextReport.In(loc).Format("Mon, 02 Jan 15:04")
		}
		fmt.Fprintf(&b, "• %s — %s\n", when, r.Message)
	}
	Reply(bot, chatID, b.String())
}

func HandleTimetable(bot *tgbotapi.BotAPI, store *storage.Storage, chatID int64, rest string) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	parts := strings.Fields(rest)
	if len(parts) == 0 {
		Reply(bot, chatID, "Использование:\n/timetable show\n/timetable clear\n/timetable set Пн 10-18 Работа")
		return
	}
	sub := strings.ToLower(parts[0])

	switch sub {
	case "show", "показать":
		var b strings.Builder
		names := []string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}
		for wd := 1; wd <= 7; wd++ {
			entries, err := store.Schedule().ListForWeekday(ctx, chatID, wd)
			if err != nil {
				Reply(bot, chatID, "Ошибка чтения расписания")
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
			Reply(bot, chatID, "Расписание пусто")
			return
		}
		Reply(bot, chatID, b.String())

	case "clear", "очистить":
		if err := store.Schedule().Clear(ctx, chatID); err != nil {
			Reply(bot, chatID, "Не удалось очистить")
			return
		}
		Reply(bot, chatID, "Расписание очищено")

	case "set", "задать":
		raw := strings.TrimSpace(strings.TrimPrefix(rest, parts[0]))
		if raw == "" {
			Reply(bot, chatID, "Пример: /timetable set Пн 10-18 Работа")
			return
		}
		entries, err := timeparse.ParseWeeklyEntries(raw)
		if err != nil {
			Reply(bot, chatID, "Не понял формат. Пример: /timetable set Пн 10-18 Работа")
			return
		}
		if err := store.Schedule().Set(ctx, chatID, entries); err != nil {
			Reply(bot, chatID, "Не удалось сохранить расписание")
			return
		}
		Reply(bot, chatID, "Расписание обновлено")

	default:
		Reply(bot, chatID, "Неизвестная подкоманда. Использование:\n/timetable show | clear | set ...")
	}
}

func HandleNaturalReminder(bot *tgbotapi.BotAPI, store *storage.Storage, m *tgbotapi.Message) {
	chatID := m.Chat.ID
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cs, _ := store.ChatSettings().Get(ctx, chatID)
	tz := cs.TimeZone
	if tz == "" {
		tz = "UTC"
	}

	p, err := timeparse.ParseRU(m.Text, tz, time.Now())
	if err != nil {
		Reply(bot, chatID, "Не понял дату/время 🙈\nПримеры:\n• 25 сентября 14:00 встреча\n• во вторник 18:00 спортзал\n• /add 2025-09-30 14:00 Встреча")
		return
	}

	if p.DueUTC != nil {
		id, err := store.Reminders().AddReminder(ctx, chatID, p.Title, p.DueUTC.UTC(), p.LeadMinutes)
		if err != nil {
			Reply(bot, chatID, "Не смог сохранить напоминание 😔")
			return
		}
		fire := p.DueUTC.Add(-time.Duration(p.LeadMinutes) * time.Minute)
		_ = store.Jobs().Create(ctx, id, fire)

		loc := storage.LoadUserLocation(tz)
		Reply(bot, chatID, fmt.Sprintf("Ок! Напомню %s — %s",
			p.DueUTC.In(loc).Format("Mon, 02 Jan 15:04"), p.Title))
		return
	}

	if p.RRULE != nil {
		next := storage.NextFromWeeklyRRULE(*p.RRULE, tz, time.Now())
		id, err := store.Reminders().AddRecurring(ctx, chatID, p.Title, p.LeadMinutes, *p.RRULE, next)
		if err != nil {
			Reply(bot, chatID, "Не смог сохранить повторяющееся напоминание 😔")
			return
		}
		_ = store.Jobs().Create(ctx, id, next.Add(-time.Duration(p.LeadMinutes)*time.Minute))

		loc := storage.LoadUserLocation(tz)
		Reply(bot, chatID, fmt.Sprintf("Ок! Каждую неделю. Ближайшее: %s — %s",
			next.In(loc).Format("Mon, 02 Jan 15:04"), p.Title))
		return
	}

	Reply(bot, chatID, "Кажется, я не распознал формат. Пример: «25 сентября 14:00 встреча»")
}
