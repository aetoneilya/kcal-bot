// Package evalset measures how close an estimator gets to weighed meals.
//
// A set is a directory with one sub-directory per meal:
//
//	evals/
//	  2026-10-21-buckwheat/
//	    photo.jpg          one to three photos (jpg, png or webp)
//	    truth.json         what the scale said
//
// truth.json:
//
//	{"note": "", "kcal": 612, "items": [{"name": "гречка", "grams": 180}, ...]}
//
// kcal is the meal's total worked out from the weighed grams and the labels
// or a nutrition table; items are optional and only reported side by side.
// The score is the error of the total, the number the diary lives by.
package evalset

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aetoneilya/kcal-bot/api/internal/estimate"
)

type Truth struct {
	Note  string  `json:"note"`
	Kcal  float64 `json:"kcal"`
	Items []struct {
		Name  string  `json:"name"`
		Grams float64 `json:"grams"`
	} `json:"items"`
}

type Case struct {
	Name   string
	Photos []estimate.Photo
	Truth  Truth
}

// Load reads every case under dir; a sub-directory without truth.json or
// photos is an error, so a half-prepared case is not silently skipped.
func Load(dir string) ([]Case, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var cases []Case
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		c := Case{Name: entry.Name()}
		raw, err := os.ReadFile(filepath.Join(path, "truth.json"))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name(), err)
		}
		if err := json.Unmarshal(raw, &c.Truth); err != nil {
			return nil, fmt.Errorf("%s/truth.json: %w", entry.Name(), err)
		}
		if c.Truth.Kcal <= 0 {
			return nil, fmt.Errorf("%s/truth.json: kcal must be positive", entry.Name())
		}
		files, _ := os.ReadDir(path)
		for _, f := range files {
			ext := strings.ToLower(filepath.Ext(f.Name()))
			if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(path, f.Name()))
			if err != nil {
				return nil, err
			}
			c.Photos = append(c.Photos, estimate.Photo{Data: data, MIME: http.DetectContentType(data)})
		}
		if len(c.Photos) == 0 && strings.TrimSpace(c.Truth.Note) == "" {
			return nil, fmt.Errorf("%s: no photos and no note", entry.Name())
		}
		if len(c.Photos) > estimate.MaxPhotos {
			return nil, fmt.Errorf("%s: at most %d photos", entry.Name(), estimate.MaxPhotos)
		}
		cases = append(cases, c)
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].Name < cases[j].Name })
	if len(cases) == 0 {
		return nil, fmt.Errorf("%s: no cases", dir)
	}
	return cases, nil
}

// Run estimates every case once and writes a report to w: one line per case,
// then the mean absolute error, the bias (positive = overestimates) and the
// money spent. Cases run one at a time to stay far from rate limits.
func Run(ctx context.Context, est estimate.Estimator, cases []Case, w io.Writer) error {
	var absSum, signedSum, cost float64
	var done int
	fmt.Fprintf(w, "%-32s %8s %8s %8s  %s\n", "case", "truth", "model", "error", "items (model)")
	for _, c := range cases {
		started := time.Now()
		result, err := est.Estimate(ctx, estimate.Input{Photos: c.Photos, Note: c.Truth.Note})
		cost += result.Cost
		if err != nil {
			fmt.Fprintf(w, "%-32s %8.0f %8s %8s  %v\n", c.Name, c.Truth.Kcal, "—", "—", err)
			continue
		}
		var kcal float64
		var names []string
		for _, item := range result.Draft.Items {
			kcal += item.Kcal()
			names = append(names, fmt.Sprintf("%s %.0fг", item.Name, item.Grams))
		}
		rel := (kcal - c.Truth.Kcal) / c.Truth.Kcal
		absSum += math.Abs(rel)
		signedSum += rel
		done++
		fmt.Fprintf(w, "%-32s %8.0f %8.0f %+7.0f%%  %s (%.1fs)\n",
			c.Name, c.Truth.Kcal, kcal, rel*100, strings.Join(names, ", "), time.Since(started).Seconds())
	}
	if done == 0 {
		return fmt.Errorf("every case failed")
	}
	fmt.Fprintf(w, "\n%s on %d of %d cases: mean abs error %.0f%%, bias %+.0f%%, spent $%.3f\n",
		est.Name(), done, len(cases), absSum/float64(done)*100, signedSum/float64(done)*100, cost)
	return nil
}
