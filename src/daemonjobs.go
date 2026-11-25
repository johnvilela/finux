package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/colorprofile"

	"finux/src/budget"
	"finux/src/card"
	"finux/src/money"
	"finux/src/note"
	"finux/src/schedule"
	"finux/src/setting"
	"finux/src/telegram"
	"finux/src/transaction"
	"finux/src/ui"

	"gorm.io/gorm"
)

// jobs does the work of the daemon that follows the date: the stored
// statuses, the automatic schedules, the reminders and the resume.
type jobs struct {
	// lock is held while the jobs use the database: the bot uses the
	// same database.
	lock *sync.Mutex
	// client sends the messages. It is nil when the bot has no token.
	client *telegram.Client
	log    func(format string, values ...any)
	// cleaned is the last time the jobs deleted the old files of the
	// inbox of the AI.
	cleaned time.Time
}

// uncolored takes the colors out of a text of the command line.
func uncolored(text string) string {
	var plain bytes.Buffer
	fmt.Fprint(&colorprofile.Writer{Forward: &plain, Profile: colorprofile.NoTTY}, text)
	return plain.String()
}

// tick runs the jobs one time and sends their messages.
func (j *jobs) tick(ctx context.Context) {
	if time.Since(j.cleaned) > time.Hour {
		j.cleaned = time.Now()
		if err := cleanInbox(); err != nil {
			j.log("Jobs: clean the inbox of the AI: %v", err)
		}
	}
	var messages []string
	var people []telegram.User
	err := func() error {
		j.lock.Lock()
		defer j.lock.Unlock()
		db, store, err := openTelegram()
		if err == nil && !transactionsReady(db) {
			closeDatabase(db)
			err = errOutdated
		}
		if err != nil {
			return err
		}
		defer closeDatabase(db)
		syncStatuses(db)
		var paid []string
		for _, line := range runAutomatic(db) {
			line = uncolored(line)
			j.log("%s", line)
			paid = append(paid, html.EscapeString(line))
		}
		if j.client == nil {
			return nil
		}
		settings := setting.NewStore(db)
		if settings.Enabled(setting.TelegramReminders) {
			reminders, err := reminders(db, store)
			if err != nil {
				return err
			}
			messages = append(paid, reminders...)
		}
		resume, err := dueResume(db, settings, store)
		if err != nil {
			return err
		}
		messages = append(messages, resume...)
		people, err = store.Users()
		return err
	}()
	if err != nil {
		j.log("Jobs: %v", err)
	}
	for _, message := range messages {
		for _, person := range people {
			if err := j.client.SendMessage(ctx, person.ID, message, nil); err != nil && !errors.Is(err, context.Canceled) {
				j.log("Telegram: send a message to %d: %v", person.ID, err)
			}
		}
	}
}

// reminders returns the reminders that were not sent yet, as messages.
// It records each one, so it comes one time.
func reminders(db *gorm.DB, store *telegram.Store) ([]string, error) {
	var messages []string
	// remind adds a message when its key is new.
	remind := func(key, message string) error {
		first, err := store.Once(key)
		if first {
			messages = append(messages, html.EscapeString(message))
		}
		return err
	}
	now := today()

	schedules, err := schedule.NewStore(db).List(false)
	if err != nil {
		return nil, err
	}
	automatic := setting.NewStore(db).Enabled(setting.AutomaticSchedules)
	for _, current := range schedules {
		pending := current.Pending(now)
		// An automatic schedule pays itself: it needs no reminder.
		if len(pending) == 0 || current.Auto && automatic {
			continue
		}
		message := fmt.Sprintf("Schedule %s (%s) is due since %s. Send /sc pay %s or /sc skip %s.",
			current.Code, current.Title, transaction.FormatDate(pending[0]), current.Code, current.Code)
		if err := remind(fmt.Sprintf("schedule:%d:%s", current.ID, pending[len(pending)-1]), message); err != nil {
			return nil, err
		}
	}

	notes, err := note.NewStore(db).List(note.Filter{Status: note.StatusDue}, now)
	if err != nil {
		return nil, err
	}
	for _, current := range notes {
		message := fmt.Sprintf("Note %d (%s) is due. Send /nt %d to see it, or /nt done %d.", current.ID, current.Title, current.ID, current.ID)
		// The date is in the key, so a new date reminds again.
		if err := remind(fmt.Sprintf("note:%d:%s", current.ID, current.RemindOn), message); err != nil {
			return nil, err
		}
	}

	cards := card.NewStore(db)
	owners, err := cards.List(false)
	if err != nil {
		return nil, err
	}
	tomorrow := now
	if date, err := time.Parse("2006-01-02", now); err == nil {
		tomorrow = date.AddDate(0, 0, 1).Format("2006-01-02")
	}
	for _, owner := range owners {
		bills, err := cards.Bills(owner.ID)
		if err != nil {
			return nil, err
		}
		for _, bill := range bills {
			if bill.Total() <= 0 {
				continue
			}
			name := fmt.Sprintf("The bill %s of %s", card.FormatPeriod(bill.Period), owner.Code)
			// The user also pays the bills of other persons: say whose it is.
			if !owner.Mine() {
				name += " (" + owner.Owner + ")"
			}
			total, pay := money.Format(bill.Total(), owner.Currency), fmt.Sprintf(" Send /cc pay %s.", owner.Code)
			key := fmt.Sprintf("bill:%d:", bill.ID)
			switch status := bill.StatusOn(now); {
			case status == card.StatusLate:
				err = remind(key+"late", fmt.Sprintf("%s is late: %s was due on %s.%s", name, total, transaction.FormatDate(bill.DueOn), pay))
			case status == card.StatusClosed && bill.DueOn == now:
				err = remind(key+"due", fmt.Sprintf("%s is due today: %s.%s", name, total, pay))
			case status == card.StatusClosed && bill.DueOn == tomorrow:
				err = remind(key+"due", fmt.Sprintf("%s is due tomorrow: %s.%s", name, total, pay))
			case status == card.StatusClosed:
				err = remind(key+"closed", fmt.Sprintf("%s closed: %s. It is due on %s.%s", name, total, transaction.FormatDate(bill.DueOn), pay))
			}
			if err != nil {
				return nil, err
			}
		}
	}

	budgets, err := budget.NewStore(db).List(false, now)
	if err != nil {
		return nil, err
	}
	for _, current := range budgets {
		usage, ok := current.Current(now)
		if !ok || usage.Level() == budget.LevelOK {
			continue
		}
		key := fmt.Sprintf("budget:%d:%s:%s", current.ID, usage.From, usage.Level())
		if err := remind(key, budgetMessage(current, usage)); err != nil {
			return nil, err
		}
	}
	return messages, nil
}

// dueResume returns the resume as messages when it is time to send it:
// one time each day, or on day 1 of a month, after the time of the
// setting.
func dueResume(db *gorm.DB, settings *setting.Store, store *telegram.Store) ([]string, error) {
	values, err := settings.All()
	if err != nil {
		return nil, err
	}
	now := today()
	switch values[setting.TelegramResume] {
	case setting.Daily:
	case setting.Monthly:
		if !strings.HasSuffix(now, "-01") {
			return nil, nil
		}
	default:
		return nil, nil
	}
	if time.Now().Format("15:04") < values[setting.TelegramResumeTime] {
		return nil, nil
	}
	if first, err := store.Once("resume:" + now); err != nil || !first {
		return nil, err
	}
	resume := ui.Resume{Today: now, Month: now[:len("2006-01")]}
	if err := gatherResume(db, &resume); err != nil {
		return nil, err
	}
	var messages []string
	for _, part := range chunks(ui.ResumeView(resume, true), botChunk) {
		messages = append(messages, "<pre>"+html.EscapeString(part)+"</pre>")
	}
	return messages, nil
}
