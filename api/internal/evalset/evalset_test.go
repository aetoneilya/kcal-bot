package evalset

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aetoneilya/kcal-bot/api/internal/estimate"
)

func TestRunScoresTotalsAgainstTheScale(t *testing.T) {
	dir := t.TempDir()
	write := func(name, truth string, photo bool) {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(path, "truth.json"), []byte(truth), 0o644)
		if photo {
			_ = os.WriteFile(filepath.Join(path, "photo.jpg"), []byte{0xFF, 0xD8, 0xFF, 0xE0}, 0o644)
		}
	}
	// The fake plate is 198 + 277.5 + 12 + 89.9 ≈ 577 kcal.
	write("a-under", `{"kcal": 500}`, true)
	write("b-over", `{"kcal": 680, "note": "без масла"}`, false)

	cases, err := Load(dir)
	if err != nil || len(cases) != 2 || len(cases[0].Photos) != 1 {
		t.Fatalf("load: %+v %v", cases, err)
	}
	var out bytes.Buffer
	if err := Run(context.Background(), estimate.Fake{Cost: 0.01}, cases, &out); err != nil {
		t.Fatal(err)
	}
	report := out.String()
	// a: 577 vs 500 → +15%; b (no oil, 488) vs 680 → −28%. Mean abs 22%, bias −6%.
	for _, want := range []string{"+15%", "-28%", "mean abs error 22%", "bias -6%", "spent $0.020"} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}

	write("c-broken", `{"kcal": 0}`, true)
	if _, err := Load(dir); err == nil {
		t.Fatal("a case without a total must be rejected")
	}
}
