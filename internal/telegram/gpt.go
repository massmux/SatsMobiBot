package telegram

import (
	"fmt"
	"github.com/massmux/SatsMobiBot/internal/gpt"
	"github.com/massmux/SatsMobiBot/internal/telegram/intercept"
	uuid "github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"gopkg.in/lightningtipbot/telebot.v3"
)

func (bot *TipBot) gptHandler(ctx intercept.Context) (intercept.Context, error) {
	question := GetMemoFromCommand(ctx.Text(), 1)
	req := gpt.Request{
		Action: "next",
		Model:  "text-davinci-002-render",
		Messages: []gpt.Messages{{
			ID:   uuid.NewV4().String(),
			Role: "user",
			Content: gpt.Content{
				ContentType: "text",
				Parts:       []string{question},
			},
		},
		},
	}

	conversationIdCacheKey := fmt.Sprintf("conversation_%d", ctx.Chat().ID)
	parentIdCacheKey := fmt.Sprintf("conversation_parent_%d", ctx.Chat().ID)
	conversationId, _ := bot.Cache.Get(conversationIdCacheKey)

	if parentId, _ := bot.Cache.Get(parentIdCacheKey); parentId != nil {
		req.ParentMessageID = parentId.(string)
	}
	if conversationId != nil {
		req.ConversationId = conversationId.(string)
	}
	cbc := 0
	var msg *telebot.Message
	completion, err := gpt.GetRawCompletion(ctx, req, func(s string) {
		cbc++
		if ctx.Chat().Type == telebot.ChatPrivate {
			if cbc == 20 {
				msg = bot.trySendMessageEditable(ctx.Sender(), s)
				return
			}
		} else {
			if cbc == 20 {
				msg = bot.tryReplyMessage(ctx.Message(), s)
				return
			}
		}
		if cbc%20 == 0 {
			bot.tryEditMessage(msg, s)
		}
	})
	if err != nil {
		bot.tryEditMessage(ctx.Message(), fmt.Sprintf(Translate(ctx, "errorReasonMessage"), "Could not create completion."))
		return ctx, err
	}
	answer := completion.Message.Content.Parts[len(completion.Message.Content.Parts)-1]
	bot.tryEditMessage(msg, answer)
	err = bot.Cache.Set(conversationIdCacheKey, completion.ConversationID, nil)
	if err != nil {
		log.Errorf("[/gpt] error setting conversation id %s: %v", conversationIdCacheKey, err)
	}
	err = bot.Cache.Set(parentIdCacheKey, completion.Message.ID, nil)
	if err != nil {
		log.Errorf("[/gpt] error setting parent message id %s: %v", parentIdCacheKey, err)
	}
	log.Infof("[/gpt] \"%s\" => \"%s\"", question, answer)
	return ctx, nil
}
