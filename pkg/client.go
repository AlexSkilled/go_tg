package pkg

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/AlexSkilled/go_tg/pkg/model"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

// Bot - allows you to interact with telegram bot
// with some features
// tgbotapi.BotAPI - realisation of API calls to Telegram;
// chats - mapping of chat ids to their current handlers;
// handlers - mapping of name of handler to realisation;
// External context - can be used to pass information (such as user info) to handlers
// menuPattern - menu interaction(todo needs to be reworked)
type Bot struct {
	Bot      *tgbotapi.BotAPI
	chats    map[int64]CommandHandler
	handlers map[string]CommandHandler
	ExternalContext
	separator string

	menuPatterns    []model.Menu
	locMenuPatterns []model.LocalizedMenu
	qm              *quitManager
	outMessage      chan Instruction
}

var instructionHandler <-chan Instruction

type quitManager struct {
	end chan struct{}
	wg  *sync.WaitGroup
}

// NewBot Bot constructor
func NewBot(token string) *Bot {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		panic(err)
	}

	return &Bot{
		Bot:       bot,
		chats:     make(map[int64]CommandHandler),
		handlers:  make(map[string]CommandHandler),
		separator: " ",
	}
}

// AddCommandHandler adds a command handler
// for command
// e.g. for command "/help"
// handler should send help information to user
func (b *Bot) AddCommandHandler(handler CommandHandler, command string) {
	if _, ok := b.handlers[command]; ok {
		panic(fmt.Sprintf("Command handler with name %s already exists", command))
	}
	b.handlers[command] = handler
}

func (b *Bot) AddMenu(pattern model.Menu) {
	b.menuPatterns = append(b.menuPatterns, pattern)
}

func (b *Bot) AddLocalizedMenu(locMenu model.LocalizedMenu) {
	b.locMenuPatterns = append(b.locMenuPatterns, locMenu)
}

func (b *Bot) Start() {
	if b.ExternalContext == nil {
		b.ExternalContext = GetContextFunc(func(_ *model.MessageIn) (context.Context, error) {
			return context.Background(), nil
		})
	}
	if len(b.menuPatterns) != 0 {
		b.handlers[model.MenuCall] = newMenuHandler(b.menuPatterns)
	}

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60

	updChan := b.Bot.GetUpdatesChan(updateConfig)

	quit := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(2)

	b.qm = &quitManager{
		quit,
		wg,
	}

	b.outMessage = make(chan Instruction)

	go b.handleInComing(updChan, b.qm)
	go b.handleOutgoing(b.qm)

}

func (b *Bot) Stop() {
	close(b.qm.end)
	b.qm.wg.Wait()
}

func (b *Bot) handleInComing(updChan tgbotapi.UpdatesChannel, qm *quitManager) {
	for {
		select {
		case update := <-updChan:
			switch {
			case update.Message != nil:

				b.handleMessage(&model.MessageIn{
					Message: update.Message,
				})
				break
			case update.CallbackQuery != nil:
				message := update.CallbackQuery.Message
				message.Text = update.CallbackQuery.Data
				message.From = update.CallbackQuery.From

				_, err := b.Bot.Request(tgbotapi.CallbackConfig{CallbackQueryID: update.CallbackQuery.ID})
				if err != nil {
					logrus.Error(err)
				}
				b.handleMessage(&model.MessageIn{
					Message: update.CallbackQuery.Message,
				})
				break
			}
		case <-qm.end:
			logrus.Println("Gracefully shutted down incoming handler")
			qm.wg.Done()
			return
		}

	}
}

func (b *Bot) handleOutgoing(qm *quitManager) {
	select {
	case inst := <-b.outMessage:
		inst.Execute(b.Bot)
	case <-qm.end:
		logrus.Println("Gracefully shutted down outgoing handler")
		qm.wg.Done()
		return
	}
}

func (b *Bot) SendMessage(t TgMessage, chatId int64) error {
	return t.Send(b.Bot, chatId)
}

func (b *Bot) sendResponse(in *model.MessageIn, out TgMessage) {
	err := out.Send(b.Bot, in.Chat.ID)
	if err != nil {
		logrus.Infof("Ошибка при отправке сообщения: %v", err)
	} else {
		logrus.Infof("Пользователь %d написал %s и получил ответ %v",
			in.From.ID,
			in.Text,
			out)
	}
}

func (b *Bot) handleMessage(message *model.MessageIn) {

	var handler CommandHandler
	if strings.HasPrefix(message.Text, "/") {
		args := strings.Split(message.Text, b.separator)
		message.Command = args[0]
		if len(args) > 1 {
			message.Args = args[1:]
		}
		handler = b.chats[message.Chat.ID]
		if handler != nil {
			handler.Dump(message.Chat.ID)
		}
		handler = b.handlers[message.Command]

		b.chats[message.Chat.ID] = handler
	}

	ctx, err := b.GetContext(message)
	if err != nil {
		// TODO
		return
	}

	message.Ctx = ctx

	var messageOut TgMessage

	if handler == nil {
		handler = b.chats[message.Chat.ID]
		if handler == nil {
			b.tryHandleAsMenuCall(message)
			return
		}
	}
	handler.Handle(message, &Responser{c: b.outMessage, chatId: message.Chat.ID})
	switch r := messageOut.(type) {
	case *model.Callback:
		b.processCallback(ctx, r, message)
	case *model.Reply:
		return
	case nil:
		return
	default:
		b.sendResponse(message, messageOut)
	}

}

func (b *Bot) processCallback(ctx context.Context, c *model.Callback, message *model.MessageIn) {
	switch c.Type {
	case model.Callback_Type_Bad:
		logrus.Errorf("Untyped callback for message %s", message.Text)
		return
	case model.Callback_Type_CallCommand:
		message.Text = c.Command
		message.Args = c.Args
	case model.Callback_Type_OpenMenu:
		if c.Menu != nil {
			menu := c.Menu.GetPage()
			c.ReplyMarkup = menu
			err := c.Send(b.Bot, message.Chat.ID)
			if err != nil {
				logrus.Errorf("Error handling callback %v", err)
			}
			return
		}
		message.Text = model.MenuCall + " " + model.OpenMenu + " " + c.Command
	case model.Callback_Type_TransitToMenu:
		message.Text = model.MenuCall + " " + c.Command
	}

	b.handleMessage(message)
}

func (b *Bot) tryHandleAsMenuCall(in *model.MessageIn) {
	menuHandler := b.handlers[model.MenuCall]
	menuHandler.Handle(in, &Responser{chatId: in.Chat.ID, c: b.outMessage})
}
