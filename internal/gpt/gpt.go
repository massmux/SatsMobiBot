package gpt

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/LightningTipBot/LightningTipBot/internal"
	"github.com/LightningTipBot/LightningTipBot/internal/telegram/intercept"
	"net/http"
)

type Response struct {
	Message struct {
		ID         string      `json:"id"`
		Role       string      `json:"role"`
		User       interface{} `json:"user"`
		CreateTime interface{} `json:"create_time"`
		UpdateTime interface{} `json:"update_time"`
		Content    struct {
			ContentType string   `json:"content_type"`
			Parts       []string `json:"parts"`
		} `json:"content"`
		EndTurn  interface{} `json:"end_turn"`
		Weight   float64     `json:"weight"`
		Metadata struct {
		} `json:"metadata"`
		Recipient string `json:"recipient"`
	} `json:"message"`
	ConversationID string      `json:"conversation_id"`
	Error          interface{} `json:"error"`
}
type Request struct {
	Action          string     `json:"action"`
	ConversationId  string     `json:"conversation_id,omitempty"`
	Messages        []Messages `json:"messages"`
	ParentMessageID string     `json:"parent_message_id"`
	Model           string     `json:"model"`
}
type Content struct {
	ContentType string   `json:"content_type"`
	Parts       []string `json:"parts"`
}
type Messages struct {
	ID      string  `json:"id"`
	Role    string  `json:"role"`
	Content Content `json:"content"`
}

var dataPrefix = []byte("data: ")
var doneSequence = []byte("[DONE]")

func GetRawCompletion(ctx intercept.Context, rr Request, cb func(s string)) (*Response, error) {
	rawClient := http.Client{}
	r, err := json.Marshal(rr)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx.Context, "POST", "https://chat.openai.com/backend-api/conversation", bytes.NewBuffer(r))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", internal.Configuration.Generate.OpenAiBearerToken))
	req.Header.Set("accept", "text/event-stream")
	req.Header.Set("authority", "chat.openai.com")
	req.Header.Set("accept-language", "de-DE,de;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Content-type", "application/json")
	req.Header.Set("user-agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36")

	resp, err := rawClient.Do(req)
	if err != nil {
		return nil, err
	}
	reader := bufio.NewReader(resp.Body)
	defer resp.Body.Close()
	output := new(Response)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		// make sure there isn't any extra whitespace before or after
		line = bytes.TrimSpace(line)
		// the completion API only returns data events
		if !bytes.HasPrefix(line, dataPrefix) {
			continue
		}
		line = bytes.TrimPrefix(line, dataPrefix)

		// the stream is completed when terminated by [DONE]
		if bytes.HasPrefix(line, doneSequence) {
			break
		}
		if err := json.Unmarshal(line, output); err != nil {
			return nil, fmt.Errorf("invalid json stream data: %v", err)
		}
		if len(output.Message.Content.Parts) > 0 {
			cb(output.Message.Content.Parts[len(output.Message.Content.Parts)-1])
		}
	}
	return output, nil
}
