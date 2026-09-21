package bot

import "github.com/slack-go/slack"

type Bot struct {
	slackClient *slack.Client
}

func NewBot(slackClient *slack.Client) *Bot {
	return &Bot{
		slackClient: slackClient,
	}
}

func (b *Bot) Run() {

}
