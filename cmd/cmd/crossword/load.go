package crossword

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/warmans/gamesmaster/pkg/flag"
	"github.com/warmans/go-crossword"
	"log/slog"
	"os"
)

func NewLoadCommand(logger *slog.Logger) *cobra.Command {

	var gameStatePath string

	cmd := &cobra.Command{
		Use:   "crossword-load",
		Short: "initialise a new crossword",
		RunE: func(cmd *cobra.Command, args []string) error {

			f, err := os.Open(gameStatePath)
			if err != nil {
				return err
			}
			defer f.Close()

			cw := &crossword.Crossword{}
			if err := json.NewDecoder(f).Decode(cw); err != nil {
				return err
			}

			canvas, err := crossword.RenderPNG(cw, 1024, 1024, crossword.WithAllSolved(true))
			if err != nil {
				return err
			}

			if err := canvas.SavePNG("preview.png"); err != nil {
				return err
			}
			fmt.Print(crossword.RenderText(cw, crossword.WithAllSolved(true)))
			return nil
		},
	}

	flag.StringVarEnv(cmd.Flags(), &gameStatePath, "", "game-state", "./var/crossword/game/current.json", "")

	flag.Parse()

	return cmd
}
