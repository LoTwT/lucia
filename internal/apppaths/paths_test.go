package apppaths

import (
	"path/filepath"
	"testing"
)

func TestResolveDarwin(t *testing.T) {
	got, err := resolve("darwin", "Lucia", "/Users/ada", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join("/Users/ada", "Library")
	want := Paths{
		Data:  filepath.Join(base, "Application Support", "Lucia"),
		Cache: filepath.Join(base, "Caches", "Lucia"),
		Logs:  filepath.Join(base, "Logs", "Lucia"),
	}
	if got != want {
		t.Errorf("paths %+v, want %+v", got, want)
	}
}

func TestResolveWindowsUsesLocalAppData(t *testing.T) {
	got, err := resolve("windows", "Lucia Dev", "", "", `C:\Users\ada\AppData\Local`, `C:\Users\ada\AppData\Local`)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(`C:\Users\ada\AppData\Local`, "Lucia Dev")
	want := Paths{
		Data:  root,
		Cache: filepath.Join(root, "cache"),
		Logs:  filepath.Join(root, "logs"),
	}
	if got != want {
		t.Errorf("paths %+v, want %+v", got, want)
	}
}

func TestResolveWindowsFallsBackToCacheDir(t *testing.T) {
	got, err := resolve("windows", "Lucia", "", "", `C:\Users\ada\AppData\Local`, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(`C:\Users\ada\AppData\Local`, "Lucia"); got.Data != want {
		t.Errorf("data %q, want %q", got.Data, want)
	}
}

func TestResolveLinux(t *testing.T) {
	got, err := resolve("linux", "Lucia", "/home/ada", "/home/ada/.config", "/home/ada/.cache", "")
	if err != nil {
		t.Fatal(err)
	}
	want := Paths{
		Data:  filepath.Join("/home/ada/.config", "Lucia"),
		Cache: filepath.Join("/home/ada/.cache", "Lucia"),
		Logs:  filepath.Join("/home/ada/.config", "Lucia", "logs"),
	}
	if got != want {
		t.Errorf("paths %+v, want %+v", got, want)
	}
}

func TestResolveRejectsEmptyName(t *testing.T) {
	if _, err := resolve("darwin", "", "/Users/ada", "", "", ""); err == nil {
		t.Fatal("resolve accepted an empty application name")
	}
}
