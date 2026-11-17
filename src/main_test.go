package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFinuxPrintsHelloWorld(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "finux")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build finux: %v\n%s", err, output)
	}

	var stdout, stderr bytes.Buffer
	command := exec.Command(binary)
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("run finux: %v\nstderr: %s", err, stderr.String())
	}
	if got, want := stdout.String(), "Hello, world!\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("stderr = %q, want empty", got)
	}
}
