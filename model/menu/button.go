package menu

import (
	"github.com/AlexSkilled/go_tg/model"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type button struct {
	Text         string
	Value        string
	IsStandAlone bool
}

type InlineKeyboard struct {
	btns []button

	Columns uint8
	Rows    uint8
}

func (b *InlineKeyboard) AddButton(text, value string) {
	b.btns = append(b.btns, button{
		Text:  text,
		Value: value,
	})
}

func (b *InlineKeyboard) AddStandAloneButton(text, value string) {
	b.btns = append(b.btns, button{
		Text:         text,
		Value:        value,
		IsStandAlone: true,
	})
}

func (b *InlineKeyboard) ToMarkup() (markup *tgbotapi.InlineKeyboardMarkup) {
	if b.Columns == 0 {
		b.Columns = model.ColumnsDefaultAmount
	}

	if b.Rows == 0 {
		b.Rows = model.RowsDefaultAmount
	}

	rows := make([][]tgbotapi.InlineKeyboardButton, 1, b.Rows)

	var cRaw, cCol uint8

	processedButtons := 0
	for _, btn := range b.btns {
		btnMark := tgbotapi.NewInlineKeyboardButtonData(btn.Text, btn.Value)

		if btn.IsStandAlone {
			rows = append(rows, []tgbotapi.InlineKeyboardButton{btnMark})
			cRaw++
			continue
		}

		if cCol >= b.Columns {
			rows = append(rows, []tgbotapi.InlineKeyboardButton{btnMark})
			cRaw++
			cCol = 0
		} else {
			rows[cRaw] = append(rows[cRaw], btnMark)
		}

		processedButtons++

		if cRaw >= b.Rows {
			return &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
		}
	}
	return &tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
}

type Keyboard struct {
	btns []string

	Columns int

	ResizeKeyboard bool `json:"resize_keyboard"`
}

func (b *Keyboard) AddKey(value string) {
	b.btns = append(b.btns, value)
}
func (b *Keyboard) toMarkup() *tgbotapi.ReplyKeyboardMarkup {
	if b.Columns == 0 {
		b.Columns = 1
	}

	finalButtonsSet := make([][]tgbotapi.KeyboardButton, 0, 1)
	i := 0
	raw := -1
	for i < len(b.btns) {
		if i%b.Columns == 0 {
			finalButtonsSet = append(finalButtonsSet, make([]tgbotapi.KeyboardButton, 0, b.Columns))
			raw++
		}
		finalButtonsSet[raw] = append(finalButtonsSet[raw],
			tgbotapi.NewKeyboardButton(b.btns[i]))
		i++
	}

	return &tgbotapi.ReplyKeyboardMarkup{Keyboard: finalButtonsSet, ResizeKeyboard: true}
}
