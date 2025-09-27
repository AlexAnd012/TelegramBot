package telegram

import (
	"TelegramBot/internal/storage"
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Notifier struct {
	Bot   *tgbotapi.BotAPI
	Store *storage.Storage

	lastDigest map[int64]time.Time
	mu         sync.Mutex
}

func (n *Notifier) Run(ctx context.Context) {
	if n.lastDigest == nil {
		n.lastDigest = make(map[int64]time.Time)
	}
	jobsTicker := time.NewTicker(1 * time.Minute)
	digestTicker := time.NewTicker(30 * time.Second)
	defer jobsTicker.Stop()
	defer digestTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-jobsTicker.C:
			n.processDueJobs()
		case <-digestTicker.C:
			n.processDailyDigests()
		}
	}
}

func (n *Notifier) processDueJobs() {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	now := time.Now().UTC()
	jobs, err := n.Store.Jobs().Due(ctx, now, 200)
	if err != nil {
		log.Printf("jobs.Due error: %v", err)
		return
	}
	for _, j := range jobs {
		msg := tgbotapi.NewMessage(j.ChatID, "Напоминание: "+j.Message)
		if _, err := n.Bot.Send(msg); err != nil {
			log.Printf("send reminder error: %v", err)
			continue
		}
		_ = n.Store.Jobs().MarkSent(context.Background(), j.ID)

		if j.ReminderRule != nil && *j.ReminderRule != "" {
			cs, _ := n.Store.ChatSettings().Get(context.Background(), j.ChatID)
			next := NextFromWeeklyRRULE(*j.ReminderRule, cs.TimeZone, time.Now())
			_ = n.Store.Reminders().UpdateNextReport(context.Background(), j.ReminderID, &next)
			_ = n.Store.Jobs().Create(context.Background(), j.ReminderID, next.Add(-time.Duration(j.ReminderTime)*time.Minute))
		}
	}
}

func (n *Notifier) processDailyDigests() {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	chats, err := n.Store.ChatSettings().ChatsToDigestNow(ctx)
	if err != nil {
		log.Printf("digest query error: %v", err)
		return
	}

	for _, ch := range chats {
		loc := storage.LoadUserLocation(ch.TimeZone)
		nowLocal := time.Now().In(loc)

		target := time.Date(
			nowLocal.Year(), nowLocal.Month(), nowLocal.Day(),
			ch.Daily.Hour(), ch.Daily.Minute(), 0, 0, loc,
		)

		diff := nowLocal.Sub(target)
		if diff < -1*time.Minute || diff > 1*time.Minute {
			continue
		}

		n.mu.Lock()
		if last, ok := n.lastDigest[ch.ChatID]; ok {
			if last.In(loc).Year() == nowLocal.Year() && last.In(loc).YearDay() == nowLocal.YearDay() {
				n.mu.Unlock()
				continue
			}
		}
		n.mu.Unlock()

		start := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc).Add(24 * time.Hour)
		end := start.Add(24 * time.Hour)
		sUTC, eUTC := start.UTC(), end.UTC()

		items, err := n.Store.Reminders().GetUpcoming(ctx, ch.ChatID, sUTC, &eUTC, 100)
		if err != nil {
			log.Printf("digest fetch error chat=%d: %v", ch.ChatID, err)
			continue
		}

		var b strings.Builder
		b.WriteString("🗓 Завтра:\n")
		if len(items) == 0 {
			b.WriteString("— ничего не запланировано\n")
		} else {
			for _, r := range items {
				when := "—"
				if r.EventTime != nil {
					when = r.EventTime.In(loc).Format("Mon, 02 Jan 15:04")
				} else if r.NextReport != nil {
					when = r.NextReport.In(loc).Format("Mon, 02 Jan 15:04")
				}
				fmt.Fprintf(&b, "• %s — %s\n", when, r.Message)
			}
		}

		if _, err := n.Bot.Send(tgbotapi.NewMessage(ch.ChatID, b.String())); err != nil {
			log.Printf("digest send error chat=%d: %v", ch.ChatID, err)
			continue
		}

		n.mu.Lock()
		n.lastDigest[ch.ChatID] = nowLocal
		n.mu.Unlock()

		log.Printf("digest sent chat=%d tz=%s at %s", ch.ChatID, ch.TimeZone, nowLocal.Format(time.RFC3339))
	}
}

func NextFromWeeklyRRULE(rrule, tz string, from time.Time) time.Time {
	parts := map[string]string{}
	for _, p := range strings.Split(rrule, ";") {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			parts[kv[0]] = kv[1]
		}
	}
	byday := parts["BYDAY"]
	hour, _ := strconv.Atoi(parts["BYHOUR"])
	min, _ := strconv.Atoi(parts["BYMINUTE"])
	want := map[string]time.Weekday{"ПН": time.Monday, "ВТ": time.Tuesday, "СР": time.Wednesday, "ЧТ": time.Thursday, "ПТ": time.Friday, "СБ": time.Saturday, "ВС": time.Sunday}[byday]

	loc := storage.LoadUserLocation(tz)
	now := from.In(loc)
	d := now
	for i := 0; i < 8; i++ {
		if d.Weekday() == want {
			cand := time.Date(d.Year(), d.Month(), d.Day(), hour, min, 0, 0, loc)
			if cand.After(now) {
				return cand.UTC()
			}
		}
		d = d.Add(24 * time.Hour)
	}
	return now.Add(7 * 24 * time.Hour).UTC()
}
