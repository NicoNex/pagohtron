package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "embed"

	"github.com/NicoNex/echotron/v3"
)

var (
	//go:embed token
	token string
	//go:embed assets/pagah.mp4
	pagah []byte

	dsp *echotron.Dispatcher

	commands = []echotron.BotCommand{
		{Command: "/start", Description: "Inizializza e configura il bot per la chat."},
		{Command: "/configura", Description: "Configura il bot."},
		{Command: "/impostazioni", Description: "Mostra le impostazioni del bot."},
		{Command: "/adesso", Description: "Richiedi il pagamento adesso."},
		{Command: "/about", Description: "Per codice, problemi e suggerimenti."},
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

	monthMap = map[time.Month]string{
		time.January:   "gennaio",
		time.February:  "febbraio",
		time.March:     "marzo",
		time.April:     "aprile",
		time.May:       "maggio",
		time.June:      "giugno",
		time.July:      "luglio",
		time.August:    "agosto",
		time.September: "settembre",
		time.October:   "ottobre",
		time.November:  "novembre",
		time.December:  "dicembre",
	}

	md = &echotron.MessageOptions{
		ParseMode: echotron.MarkdownV2,
	}

	escape = strings.NewReplacer(
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
	).Replace

	pagahID string
)

type stateFn func(*echotron.Update) stateFn

type bot struct {
	chatID   int64
	usrstate map[int64]stateFn
	state    stateFn
	admins   map[int64]bool
	*cachable
	echotron.API
}

func (b bot) messagef(f string, a ...any) {
	_, err := b.SendMessage(
		fmt.Sprintf(f, a...),
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
		chatID:   chatID,
		API:      echotron.NewAPI(token),
		usrstate: make(map[int64]stateFn),
		admins:   make(map[int64]bool),
	}
	if err := b.init(); err != nil {
		b.SendMessage("Non riesco a recuperare le informazioni relative a questa chat.\nPer favore riconfigura il bot con il comando /start.", chatID, nil)
		del(chatID)
	}
	go b.tick()

	return b
}

func (b *bot) init() error {
	b.state = b.handleMessage

	// Load the cachable object.
	cachable, err := Cachable(b.chatID)
	if err != nil {
		log.Println("b.init", "Cachable", err)
	}
	b.cachable = &cachable

	// Set isGroup field.
	res, err := b.GetChat(b.chatID)
	if err != nil {
		log.Println("b.init", "b.GetChat", err)
		return err
	}
	chatType := res.Result.Type

	// Set the admins' ID.
	if chatType == "group" || chatType == "supergroup" {
		res, err := b.GetChatAdministrators(b.chatID)
		if err != nil {
			log.Println("b.init", "b.GetChatAdministrators", err)
			return err
		}

		for _, chatMember := range res.Result {
			if chatMember.User != nil {
				b.admins[chatMember.User.ID] = true
			}
		}
	} else {
		b.admins[b.chatID] = true
	}
	return nil
}

func (b *bot) setAmount(update *echotron.Update) stateFn {
	if update.Message == nil {
		return b.setAmount
	}

	switch msg := update.Message.Text; {
	case strings.HasPrefix(msg, "/annulla"):
		b.messagef("Operazione annullata!")
		return nil

	default:
		msg = strings.NewReplacer(",", ".", "€", "", "£", "", "$", "").Replace(msg)
		a, err := strconv.ParseFloat(msg, 64)
		if err != nil {
			log.Println("setAmount", err)
			b.messagef("Formato non valido, per favore riprova\\.")
			return b.setAmount
		}
		b.PPAmount = a
		go b.save()
		if b.IsYearly {
			b.messagef(
				"Perfetto, ricorderò di pagare la somma di *%s€* ogni *%02d\\-%02d*\\!",
				escape(fmt.Sprintf("%.2f", b.PPAmount)),
				b.ReminderDay,
				b.ReminderMonth,
			)
		} else {
			b.messagef(
				"Perfetto, ricorderò di pagare la somma di *%s€* ogni *%d* del mese\\!",
				escape(fmt.Sprintf("%.2f", b.PPAmount)),
				b.ReminderDay,
			)
		}
		return nil
	}
}

func (b *bot) setNick(update *echotron.Update) stateFn {
	if update.Message == nil {
		return b.setNick
	}

	switch msg := update.Message.Text; {
	case strings.HasPrefix(msg, "/annulla"):
		b.messagef("Operazione annullata!")
		return nil

	default:
		b.PPNick = msg
		go b.save()
		b.messagef("Perfetto, ora mandami la *somma* da richiedere mensilmente\\.\nEsempio: 1\\.50")
		return b.setAmount
	}
}

func (b *bot) setDay(update *echotron.Update) stateFn {
	if update.Message == nil {
		return b.setDay
	}

	switch msg := update.Message.Text; {
	case strings.HasPrefix(msg, "/annulla"):
		b.messagef("Operazione annullata!")
		return nil

	default:
		d, err := strconv.ParseInt(msg, 10, 32)
		if err != nil {
			b.messagef("Formato non valido, per favore riprova\\.")
			return b.setDay
		}
		if d < 1 || d > 28 {
			b.messagef("Per favore inserisci una data *compresa tra 1 e 28*\\!")
			return b.setDay
		}

		b.ReminderDay = int(d)
		go b.save()
		b.messagef("Perfetto, ora dimmi il *nickname* di *PayPal* del ricevente\\.")
		return b.setNick
	}
}

func (b *bot) setMonthAndDay(update *echotron.Update) stateFn {
	if update.Message == nil {
		return b.setMonthAndDay
	}

	switch msg := update.Message.Text; {
	case strings.HasPrefix(msg, "/annulla"):
		b.messagef("Operazione annullata!")
		return nil

	default:
		t, err := time.Parse("02-01", strings.ReplaceAll(msg, "/", "-"))
		if err != nil {
			log.Println("b.setMonthAndDay", "time.Parse", err)
			b.messagef("Formato non valido, per favore riprova\\.")
			return b.setMonthAndDay
		}
		b.ReminderDay = t.Day()
		b.ReminderMonth = t.Month()
		go b.save()
		b.messagef("Perfetto, ora dimmi il *nickname* di *PayPal* del ricevente\\.")
		return b.setNick

	}
}

func (b *bot) setPlan(update *echotron.Update) stateFn {
	if update.Message == nil {
		return b.setPlan
	}

	switch msg := update.Message.Text; {
	case strings.HasPrefix(msg, "/annulla"):
		_, err := b.SendMessage(
			"Operazione annullata!",
			b.chatID,
			&echotron.MessageOptions{
				ReplyMarkup: echotron.ReplyKeyboardRemove{
					RemoveKeyboard: true,
				},
			},
		)
		if err != nil {
			log.Println("b.setPlan", "b.SendMessage", err)
		}
		return nil

	case msg == "Mensile":
		b.IsYearly = false
		go b.save()
		_, err := b.SendMessage(
			"Perfetto, ora specifica il *giorno* in cui ricordare il pagamento \\(*compreso tra 1 e 28*\\)\\.",
			b.chatID,
			&echotron.MessageOptions{
				ParseMode: echotron.MarkdownV2,
				ReplyMarkup: echotron.ReplyKeyboardRemove{
					RemoveKeyboard: true,
				},
			},
		)
		if err != nil {
			log.Println("b.setPlan", "b.SendMessage", err)
		}
		return b.setDay

	case msg == "Annuale":
		b.IsYearly = true
		go b.save()
		_, err := b.SendMessage(
			"Perfetto, ora specifica la data in cui mandare il reminder nel formato *DD\\-MM*\\.\nEsempio: \"15\\-07\" per indicare il 15 luglio\\.",
			b.chatID,
			&echotron.MessageOptions{
				ParseMode: echotron.MarkdownV2,
				ReplyMarkup: echotron.ReplyKeyboardRemove{
					RemoveKeyboard: true,
				},
			},
		)
		if err != nil {
			log.Println("b.setPlan", "b.SendMessage", err)
		}
		return b.setMonthAndDay
	}

	return b.setPlan
}

func (b *bot) handleMessage(update *echotron.Update) stateFn {
	if update.CallbackQuery != nil {
		b.handleGenericCallback(update)
		return b.handleMessage
	}

	switch msg := update.Message.Text; {
	case strings.HasPrefix(msg, "/configura") && b.isAdmin(userID(update)):
		_, err := b.SendMessage(
			"Per prima cosa dimmi se il pagamento è mensile o annuale\\.\nPuoi mandare /annulla in qualsiasi momento per annullare l'operazione\\.",
			b.chatID,
			&echotron.MessageOptions{
				ParseMode:        echotron.MarkdownV2,
				ReplyToMessageID: update.Message.ID,
				ReplyMarkup: echotron.ReplyKeyboardMarkup{
					OneTimeKeyboard:       true,
					Selective:             true,
					ResizeKeyboard:        true,
					InputFieldPlaceholder: "Seleziona il piano...",
					Keyboard: [][]echotron.KeyboardButton{
						{{Text: "Mensile"}, {Text: "Annuale"}},
					},
				},
			},
		)
		if err != nil {
			log.Println("b.handleMessage", "b.SendMessage", err)
		}
		b.usrstate[userID(update)] = b.setPlan

	case strings.HasPrefix(msg, "/start") && b.isAdmin(userID(update)):
		b.messagef("Ciao sono *Pagohtron*, il bot che ricorda i pagamenti mensili di gruppo\\!")
		b.messagef("Prima di cominciare ho bisogno di sapere:\n\\- il *piano* del pagamento \\(mensile o annuale\\)\n\\- il *nickname* di PayPal del ricevente\n\\- la *somma di denaro* da chiedere\n\\- il *giorno* in cui devo ricordare a tutti il pagamento")
		_, err := b.SendMessage(
			"Per prima cosa dimmi se il pagamento è mensile o annuale\\.\nPuoi mandare /annulla in qualsiasi momento per annullare l'operazione\\.",
			b.chatID,
			&echotron.MessageOptions{
				ParseMode:        echotron.MarkdownV2,
				ReplyToMessageID: update.Message.ID,
				ReplyMarkup: echotron.ReplyKeyboardMarkup{
					OneTimeKeyboard:       true,
					Selective:             true,
					ResizeKeyboard:        true,
					InputFieldPlaceholder: "Seleziona il piano...",
					Keyboard: [][]echotron.KeyboardButton{
						{{Text: "Mensile"}, {Text: "Annuale"}},
					},
				},
			},
		)
		if err != nil {
			log.Println("b.handleMessage", "b.SendMessage", err)
		}
		b.usrstate[userID(update)] = b.setPlan

	case strings.HasPrefix(msg, "/impostazioni"):
		b.sendSettings()

	case strings.HasPrefix(msg, "/about"):
		b.sendAbout()

	case strings.HasPrefix(msg, "/adesso") && b.isAdmin(userID(update)):
		b.remind()
	}

	return b.handleMessage
}

func (b *bot) handleGenericCallback(update *echotron.Update) {
	if update.CallbackQuery.Data != "confirm" {
		b.AnswerCallbackQuery(update.CallbackQuery.ID, nil)
		return
	}

	// If the user is among the payers tell them they have already paid.
	if isIn(userID(update), b.Payers) {
		b.alreadyPaidAlert(update.CallbackQuery)
		return
	}

	b.Payers = append(b.Payers, userID(update))
	b.sendConfirmation(update.CallbackQuery)
	b.AnswerCallbackQuery(
		update.CallbackQuery.ID,
		&echotron.CallbackQueryOptions{
			Text:      thanksMsg(),
			ShowAlert: true,
		},
	)
}

func (b *bot) Update(update *echotron.Update) {
	if state, ok := b.usrstate[userID(update)]; ok {
		if next := state(update); next != nil {
			b.usrstate[userID(update)] = next
		} else {
			delete(b.usrstate, userID(update))
		}
	} else {
		b.state = b.state(update)
	}
}

func (b bot) save() {
	if err := b.Put(b.chatID); err != nil {
		log.Println("b.save", err)
	}
}

func (b bot) sendSettings() {
	if b.IsYearly {
		b.messagef(
			"Nickname PayPal ricevente: *%s*\nSomma da versare: *%s€*\nData del reminder: *%s*",
			escape(b.PPNick),
			escape(fmt.Sprintf("%.2f", b.PPAmount)),
			escape(fmt.Sprintf("%02d %s", b.ReminderDay, monthMap[b.ReminderMonth])),
		)
	} else {
		b.messagef(
			"Nickname PayPal ricevente: *%s*\nSomma da versare: *%s€*\nGiorno del reminder: *%d di ogni mese*",
			escape(b.PPNick),
			escape(fmt.Sprintf("%.2f", b.PPAmount)),
			b.ReminderDay,
		)
	}
}

func (b bot) sendPagah() {
	if pagahID != "" {
		_, err := b.SendVideoNote(
			echotron.NewInputFileID(pagahID),
			b.chatID,
			nil,
		)
		if err != nil {
			log.Println("b.sendPagah", "b.SendVideoNote", err)
			return
		}
		return
	}

	res, err := b.SendVideoNote(
		echotron.NewInputFileBytes("pagah.mp4", pagah),
		b.chatID,
		nil,
	)
	if err != nil {
		log.Println("b.sendPagah", "b.SendVideoNote", err)
		return
	}
	// Set the video note file ID into pagahID and store it in a file.
	pagahID = res.Result.VideoNote.FileID
	go func() {
		if err := os.WriteFile(pagahPath, []byte(pagahID), 0644); err != nil {
			log.Println("b.sendPagah", "os.WriteFile", err)
		}
	}()
}

func (b *bot) remind() {
	defer b.save()

	// Reset the payers array.
	b.Payers = []int64{}
	// Send Zeb89's Pagah video note.
	b.sendPagah()

	// Send the reminder message.
	msg := fmt.Sprintf(
		"*Pagah\\!*\nManda %s€ a %s\\!\n",
		escape(fmt.Sprintf("%.2f", b.PPAmount)),
		b.PPNick,
	)
	res, err := b.SendMessage(
		msg,
		b.chatID,
		&echotron.MessageOptions{
			ParseMode:   echotron.MarkdownV2,
			ReplyMarkup: b.reminderKbd(),
		},
	)
	if err != nil {
		log.Println("remind", "b.SendMessage", err)
		return
	}

	// Save the reminder message ID for mentioning it later.
	b.ReminderID = res.Result.ID
	if _, err := b.PinChatMessage(b.chatID, b.ReminderID, nil); err != nil {
		log.Println("b.remind", "b.PinChatMessage", err)
	}
}

func (b bot) alreadyPaidAlert(q *echotron.CallbackQuery) {
	_, err := b.AnswerCallbackQuery(
		q.ID,
		&echotron.CallbackQueryOptions{
			Text:      fmt.Sprintf("Fra, hai già %s.", random(verbs)),
			ShowAlert: true,
		},
	)
	if err != nil {
		log.Println("b.alreadyPaidAlert", "b.AnswerCallbackQuery", err)
	}
}

func (b bot) sendConfirmation(q *echotron.CallbackQuery) {
	_, err := b.SendMessage(
		fmt.Sprintf(
			"%s ha %s %s!",
			usernameOrName(q),
			random(verbs),
			random(currencies),
		),
		b.chatID,
		&echotron.MessageOptions{
			ReplyToMessageID: b.ReminderID,
		},
	)
	if err != nil {
		log.Println("sendConfirmation", "b.SendMessage", err)
	}
}

func (b bot) sendAbout() {
	_, err := b.SendMessage(`Bot scritto con [Echotron](https://github\.com/NicoNex/echotron) da @NicoNex e @Dj\_Mike238\.
Il codice del bot è aperto e lo trovi su [GitHub](https://github\.com/NicoNex/pagohtron)\.
Se hai suggerimenti o problemi contattaci su Telegram o apri una [issue](https://github\.com/NicoNex/pagohtron/issues) su GitHub\.`,
		b.chatID,
		&echotron.MessageOptions{
			ParseMode: echotron.MarkdownV2,
		},
	)
	if err != nil {
		log.Println("b.sendAbout", "b.SendMessage", err)
	}
}

func (b bot) tick() {
	for t := range time.Tick(time.Hour) {
		if t.Day() == b.ReminderDay && t.Hour() == 8 && (!b.IsYearly || t.Month() == b.ReminderMonth) {
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

func thanksMsg() string {
	return fmt.Sprintf("Grazie per aver %s %s!", random(verbs), random(currencies))
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

func usernameOrName(q *echotron.CallbackQuery) string {
	if uname := q.From.Username; uname != "" {
		return "@" + uname
	} else {
		return q.From.FirstName
	}
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

	flag.StringVar(&token, "t", token, "The Telegram token of the bot.")
	flag.Parse()

	// Intercept SIGINT, SIGTERM, SIGSEGV and SIGKILL and gracefully close the DB.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(
		sigChan,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGABRT,
		syscall.SIGSEGV,
		syscall.SIGKILL,
		syscall.SIGQUIT,
	)
	go func() {
		<-sigChan
		cc.Close()
		os.Exit(0)
	}()

	api := echotron.NewAPI(token)
	api.SetMyCommands(nil, commands...)

	dopts := echotron.UpdateOptions{
		AllowedUpdates: []echotron.UpdateType{
			echotron.MessageUpdate,
			echotron.CallbackQueryUpdate,
		},
		Timeout: 120,
	}
	dsp = echotron.NewDispatcher(token, newBot)
	for _, k := range keys() {
		log.Printf("starting dispatcher session with ID: %d", k)
		dsp.AddSession(k)
	}
	for {
		log.Println(dsp.PollOptions(true, dopts))
		time.Sleep(5 * time.Second)
	}
}

func init() {
	id, err := os.ReadFile(pagahPath)
	if err != nil {
		log.Println("init", "os.ReadFile", err)
		return
	}
	pagahID = string(id)
}
