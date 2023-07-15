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
	pagah  []byte
	pplink = "https://paypal.me/%s/%s"

	commands = []echotron.BotCommand{
		{Command: "/impostazioni", Description: "Generate a new Jitsi meeting."},
	}
)

type stateFn func(*echotron.Update) stateFn

type cachable struct {
	ppnick      string
	ppamount    float64
	reminderDay int
	lastAsked   int64
}

func (c cachable) String() string {
	return fmt.Sprintf(
		"Nickname PayPal ricevente: %s\nSomma richiesta: %f\nGiorno del reminder: %d",
		c.ppnick,
		c.ppamount,
		c.reminderDay,
	)
}

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
	b := &bot{
		chatID: chatID,
		API:    echotron.NewAPI(token),
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

		b.reminderDay = int(d)
		// go b.writeDB()
		b.messagef("Perfetto, ricorderò di pagare la somma di %.2f€ ogni %d del mese!", b.ppamount, b.reminderDay)
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
		b.ppamount = a
		// go b.writeDB()
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
		b.ppnick = msg
		// go b.writeDB()
		b.messagef("Perfetto, ora mandami la somma da richiedere mensilmente.")
		return b.setAmount
	}
}

func (b *bot) handleMessage(update *echotron.Update) stateFn {
	switch msg := update.Message.Text; {
	case strings.HasPrefix(msg, "/annulla"):
		b.messagef("Operazione annullata.")

	case strings.HasPrefix(msg, "/impostazioni"):
		b.messagef("Per prima cosa dimmi il nikname di PayPal del ricevente.\nPuoi mandare /annulla in qualsiasi momento per annullare l'operazione.")
		return b.setNick

	case strings.HasPrefix(msg, "/start"):
		b.messagef("Ciao sono Pagohtron, il bot che ricorda i pagamenti mensili di gruppo!")
		b.messagef("Prima di cominciare ho bisogno di sapere:\n- il nikname di PayPal del ricevente\n- la somma di denaro da chiedere\n- il giorno in cui devo ricordare a tutti il pagamento")
		b.messagef("Per prima cosa dimmi il nikname di PayPal del ricevente.\nPuoi mandare /annulla in qualsiasi momento per annullare l'operazione.")
		return b.setNick

	case strings.HasPrefix(msg, "/test"):
		b.remind()
	}

	return b.handleMessage
}

func (b *bot) Update(update *echotron.Update) {
	b.state = b.state(update)
}

func (b bot) remind() {
	b.SendVideoNote(
		echotron.NewInputFileBytes("pagah.mp4", pagah),
		b.chatID,
		nil,
	)
}

func (b bot) tick() {
	for t := range time.Tick(time.Hour) {
		if t.Day() == b.reminderDay && t.Hour() == 8 {
			b.messagef("pagah")
		}
	}
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
