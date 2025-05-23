package util

import (
	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
	"regexp"
	"strings"
)

var whitespace = regexp.MustCompile(`\s+`)

func WithoutSpaces(guess string) string {
	return whitespace.ReplaceAllString(guess, "")
}

func GuessRoughlyMatchesAnswer(guess string, answer string) bool {
	return strutil.Similarity(SimplifyGuess(guess), SimplifyGuess(answer), metrics.NewHamming()) >= 0.8
}

func SimplifyGuess(guess string) string {
	return trimAllPrefix(strings.ToLower(guess), "a ", "the ")
}

func trimAllPrefix(str string, trim ...string) string {
	str = strings.TrimSpace(str)
	for _, v := range trim {
		str = strings.TrimSpace(strings.TrimPrefix(str, v))
	}
	return str
}
