package storage

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Storage struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string) (*Storage, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &Storage{pool: pool}, nil
}

func (s *Storage) Close() { s.pool.Close() }

func (s *Storage) Now(ctx context.Context) (time.Time, error) {
	var t time.Time
	err := s.pool.QueryRow(ctx, `SELECT now()`).Scan(&t)
	return t, err
}

type ChatSettings struct {
	ChatID          int64
	TimeZone        string
	LocaleLanguage  string
	DailyReportTime *time.Time
}

type ChatDigestSlot struct {
	ChatID   int64
	TimeZone string
	Daily    time.Time
}

type ChatSettingsRepo interface {
	Get(ctx context.Context, chatID int64) (ChatSettings, error)
	UpsertTZ(ctx context.Context, chatID int64, tz string) error
	UpsertDigest(ctx context.Context, chatID int64, t *time.Time) error
	ChatsToDigestNow(ctx context.Context) ([]ChatDigestSlot, error)
}

type chatSettingsPG struct{ db *pgxpool.Pool }

func (s *Storage) ChatSettings() ChatSettingsRepo { return &chatSettingsPG{s.pool} }

func (r *chatSettingsPG) Get(ctx context.Context, chatID int64) (ChatSettings, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	const q = `SELECT chat_id, time_zone, locale_language, daily_report_time
	           FROM chat_settings WHERE chat_id=$1`
	var cs ChatSettings
	err := r.db.QueryRow(ctx, q, chatID).Scan(&cs.ChatID, &cs.TimeZone, &cs.LocaleLanguage, &cs.DailyReportTime)
	return cs, err
}

func (r *chatSettingsPG) ChatsToDigestNow(ctx context.Context) ([]ChatDigestSlot, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	const q = `
        SELECT chat_id, time_zone, daily_report_time
        FROM chat_settings
        WHERE daily_report_time IS NOT NULL
    `

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ChatDigestSlot
	for rows.Next() {
		var s ChatDigestSlot
		if err := rows.Scan(&s.ChatID, &s.TimeZone, &s.Daily); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *chatSettingsPG) UpsertTZ(ctx context.Context, chatID int64, tz string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	const q = `
INSERT INTO chat_settings (chat_id, time_zone)
VALUES ($1,$2)
ON CONFLICT (chat_id) DO UPDATE SET time_zone=EXCLUDED.time_zone`
	_, err := r.db.Exec(ctx, q, chatID, tz)
	return err
}

func (r *chatSettingsPG) UpsertDigest(ctx context.Context, chatID int64, t *time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	const q = `
INSERT INTO chat_settings (chat_id, daily_report_time)
VALUES ($1,$2)
ON CONFLICT (chat_id) DO UPDATE SET daily_report_time=EXCLUDED.daily_report_time`
	_, err := r.db.Exec(ctx, q, chatID, t)
	return err
}

type Reminder struct {
	ID           int64
	ChatID       int64
	Message      string
	EventTime    *time.Time
	ReminderTime int
	ReminderRule *string
	NextReport   *time.Time
	CreatedAt    time.Time
}

type RemindersRepo interface {
	Create(ctx context.Context, r *Reminder) (int64, error)
	UpdateDue(ctx context.Context, id int64, eventTime time.Time, leadMin int) error
	UpdateNextReport(ctx context.Context, id int64, t *time.Time) error
	GetUpcoming(ctx context.Context, chatID int64, from time.Time, to *time.Time, limit int) ([]Reminder, error)
}

type remindersPG struct{ db *pgxpool.Pool }

func (s *Storage) Reminders() RemindersRepo { return &remindersPG{s.pool} }

func (r *remindersPG) Create(ctx context.Context, m *Reminder) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	const q = `
INSERT INTO reminders (chat_id, message, event_time, reminder_time, reminder_rule, next_report)
VALUES ($1,$2,$3,$4,$5,$6)
RETURNING id`
	var id int64
	err := r.db.QueryRow(ctx, q, m.ChatID, m.Message, m.EventTime, m.ReminderTime, m.ReminderRule, m.NextReport).Scan(&id)
	return id, err
}

func (r *remindersPG) UpdateDue(ctx context.Context, id int64, eventTime time.Time, leadMin int) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	const q = `UPDATE reminders SET event_time=$2, reminder_time=$3 WHERE id=$1`
	_, err := r.db.Exec(ctx, q, id, eventTime, leadMin)
	return err
}

func (r *remindersPG) UpdateNextReport(ctx context.Context, id int64, t *time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	const q = `UPDATE reminders SET next_report=$2 WHERE id=$1`
	_, err := r.db.Exec(ctx, q, id, t)
	return err
}

func (r *remindersPG) GetUpcoming(ctx context.Context, chatID int64, from time.Time, to *time.Time, limit int) ([]Reminder, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	base := `
SELECT id, chat_id, message, event_time, reminder_time, reminder_rule, next_report, created_at
FROM reminders
WHERE chat_id=$1
  AND (
       (event_time IS NOT NULL AND event_time >= $2)
    OR (next_report IS NOT NULL AND next_report >= $2)
  )`
	args := []any{chatID, from}
	if to != nil {
		base += ` AND COALESCE(next_report, event_time) <= $3`
		args = append(args, *to)
	}
	base += ` ORDER BY COALESCE(next_report, event_time) LIMIT $4`
	args = append(args, limit)

	rows, err := r.db.Query(ctx, base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Reminder
	for rows.Next() {
		var m Reminder
		if err := rows.Scan(&m.ID, &m.ChatID, &m.Message, &m.EventTime, &m.ReminderTime, &m.ReminderRule, &m.NextReport, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

type Job struct {
	ID           int64
	ReminderID   int64
	ReportTime   time.Time
	SentAt       *time.Time
	ChatID       int64
	Message      string
	ReminderTime int
	ReminderRule *string
}

type JobsRepo interface {
	Create(ctx context.Context, reminderID int64, reportTime time.Time) error
	Due(ctx context.Context, now time.Time, limit int) ([]Job, error)
	MarkSent(ctx context.Context, jobID int64) error
	Snooze(ctx context.Context, jobID int64, d time.Duration) error
}

type jobsPG struct{ db *pgxpool.Pool }

func (s *Storage) Jobs() JobsRepo { return &jobsPG{s.pool} }

func (r *jobsPG) Create(ctx context.Context, reminderID int64, reportTime time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	const q = `
INSERT INTO reminder_jobs (reminder_id, report_time)
VALUES ($1,$2)
ON CONFLICT (reminder_id, report_time) DO NOTHING`
	_, err := r.db.Exec(ctx, q, reminderID, reportTime)
	return err
}

func (r *jobsPG) Due(ctx context.Context, now time.Time, limit int) ([]Job, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	const q = `
SELECT j.id, j.reminder_id, j.report_time, j.sent_at,
       r.chat_id, r.message, r.reminder_time, r.reminder_rule
FROM reminder_jobs j
JOIN reminders r ON r.id=j.reminder_id
WHERE j.sent_at IS NULL AND j.report_time <= $1
ORDER BY j.report_time
LIMIT $2`
	rows, err := r.db.Query(ctx, q, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.ReminderID, &j.ReportTime, &j.SentAt, &j.ChatID, &j.Message, &j.ReminderTime, &j.ReminderRule); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (r *jobsPG) MarkSent(ctx context.Context, jobID int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	const q = `UPDATE reminder_jobs SET sent_at=now() WHERE id=$1 AND sent_at IS NULL`
	_, err := r.db.Exec(ctx, q, jobID)
	return err
}

func (r *jobsPG) Snooze(ctx context.Context, jobID int64, d time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	const q = `UPDATE reminder_jobs SET report_time = report_time + $2 WHERE id=$1 AND sent_at IS NULL`
	_, err := r.db.Exec(ctx, q, jobID, d)
	return err
}

type WeeklyEntry struct {
	ID        int64
	ChatID    int64
	Weekday   int
	StartTime time.Time
	EndTime   *time.Time
	Title     string
}

type WeeklyScheduleRepo interface {
	Set(ctx context.Context, chatID int64, entries []WeeklyEntry) error
	ListForWeekday(ctx context.Context, chatID int64, weekday int) ([]WeeklyEntry, error)
	Clear(ctx context.Context, chatID int64) error
}

type weeklySchedulePG struct{ db *pgxpool.Pool }

func (s *Storage) Schedule() WeeklyScheduleRepo { return &weeklySchedulePG{s.pool} }

func (r *weeklySchedulePG) Set(ctx context.Context, chatID int64, entries []WeeklyEntry) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM weekly_schedule WHERE chat_id=$1`, chatID); err != nil {
		return err
	}
	const ins = `
INSERT INTO weekly_schedule (chat_id, weekday, start_time, end_time, title)
VALUES ($1,$2,$3,$4,$5)`
	for _, e := range entries {
		if _, err := tx.Exec(ctx, ins, chatID, e.Weekday, e.StartTime.Format("15:04:05"), nilOrTime(e.EndTime), e.Title); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *weeklySchedulePG) ListForWeekday(ctx context.Context, chatID int64, weekday int) ([]WeeklyEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	const q = `
SELECT id, chat_id, weekday, start_time, end_time, title
FROM weekly_schedule
WHERE chat_id=$1 AND weekday=$2
ORDER BY start_time`
	rows, err := r.db.Query(ctx, q, chatID, weekday)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WeeklyEntry
	for rows.Next() {
		var e WeeklyEntry
		var st, et *time.Time
		if err := rows.Scan(&e.ID, &e.ChatID, &e.Weekday, &st, &et, &e.Title); err != nil {
			return nil, err
		}
		if st != nil {
			e.StartTime = *st
		}
		e.EndTime = et
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *weeklySchedulePG) Clear(ctx context.Context, chatID int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := r.db.Exec(ctx, `DELETE FROM weekly_schedule WHERE chat_id=$1`, chatID)
	return err
}

func nilOrTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}
func LoadUserLocation(tz string) *time.Location {
	if loc, ok := loadFixedUTC(tz); ok {
		return loc
	}
	if l, err := time.LoadLocation(tz); err == nil {
		return l
	}
	return time.UTC
}
func loadFixedUTC(name string) (*time.Location, bool) {
	name = strings.TrimSpace(strings.ToUpper(name))
	if !strings.HasPrefix(name, "UTC") {
		return nil, false
	}
	rest := strings.TrimPrefix(name, "UTC")
	if rest == "" || rest == "Z" || rest == "+0" || rest == "+00:00" {
		return time.UTC, true
	}
	sign := 1
	if strings.HasPrefix(rest, "+") {
		rest = rest[1:]
	} else if strings.HasPrefix(rest, "-") {
		sign = -1
		rest = rest[1:]
	} else {
		return nil, false
	}
	ps := strings.SplitN(rest, ":", 2)
	hh, err := strconv.Atoi(ps[0])
	if err != nil {
		return nil, false
	}
	mm := 0
	if len(ps) == 2 {
		mm, err = strconv.Atoi(ps[1])
		if err != nil {
			return nil, false
		}
	}
	offset := sign * (hh*3600 + mm*60)
	return time.FixedZone(fmt.Sprintf("UTC%+02d:%02d", sign*hh, mm), offset), true
}
