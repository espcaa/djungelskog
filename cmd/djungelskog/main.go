package djungelskog

import (
	"os"

	"github.com/espcaa/djungelskog/internal/bot"
	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
)

func main() {
	godotenv.Load()

	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		panic("BOT_TOKEN environment variable is not set")
	}
	appToken := os.Getenv("APP_TOKEN")
	if appToken == "" {
		panic("APP_TOKEN environment variable is not set")
	}

	slackClient := slack.New(
		botToken,
		slack.OptionAppLevelToken(appToken),
	)

	bot := bot.NewBot(slackClient)
	bot.Run()
}
