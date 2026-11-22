package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// daemonTickVariable names the environment variable that replaces the
// time between two runs of the jobs of the daemon, for tests.
const daemonTickVariable = "FINUX_DAEMON_TICK"

func daemonHelp() string {
	return "Run Finux in the background\n\n" +
		green.Render("Usage:") + " finux daemon <command>\n\n" +
		green.Render("Commands:") + `
  run                 Run the daemon in this terminal, until Ctrl+C
  help, --help, -h    Show this help

The daemon is optional: all the commands work without it. While it runs,
it does the work that needs a process that stays alive:

- It runs the Telegram bot, when a token is set. See
  'finux config telegram --help'.
`
}

func runDaemon(args []string, io streams) int {
	if len(args) == 0 || isHelp(args[0]) {
		return write(io, daemonHelp())
	}
	switch args[0] {
	case "run":
		if len(args) > 1 {
			return exitStatus(io, "daemon", usageError("run: unexpected argument: "+args[1]))
		}
		return exitStatus(io, "daemon", serveDaemon(io))
	}
	return exitStatus(io, "daemon", usageError("unknown command: "+args[0]))
}

// daemonTick returns the time between two runs of the jobs.
func daemonTick() time.Duration {
	if tick, err := time.ParseDuration(os.Getenv(daemonTickVariable)); err == nil && tick > 0 {
		return tick
	}
	return time.Minute
}

// serveDaemon runs the daemon until the process gets an interrupt. Its
// log goes to stderr: systemd keeps it in the journal.
func serveDaemon(io streams) error {
	db, _, err := openTelegram()
	if err == nil && !transactionsReady(db) {
		closeDatabase(db)
		err = errOutdated
	}
	if err != nil {
		return err
	}
	token := telegramToken(db)
	closeDatabase(db)

	var logLock sync.Mutex
	log := func(format string, values ...any) {
		logLock.Lock()
		defer logLock.Unlock()
		fmt.Fprintf(io.stderr, format+"\n", values...)
	}
	ctx, release := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer release()
	// The bot and the jobs use the database one at a time.
	var database sync.Mutex
	var running sync.WaitGroup
	log("Finux daemon started")
	if token == "" {
		log("Telegram bot is off: no token. Run 'finux config telegram token <token>' and start the daemon again.")
	} else {
		log("Telegram bot is on")
		running.Go(func() { newBot(token, &database, log).poll(ctx) })
	}
	<-ctx.Done()
	running.Wait()
	log("Finux daemon stopped")
	return nil
}
