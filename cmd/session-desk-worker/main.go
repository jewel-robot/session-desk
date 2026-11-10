package main

import (
	"fmt"
	"os"
	"sessiondesk/internal/sessions"
)

func main() {
	if err := sessions.RunWorker(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
