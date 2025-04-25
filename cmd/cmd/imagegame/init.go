package imagegame

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/warmans/gamesmaster/pkg/flag"
	"github.com/warmans/gamesmaster/pkg/imagegame"
	"github.com/warmans/gamesmaster/pkg/scores"
	"log/slog"
	"math/rand"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
)

var dateInParens = regexp.MustCompile(`(\s+)?\([0-9]+\)(\s+)?`)
var spaces = regexp.MustCompile(`\s+`)

func NewInitCommand(logger *slog.Logger) *cobra.Command {

	var gameStateDir string
	var imagesDir string
	var gameName string
	var imageWidth int64
	var imageHeight int64
	var preview bool

	cmd := &cobra.Command{
		Use:   "imagegame-init",
		Short: "initialise a new imagegame",
		RunE: func(cmd *cobra.Command, args []string) error {
			if gameName == "" {
				return fmt.Errorf("-name is required")
			}

			if err := renameImages(imagesDir); err != nil {
				return fmt.Errorf("failed to rename images: %w", err)
			}

			state, err := createStateFromImages(imagesDir, gameName)
			if err != nil {
				return err
			}
			state.Cfg = &imagegame.Config{
				ImagesWidth:  imageWidth,
				ImagesHeight: imageHeight,
			}

			fmt.Println("Rendering...")
			canvas, err := imagegame.Render(imagesDir, state)
			if err != nil {
				return err
			}
			if preview {
				if err := canvas.SavePNG("./imagegame.png"); err != nil {
					return err
				}
			}

			currentState, err := os.Create(path.Join(gameStateDir, "current.json"))
			if err != nil {
				return err
			}
			defer currentState.Close()

			enc := json.NewEncoder(currentState)
			enc.SetIndent("", "  ")

			return enc.Encode(state)
		},
	}

	flag.StringVarEnv(cmd.Flags(), &gameStateDir, "", "output-dir", "./var/imagegame/game", "")
	flag.StringVarEnv(cmd.Flags(), &imagesDir, "", "images-dir", "./var/imagegame/game/images", "")
	flag.BoolVarEnv(cmd.Flags(), &preview, "", "preview", true, "dump an image of the complete crossword")
	flag.StringVarEnv(cmd.Flags(), &gameName, "", "name", "", "name to give the game")
	flag.Int64VarEnv(cmd.Flags(), &imageWidth, "", "image-width", 200, "image width")
	flag.Int64VarEnv(cmd.Flags(), &imageHeight, "", "image-height", 300, "image height")

	flag.Parse()

	return cmd
}

func createStateFromImages(imagesDir string, gameTitle string) (*imagegame.State, error) {
	files, err := os.ReadDir(imagesDir)
	if err != nil {
		return nil, err
	}
	state := &imagegame.State{Posters: make([]*imagegame.Image, 0), GameTitle: gameTitle}
	for _, fd := range files {
		if fd.IsDir() || strings.Contains(fd.Name(), ".blur.") {
			continue
		}
		state.Posters = append(state.Posters, &imagegame.Image{
			Path:    fd.Name(),
			Answer:  filmNameFromFilename(fd.Name()),
			Guessed: false,
		})
	}

	state.Scores = scores.NewTiered(len(state.Posters))

	slices.SortFunc(state.Posters, func(a, b *imagegame.Image) int {
		if rand.Float64() < rand.Float64() {
			return 1
		}
		return -1
	})

	return state, nil
}

func renameImages(imagesDir string) error {
	files, err := os.ReadDir(imagesDir)
	if err != nil {
		return err
	}
	for _, fd := range files {
		if fd.IsDir() {
			continue
		}
		oldPath := path.Join(imagesDir, fd.Name())
		newPath := path.Join(imagesDir, fixFileName(fd.Name()))
		fmt.Printf("Moving %s to %s...\n", oldPath, newPath)
		if err := os.Rename(oldPath, newPath); err != nil {
			return fmt.Errorf("failed ot rename %s: %w", fd.Name(), err)
		}
	}
	return nil
}

func fixFileName(oldName string) string {
	return strings.ToLower(
		spaces.ReplaceAllString(
			spaces.ReplaceAllString(dateInParens.ReplaceAllString(oldName, ""), " "),
			"-",
		),
	)
}

func filmNameFromFilename(fileName string) string {
	return strings.ReplaceAll(
		strings.TrimSuffix(
			strings.TrimSuffix(fileName, path.Ext(fileName)),
			".blur",
		),
		"-",
		" ",
	)
}
