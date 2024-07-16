package command

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/warmans/gamesmaster/pkg/discord"
	"github.com/warmans/go-crossword"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
)

const (
	crosswordCommand = "crossword"
)
const (
	crosswordAnswerModalOpen string = "handleAnswerModalOpen"
	crosswordAnswerCheck     string = "handleCheckWordSubmission"
)

const (
	crosswordCmdClues string = "clues"
	crosswordCmdShow  string = "show"
)

func NewCrosswordCommand() *Crossword {
	return &Crossword{}
}

type Crossword struct {
	gameLock sync.RWMutex
}

func (c *Crossword) Prefix() string {
	return "cwd"
}

func (c *Crossword) RootCommand() string {
	return crosswordCommand
}

func (c *Crossword) Description() string {
	return "Crossword game"
}

func (c *Crossword) AutoCompleteHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{}
}

func (c *Crossword) ButtonHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{
		crosswordAnswerModalOpen: c.handleAnswerModalOpen,
	}
}

func (c *Crossword) ModalHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{
		crosswordAnswerCheck: c.handleCheckWordSubmission,
	}
}

func (c *Crossword) CommandHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{
		crosswordCmdClues: c.showCrossword,
	}
}

func (c *Crossword) SubCommands() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Name:        crosswordCmdClues,
			Description: "Show the clues or submit an answer.",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
		},
	}
}

func (c *Crossword) handleAnswerModalOpen(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: c.withPrefix(crosswordAnswerCheck),
			Title:    "Submit Word",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  "id",
							Label:     "Word ID (e.g. 4D)",
							Style:     discordgo.TextInputShort,
							Required:  true,
							MaxLength: 3,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  "word",
							Label:     "Word",
							Style:     discordgo.TextInputShort,
							Required:  true,
							MaxLength: 128,
						},
					},
				},
			},
		},
	})
}

func (c *Crossword) handleCheckWordSubmission(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	id := i.Interaction.ModalSubmitData().Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
	word := i.Interaction.ModalSubmitData().Components[1].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value

	alreadySolved := false
	correct := false
	clue := ""
	err := c.openCrosswordForWriting(func(cw *crossword.Crossword) *crossword.Crossword {
		for k, w := range cw.Words {
			if w.ClueID() != strings.ToUpper(id) {
				continue
			}
			if w.Solved {
				alreadySolved = true
				break
			}
			if strings.TrimSpace(strings.ToUpper(word)) == strings.ToUpper(w.Word.Word) {
				correct = true
				solved := cw.Words[k]
				clue = cw.Words[k].Word.Clue
				solved.Solved = true
				cw.Words[k] = solved
				break
			}
		}
		return cw
	})
	if err != nil {
		return err
	}

	if correct {
		err := c.openCrosswordForReading(func(cw *crossword.Crossword) error {
			canvas, err := crossword.RenderPNG(cw, 1200, 1200)
			if err != nil {
				return err
			}
			buff := &bytes.Buffer{}
			if err := canvas.EncodePNG(buff); err != nil {
				return err
			}
			return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf(
						"\n> %s\n\n`%s` was solved by %s: `%s`\n",
						clue,
						id,
						i.Interaction.Member.DisplayName(),
						strings.ToUpper(word),
					),
					Files: []*discordgo.File{
						{
							Name:        "crossword.png",
							ContentType: "images/png",
							Reader:      buff,
						},
					},
				},
			})
		})
		if err != nil {
			return err
		}
	} else {
		if alreadySolved {
			err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("%s has already been solved", id),
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			if err != nil {
				return fmt.Errorf("failed to respond: %w", err)
			}
		} else {
			err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("%s was not correct: %s", id, word),
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			if err != nil {
				return fmt.Errorf("failed to respond: %w", err)
			}
		}
	}
	return nil
}

func (c *Crossword) showCrossword(s *discordgo.Session, i *discordgo.InteractionCreate) error {

	var cw crossword.Crossword
	err := c.openCrosswordForReading(func(c *crossword.Crossword) error {
		cw = *c
		return nil
	})
	if err != nil {
		return err
	}
	canvas, err := crossword.RenderPNG(&cw, 1024, 1024)
	if err != nil {
		return err
	}

	slices.SortFunc(cw.Words, func(a, b crossword.Placement) int {
		return cmp.Compare(a.ID, b.ID)
	})

	solved := []crossword.Placement{}
	unsolvedDown := []crossword.Placement{}
	unsolvedAcross := []crossword.Placement{}
	for _, w := range cw.Words {
		if !w.Solved {
			if w.Vertical {
				unsolvedDown = append(unsolvedDown, w)
			} else {
				unsolvedAcross = append(unsolvedAcross, w)
			}
		} else {
			solved = append(solved, w)
		}
	}

	unsolvedClues := &bytes.Buffer{}
	fmt.Fprintf(unsolvedClues, "**DOWN**")
	for _, w := range unsolvedDown {
		fmt.Fprintf(unsolvedClues, "\n`[%s | %d letters]` %s", w.ClueID(), len(w.Word.Word), w.Word.Clue)
	}
	fmt.Fprintf(unsolvedClues, "\n\n**ACROSS**")
	for _, w := range unsolvedAcross {
		fmt.Fprintf(unsolvedClues, "\n`[%s | %d letters]` %s", w.ClueID(), len(w.Word.Word), w.Word.Clue)
	}

	solvedClues := &bytes.Buffer{}
	if len(solved) > 0 {
		fmt.Fprintf(solvedClues, "\n\n**SOLVED**")
		for _, w := range solved {
			fmt.Fprintf(solvedClues, "\n `[%s | %d letters]` %s", w.ClueID(), len(w.Word.Word), w.Word.Clue)
		}
	}
	buff := &bytes.Buffer{}
	if err := canvas.EncodePNG(buff); err != nil {
		return err
	}

	var messageBody string
	if unsolvedClues.Len() < 2000 {
		messageBody = unsolvedClues.String()
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: messageBody,
			Files: []*discordgo.File{
				{
					Name:        "crossword.png",
					ContentType: "images/png",
					Reader:      buff,
				},
				{
					Name:        "clues.txt",
					ContentType: "text/plain",
					Reader:      io.MultiReader(unsolvedClues, solvedClues),
				},
			},
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{discordgo.Button{
						Label: "Submit Answer",
						Emoji: &discordgo.ComponentEmoji{
							Name: "✅",
						},
						Style:    discordgo.PrimaryButton,
						Disabled: false,
						CustomID: c.withPrefix(crosswordAnswerModalOpen),
					}},
				},
			},
		},
	})
	return err
}

func (c *Crossword) openCrosswordForReading(cb func(cw *crossword.Crossword) error) error {
	c.gameLock.RLock()
	defer c.gameLock.RUnlock()

	f, err := os.Open("var/crossword/game/current.json")
	if err != nil {
		return err
	}
	defer f.Close()

	cw := crossword.Crossword{}
	if err := json.NewDecoder(f).Decode(&cw); err != nil {
		return err
	}

	return cb(&cw)
}

func (c *Crossword) openCrosswordForWriting(cb func(cw *crossword.Crossword) *crossword.Crossword) error {
	c.gameLock.Lock()
	defer c.gameLock.Unlock()

	f, err := os.OpenFile("var/crossword/game/current.json", os.O_RDWR|os.O_EXCL, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	cw := &crossword.Crossword{}
	if err := json.NewDecoder(f).Decode(cw); err != nil {
		return err
	}

	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}

	cw = cb(cw)

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(cw)
}

func (c *Crossword) withPrefix(id string) string {
	return fmt.Sprintf("%s:%s", c.Prefix(), id)
}
