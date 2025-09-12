package main

import (
	"TelegramBot/internal/config"
	"fmt"
	"log"
	"net/http"

	BotApi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	cfg := config.Load()

	bot, err := BotApi.NewBotAPI(cfg.BotToken)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Authorized on account %s\n", bot.Self.UserName)

	webhook, err := BotApi.NewWebhook(cfg.SelfURL + "/webhook")
	if err != nil {
		log.Fatalf("new webhook: %v", err)
	}
	resp, err := bot.Request(webhook)
	if err != nil || !resp.Ok {
		log.Fatalf("setWebhook failed: err=%v ok=%v desc=%s", err, resp.Ok, resp.Description)
	}
	log.Printf("Webhook set to %s/webhook", cfg.SelfURL)

	updates := bot.ListenForWebhook("/webhook")
	log.Printf("HTTP server listening on :%s", cfg.Port)

	if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil {
		log.Fatalf("http server error: %v", err)
	}

	for update := range updates {
		log.Printf("update: %#v", update)
		if update.Message == nil {
			continue
		}
		msg := BotApi.NewMessage(update.Message.Chat.ID, "hi")
		if _, err := bot.Send(msg); err != nil {
			log.Printf("send error: %v", err)
		}
	}
}
