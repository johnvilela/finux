package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"finux/src/setting"
)

// daemonTickVariable names the environment variable that replaces the
// time between two runs of the jobs of the daemon, for tests.
const daemonTickVariable = "FINUX_DAEMON_TICK"

func daemonHelp() string {
	return "Run Finux in the background\n\n" +
		green.Render("Usage:") + " finux daemon <command>\n\n" +
		green.Render("Commands:") + `
  run                 Run the daemon in this terminal, until Ctrl+C
  install             Make the daemon a service: it starts now and at each login
  uninstall           Stop the service and remove it
  start, stop         Start or stop the service
  restart             Start the service again
  status              Show if the service runs
  logs [--follow]     Show the last lines of the log of the service
  help, --help, -h    Show this help

The daemon is optional: all the commands work without it. While it runs,
it does the work that needs a process that stays alive:

- It runs the Telegram bot, when a token is set. See
  'finux config telegram --help'.
- It keeps the status of the bills and the transactions current, and
  creates the transactions of the automatic schedules on their dates.
- With the setting telegram-reminders on, it tells the allowed people in
  Telegram about a schedule that is due, a bill that closed, is due or is
  late, and a budget near or over its limit. Each reminder comes one time.
- With the setting telegram-resume on daily or monthly, it sends the
  resume at the time in telegram-resume-time.
- With an address in the setting daemon-mcp, it serves the MCP server for
  AI agents there. See 'finux config mcp --help'.

The service is a systemd user unit: ~/.config/systemd/user/finux.service.
'finux daemon install' writes it for this binary and for the database in
the FINUX_DB environment variable, when it is set. Run it again after you
move the binary. 'finux update' starts the service again by itself. The
service runs while you are logged in; 'loginctl enable-linger' keeps it
running after you log out. Without systemd, use 'finux daemon run'.

The daemon reads the token of the bot and the setting daemon-mcp when it
starts: start it again after you change them. The other settings and the
list of the allowed people need no restart.
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
	case "install", "uninstall", "start", "stop", "restart", "status", "logs":
		return exitStatus(io, "daemon", serviceCommand(args[0], args[1:], io))
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
	address, _ := setting.NewStore(db).Get(setting.DaemonMCP)
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
	work := &jobs{lock: &database, log: log}
	if token == "" {
		log("Telegram bot is off: no token. Run 'finux config telegram token <token>' and start the daemon again.")
	} else {
		log("Telegram bot is on")
		chat := newBot(token, &database, log)
		work.client = chat.client
		running.Go(func() { chat.poll(ctx) })
	}
	if address != "" {
		running.Go(func() {
			if err := serveHTTP(ctx, newMCPServer(false), address, io); err != nil {
				log("MCP server: %v", err)
			}
		})
	}
	ticker := time.NewTicker(daemonTick())
	defer ticker.Stop()
	for work.tick(ctx); ctx.Err() == nil; {
		select {
		case <-ctx.Done():
		case <-ticker.C:
			work.tick(ctx)
		}
	}
	running.Wait()
	log("Finux daemon stopped")
	return nil
}
