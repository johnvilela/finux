package main

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
)

// processVariable names the environment variable that replaces /proc,
// for tests.
const processVariable = "FINUX_PROC"

// mcpProcesses returns the ids of the processes of this user that run
// the MCP server: the ones an agent started, which talk through their
// standard input and output, and the ones that serve HTTP. A process
// keeps the binary it started with, so after an update it is the old
// version. The MCP server of the daemon is in the daemon and is not
// here. A system without /proc gives no process.
func mcpProcesses() (stdio, http []int) {
	folder := os.Getenv(processVariable)
	if folder == "" {
		folder = "/proc"
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, nil
	}
	for _, entry := range entries {
		id, err := strconv.Atoi(entry.Name())
		if err != nil || id == os.Getpid() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if owner, ok := info.Sys().(*syscall.Stat_t); ok && int(owner.Uid) != os.Getuid() {
			continue
		}
		line, err := os.ReadFile(filepath.Join(folder, entry.Name(), "cmdline"))
		if err != nil {
			continue
		}
		// The arguments end with a zero byte each.
		args := strings.Split(strings.TrimRight(string(line), "\x00"), "\x00")
		// 'finux config mcp config <agent>' only shows text and ends.
		if len(args) < 3 || filepath.Base(args[0]) != "finux" || args[1] != "config" || args[2] != "mcp" || slices.Contains(args[3:], "config") {
			continue
		}
		if slices.Contains(args[3:], "--http") || slices.ContainsFunc(args[3:], func(arg string) bool { return strings.HasPrefix(arg, "--http=") || strings.HasPrefix(arg, "-http") }) {
			http = append(http, id)
		} else {
			stdio = append(stdio, id)
		}
	}
	slices.Sort(stdio)
	slices.Sort(http)
	return stdio, http
}
