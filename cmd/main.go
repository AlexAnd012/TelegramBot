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
		log.Fatal(err)
	}
	resp, err := bot.Request(webhook)
	if err != nil || !resp.Ok {
		log.Fatal(err)
	}

	updates := bot.ListenForWebhook("/webhook")

	// позже заменить на порт от render

	if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil {
		log.Fatal(err)
	}

	for update := range updates {
		if update.Message != nil {
			continue
		}
		msg := BotApi.NewMessage(update.Message.Chat.ID, "hi")
		if _, err := bot.Send(msg); err != nil {
			log.Printf("send error: %v", err)
		}
	}
}
