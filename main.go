package main

import (
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"time"

	_ "embed"

	"github.com/NicoNex/echotron/v3"
)

var (
	//go:embed token
	token string
	//go:embed assets/pagah.mp4
	pagah []byte

	commands = []echotron.BotCommand{
		{Command: "/impostazioni", Description: "Modifica le impostazioni del gruppo."},
	}

	verbs = []string{
		"pagato",
		"sborsato",
		"spillato",
		"sganciato",
		"investito",
		"elargito",
		"silurato",
		"depositato",
		"versato",
	}

	currencies = []string{
		"gli euri",
		"i quattrini",
		"i fiorini",
		"i sacchi",
		"la pecunia",
		"il danaro",
		"il grano",
		"la grana",
		"la moneta",
		"l'obolo",
		"i fondi",
		"il capitale",
		"gli spicci",
		"il patrimonio",
		"gli averi",
		"le finanze",
		"il dazio",
		"il tributo",
	}

	md2Esc = strings.NewReplacer(
		"_", "\\_",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
)

type stateFn func(*echotron.Update) stateFn

type bot struct {
	chatID int64
	state  map[int64]stateFn
	admins map[int64]bool
	*cachable
	echotron.API
}

func (b bot) messagef(f string, a ...any) {
	_, err := b.SendMessage(
		md2Esc.Replace(fmt.Sprintf(f, a...)),
		b.chatID,
		&echotron.MessageOptions{
			ParseMode: echotron.MarkdownV2,
		},
	)

	if err != nil {
		log.Println("messagef", "b.SendMessage", err)
	}
}

func newBot(chatID int64) echotron.Bot {
	b := &bot{
		chatID: chatID,
		API:    echotron.NewAPI(token),
		state:  make(map[int64]stateFn),
		admins: make(map[int64]bool),
	}
	b.init()
	go b.tick()
	return b
}

func (b *bot) init() {
	// Load the cachable object.
	cachable, err := Cachable(b.chatID)
	if err != nil {
		log.Println("b.init", "Cachable", err)
	}
	b.cachable = &cachable

	// Set isGroup field.
	res, err := b.GetChat(b.chatID)
	if err != nil {
		log.Fatal("b.init", "b.GetChat", err)
	}
	chatType := res.Result.Type

	// Set the admins' ID.
	if chatType == "group" || chatType == "supergroup" {
		res, err := b.GetChatAdministrators(b.chatID)
		if err != nil {
			log.Fatal("b.init", "b.GetChatAdministrators", err)
		}

		for _, chatMember := range res.Result {
			if chatMember.User != nil {
				b.admins[chatMember.User.ID] = true
			}
		}
	} else {
		b.admins[b.chatID] = true
	}
}

func (b *bot) setDay(update *echotron.Update) stateFn {
	switch msg := update.Message.Text; {
	case strings.HasPrefix(msg, "/annulla"):
		b.messagef("Operazione annullata!")
		return nil

	default:
		d, err := strconv.ParseInt(msg, 10, 32)
		if err != nil {
			b.messagef("Formato non valido, per favore riprova.")
			return b.setDay
		}
		if d < 1 || d > 28 {
			b.messagef("Per favore inserisci una data compresa tra 1 e 28!")
			return b.setDay
		}

		b.ReminderDay = int(d)
		go b.save()
		b.messagef("Perfetto, ricorderò di pagare la somma di %.2f€ ogni %d del mese!", b.PPAmount, b.ReminderDay)
		return nil
	}
}

func (b *bot) setAmount(update *echotron.Update) stateFn {
	switch msg := update.Message.Text; {
	case strings.HasPrefix(msg, "/annulla"):
		b.messagef("Operazione annullata!")
		return nil

	default:
		a, err := strconv.ParseFloat(msg, 64)
		if err != nil {
			log.Println("setAmount", err)
			b.messagef("Formato non valido, per favore riprova.")
			return b.setAmount
		}
		b.PPAmount = a
		go b.save()
		b.messagef("Perfetto, ora specifica il giorno in cui ricordare il pagamento (compreso tra 1 e 28).")
		return b.setDay
	}
}

func (b *bot) setNick(update *echotron.Update) stateFn {
	switch msg := update.Message.Text; {
	case strings.HasPrefix(msg, "/annulla"):
		b.messagef("Operazione annullata!")
		return nil

	default:
		b.PPNick = msg
		go b.save()
		b.messagef("Perfetto, ora mandami la somma da richiedere mensilmente.\nEsempio: 1.50")
		return b.setAmount
	}
}

func (b bot) handleMessage(update *echotron.Update) stateFn {
	switch msg := update.Message.Text; {
	case strings.HasPrefix(msg, "/annulla"):
		b.messagef("Operazione annullata.")

	case strings.HasPrefix(msg, "/impostazioni") && b.isAdmin(userID(update)):
		b.messagef("Per prima cosa dimmi il nickname di PayPal del ricevente.\nPuoi mandare /annulla in qualsiasi momento per annullare l'operazione.")
		return b.setNick

	case strings.HasPrefix(msg, "/start") && b.isAdmin(userID(update)):
		b.messagef("Ciao sono Pagohtron, il bot che ricorda i pagamenti mensili di gruppo!")
		b.messagef("Prima di cominciare ho bisogno di sapere:\n- il nickname di PayPal del ricevente\n- la somma di denaro da chiedere\n- il giorno in cui devo ricordare a tutti il pagamento")
		b.messagef("Per prima cosa dimmi il nickname di PayPal del ricevente.\nPuoi mandare /annulla in qualsiasi momento per annullare l'operazione.")
		return b.setNick

	case strings.HasPrefix(msg, "/test"):
		b.remind()
	}

	return b.handleMessage
}

func (b *bot) handleCallback(update *echotron.Update) {
	if update.CallbackQuery.Data != "confirm" {
		b.AnswerCallbackQuery(update.CallbackQuery.ID, nil)
		return
	}

	// If the user is among the payers tell him he has already paid.
	if isIn(userID(update), b.Payers) {
		b.alreadyPaidAlert(update)
		return
	}

	b.Payers = append(b.Payers, userID(update))
	b.ReminderMsg = b.reminderMsg(update)
	kbd := b.reminderKbd()

	if len(b.Payers) == b.TotalPayers {
		b.ReminderMsg = b.allPaidMsg(update)
		kbd = echotron.InlineKeyboardMarkup{}
	}
	b.editReminder(kbd)

	b.AnswerCallbackQuery(
		update.CallbackQuery.ID,
		&echotron.CallbackQueryOptions{
			Text:      thanksMsg(),
			ShowAlert: true,
		},
	)
}

func (b *bot) Update(update *echotron.Update) {
	if update.CallbackQuery != nil {
		b.handleCallback(update)
		return
	}

	state, ok := b.state[userID(update)]
	if !ok {
		state = b.handleMessage
	}

	if next := state(update); next != nil {
		b.state[userID(update)] = next
	} else {
		delete(b.state, userID(update))
	}
}

func (b bot) save() {
	if err := b.Put(b.chatID); err != nil {
		log.Println("b.save", err)
	}
}

func (b *bot) remind() {
	b.Payers = []int64{}

	count, err := b.GetChatMemberCount(b.chatID)
	if err != nil {
		log.Println("remind", "b.GetChatMemberCount", err)
	}
	// Subtract 1 to exclude the bot from the payers, otherwise
	// there will always be a missing payer.
	b.TotalPayers = count.Result - 1

	_, err = b.SendVideoNote(
		echotron.NewInputFileBytes("pagah.mp4", pagah),
		b.chatID,
		nil,
	)
	if err != nil {
		log.Println("remind", "b.SendVideoNote", err)
	}

	b.ReminderMsg = fmt.Sprintf("*Pagah!*\nManda %.2f€ a %s!\n", b.PPAmount, b.PPNick)
	res, err := b.SendMessage(
		md2Esc.Replace(b.ReminderMsg),
		b.chatID,
		&echotron.MessageOptions{
			ParseMode:   echotron.MarkdownV2,
			ReplyMarkup: b.reminderKbd(),
		},
	)
	if err != nil {
		log.Println("remind", "b.SendMessage", err)
	}
	b.ReminderID = res.Result.ID
	b.save()
}

func thanksMsg() string {
	return fmt.Sprintf("Grazie per aver %s %s!", random(verbs), random(currencies))
}

func (b bot) alreadyPaidAlert(update *echotron.Update) {
	b.AnswerCallbackQuery(
		update.CallbackQuery.ID,
		&echotron.CallbackQueryOptions{
			Text:      fmt.Sprintf("Fra, hai già %s.", random(verbs)),
			ShowAlert: true,
		},
	)
}

func (b bot) reminderMsg(update *echotron.Update) string {
	return fmt.Sprintf(
		"%s\n@%s ha già %s %s.",
		b.ReminderMsg,
		update.CallbackQuery.From.Username,
		random(verbs),
		random(currencies),
	)
}

func (b bot) allPaidMsg(update *echotron.Update) string {
	return fmt.Sprintf(
		"%s\n\nHanno pagato tutti, passo il mese prossimo a chiedere %s!",
		b.ReminderMsg,
		random(currencies),
	)
}

func (b bot) sendConfirmation(update *echotron.Update) {
	b.messagef(
		"@%s ha %s %s!",
		update.CallbackQuery.From.Username,
		random(verbs),
		random(currencies),
	)
}

func (b bot) editReminder(kbd echotron.InlineKeyboardMarkup) {
	b.EditMessageText(
		md2Esc.Replace(b.ReminderMsg),
		echotron.NewMessageID(b.chatID, b.ReminderID),
		&echotron.MessageTextOptions{
			ParseMode:   echotron.MarkdownV2,
			ReplyMarkup: kbd,
		},
	)
}

func (b bot) tick() {
	for t := range time.Tick(time.Hour) {
		if t.Day() == b.ReminderDay && t.Hour() == 8 {
			b.remind()
		}
	}
}

func (b bot) reminderKbd() echotron.InlineKeyboardMarkup {
	return echotron.InlineKeyboardMarkup{
		InlineKeyboard: [][]echotron.InlineKeyboardButton{
			{{
				Text: "PayPal",
				URL:  b.paypal(),
			}},
			{{
				Text:         "Ho pagato",
				CallbackData: "confirm",
			}},
		},
	}
}

func (b bot) paypal() string {
	return fmt.Sprintf("https://paypal.me/%s/%.2f", b.PPNick, b.PPAmount)
}

func isIn[T comparable](item T, list []T) (found bool) {
	for _, v := range list {
		if item == v {
			found = true
		}
	}

	return
}

func random[T any](list []T) T {
	return list[rand.Intn(len(list))]
}

func (b bot) isAdmin(id int64) bool {
	return b.admins[id]
}

func userID(u *echotron.Update) int64 {
	switch {
	case u.Message != nil:
		return u.Message.From.ID
	case u.CallbackQuery != nil:
		return u.CallbackQuery.From.ID
	}

	return 0
}

func main() {
	rand.Seed(time.Now().UnixMilli())

	api := echotron.NewAPI(token)
	api.SetMyCommands(nil, commands...)

	dopts := echotron.UpdateOptions{
		AllowedUpdates: []echotron.UpdateType{
			echotron.MessageUpdate,
			echotron.CallbackQueryUpdate,
		},
		Timeout: 120,
	}
	dsp := echotron.NewDispatcher(token, newBot)
	for _, k := range keys() {
		log.Printf("starting dispatcher session with ID: %d", k)
		dsp.AddSession(k)
	}
	for {
		log.Println(dsp.PollOptions(true, dopts))
		time.Sleep(5 * time.Second)
	}
}
