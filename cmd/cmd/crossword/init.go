package crossword

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/warmans/gamesmaster/pkg/flag"
	"github.com/warmans/go-crossword"
	"log/slog"
	"os"
	"path"
)

const GridSize = 30

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

			cw := crossword.Generate(GridSize, words, 50, crossword.WithRevealFirstLetterOfEachWord(true), crossword.WithAllAttempts(true))
			canvas, err := crossword.RenderPNG(cw, 1200, 1200, crossword.WithAllSolved(false))
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
			fmt.Print(crossword.RenderText(cw, crossword.WithAllSolved(false)))
			fmt.Printf("\nInput words: %d\nPlaced Words: %d\n", len(words), len(cw.Words))

			if len(words) > len(cw.Words) {
				fmt.Println("\nUnplaced:")
				for _, v := range words {
					found := false
					for _, vv := range cw.Words {
						if v.Word == vv.Word.Word {
							found = true
							break
						}
					}
					if !found {
						fmt.Println("- " + v.Word)
					}
				}
			}

			return enc.Encode(cw)
		},
	}

	flag.StringVarEnv(cmd.Flags(), &gameStateDir, "", "output-dir", "./var/crossword/game", "")
	flag.StringVarEnv(cmd.Flags(), &wordListPath, "", "word-list", "./var/crossword/wordlist/current.json", "")
	flag.BoolVarEnv(cmd.Flags(), &preview, "", "preview", true, "dump an image of the complete crossword")

	flag.Parse()

	return cmd
}
