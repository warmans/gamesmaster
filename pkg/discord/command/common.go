package command

import (
	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
	"regexp"
	"strings"
)

var posterGuessRegex = regexp.MustCompile(`[Gg]uess\s([0-9]+)\s(.+)`)
var posterClueRegex = regexp.MustCompile(`[Cc]lue\s([0-9]+)`)
var adminRegex = regexp.MustCompile(`[Aa]dmin\s(.+)`)
