package telegram

import (
	"TelegramBot/internal/storage"
	"context"
	"log"
	"strings"
	"time"

	BotApi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func reply(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := bot.Send(msg); err != nil {
		log.Printf("reply send error (chatID=%d): %v", chatID, err)
	}
}

func HandleMessage(bot *BotApi.BotAPI, store *storage.Storage, message *tgbotapi.Message) {
	chatId := message.Chat.ID
	text := strings.TrimSpace(message.Text)

	_ = store.ChatSettings().UpsertTZ(context.Background(), chatId, "UTC")

	switch {
	case strings.HasPrefix(text, "/start"):
		reply(bot, chatId, "Привет! Я — твой персональный помощник и ассистент от Александра.\nУ меня есть несколько команд, которые я могу выполнить:\n• /timezone — установить часовой пояс\n• /report 20:00 — включить ежедневный отчёт \n• /list — показать запланированные дела\n• /timetable — задать расписание\nА ещё можно просто написать: «во вторник в 14:00 встреча» и я напомню тебе о ней")

	case strings.HasPrefix(text, "/timezone"):
		timezone := strings.TrimSpace(strings.TrimPrefix(text, "/timezone"))
		if timezone == "" {
			reply(bot, chatId, "Пример: \n /timezone Europe/Moscow \n /timezone Asia/Krasnoyarsk ")
			return
		}
		if err := store.ChatSettings().UpsertTZ(context.Background(), chatId, timezone); err != nil {
			reply(bot, chatId, "Не смог сохранить timezone")
		} else {
			reply(bot, chatId, "Часовой пояс обновлён: "+timezone)
		}

	case strings.HasPrefix(text, "/report"):
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/report"))
		if strings.ToLower(arg) == "off" {
			_ = store.ChatSettings().UpsertDigest(context.Background(), chatId, nil)
			reply(bot, chatId, "Ежевечерний отчёт выключен")
			return
		}
		t, err := time.Parse("15:04", arg)
		if err != nil {
			reply(bot, chatId, "Пример:\n /report 20:00 (Ежевечерний отчёт будет приходить в указанное время)\n /digest off(Выключение ежевечернего отчёта)")
			return
		}
		_ = store.ChatSettings().UpsertDigest(context.Background(), chatId, &t)
		reply(bot, chatId, "Ок, буду слать отчёт в "+arg)

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

	}
}
