package filmgame

import (
	"encoding/json"
	"github.com/spf13/cobra"
	"github.com/warmans/gamesmaster/pkg/filmgame"
	"github.com/warmans/gamesmaster/pkg/flag"
	"log/slog"
	"os"
	"path"
)

type State struct {
	Posters []*filmgame.Poster
}

func NewInitCommand(logger *slog.Logger) *cobra.Command {

	var gameStateDir string
	var imagesDir string
	var preview bool

	cmd := &cobra.Command{
		Use:   "filmgame-init",
		Short: "initialise a new filmgame",
		RunE: func(cmd *cobra.Command, args []string) error {

			current, err := os.Open(path.Join(gameStateDir, "current.json"))
			if err != nil {
				return err
			}

			state := State{}
			if err := json.NewDecoder(current).Decode(&state); err != nil {
				return err
			}

			canvas, err := filmgame.Render(imagesDir, state.Posters)
			if err != nil {
				return err
			}

			if preview {
				return canvas.SavePNG("./filmgame.png")
			}
			return nil
		},
	}

	flag.StringVarEnv(cmd.Flags(), &gameStateDir, "", "output-dir", "./var/filmgame/game", "")
	flag.StringVarEnv(cmd.Flags(), &imagesDir, "", "images-dir", "./var/filmgame/game/images", "")
	flag.BoolVarEnv(cmd.Flags(), &preview, "", "preview", true, "dump an image of the complete crossword")

	flag.Parse()

	return cmd
}
