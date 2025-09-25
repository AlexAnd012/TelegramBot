package main

import (
	"TelegramBot/internal/config"
	"TelegramBot/internal/httpserver"
	"TelegramBot/internal/storage"
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	BotApi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {

	cfg := config.Load()

	store, err := storage.New(context.Background(), cfg.DBUrl)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dbNow, err := store.Now(ctx)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("DB OK, now: %s", dbNow.Format(time.RFC3339))

	bot, err := BotApi.NewBotAPI(cfg.BotToken)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Authorized on account %s\n", bot.Self.UserName)

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
				HandleUpdate(bot, update)
			}
		}(i)
	}
	handler := httpserver.New(cfg.WebhookSecret, updates)

	log.Printf("HTTP server listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, handler); err != nil {
		log.Fatalf("http server error: %v", err)
	}
}

func HandleUpdate(bot *BotApi.BotAPI, update BotApi.Update) {
	if update.Message == nil {
		return
	}
	msg := BotApi.NewMessage(update.Message.Chat.ID, "привет")
	if _, err := bot.Send(msg); err != nil {
		log.Printf("send error: %v", err)
	}
}
