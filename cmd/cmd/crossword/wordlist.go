package crossword

import (
	"encoding/json"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/spf13/cobra"
	"github.com/warmans/gamesmaster/pkg/flag"
	"github.com/warmans/go-crossword"
	"os"
	"strings"

	"log/slog"
)

func NewRandomWordListCommand(logger *slog.Logger) *cobra.Command {

	var wordListPath string
	var wordListLength int64

	cmd := &cobra.Command{
		Use:   "crossword-random-words",
		Short: "generate a new sample wordlist",
		RunE: func(cmd *cobra.Command, args []string) error {

			f, err := os.Create(wordListPath)
			if err != nil {
				return err
			}
			defer f.Close()

			words := []crossword.Word{}
			for i := int64(0); i < wordListLength; i++ {
				words = append(
					words,
					crossword.Word{
						Word: strings.Join(strings.Split(gofakeit.HipsterWord(), " "), ""),
						Clue: gofakeit.ProductDescription(),
					},
				)
			}

			enc := json.NewEncoder(f)
			enc.SetIndent("", "    ")
			return enc.Encode(&words)
		},
	}

	flag.StringVarEnv(cmd.Flags(), &wordListPath, "", "word-list", "./var/crossword/wordlist/current.json", "")
	flag.Int64VarEnv(cmd.Flags(), &wordListLength, "", "length", 20, "")

	flag.Parse()

	return cmd
}
