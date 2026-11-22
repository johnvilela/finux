package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"finux/src/ui"
)

const (
	// memoryProgram is the program of ai-memory: the memory that the AI
	// agents share between their sessions.
	memoryProgram = "ai-memory"
	// memoryScript is the install script that the ai-memory project
	// publishes. It checks the release against its checksum.
	memoryScript = "https://github.com/akitaonrails/ai-memory/releases/latest/download/install.sh"
	// memorySourceVariable names the environment variable that replaces
	// the address of the install script, for tests.
	memorySourceVariable = "FINUX_AI_MEMORY_SOURCE"
	// memoryProject is the content of the file that makes all the /ai
	// sessions one project of ai-memory.
	memoryProject = "workspace = \"default\"\nproject = \"finux-finances\"\n"
)

// memoryAgents has the name that ai-memory gives to each agent.
var memoryAgents = map[string]string{"claude": "claude-code", "codex": "codex", "opencode": "open-code"}

// aiFolder returns the folder where the agent of /ai runs. It makes
// the folder, and in it the file that names the project of ai-memory,
// when they do not exist.
func aiFolder() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	folder := filepath.Join(home, ".finux", "ai")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		return "", err
	}
	project := filepath.Join(folder, ".ai-memory.toml")
	if _, err := os.Stat(project); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(project, []byte(memoryProject), 0o644); err != nil {
			return "", err
		}
	}
	return folder, nil
}

// findMemory returns the path of the ai-memory program, or "". Its
// install script puts it in ~/.local/bin, which is not always in PATH.
func findMemory() string {
	if found, err := exec.LookPath(memoryProgram); err == nil {
		return found
	}
	if home, err := os.UserHomeDir(); err == nil {
		local := filepath.Join(home, ".local", "bin", memoryProgram)
		if info, err := os.Stat(local); err == nil && !info.IsDir() {
			return local
		}
	}
	return ""
}

// installMemory runs the install script of the ai-memory project.
func installMemory(io streams) error {
	source := os.Getenv(memorySourceVariable)
	if source == "" {
		source = memoryScript
	}
	script, err := fetch(source)
	if err != nil {
		return fmt.Errorf("get the install script of ai-memory: %w", err)
	}
	command := exec.Command("sh", "-s")
	command.Stdin, command.Stdout, command.Stderr = bytes.NewReader(script), io.stdout, io.stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("the install script of ai-memory failed: %w", err)
	}
	return nil
}

// setupMemory connects ai-memory to an agent, so the AI keeps what it
// learns and what the user decides between sessions. It installs the
// program when the user agrees. It returns the lines that tell what
// it did.
func setupMemory(agent aiAgent, options aiOptions, io streams) (string, error) {
	if options.noMemory {
		return "", nil
	}
	binary := findMemory()
	if binary == "" {
		install := options.yes
		if !install && io.interactive {
			confirm, err := program(ui.NewConfirm("ai-memory is not installed. Install it now, with the install script of the ai-memory project?", "Install ai-memory", false))
			if err != nil {
				return "", err
			}
			install = confirm.Confirmed()
		}
		if !install {
			return "ai-memory is not installed, so the AI keeps no memory between sessions.\n" +
				"To install it, run 'finux config ai " + agent.key + " --yes'. That runs the install script of the ai-memory project:\n" +
				"  curl -fsSL " + memoryScript + " | sh\n", nil
		}
		if err := installMemory(io); err != nil {
			return "", err
		}
		if binary = findMemory(); binary == "" {
			return "", errors.New("the install script ran, but the ai-memory program was not found: add ~/.local/bin to your PATH, or use --no-memory")
		}
	}
	// A second setup changes nothing, so no check comes before it.
	output, err := exec.Command(binary, "setup", "--yes", "--agent", memoryAgents[agent.key]).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("'ai-memory setup' failed: %s\nTo set up %s without memory, add --no-memory", strings.TrimSpace(string(output)+" "+err.Error()), agent.name)
	}
	return ui.Success("ai-memory connected to "+agent.name) + "\n", nil
}
