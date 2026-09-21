package slack

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/espcaa/djungelskog/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	goslack "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

const (
	anonPostViewCallbackID  = "anon_post_view"
	anonPostConfirmCallback = "anon_post_confirmation"
	anonPostTextActionID    = "anon_post_text"

	replyAnonShortcutID     = "reply_anon"
	replyAnonViewCallbackID = "reply_anon_view"
	replyAnonKeyActionID    = "reply_anon_key"
	replyAnonTextActionID   = "reply_anon_text"

	acceptActionID = "accept_confession"
	rejectActionID = "reject_confession"

	reviewedAtTimeFormat = "Jan 2, 2006 15:04"
)

type Channels struct {
	Log    string
	Review string
	Post   string
}

type Config struct {
	Channels Channels
}

type EventHandler struct {
	client        *goslack.Client
	signingSecret string
	config        Config
	queries       *db.Queries
}

func NewEventHandler(client *goslack.Client, signingSecret string, config Config, pool *pgxpool.Pool) *EventHandler {
	return &EventHandler{
		client:        client,
		signingSecret: signingSecret,
		config:        config,
		queries:       db.New(pool),
	}
}

func (h *EventHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	sv, err := goslack.NewSecretsVerifier(r.Header, h.signingSecret)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if _, err := sv.Write(body); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if err := sv.Ensure(); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		h.handleForm(w, r, body)
		return
	}

	h.handleEvent(w, body)
}

func (h *EventHandler) handleForm(w http.ResponseWriter, r *http.Request, body []byte) {
	r.Body = io.NopCloser(bytes.NewReader(body))
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if r.PostForm.Get("payload") != "" {
		h.handleInteraction(w, r, body)
		return
	}

	cmd, err := goslack.SlashCommandParse(r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if _, err := h.client.OpenView(cmd.TriggerID, h.newAnonPostView()); err != nil {
		log.Printf("opening anon post view: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *EventHandler) handleInteraction(w http.ResponseWriter, r *http.Request, body []byte) {
	r.Body = io.NopCloser(bytes.NewReader(body))
	ic, err := goslack.InteractionCallbackParse(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	switch ic.Type {
	case goslack.InteractionTypeViewSubmission:
		h.handleViewSubmission(w, ic)
	case goslack.InteractionTypeBlockActions:
		h.handleBlockActions(w, ic)
	case goslack.InteractionTypeMessageAction:
		h.handleMessageAction(w, ic)
	default:
		w.WriteHeader(http.StatusOK)
	}
}

func (h *EventHandler) handleViewSubmission(w http.ResponseWriter, ic goslack.InteractionCallback) {
	switch ic.CallbackID {
	case anonPostViewCallbackID:
		h.handleAnonPostSubmit(w, ic)
	case replyAnonViewCallbackID:
		h.handleAnonReplySubmit(w, ic)
	default:
		respondView(w, goslack.NewClearViewSubmissionResponse())
	}
}

func (h *EventHandler) handleBlockActions(w http.ResponseWriter, ic goslack.InteractionCallback) {
	if len(ic.ActionCallback.BlockActions) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}
	a := ic.ActionCallback.BlockActions[0]
	id, err := strconv.ParseInt(a.Value, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	switch a.ActionID {
	case acceptActionID:
		h.acceptConfession(w, ic, id)
	case rejectActionID:
		h.rejectConfession(w, ic, id)
	default:
		w.WriteHeader(http.StatusOK)
	}
}

func (h *EventHandler) handleMessageAction(w http.ResponseWriter, ic goslack.InteractionCallback) {
	view := goslack.ModalViewRequest{
		Type:            goslack.VTModal,
		Title:           ptxt("reply anonymously"),
		Close:           ptxt("Cancel"),
		Submit:          ptxt("Submit"),
		CallbackID:      replyAnonViewCallbackID,
		PrivateMetadata: h.replyContext(ic),
		Blocks: goslack.Blocks{BlockSet: []goslack.Block{
			goslack.NewInputBlock(replyAnonKeyActionID,
				ptxt("your anon reply key"),
				ptxt("one you got when you submitted the post"),
				goslack.NewPlainTextInputBlockElement(nil, replyAnonKeyActionID),
			),
			goslack.NewInputBlock(replyAnonTextActionID,
				ptxt("your anonymous reply"),
				nil,
				goslack.NewPlainTextInputBlockElement(nil, replyAnonTextActionID).WithMultiline(true),
			),
		}},
	}
	if _, err := h.client.OpenView(ic.TriggerID, view); err != nil {
		log.Printf("opening reply view: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *EventHandler) handleAnonPostSubmit(w http.ResponseWriter, ic goslack.InteractionCallback) {
	text := strings.TrimSpace(ic.View.State.Values[anonPostTextActionID][anonPostTextActionID].Value)
	if text == "" {
		respondView(w, goslack.NewErrorsViewSubmissionResponse(map[string]string{
			anonPostTextActionID: "your message can't be empty",
		}))
		return
	}

	secret, err := newSecret()
	if err != nil {
		log.Printf("generating secret: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	conf, err := h.queries.CreateConfession(context.Background(), db.CreateConfessionParams{
		Text:        text,
		ReplyKey:    replyKey(secret, ic.User.ID),
		PostChannel: h.config.Channels.Post,
	})
	if err != nil {
		log.Printf("creating confession: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ts, _, err := h.client.PostMessage(h.config.Channels.Review, goslack.MsgOptionBlocks(h.reviewBlocks(conf.ID, text)...))
	if err != nil {
		log.Printf("posting to review channel: %v", err)
	} else if ts != "" {
		if _, err := h.queries.SetConfessionReviewTs(context.Background(), db.SetConfessionReviewTsParams{
			ID:       conf.ID,
			ReviewTs: pgtype.Text{String: ts, Valid: true},
		}); err != nil {
			log.Printf("storing review ts: %v", err)
		}
	}

	respondView(w, goslack.NewPushViewSubmissionResponse(h.confirmationView(conf.ID, secret)))
}

func (h *EventHandler) acceptConfession(w http.ResponseWriter, ic goslack.InteractionCallback, id int64) {
	ctx := context.Background()
	conf, err := h.queries.GetConfessionByID(ctx, id)
	if err != nil {
		log.Printf("getting confession %d: %v", id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ts, _, err := h.client.PostMessage(conf.PostChannel, goslack.MsgOptionBlocks(h.postBlocks(conf.ID, conf.Text)...))
	if err != nil {
		log.Printf("posting accepted confession %d: %v", id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if _, err := h.queries.AcceptConfession(ctx, db.AcceptConfessionParams{
		ID:     conf.ID,
		PostTs: pgtype.Text{String: ts, Valid: true},
	}); err != nil {
		log.Printf("accepting confession %d: %v", id, err)
	}

	h.updateReviewMessage(ctx, conf, "approved", ":white_check_mark:", ic.User.ID)
	w.WriteHeader(http.StatusOK)
}

func (h *EventHandler) rejectConfession(w http.ResponseWriter, ic goslack.InteractionCallback, id int64) {
	ctx := context.Background()
	conf, err := h.queries.GetConfessionByID(ctx, id)
	if err != nil {
		log.Printf("getting confession %d: %v", id, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := h.queries.DeleteConfession(ctx, conf.ID); err != nil {
		log.Printf("deleting confession %d: %v", id, err)
	}
	if _, _, err := h.client.PostMessage(h.config.Channels.Log, goslack.MsgOptionText(fmt.Sprintf("anon post #%d rejected", conf.ID), false)); err != nil {
		log.Printf("logging rejection: %v", err)
	}

	h.updateReviewMessage(ctx, conf, "rejected", ":x:", ic.User.ID)
	w.WriteHeader(http.StatusOK)
}

func (h *EventHandler) handleAnonReplySubmit(w http.ResponseWriter, ic goslack.InteractionCallback) {
	secret := strings.TrimSpace(ic.View.State.Values[replyAnonKeyActionID][replyAnonKeyActionID].Value)
	text := strings.TrimSpace(ic.View.State.Values[replyAnonTextActionID][replyAnonTextActionID].Value)
	if secret == "" {
		respondView(w, goslack.NewErrorsViewSubmissionResponse(map[string]string{
			replyAnonKeyActionID: "enter your anon reply key",
		}))
		return
	}
	if text == "" {
		respondView(w, goslack.NewErrorsViewSubmissionResponse(map[string]string{
			replyAnonTextActionID: "write your reply",
		}))
		return
	}

	conf, err := h.queries.GetConfessionByReplyKey(context.Background(), replyKey(secret, ic.User.ID))
	if err != nil {
		respondView(w, goslack.NewErrorsViewSubmissionResponse(map[string]string{
			replyAnonKeyActionID: "no anon post matches that key",
		}))
		return
	}
	if !conf.PostTs.Valid {
		respondView(w, goslack.NewErrorsViewSubmissionResponse(map[string]string{
			replyAnonKeyActionID: "this post hasn't been approved yet",
		}))
		return
	}
	if conf.PostChannel != ic.Channel.ID || conf.PostTs.String != ic.MessageTs {
		respondView(w, goslack.NewErrorsViewSubmissionResponse(map[string]string{
			replyAnonKeyActionID: "that key doesn't belong to this post message",
		}))
		return
	}

	if _, _, err := h.client.PostMessage(conf.PostChannel, goslack.MsgOptionTS(conf.PostTs.String), goslack.MsgOptionText(text, false)); err != nil {
		log.Printf("posting anon reply: %v", err)
		respondView(w, goslack.NewErrorsViewSubmissionResponse(map[string]string{
			replyAnonTextActionID: "couldn't post your reply",
		}))
		return
	}
	respondView(w, goslack.NewClearViewSubmissionResponse())
}

func (h *EventHandler) replyContext(ic goslack.InteractionCallback) string {
	return fmt.Sprintf("%s:%s", ic.Channel.ID, ic.MessageTs)
}

func (h *EventHandler) updateReviewMessage(ctx context.Context, conf db.Confession, verdict, emoji, reviewer string) {
	if !conf.ReviewTs.Valid {
		return
	}
	text := fmt.Sprintf("anon post #%d %s %s by <@%s> at %s", conf.ID, verdict, emoji, reviewer, time.Now().Format(reviewedAtTimeFormat))
	if _, _, _, err := h.client.UpdateMessage(h.config.Channels.Review, conf.ReviewTs.String,
		goslack.MsgOptionBlocks(goslack.NewSectionBlock(mdtxt(text), nil, nil)),
	); err != nil {
		log.Printf("updating review message for confession %d: %v", conf.ID, err)
	}
}

func (h *EventHandler) handleEvent(w http.ResponseWriter, body []byte) {
	ev, err := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	switch ev.Type {
	case slackevents.URLVerification:
		challenge, ok := ev.Data.(*slackevents.EventsAPIURLVerificationEvent)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(challenge.Challenge))
	case slackevents.CallbackEvent:
		h.handleCallback(&ev)
		w.WriteHeader(http.StatusOK)
	}
}

func (h *EventHandler) handleCallback(ev *slackevents.EventsAPIEvent) {
	switch e := ev.InnerEvent.Data.(type) {
	case *slackevents.MessageEvent:
		if e.BotID != "" {
			return
		}
		log.Printf("message from %s in %s: %s", e.User, e.Channel, e.Text)
	}
}

func (h *EventHandler) newAnonPostView() goslack.ModalViewRequest {
	return goslack.ModalViewRequest{
		Type:       goslack.VTModal,
		Title:      ptxt("new anon post"),
		Close:      ptxt("Cancel"),
		Submit:     ptxt("Post"),
		CallbackID: anonPostViewCallbackID,
		Blocks: goslack.Blocks{BlockSet: []goslack.Block{
			goslack.NewInputBlock(anonPostTextActionID,
				ptxt("your message"),
				ptxt("this message will be posted anonymously in #lgbtq-space"),
				goslack.NewPlainTextInputBlockElement(nil, anonPostTextActionID).WithMultiline(true),
			),
		}},
	}
}

func (h *EventHandler) confirmationView(id int64, secret string) *goslack.ModalViewRequest {
	return &goslack.ModalViewRequest{
		Type:       goslack.VTModal,
		Title:      ptxt("anon post submitted"),
		Close:      ptxt("Done"),
		CallbackID: anonPostConfirmCallback,
		Blocks: goslack.Blocks{BlockSet: []goslack.Block{
			goslack.NewSectionBlock(
				mdtxt(fmt.Sprintf("your anon post is *#%d*.\n\n_your anon reply key:_\n`%s`\n\nkeep it safe — you'll need it to reply anonymously from your post's thread.", id, secret)),
				nil, nil,
			),
		}},
	}
}

func (h *EventHandler) reviewBlocks(id int64, text string) []goslack.Block {
	return []goslack.Block{
		goslack.NewSectionBlock(mdtxt(fmt.Sprintf("*anon post #%d:*\n%s", id, text)), nil, nil),
		goslack.NewActionBlock("review_actions",
			goslack.NewButtonBlockElement(acceptActionID, fmt.Sprint(id), ptxt("Accept")).WithStyle(goslack.StylePrimary),
			goslack.NewButtonBlockElement(rejectActionID, fmt.Sprint(id), ptxt("Reject")).WithStyle(goslack.StyleDanger),
		),
	}
}

func (h *EventHandler) postBlocks(id int64, text string) []goslack.Block {
	return []goslack.Block{
		goslack.NewSectionBlock(mdtxt(fmt.Sprintf("*anon post #%d:*\n%s", id, text)), nil, nil),
	}
}

func respondView(w http.ResponseWriter, resp *goslack.ViewSubmissionResponse) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("encoding view response: %v", err)
	}
}

func ptxt(text string) *goslack.TextBlockObject {
	return goslack.NewTextBlockObject(goslack.PlainTextType, text, false, false)
}

func mdtxt(text string) *goslack.TextBlockObject {
	return goslack.NewTextBlockObject(goslack.MarkdownType, text, false, false)
}

func newSecret() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func replyKey(secret, userID string) string {
	sum := sha256.Sum256([]byte(secret + ":" + userID))
	return hex.EncodeToString(sum[:])
}
