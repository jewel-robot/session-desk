// Package applog sends log output to a size-capped file, because the Windows GUI build has no console.
package applog

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

const maxSize = 2 << 20

// Open redirects the standard logger to dir/session-desk.log, keeping one previous file.
// It returns the log path and a closer; on failure logging stays on stderr.
func Open(dir string) (string, io.Closer, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	path := filepath.Join(dir, "session-desk.log")
	if info, err := os.Stat(path); err == nil && info.Size() > maxSize {
		os.Remove(path + ".1")
		os.Rename(path, path+".1")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", nil, err
	}
	log.SetOutput(file)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	return path, file, nil
}
