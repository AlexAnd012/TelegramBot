package config

import (
	"log"
	"os"
)

type Config struct {
	BotToken      string
	SelfURL       string
	DBUrl         string
	TimeZone      string
	WebhookSecret string
	Port          string
}

func Load() Config {
	cfg := Config{
		BotToken:      os.Getenv("TELEGRAM_BOT_TOKEN"),
		SelfURL:       os.Getenv("SELF_URL"),
		DBUrl:         os.Getenv("DATABASE_URL"),
		TimeZone:      os.Getenv("TIMEZONE"),
		WebhookSecret: os.Getenv("TG_WEBHOOK_SECRET"),
		Port:          os.Getenv("PORT"),
	}

	if cfg.BotToken == "" {
		log.Fatal("BotToken is empty")
	}
	if cfg.DBUrl == "" {
		log.Fatal("DataBase is not declared")
	}
	if cfg.SelfURL == "" {
		log.Fatal("WebService is not declared")
	}
	if cfg.TimeZone == "" {
		log.Fatal("TimeZone is empty")
	}
	if cfg.WebhookSecret == "" {
		log.Fatal("WebHookSecret is empty")
	}

	return cfg
}
