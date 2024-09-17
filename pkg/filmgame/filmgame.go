package filmgame

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

type State struct {
	GameTitle              string
	OriginalMessageID      string
	OriginalMessageChannel string
	AnswerThreadID         string
	Posters                []*Poster
	Scores                 *scores.Tiered
	StartedAt              time.Time
}

type Poster struct {
	OriginalImage string
	ObscuredImage string
	Answer        string
	Guessed       bool
}

func Render(imagesDir string, posters []*Poster) (*gg.Context, error) {
	var imageWidth = 200
	var imageHeight = 300
	var numImages = len(posters)
	var imagesPerRow = int(math.Ceil(float64(numImages) / 5))
	var boardWidth = imagesPerRow * imageWidth
	var boardHeight = (numImages / imagesPerRow) * imageHeight

	dc := gg.NewContext(boardWidth, boardHeight)
	dc.SetColor(color.Black)
	dc.Clear()

	row := 0
	xPosition := 0
	for k, v := range posters {
		var imagePath string
		labelBackground := color.RGBA{R: 0, G: 0, B: 0, A: 255}
		labelForeground := color.RGBA{R: 255, G: 255, B: 255, A: 255}
		if v.Guessed {
			imagePath = path.Join(imagesDir, v.OriginalImage)
			labelBackground = color.RGBA{R: 0, G: 255, B: 0, A: 255}
			labelForeground = color.RGBA{R: 0, G: 0, B: 0, A: 255}
		} else {
			imagePath = path.Join(imagesDir, v.ObscuredImage)
		}

		im, err := gg.LoadImage(imagePath)
		if err != nil {
			return nil, err
		}
		dc.DrawImage(im, xPosition*imageWidth, row*imageHeight)
		dc.SetColor(labelBackground)
		dc.DrawRectangle(float64(xPosition*imageWidth), float64(row*imageHeight), 35, 35)
		dc.Fill()

		dc.SetColor(labelForeground)
		dc.SetFontFace(truetype.NewFace(font, &truetype.Options{Size: 20}))
		dc.DrawString(fmt.Sprintf("%d", k+1), float64(xPosition*imageWidth)+10, float64(row*imageHeight)+25)

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
