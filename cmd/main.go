package main

import (
	"TelegramBot/internal/config"
	"TelegramBot/internal/httpserver"
	"TelegramBot/internal/storage"
	"TelegramBot/internal/telegram"
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	cfg := config.Load()

	bot, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Authorized on account %s\n", bot.Self.UserName)
	cmds := []tgbotapi.BotCommand{
		{Command: "start", Description: "Помощь и кнопки"},
		{Command: "timezone", Description: "Часовой пояс"},
		{Command: "report", Description: "Ежедневный отчёт (HH:MM | off)"},
		{Command: "list", Description: "Список: today | week | all"},
		{Command: "timetable", Description: "Расписание"},
	}
	if _, err := bot.Request(tgbotapi.NewSetMyCommands(cmds...)); err != nil {
		log.Printf("setMyCommands: %v", err)
	}

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

	params := tgbotapi.Params{}
	params.AddNonEmpty("url", cfg.SelfURL+"/webhook")
	params.AddNonEmpty("secret_token", cfg.WebhookSecret)
	params.AddBool("drop_pending_updates", true)

	resp, err := bot.MakeRequest("setWebhook", params)
	if err != nil || !resp.Ok {
		log.Fatalf("setWebhook failed: err=%v ok=%v desc=%s", err, resp.Ok, resp.Description)
	}

	updates := make(chan tgbotapi.Update, 100)

	workers := 2
	for i := 0; i < workers; i++ {
		go func(id int) {
			for update := range updates {
				HandleUpdate(bot, update, store)
			}
		}(i)
	}

	notifier := &telegram.Notifier{Bot: bot, Store: store}
	go notifier.Run(context.Background())

	handler := httpserver.New(cfg.WebhookSecret, updates)
	log.Printf("HTTP server listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, handler); err != nil {
		log.Fatalf("http server error: %v", err)
	}

}

func HandleUpdate(bot *tgbotapi.BotAPI, update tgbotapi.Update, store *storage.Storage) {
	if update.Message != nil {
		telegram.HandleMessage(bot, store, update.Message)
		return
	}
}
