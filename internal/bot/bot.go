package bot

import (
	"net/http"

	"github.com/espcaa/djungelskog/internal/slack"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goslack "github.com/slack-go/slack"
)

type Bot struct {
	slackClient   *goslack.Client
	signingSecret string
	config        slack.Config
	pool          *pgxpool.Pool
}

func NewBot(slackClient *goslack.Client, signingSecret string, config slack.Config, pool *pgxpool.Pool) *Bot {
	return &Bot{
		slackClient:   slackClient,
		signingSecret: signingSecret,
		config:        config,
		pool:          pool,
	}
}

func (b *Bot) Run() {
	r := chi.NewRouter()

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(":3c"))
	})

	r.Post("/events", slack.NewEventHandler(b.slackClient, b.signingSecret, b.config, b.pool).ServeHTTP)

	http.ListenAndServe(":8080", r)
}
