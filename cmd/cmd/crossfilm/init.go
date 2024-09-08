package crossfilm

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/warmans/gamesmaster/pkg/crossfilm"
	"github.com/warmans/gamesmaster/pkg/filmgame"
	"github.com/warmans/gamesmaster/pkg/flag"
	"github.com/warmans/gamesmaster/pkg/util"
	"github.com/warmans/go-crossword"
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
	var preview bool

	cmd := &cobra.Command{
		Use:   "crossfilm-init",
		Short: "initialise a new filmgame",
		RunE: func(cmd *cobra.Command, args []string) error {

			if err := renameImages(imagesDir); err != nil {
				return fmt.Errorf("failed to rename images: %w", err)
			}

			state, err := createStateFromImages(imagesDir)
			if err != nil {
				return err
			}

			fmt.Println("Rendering...")
			canvas, err := crossfilm.Render(imagesDir, *state)
			if err != nil {
				return err
			}
			if preview {
				if err := canvas.SavePNG("./crossfilm.png"); err != nil {
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

	flag.StringVarEnv(cmd.Flags(), &gameStateDir, "", "output-dir", "./var/crossfilm/game", "")
	flag.StringVarEnv(cmd.Flags(), &imagesDir, "", "images-dir", "./var/crossfilm/game/images", "")
	flag.BoolVarEnv(cmd.Flags(), &preview, "", "preview", true, "dump an image of the complete crossfilm")

	flag.Parse()

	return cmd
}

func createStateFromImages(imagesDir string) (*crossfilm.State, error) {
	files, err := os.ReadDir(imagesDir)
	if err != nil {
		return nil, err
	}
	state := &crossfilm.State{
		FilmgameState: make([]*filmgame.Poster, 0),
	}

	for _, fd := range files {
		if fd.IsDir() || strings.Contains(fd.Name(), ".blur.") {
			continue
		}
		name := filmNameFromFilename(fd.Name())
		state.FilmgameState = append(state.FilmgameState, &filmgame.Poster{
			OriginalImage: fd.Name(),
			ObscuredImage: obscuredImageName(fd.Name()),
			Answer:        name,
			Guessed:       false,
		})
	}
	slices.SortFunc(state.FilmgameState, func(a, b *filmgame.Poster) int {
		if rand.Float64() < rand.Float64() {
			return 1
		}
		return -1
	})

	// build the word list based on the final sorted poster list
	var words []crossword.Word
	for k, v := range state.FilmgameState {
		words = append(
			words,
			crossword.Word{
				Word:  v.Answer,
				Label: util.ToPtr(fmt.Sprintf("%d", k+1)),
			},
		)
	}

	state.CrosswordState = crossword.Generate(
		30,
		words,
		100,
		crossword.WithAllAttempts(true),
	)

	if len(state.CrosswordState.Words) != len(words) {
		return nil, fmt.Errorf("not all words were placed: expected %d got %d", len(words), len(state.CrosswordState.Words))
	}

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

func obscuredImageName(originalName string) string {
	extension := path.Ext(originalName)
	return fmt.Sprintf("%s.blur%s", strings.TrimSuffix(originalName, extension), extension)
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
