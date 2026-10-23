// Package treats keeps the little extras: short dancing-cat videos anyone
// can summon, and compliments for chosen people. Both live in a directory on
// the server, outside git:
//
//	treats/
//	  compliments.txt   one per line; blank lines and # comments are skipped
//	  cats/*.mp4        short videos (mp4, webm or mov)
//
// The directory is read on every call, so adding a cat or a line needs no
// restart.
package treats

import (
	"bufio"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/aetoneilya/kcal-bot/api/internal/model"
)

type Treats struct {
	Dir   string
	ids   map[int64]bool
	names map[string]bool // lower case, without @
}

// New takes who gets compliments as a comma-separated list of Telegram ids
// or usernames ("@name" or "name"). It returns nil, which turns everything
// off, when there is no directory; with no people the cats still dance.
func New(dir, people string) *Treats {
	t := &Treats{Dir: dir, ids: map[int64]bool{}, names: map[string]bool{}}
	for _, field := range strings.Split(people, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if id, err := strconv.ParseInt(field, 10, 64); err == nil {
			t.ids[id] = true
		} else {
			t.names[strings.ToLower(strings.TrimPrefix(field, "@"))] = true
		}
	}
	if dir == "" {
		return nil
	}
	return t
}

// For reports whether user gets treats. Safe on a nil Treats.
func (t *Treats) For(user model.User) bool {
	if t == nil {
		return false
	}
	return t.ids[user.ID] || (user.Username != "" && t.names[strings.ToLower(user.Username)])
}

// Compliments lists the lines of compliments.txt; none when it is missing.
func (t *Treats) Compliments() ([]string, error) {
	file, err := os.Open(filepath.Join(t.Dir, "compliments.txt"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

// Compliment picks one at random; "" when there are none.
func (t *Treats) Compliment() string {
	lines, err := t.Compliments()
	if err != nil || len(lines) == 0 {
		return ""
	}
	return lines[rand.IntN(len(lines))]
}

var videoTypes = map[string]string{".mp4": "video/mp4", ".webm": "video/webm", ".mov": "video/quicktime"}

// Cats lists the video file names in cats/, sorted; none when it is missing
// or on a nil Treats.
func (t *Treats) Cats() ([]string, error) {
	if t == nil {
		return nil, nil
	}
	entries, err := os.ReadDir(filepath.Join(t.Dir, "cats"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if _, ok := videoTypes[strings.ToLower(filepath.Ext(entry.Name()))]; ok && entry.Type().IsRegular() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// Cat returns the path and media type of one listed video. A name that is
// not in the listing — a path, a dot file — is os.ErrNotExist.
func (t *Treats) Cat(name string) (path, mediaType string, err error) {
	if t == nil {
		return "", "", os.ErrNotExist
	}
	names, err := t.Cats()
	if err != nil {
		return "", "", err
	}
	for _, n := range names {
		if n == name {
			return filepath.Join(t.Dir, "cats", n), videoTypes[strings.ToLower(filepath.Ext(n))], nil
		}
	}
	return "", "", os.ErrNotExist
}
