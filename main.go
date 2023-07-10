package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"

	bot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/joho/godotenv"
	openai "github.com/sashabaranov/go-openai"
)

var chatMode map[int64]string = make(map[int64]string)
var translateLang map[int64]string = make(map[int64]string)
var openaiClient *openai.Client

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	godotenv.Load(".env")

	openaiClient = openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	opts := []bot.Option{
		bot.WithDefaultHandler(handler),
	}

	b, err := bot.New(os.Getenv("BOT_TOKEN"), opts...)

	if err != nil {
		panic(err)
	}

	b.SetWebhook(ctx, &bot.SetWebhookParams{
		URL: os.Getenv("WEBHOOK_URL"),
	})

	go b.StartWebhook(ctx)

	b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: []models.BotCommand{
			{
				Command:     "explain",
				Description: "Explain the following text",
			},
			{
				Command:     "translate",
				Description: "Translate from any language to any other language",
			},
			{
				Command:     "image",
				Description: "Generate an image from text",
			},
		},
	})

	go func() {
		err = http.ListenAndServe(":"+os.Getenv("PORT"), b.WebhookHandler())
		if err != nil {
			panic(err)
		}
	}()

	<-ctx.Done()

	defer b.DeleteWebhook(ctx, &bot.DeleteWebhookParams{true})

}

func handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	msg, _ := json.Marshal(update)
	log.Default().Println(string(msg))

	if len(update.Message.Entities) > 0 {
		if update.Message.Entities[0].Type == "bot_command" && strings.HasPrefix(update.Message.Text, "/explain") {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   "What do you want me to explain?",
			})
			chatMode[update.Message.Chat.ID] = "explain"
			return
		}
		if update.Message.Entities[0].Type == "bot_command" && strings.HasPrefix(update.Message.Text, "/translate") {
			lang := ""
			if len(update.Message.Text) > update.Message.Entities[0].Offset+update.Message.Entities[0].Length {
				lang = update.Message.Text[update.Message.Entities[0].Offset+update.Message.Entities[0].Length:]
			} else {
				b.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: update.Message.Chat.ID,
					Text:   "Select the language you want me to translate to:",
				})
				chatMode[update.Message.Chat.ID] = "ask_language"
				return
			}
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   "What do you want me to translate?",
			})
			chatMode[update.Message.Chat.ID] = "translate"
			translateLang[update.Message.Chat.ID] = lang

			return
		}

		if update.Message.Entities[0].Type == "bot_command" && strings.HasPrefix(update.Message.Text, "/image") {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   "What do you want me to generate?",
			})
			chatMode[update.Message.Chat.ID] = "image"
			return
		}

	}

	if chatMode[update.Message.Chat.ID] == "ask_language" {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "What do you want me to translate?",
		})
		chatMode[update.Message.Chat.ID] = "translate"
		translateLang[update.Message.Chat.ID] = update.Message.Text
		return
	}

	if chatMode[update.Message.Chat.ID] == "translate" {
		resp, err := openaiClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model: "gpt-3.5-turbo",
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    "user",
					Content: fmt.Sprintf("Translate `%s` to `%s`", update.Message.Text, translateLang[update.Message.Chat.ID]),
				},
			},
		})
		var msg string
		if err != nil {
			log.Default().Println("Error:", err)
			msg = "I'm sorry, I couldn't translate that. Please try again."
		} else {
			msg = resp.Choices[0].Message.Content
		}
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   msg,
		})
		delete(chatMode, update.Message.Chat.ID)
		delete(translateLang, update.Message.Chat.ID)
		return
	}

	if chatMode[update.Message.Chat.ID] == "explain" {
		resp, err := openaiClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model: "gpt-3.5-turbo",
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    "user",
					Content: "Please explain:\n" + update.Message.Text,
				},
			},
		})
		var msg string
		if err != nil {
			log.Default().Println("Error:", err)
			msg = "I'm sorry, I couldn't explain that. Please try again."
		} else {
			msg = resp.Choices[0].Message.Content
		}
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   msg,
		})
		delete(chatMode, update.Message.Chat.ID)
		return
	}

	if chatMode[update.Message.Chat.ID] == "image" {
		openaiClient.CreateImage()
	}
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   update.Message.Text,
	})
}
