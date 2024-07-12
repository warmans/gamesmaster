package crossword

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/warmans/gamesmaster/pkg/crossword"
	"github.com/warmans/gamesmaster/pkg/flag"
	"os"
	"path"

	"log/slog"
)

const GridSize = 25

func NewInitCommand(logger *slog.Logger) *cobra.Command {

	var gameStateDir string
	var wordListPath string
	var preview bool

	cmd := &cobra.Command{
		Use:   "crossword-init",
		Short: "initialise a new crossword",
		RunE: func(cmd *cobra.Command, args []string) error {

			f, err := os.Open(wordListPath)
			if err != nil {
				return err
			}
			defer f.Close()

			words := []crossword.Word{}
			if err := json.NewDecoder(f).Decode(&words); err != nil {
				return err
			}

			for _, v := range words {
				if len(v.Word) > GridSize {
					return fmt.Errorf("word list contained workd longer than grid size (%d): %s", GridSize, v.Word)
				}
			}

			cw := crossword.NewGenerator(GridSize, GridSize, words).Generate(5, 4)
			canvas, err := cw.Render(1200, 1200)
			if err != nil {
				return err
			}

			gameState, err := os.Create(path.Join(gameStateDir, path.Base(wordListPath)))
			if err != nil {
				return err
			}
			defer gameState.Close()

			enc := json.NewEncoder(gameState)
			enc.SetIndent("", "    ")

			if preview {
				if err := canvas.SavePNG("preview.png"); err != nil {
					return err
				}
			}
			fmt.Print(cw.String())
			fmt.Printf("\n Input words: %d\n Placed Words: %d", len(words), len(cw.WordList))

			return enc.Encode(cw)
		},
	}

	flag.StringVarEnv(cmd.Flags(), &gameStateDir, "", "output-dir", "./var/crossword/game", "")
	flag.StringVarEnv(cmd.Flags(), &wordListPath, "", "word-list", "./var/crossword/wordlist/current.json", "")
	flag.BoolVarEnv(cmd.Flags(), &preview, "", "preview", true, "dump an image of the complete crossword")

	flag.Parse()

	return cmd
}
