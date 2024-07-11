package crossword

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/warmans/gamesmaster/pkg/crossword"
	"github.com/warmans/gamesmaster/pkg/flag"
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

			cw := crossword.Crossword{}
			if err := json.NewDecoder(f).Decode(&cw); err != nil {
				return err
			}

			canvas, err := cw.Render(1024, 1024)
			if err != nil {
				return err
			}

			if err := canvas.SavePNG("preview.png"); err != nil {
				return err
			}
			fmt.Print(cw.String())
			return nil
		},
	}

	flag.StringVarEnv(cmd.Flags(), &gameStatePath, "", "game-state", "./var/crossword/game/current.json", "")

	flag.Parse()

	return cmd
}
