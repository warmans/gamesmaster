package command

import (
	"regexp"
	"strings"
)

var posterGuessRegex = regexp.MustCompile(`[Gg]uess\s([0-9]+)\s(.+)`)
var posterClueRegex = regexp.MustCompile(`[Cc]lue\s([0-9]+)`)
var adminRegex = regexp.MustCompile(`[Aa]dmin\s(.+)`)

func simplifyGuess(guess string) string {
	return trimAllPrefix(guess, "a ", "the ")
}

func trimAllPrefix(str string, trim ...string) string {
	str = strings.TrimSpace(str)
	for _, v := range trim {
		str = strings.TrimSpace(strings.TrimPrefix(str, v))
	}
	return str
}
