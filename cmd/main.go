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

	BotApi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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
	// запускаем notifier
	notifier := &telegram.Notifier{Bot: bot, Store: store}
	go notifier.Run(context.Background())

	// HTTP сервер
	handler := httpserver.New(cfg.WebhookSecret, updates)

	log.Printf("HTTP server listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, handler); err != nil {
		log.Fatalf("http server error: %v", err)
	}

}

func HandleUpdate(bot *BotApi.BotAPI, update BotApi.Update, store *storage.Storage) {
	if update.Message != nil {
		telegram.HandleMessage(bot, store, update.Message)
		return
	}
}
