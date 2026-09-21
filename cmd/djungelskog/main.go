package main

import (
	"context"
	"database/sql"
	"os"

	"github.com/espcaa/djungelskog/db/migrations"
	"github.com/espcaa/djungelskog/internal/bot"
	groupslack "github.com/espcaa/djungelskog/internal/slack"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
	"github.com/slack-go/slack"
)

func main() {
	godotenv.Load()

	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		panic("DATABASE_URL environment variable is not set")
	}
	if err := runMigrations(ctx, dbURL); err != nil {
		panic(err)
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		panic("BOT_TOKEN environment variable is not set")
	}
	signingSecret := os.Getenv("SLACK_SIGNING_SECRET")
	if signingSecret == "" {
		panic("SLACK_SIGNING_SECRET environment variable is not set")
	}

	logChannel := os.Getenv("LOG_CHANNEL")
	if logChannel == "" {
		panic("LOG_CHANNEL environment variable is not set")
	}
	reviewChannel := os.Getenv("REVIEW_CHANNEL")
	if reviewChannel == "" {
		panic("REVIEW_CHANNEL environment variable is not set")
	}
	postChannel := os.Getenv("POST_CHANNEL")
	if postChannel == "" {
		panic("POST_CHANNEL environment variable is not set")
	}

	slackClient := slack.New(botToken)

	config := groupslack.Config{
		Channels: groupslack.Channels{
			Log:    logChannel,
			Review: reviewChannel,
			Post:   postChannel,
		},
	}
	bot := bot.NewBot(slackClient, signingSecret, config, pool)
	bot.Run()
}

func runMigrations(ctx context.Context, dbURL string) error {
	sqlDB, err := sql.Open("pgx", dbURL)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.UpContext(ctx, sqlDB, ".")
}
