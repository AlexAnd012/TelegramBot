package httpserver

import (
	"encoding/json"
	"io"
	"net/http"

	BotApi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type WebhookHandler struct {
	Secret  string
	Updates chan<- BotApi.Update
}

func NewWebhookHandler(secret string, updates chan<- BotApi.Update) *WebhookHandler {
	return &WebhookHandler{
		Secret:  secret,
		Updates: updates,
	}
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != h.Secret {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	w.WriteHeader(http.StatusOK)

	go func(b []byte) {
		var upd BotApi.Update
		if err := json.Unmarshal(b, &upd); err != nil {
			http.Error(w, "error with Unmarshaling", http.StatusBadRequest)
			return
		}
		h.Updates <- upd
	}(body)

}
