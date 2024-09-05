package filmgame

import (
	"fmt"
	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font/gofont/goregular"
	"image/color"
	"log"
	"path"
)

var font *truetype.Font

func init() {
	var err error
	font, err = truetype.Parse(goregular.TTF)
	if err != nil {
		log.Fatal(err)
	}
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