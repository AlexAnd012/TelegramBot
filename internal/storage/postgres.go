package storage

import (
	"context"
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

type ChatSettingsRepo interface {
	Get(ctx context.Context, chatID int64) (ChatSettings, error)
	UpsertTZ(ctx context.Context, chatID int64, tz string) error
	UpsertDigest(ctx context.Context, chatID int64, t *time.Time) error
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
