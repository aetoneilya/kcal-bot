package treats

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aetoneilya/kcal-bot/api/internal/model"
)

func TestForMatchesIDsAndUsernames(t *testing.T) {
	tr := New(t.TempDir(), " @Sunny, 42 ")
	cases := []struct {
		user model.User
		want bool
	}{
		{model.User{ID: 1, Username: "sunny"}, true},
		{model.User{ID: 1, Username: "SUNNY"}, true},
		{model.User{ID: 42}, true},
		{model.User{ID: 2, Username: "other"}, false},
		{model.User{ID: 3}, false},
	}
	for _, c := range cases {
		if got := tr.For(c.user); got != c.want {
			t.Errorf("For(%+v) = %v, want %v", c.user, got, c.want)
		}
	}
}

func TestNewIsOffWithoutPeopleOrDir(t *testing.T) {
	if New(t.TempDir(), " , ") != nil || New("", "sunny") != nil {
		t.Fatal("expected nil")
	}
	var tr *Treats
	if tr.For(model.User{ID: 1, Username: "sunny"}) {
		t.Fatal("nil Treats gave a treat")
	}
}

func TestComplimentsAndCats(t *testing.T) {
	dir := t.TempDir()
	tr := New(dir, "sunny")
	if lines, err := tr.Compliments(); err != nil || lines != nil {
		t.Fatalf("missing file: %v %v", lines, err)
	}
	if tr.Compliment() != "" {
		t.Fatal("compliment from nothing")
	}
	write(t, filepath.Join(dir, "compliments.txt"), "# mine\nТы лучшая\n\n  Солнце  \n")
	lines, err := tr.Compliments()
	if err != nil || len(lines) != 2 || lines[0] != "Ты лучшая" || lines[1] != "Солнце" {
		t.Fatalf("got %q %v", lines, err)
	}

	write(t, filepath.Join(dir, "cats", "b.mp4"), "x")
	write(t, filepath.Join(dir, "cats", "a.WEBM"), "x")
	write(t, filepath.Join(dir, "cats", "notes.txt"), "x")
	cats, err := tr.Cats()
	if err != nil || len(cats) != 2 || cats[0] != "a.WEBM" || cats[1] != "b.mp4" {
		t.Fatalf("got %q %v", cats, err)
	}
	path, mediaType, err := tr.Cat("b.mp4")
	if err != nil || path != filepath.Join(dir, "cats", "b.mp4") || mediaType != "video/mp4" {
		t.Fatalf("got %q %q %v", path, mediaType, err)
	}
	for _, name := range []string{"notes.txt", "../compliments.txt", "missing.mp4"} {
		if _, _, err := tr.Cat(name); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Cat(%q) err = %v", name, err)
		}
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
