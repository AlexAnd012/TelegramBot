package main

import (
	"TelegramBot/internal/config"
	"TelegramBot/internal/httpserver"
	"TelegramBot/internal/storage"
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	BotApi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	// загрузка конфига
	cfg := config.Load()

	// запуска бота
	bot, err := BotApi.NewBotAPI(cfg.BotToken)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Authorized on account %s\n", bot.Self.UserName)

	// бд
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	store, err := storage.New(ctx, cfg.DBUrl)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	if _, err := store.Now(ctx); err != nil {
		log.Fatalf("db ping failed: %v", err)
	}
	// webhook
	params := BotApi.Params{}
	params.AddNonEmpty("url", cfg.SelfURL+"/webhook")
	params.AddNonEmpty("secret_token", cfg.WebhookSecret)
	params.AddBool("drop_pending_updates", true)

	resp, err := bot.MakeRequest("setWebhook", params)
	if err != nil || !resp.Ok {
		log.Fatalf("setWebhook failed: err=%v ok=%v desc=%s", err, resp.Ok, resp.Description)
	}

	updates := make(chan BotApi.Update, 100)

	workers := 2
	for i := 0; i < workers; i++ {
		go func(id int) {
			for update := range updates {
				HandleUpdate(bot, update, store)
			}
		}(i)
	}
	// HTTP сервер с /webhook и /live
	handler := httpserver.New(cfg.WebhookSecret, updates)

	log.Printf("HTTP server listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, handler); err != nil {
		log.Fatalf("http server error: %v", err)
	}
}

func HandleUpdate(bot *BotApi.BotAPI, update BotApi.Update, store *storage.Storage) {
	if update.Message != nil {
		handleMessage(bot, store, update.Message)
		return

	}
	//if update.CallbackQuery != nil {
	//	handleCallback(bot, store, update.CallbackQuery)
	//	return
	//}
}
func reply(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := bot.Send(msg); err != nil {
		log.Printf("reply send error (chatID=%d): %v", chatID, err)
	}
}

func handleMessage(bot *BotApi.BotAPI, store *storage.Storage, message *tgbotapi.Message) {
	chatId := message.Chat.ID
	text := strings.TrimSpace(message.Text)

	_ = store.ChatSettings().UpsertTZ(context.Background(), chatId, "UTC")

	switch {
	case strings.HasPrefix(text, "/start"):
		reply(bot, chatId, "Привет! Я — персональный секретарь от Александра.\nУ меня есть несколько команд, которые я могу выполнить:\n• /timezone Europe/Moscow — установить часовой пояс\n• /report 20:00 — включить ежедневный отчёт\n• /report off — выключить ежедневный отчёт\n• /list — показать запланированные дела\n• /timetable — задать расписание\nА ещё можно просто написать: «во вторник в 14:00 встреча» и я напомню тебе о ней")

	case strings.HasPrefix(text, "/timezone"):
		// /timezone Europe/Moscow
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
			reply(bot, chatId, "Пример: /digest 20:00 или /digest off")
			return
		}
		_ = store.ChatSettings().UpsertDigest(context.Background(), chatId, &t)
		reply(bot, chatId, "Ок, буду слать отчёт в "+arg)

	case strings.HasPrefix(text, "/list"):

	case strings.HasPrefix(text, "/timetable"):

	default:

	}
}
