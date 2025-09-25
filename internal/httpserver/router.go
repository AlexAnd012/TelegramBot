package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	BotApi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Router struct {
	Secret  string
	Updates chan<- BotApi.Update
	handler http.Handler
}

func New(secret string, updates chan<- BotApi.Update) *Router {
	router := chi.NewRouter()

	router.Post("/webhook", func(w http.ResponseWriter, r *http.Request) {
		NewWebhookHandler(secret, updates).ServeHTTP(w, r)
	})

	router.Get("/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return &Router{
		Secret:  secret,
		Updates: updates,
		handler: router,
	}
}

func (rout *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rout.handler.ServeHTTP(w, r)
}
