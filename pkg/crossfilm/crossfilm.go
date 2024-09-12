package crossfilm

import (
	"bytes"
	"fmt"
	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"github.com/warmans/gamesmaster/pkg/filmgame"
	"github.com/warmans/go-crossword"
	"golang.org/x/image/font/gofont/goregular"
	"image"
	"image/color"
	"log"
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

type Score struct {
	Points  int
	Answers int
}

type State struct {
	GameTitle              string
	OriginalMessageID      string
	OriginalMessageChannel string
	AnswerThreadID         string
	FilmgameState          []*filmgame.Poster
	CrosswordState         *crossword.Crossword
	StartedAt              time.Time
	Scores                 map[string]*Score
}

func Render(imagesDir string, state State) (*gg.Context, error) {
	posterCtx, err := renderPosters(imagesDir, state.FilmgameState)
	if err != nil {
		return nil, err
	}
	postersImage, err := canvasToImage(posterCtx)
	if err != nil {
		return nil, err
	}

	crosswordCtx, err := crossword.RenderPNG(state.CrosswordState, 1000, 1000)
	if err != nil {
		return nil, err
	}
	crosswordImage, err := canvasToImage(crosswordCtx)
	if err != nil {
		return nil, err
	}

	dc := gg.NewContext(2000, 1800)
	dc.SetColor(color.Black)
	dc.Clear()
	dc.DrawImage(postersImage, 0, 0)
	dc.DrawImage(crosswordImage, 1000, 0)

	return dc, nil
}

func renderPosters(imagesDir string, posters []*filmgame.Poster) (*gg.Context, error) {
	var imageWidth = 200
	var imageHeight = 300

	dc := gg.NewContext(1000, 1800)
	dc.SetColor(color.Black)
	dc.Clear()

	row := 0
	xPosition := 0
	for k, v := range posters {
		var imagePath string
		if v.Guessed {
			imagePath = path.Join(imagesDir, v.OriginalImage)
		} else {
			imagePath = path.Join(imagesDir, v.ObscuredImage)
		}

		im, err := gg.LoadImage(imagePath)
		if err != nil {
			return nil, err
		}
		dc.DrawImage(im, xPosition*imageWidth, row*imageHeight)
		dc.SetColor(color.Black)
		dc.DrawRectangle(float64(xPosition*imageWidth), float64(row*imageHeight), 35, 35)
		dc.Fill()

		dc.SetColor(color.White)
		dc.SetFontFace(truetype.NewFace(font, &truetype.Options{Size: 20}))
		dc.DrawString(fmt.Sprintf("%d", k+1), float64(xPosition*imageWidth)+10, float64(row*imageHeight)+25)

		dc.Stroke()

		if (xPosition+1)*imageWidth >= 1000 {
			row++
			xPosition = 0
		} else {
			xPosition++
		}
	}

	return dc, nil
}

func canvasToImage(ctx *gg.Context) (image.Image, error) {
	buff := &bytes.Buffer{}
	if err := ctx.EncodePNG(buff); err != nil {
		return nil, err
	}

	postersImage, _, err := image.Decode(buff)
	if err != nil {
		return nil, err
	}
	return postersImage, nil
}
