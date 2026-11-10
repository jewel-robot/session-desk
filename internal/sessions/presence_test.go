package sessions

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAgyPresenceLockHelper(t *testing.T) {
	path := os.Getenv("SESSION_DESK_TEST_LOCK")
	if path == "" {
		return
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := lockFile(file); err != nil {
		t.Fatal(err)
	}
	fmt.Println("locked")
	io.Copy(io.Discard, os.Stdin)
}

func TestAgyPresenceLockBlocksMutations(t *testing.T) {
	s := demoService(t)
	source := s.fixed[0]
	path := filepath.Join(source.AgyDir, "presence", "test.lock")
	writeFixture(t, path, "lock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAgyPresenceLockHelper$")
	configureCommand(command)
	command.Env = append(os.Environ(), "SESSION_DESK_TEST_LOCK="+path)
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { input.Close(); command.Wait() }()
	scanner := bufio.NewScanner(output)
	if !scanner.Scan() || scanner.Text() != "locked" {
		t.Fatal("presence helper did not acquire lock")
	}
	var key string
	for _, session := range s.View().Sessions {
		if session.SourceID == source.ID && session.Provider == "agy" {
			key = session.Key
			break
		}
	}
	plan, err := s.PrepareDelete(context.Background(), []string{key})
	if err != nil {
		t.Fatal(err)
	}
	result := s.Delete(context.Background(), plan.Keys, plan.Revisions)
	if len(result.Done) != 0 || len(result.Failed) == 0 {
		t.Fatalf("running Antigravity deleted: %+v", result)
	}
	entries, err := s.Trash(context.Background())
	if err != nil || len(entries) != 0 {
		t.Fatalf("mutation occurred: %+v %v", entries, err)
	}
}
