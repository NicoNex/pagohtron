package main

import (
	"fmt"
	"log"
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
)

type stateFn func(*echotron.Update) stateFn

type bot struct {
	chatID int64
	state  stateFn
	cachable
	echotron.API
}

func (b bot) messagef(f string, a ...any) {
	if _, err := b.SendMessage(fmt.Sprintf(f, a...), b.chatID, nil); err != nil {
		log.Println("messagef", "b.SendMessage", err)
	}
}

func newBot(chatID int64) echotron.Bot {
	cachable, err := Cachable(chatID)
	if err != nil {
		log.Println("newBot", "Cachable", err)
	}
	b := &bot{
		chatID:   chatID,
		API:      echotron.NewAPI(token),
		cachable: cachable,
	}
	b.state = b.handleMessage
	go b.tick()
	return b
}

func (b *bot) setDay(update *echotron.Update) stateFn {
	switch msg := update.Message.Text; {
	case strings.HasPrefix(msg, "/annulla"):
		b.messagef("Operazione annullata!")
		return b.handleMessage

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
		return b.handleMessage
	}
}

func (b *bot) setAmount(update *echotron.Update) stateFn {
	switch msg := update.Message.Text; {
	case strings.HasPrefix(msg, "/annulla"):
		b.messagef("Operazione annullata!")
		return b.handleMessage

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
		return b.handleMessage

	default:
		b.PPNick = msg
		go b.save()
		b.messagef("Perfetto, ora mandami la somma da richiedere mensilmente.\nEsempio: 1.50")
		return b.setAmount
	}
}

func (b *bot) handleMessage(update *echotron.Update) stateFn {
	switch msg := update.Message.Text; {
	case strings.HasPrefix(msg, "/annulla"):
		b.messagef("Operazione annullata.")

	case strings.HasPrefix(msg, "/impostazioni") /*&& b.isAdmin(userID(update))*/ :
		b.messagef("Per prima cosa dimmi il nickname di PayPal del ricevente.\nPuoi mandare /annulla in qualsiasi momento per annullare l'operazione.")
		return b.setNick

	case strings.HasPrefix(msg, "/start") /*&& b.isAdmin(userID(update))*/ :
		b.messagef("Ciao sono Pagohtron, il bot che ricorda i pagamenti mensili di gruppo!")
		b.messagef("Prima di cominciare ho bisogno di sapere:\n- il nickname di PayPal del ricevente\n- la somma di denaro da chiedere\n- il giorno in cui devo ricordare a tutti il pagamento")
		b.messagef("Per prima cosa dimmi il nickname di PayPal del ricevente.\nPuoi mandare /annulla in qualsiasi momento per annullare l'operazione.")
		return b.setNick

	case strings.HasPrefix(msg, "/test"):
		b.remind()
	}

	return b.handleMessage
}

func (b *bot) Update(update *echotron.Update) {
	b.state = b.state(update)
}

func (b bot) save() {
	if err := b.Put(b.chatID); err != nil {
		log.Println(err)
	}
}

func (b bot) remind() {
	_, err := b.SendVideoNote(
		echotron.NewInputFileBytes("pagah.mp4", pagah),
		b.chatID,
		nil,
	)
	if err != nil {
		log.Println("remind", "b.SendVideoNote", err)
	}
	msg := fmt.Sprintf("Pagah!\nManda %.2f€ a %s!", b.PPAmount, b.PPNick)
	if _, err = b.SendMessage(msg, b.chatID, b.paypalButton()); err != nil {
		log.Println("remind", "b.SendMessage", err)
	}
}

func (b bot) tick() {
	for t := range time.Tick(time.Hour) {
		if t.Day() == b.ReminderDay && t.Hour() == 8 {
			b.remind()
		}
	}
}

func (b bot) paypal() string {
	return fmt.Sprintf("https://paypal.me/%s/%.2f", b.PPNick, b.PPAmount)
}

func (b bot) paypalButton() *echotron.MessageOptions {
	return &echotron.MessageOptions{
		ReplyMarkup: echotron.InlineKeyboardMarkup{
			InlineKeyboard: [][]echotron.InlineKeyboardButton{
				{{
					Text: "PayPal",
					URL:  b.paypal(),
				}},
			},
		},
	}
}

func (b bot) isAdmin(id int64) bool {
	res, err := b.GetChatAdministrators(b.chatID)
	if err != nil {
		log.Println("isAdmin", "b.GetChatAdministrators", err)
		return false
	}

	for _, r := range res.Result {
		if id == r.User.ID {
			return true
		}
	}
	return false
}

func userID(u *echotron.Update) int64 {
	return u.Message.From.ID
}

func main() {
	api := echotron.NewAPI(token)
	api.SetMyCommands(nil, commands...)

	dopts := echotron.UpdateOptions{
		AllowedUpdates: []echotron.UpdateType{echotron.MessageUpdate},
		Timeout:        120,
	}
	dsp := echotron.NewDispatcher(token, newBot)
	for {
		log.Println(dsp.PollOptions(true, dopts))
		time.Sleep(5 * time.Second)
	}
}
