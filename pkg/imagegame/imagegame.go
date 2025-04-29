package imagegame

import (
	"fmt"
	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"github.com/warmans/gamesmaster/pkg/scores"
	"golang.org/x/image/font/gofont/goregular"
	"image/color"
	"log"
	"math"
	"path"
	"time"
)

var font *truetype.Font

func init() {
	var err error
	font, err = truetype.Parse(goregular.TTF)
	if err != nil {
		log.Fatal(err)
	}
}

type Config struct {
	ImagesWidth             int64
	ImagesHeight            int64
	RequireAlternatingUsers bool
}

type State struct {
	GameTitle              string
	Cfg                    *Config
	GuildID                string
	OriginalMessageID      string
	OriginalMessageChannel string
	AnswerThreadID         string
	Posters                []*Image
	Scores                 *scores.Tiered
	StartedAt              time.Time
}

func (s *State) NumUnsolved() int {
	numUnsolved := len(s.Posters)
	for _, v := range s.Posters {
		if v.Guessed {
			numUnsolved--
		}
	}
	return numUnsolved
}

type Image struct {
	Path    string
	Answer  string
	Guessed bool
}

func Render(imagesDir string, state *State) (*gg.Context, error) {
	var imagesPerRow = 8.0
	var imageWidth = int(state.Cfg.ImagesWidth)
	var imageHeight = int(state.Cfg.ImagesHeight)
	var numImages = len(state.Posters)
	var imagesPerColumn = math.Ceil(float64(numImages) / imagesPerRow)
	var boardWidth = int(math.Ceil(imagesPerRow * float64(imageWidth)))
	var boardHeight = int(math.Ceil(imagesPerColumn * float64(imageHeight)))

	dc := gg.NewContext(boardWidth, boardHeight)
	dc.SetColor(color.Black)
	dc.Clear()

	row := 0
	xPosition := 0
	for k, v := range state.Posters {
		var imagePath string
		labelBackground := color.RGBA{R: 0, G: 0, B: 0, A: 255}
		labelForeground := color.RGBA{R: 255, G: 255, B: 255, A: 255}

		imagePath = path.Join(imagesDir, v.Path)
		if v.Guessed {
			labelBackground = color.RGBA{R: 0, G: 255, B: 0, A: 255}
			labelForeground = color.RGBA{R: 0, G: 0, B: 0, A: 255}
		}

		im, err := gg.LoadImage(imagePath)
		if err != nil {
			return nil, err
		}
		dc.DrawImage(im, xPosition*imageWidth, row*imageHeight)
		dc.SetColor(labelBackground)
		dc.DrawRectangle(float64(xPosition*imageWidth), float64(row*imageHeight), 50, 45)
		dc.Fill()

		// center single-digit numbers
		imageNumberXOffset := 0.0
		if k+1 < 10 {
			imageNumberXOffset = 8.0
		}

		dc.SetColor(labelForeground)
		dc.SetFontFace(truetype.NewFace(font, &truetype.Options{Size: 30}))
		dc.DrawString(fmt.Sprintf("%d", k+1), float64(xPosition*imageWidth)+8+imageNumberXOffset, float64(row*imageHeight)+32)

		dc.Stroke()

		if (xPosition+1)*imageWidth >= boardWidth {
			row++
			xPosition = 0
		} else {
			xPosition++
		}
	}

	return dc, nil
}
