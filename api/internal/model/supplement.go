package model

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Supplement is something a person takes every day. Taken is whether they
// marked it on the day asked about.
type Supplement struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Dose  string `json:"dose"` // free text, may be empty
	Taken bool   `json:"taken"`
}

const (
	MaxSupplements     = 30
	MaxSupplementRunes = 60
	MaxDoseRunes       = 40
)

// Normalize trims the text the way the form shows it.
func (s Supplement) Normalize() Supplement {
	s.Name = strings.TrimSpace(s.Name)
	s.Dose = strings.TrimSpace(s.Dose)
	return s
}

func ValidateSupplement(s Supplement) error {
	if s.Name == "" || utf8.RuneCountInString(s.Name) > MaxSupplementRunes {
		return invalid("name must be 1–%d characters", MaxSupplementRunes)
	}
	if utf8.RuneCountInString(s.Dose) > MaxDoseRunes {
		return invalid("dose must be at most %d characters", MaxDoseRunes)
	}
	if strings.ContainsAny(s.Name+s.Dose, "\r\n") {
		return invalid("name and dose must be one line")
	}
	return nil
}

// ValidateDay checks a local calendar day, YYYY-MM-DD.
func ValidateDay(day string) error {
	if _, err := time.Parse(time.DateOnly, day); err != nil {
		return invalid("day must be YYYY-MM-DD")
	}
	return nil
}
