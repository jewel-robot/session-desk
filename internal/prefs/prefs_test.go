package prefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsAndRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "preferences.json")
	store := Open(path)
	if !store.Get().AutoUpdateCheck() {
		t.Fatal("update checks should default to on")
	}
	off := false
	if err := store.Update(func(p *Preferences) {
		p.UpdateCheck = &off
		p.Window = Window{Width: 1300, Height: 900, Maximised: true}
	}); err != nil {
		t.Fatal(err)
	}
	reopened := Open(path).Get()
	if reopened.AutoUpdateCheck() || reopened.Window.Width != 1300 || !reopened.Window.Maximised {
		t.Fatalf("unexpected preferences %+v", reopened)
	}
}

func TestDamagedFileFallsBackToDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	os.WriteFile(path, []byte("{broken"), 0o644)
	if !Open(path).Get().AutoUpdateCheck() {
		t.Fatal("damaged preferences should fall back to defaults")
	}
}
