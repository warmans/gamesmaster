package crossword

import (
	"fmt"
	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font/gofont/goregular"
	"log"
	"sort"
	"strings"
)

var font *truetype.Font

func init() {
	var err error
	font, err = truetype.Parse(goregular.TTF)
	if err != nil {
		log.Fatal(err)
	}
}

const EMPTY_CHAR = ' '
const EMPTY_INDEX = 0

type Cell struct {
	Char  rune
	Index int
	Value rune
}

type Grid [][]Cell

type Word struct {
	Word string
	Clue string
}

type Words []Word

func (w Words) Len() int           { return len(w) }
func (w Words) Swap(i, j int)      { w[i], w[j] = w[j], w[i] }
func (w Words) Less(i, j int) bool { return len(w[i].Word) > len(w[j].Word) }

type ActiveWord struct {
	Word     Word
	X        int
	Y        int
	Vertical bool
	Number   int
	Solved   bool
}

func (w ActiveWord) String() string {
	// vertical seems to mean horizontal
	direction := "D"
	if w.Vertical {
		direction = "A"
	}
	return fmt.Sprintf("%d%s", w.Number, direction)
}

type Crossword struct {
	Grid     Grid
	WordList []ActiveWord
}

func (c *Crossword) String() string {
	result := ""
	for _, v := range c.WordList {
		result += fmt.Sprintf("%s %d, %t\n", v.Word, v.X, v.Vertical)
	}
	gridHeight := len(c.Grid[0])
	for y := 0; y < gridHeight; y++ {
		for x := 0; x < len(c.Grid[y]); x++ {
			if c.Grid[x][y].Char == EMPTY_CHAR {
				result = result + "."
			} else {
				result = result + string(c.Grid[x][y].Char)
			}
		}
		result = result + "\n"
	}
	return result
}

func (c *Crossword) Render(width, height int) (*gg.Context, error) {

	cellWidth := float64(width / len(c.Grid))
	cellHeight := float64(height / len(c.Grid))

	dc := gg.NewContext(width, height)
	dc.SetRGB(0, 0, 0)
	dc.Clear()

	for gridX := 0; gridX < len(c.Grid); gridX++ {
		for gridY, cell := range c.Grid[gridX] {

			dc.DrawRectangle(float64(gridX)*cellWidth, float64(gridY)*cellHeight, cellWidth, cellHeight)

			if cell.Char != EMPTY_CHAR {

				var words []ActiveWord
				for k, v := range c.WordList {
					var intersectWord bool
					if v.Vertical {
						intersectWord = (gridY >= v.Y && gridY < v.Y+len(v.Word.Word)) && gridX == v.X
					} else {
						intersectWord = (gridX >= v.X && gridX < v.X+len(v.Word.Word)) && gridY == v.Y
					}
					if intersectWord {
						words = append(words, c.WordList[k])
					}
					fmt.Printf("%dx%d -> %s | %s %v %dx%d | %v\n", gridX, gridY, string(cell.Char), v.Word.Word, v.Vertical, v.X, v.Y, intersectWord)
				}

				dc.SetRGB(1, 1, 1)
				dc.FillPreserve()

				dc.SetRGB(0, 0, 0)

				if words != nil {
					dc.SetFontFace(truetype.NewFace(font, &truetype.Options{Size: 10}))
					for k, w := range words {
						if w.X == gridX && w.Y == gridY {
							// draw the word start identifier
							dc.DrawString(w.String(), float64(gridX)*cellWidth+2+(14*float64(k)), float64(gridY)*cellHeight+12)
						}
					}
					dc.Stroke()
				}

				dc.SetFontFace(truetype.NewFace(font, &truetype.Options{Size: 24}))
				dc.DrawStringAnchored(
					fmt.Sprintf("%s", strings.ToUpper(string(cell.Char))),
					float64(gridX)*cellWidth+cellHeight/2,
					float64(gridY)*cellHeight+cellWidth/2,
					0.5,
					0.5,
				)
				dc.SetLineWidth(0.3)
				dc.Stroke()
			} else {
				dc.SetRGB(1, 1, 1)
				dc.SetLineWidth(0.3)
				dc.Stroke()
			}
		}
	}

	return dc, nil
}

func NewGenerator(cols, rows int, words Words) *Generator {
	var grid Grid
	grid = make([][]Cell, rows)
	for x := 0; x < rows; x++ {
		grid[x] = make([]Cell, cols)
		for y := 0; y < cols; y++ {
			grid[x][y] = Cell{
				EMPTY_CHAR,
				EMPTY_INDEX,
				EMPTY_CHAR,
			}
		}
	}

	sort.Sort(words)

	gen := &Generator{
		cols,
		rows,
		words,
		grid,
		make([]ActiveWord, 0),
		0,
		0,
	}

	return gen
}

type Generator struct {
	cols           int
	rows           int
	words          Words
	grid           Grid
	activeWordList []ActiveWord
	downCount      int
	acrossCount    int
}

func (c *Generator) Generate(seed int, loops int) *Crossword {
	//manually place the longest Word horizontally at 0,0, try others if the generated board is too weak
	c.placeWord(c.words[seed], c.rows/2, 0, false)
	c.generate(seed, loops)
	return &Crossword{Grid: c.grid, WordList: c.activeWordList}
}

func (c *Generator) generate(seed int, loops int) {

	//attempt to fill the rest of the board
	for iy := 0; iy < loops; iy++ { //usually 2 times is enough for max fill potential
		for ix := 1; ix < len(c.words); ix++ {
			if !c.isActiveWord(c.words[ix].Word) { //only add if not already in the active Word list
				topScore := 0
				bestScoreIndex := 0
				fitScore := 0

				coordList := c.suggestCoords([]rune(c.words[ix].Word)) //fills coordList and coordCount

				if len(coordList) > 0 {
					//coordList = shuffleArray(coordList)     //adds some randomization
					for cl := 0; cl < len(coordList); cl++ { //get the best fit score from the list of possible valid coordinates
						fitScore = c.checkFitScore([]rune(c.words[ix].Word), coordList[cl].x, coordList[cl].y, coordList[cl].vertical)
						if fitScore > topScore {
							topScore = fitScore
							bestScoreIndex = cl
						}
					}
				}

				if topScore > 1 { //only place a Word if it has a fitscore of 2 or higher
					c.placeWord(c.words[ix], coordList[bestScoreIndex].x, coordList[bestScoreIndex].y, coordList[bestScoreIndex].vertical)
				}
			}
		}
	}
}

func (c *Generator) placeWord(w Word, x, y int, vertical bool) bool { //places a new active Word on the board

	wordPlaced := false

	word := []rune(w.Word)
	l := len(word)

	if vertical {
		if l+x < c.rows {
			for i := 0; i < l; i++ {
				c.grid[x+i][y].Char = word[i]
			}
			wordPlaced = true
		}
	} else {
		if l+y < c.cols {
			for i := 0; i < l; i++ {
				c.grid[x][y+i].Char = word[i]
			}
			wordPlaced = true
		}
	}

	if wordPlaced {
		number := 0
		if vertical {
			c.downCount++
			number = c.downCount
		} else {
			c.acrossCount++
			number = c.acrossCount
		}

		aw := ActiveWord{
			Word:     w,
			X:        x,
			Y:        y,
			Vertical: vertical,
			Number:   number,
		}

		c.activeWordList = append(c.activeWordList, aw)
	}
	return wordPlaced
}

func (c *Generator) isActiveWord(word string) bool {
	l := len(c.activeWordList)
	for w := 0; w < l; w++ {
		if word == c.activeWordList[w].Word.Word {
			return true
		}
	}
	return false
}

type coord struct {
	x        int
	y        int
	score    int
	vertical bool
}

func (c *Generator) suggestCoords(word []rune) []coord { //search for potential cross placement locations
	coordList := make([]coord, 0)
	coordCount := 0
	for i := 0; i < len(word); i++ { //cycle through each character of the Word
		ch := word[i]
		for x := 0; x < c.rows; x++ {
			for y := 0; y < c.cols; y++ {
				if c.grid[x][y].Char == ch { //check for letter match in cell
					if x-i+1 > 0 && x-i+len(word)-1 < c.rows { //would fit vertically?
						coordList = append(coordList, coord{
							x - i,
							y,
							0,
							true,
						})
						coordCount++
					}

					if y-i+1 > 0 && y-i+len(word)-1 < c.cols { //would fit horizontally?
						coordList = append(coordList, coord{
							x,
							y - i,
							0,
							false,
						})
						coordCount++
					}
				}
			}
		}
	}
	return coordList
}

func (c *Generator) checkFitScore(word []rune, x, y int, vertical bool) int {
	fitScore := 1 //default is 1, 2+ has crosses, 0 is invalid due to collision

	if vertical { //Vertical checking
		for i := 0; i < len(word); i++ {
			if i == 0 && x > 0 { //check for empty space preceeding first character of Word if not on edge
				if c.grid[x-1][y].Char != EMPTY_CHAR { //adjacent letter collision
					fitScore = 0
					break
				}
			} else if i == len(word) && x < c.rows { //check for empty space after last character of Word if not on edge
				if c.grid[x+i+1][y].Char != EMPTY_CHAR { //adjacent letter collision
					fitScore = 0
					break
				}
			}
			if x+i < c.rows {
				if c.grid[x+i][y].Char == word[i] { //letter match - aka cross point
					fitScore += 1
				} else if c.grid[x+i][y].Char != EMPTY_CHAR { //letter doesn't match and it isn't empty so there is a collision
					fitScore = 0
					break
				} else { //verify that there aren't letters on either side of placement if it isn't a crosspoint
					if y < c.cols-1 { //check right side if it isn't on the edge
						if c.grid[x+i][y+1].Char != EMPTY_CHAR { //adjacent letter collision
							fitScore = 0
							break
						}
					}
					if y > 0 { //check left side if it isn't on the edge
						if c.grid[x+i][y-1].Char != EMPTY_CHAR { //adjacent letter collision
							fitScore = 0
							break
						}
					}
				}
			}

		}

	} else { //horizontal checking
		for i := 0; i < len(word); i++ {
			if i == 0 && y > 0 { //check for empty space preceeding first character of Word if not on edge
				if c.grid[x][y-1].Char != EMPTY_CHAR { //adjacent letter collision
					fitScore = 0
					break
				}
			} else if i == len(word)-1 && y+i < c.cols-1 { //check for empty space after last character of Word if not on edge
				if c.grid[x][y+i+1].Char != EMPTY_CHAR { //adjacent letter collision
					fitScore = 0
					break
				}
			}
			if y+i < c.cols {
				if c.grid[x][y+i].Char == word[i] { //letter match - aka cross point
					fitScore++
				} else if c.grid[x][y+i].Char != EMPTY_CHAR { //letter doesn't match and it isn't empty so there is a collision
					fitScore = 0
					break
				} else { //verify that there aren't letters on either side of placement if it isn't a crosspoint
					if x < c.rows-1 { //check top side if it isn't on the edge
						if c.grid[x+1][y+i].Char != EMPTY_CHAR { //adjacent letter collision
							fitScore = 0
							break
						}
					}
					if x > 0 { //check bottom side if it isn't on the edge
						if c.grid[x-1][y+i].Char != EMPTY_CHAR { //adjacent letter collision
							fitScore = 0
							break
						}
					}
				}
			}
		}
	}

	return fitScore
}
