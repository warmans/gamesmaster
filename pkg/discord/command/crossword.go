package command

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/warmans/gamesmaster/pkg/discord"
	"github.com/warmans/gamesmaster/pkg/util"
	"github.com/warmans/go-crossword"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
)

var answerRegex = regexp.MustCompile(`([A-Za-z][0-9]+)\s(.+)`)

type CrosswordState struct {
	OriginalMessageID      string
	OriginalMessageChannel string
	AnswerThreadID         string
	Game                   *crossword.Crossword
}

const (
	crosswordCommand = "crossword"
)

const (
	crosswordCmdStart string = "start"
)

func NewCrosswordCommand() *Crossword {
	return &Crossword{}
}

type Crossword struct {
	gameLock       sync.RWMutex
	answerThreadID string
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
	return discord.InteractionHandlers{}
}

func (c *Crossword) ModalHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{}
}

func (c *Crossword) CommandHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{
		crosswordCmdStart: c.startCrossword,
	}
}

func (c *Crossword) MessageHandlers() discord.MessageHandlers {
	return discord.MessageHandlers{
		func(s *discordgo.Session, m *discordgo.MessageCreate) {
			if c.answerThreadID == "" {
				if err := c.openCrosswordForReading(func(cw *CrosswordState) error {
					c.answerThreadID = cw.AnswerThreadID
					return nil
				}); err != nil {
					fmt.Println("Failed to get current crossword answer thread ID: ", err.Error())
					return
				}
			}
			if m.ChannelID == c.answerThreadID {
				matches := answerRegex.FindStringSubmatch(m.Content)
				if matches == nil || len(matches) != 3 {
					return
				}
				if err := c.handleCheckWordSubmission(s, matches[1], matches[2], m.ChannelID, m.ID); err != nil {
					fmt.Println("Failed to check work: ", err.Error())
					return
				}
			}
		},
	}
}

func (c *Crossword) SubCommands() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Name:        crosswordCmdStart,
			Description: "Start the game (if available).",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
		},
	}
}

func (c *Crossword) handleCheckWordSubmission(s *discordgo.Session, clueID string, word string, channelID string, messageID string) error {

	alreadySolved := false
	correct := false
	err := c.openCrosswordForWriting(func(cw *CrosswordState) *CrosswordState {
		for k, w := range cw.Game.Words {
			if w.ClueID() != strings.ToUpper(clueID) {
				continue
			}
			if w.Solved {
				alreadySolved = true
				break
			}
			if strings.ToUpper(util.WithoutSpaces(word)) == strings.ToUpper(w.Word.Word) {
				correct = true
				solved := cw.Game.Words[k]
				solved.Solved = true
				cw.Game.Words[k] = solved
				break
			}
		}
		return cw
	})
	if err != nil {
		return err
	}

	if correct {
		fmt.Println("Correct!")
		err := c.openCrosswordForReading(func(cw *CrosswordState) error {
			canvas, err := crossword.RenderPNG(cw.Game, 1200, 1200)
			if err != nil {
				return err
			}
			buff := &bytes.Buffer{}
			if err := canvas.EncodePNG(buff); err != nil {
				return err
			}

			solvedClues, unsolvedClues := c.renderClues(*cw.Game)

			var messageBody string
			if unsolvedClues.Len() < 2000 {
				messageBody = unsolvedClues.String()
			}

			_, err = s.ChannelMessageEditComplex(
				&discordgo.MessageEdit{
					Channel: cw.OriginalMessageChannel,
					ID:      cw.OriginalMessageID,
					Content: util.ToPtr(messageBody),
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
					Attachments: util.ToPtr([]*discordgo.MessageAttachment{}),
				},
			)
			if err != nil {
				return err
			}
			return s.MessageReactionAdd(channelID, messageID, "✅")
		})
		if err != nil {
			return err
		}
	} else {
		if alreadySolved {
			if err := s.MessageReactionAdd(channelID, messageID, "🕣"); err != nil {
				return err
			}
		} else {
			if err := s.MessageReactionAdd(channelID, messageID, "❌"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Crossword) startCrossword(s *discordgo.Session, i *discordgo.InteractionCreate) error {

	var cw CrosswordState
	err := c.openCrosswordForReading(func(c *CrosswordState) error {
		cw = *c
		return nil
	})
	if err != nil {
		return err
	}

	if cw.AnswerThreadID != "" {
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Flags:   discordgo.MessageFlagsEphemeral,
				Content: "Game already started",
				//todo: how to add link to existing thread.
			},
		})
	}

	canvas, err := crossword.RenderPNG(cw.Game, 1024, 1024)
	if err != nil {
		return err
	}
	buff := &bytes.Buffer{}
	if err := canvas.EncodePNG(buff); err != nil {
		return err
	}

	solvedClues, unsolvedClues := c.renderClues(*cw.Game)

	var messageBody string
	if unsolvedClues.Len() < 2000 {
		messageBody = unsolvedClues.String()
	}

	initialMessage, err := s.ChannelMessageSendComplex(i.ChannelID, &discordgo.MessageSend{
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
	})
	if err != nil {
		fmt.Printf("Failed to start game: %s\n", err.Error())
		return err
	}

	thread, err := s.MessageThreadStartComplex(initialMessage.ChannelID, initialMessage.ID, &discordgo.ThreadStart{
		Name: "Answers Thread",
		Type: discordgo.ChannelTypeGuildPublicThread,
	})
	if err != nil {
		panic(err)
	}
	if err := c.openCrosswordForWriting(func(cw *CrosswordState) *CrosswordState {
		cw.AnswerThreadID = thread.ID
		c.answerThreadID = thread.ID

		cw.OriginalMessageID = initialMessage.ID
		cw.OriginalMessageChannel = initialMessage.ChannelID

		fmt.Printf("starting game. ThreadID: %s OriginalMessageID: %s OriginalMessageChannel: %s", cw.AnswerThreadID, cw.OriginalMessageID, cw.OriginalMessageChannel)
		return cw
	}); err != nil {
		fmt.Printf("Failed to store answer thread ID: %s\n", err.Error())
		return err
	}

	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: "Starting Game...",
		},
	})
}

func (c *Crossword) renderClues(cw crossword.Crossword) (*bytes.Buffer, *bytes.Buffer) {
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
	if len(unsolvedDown) > 0 {
		fmt.Fprintf(unsolvedClues, "**DOWN**")
		for _, w := range unsolvedDown {
			fmt.Fprintf(unsolvedClues, "\n`[%s | %d letters]` %s", w.ClueID(), len(w.Word.Word), w.Word.Clue)
		}
	}
	if len(unsolvedAcross) > 0 {
		fmt.Fprintf(unsolvedClues, "\n\n**ACROSS**")
		for _, w := range unsolvedAcross {
			fmt.Fprintf(unsolvedClues, "\n`[%s | %d letters]` %s", w.ClueID(), len(w.Word.Word), w.Word.Clue)
		}
	}

	solvedClues := &bytes.Buffer{}
	if len(solved) > 0 {
		fmt.Fprintf(solvedClues, "\n\n**SOLVED**")
		for _, w := range solved {
			fmt.Fprintf(solvedClues, "\n `[%s | %d letters]` %s", w.ClueID(), len(w.Word.Word), w.Word.Clue)
		}
	}

	return solvedClues, unsolvedClues
}

func (c *Crossword) openCrosswordForReading(cb func(cw *CrosswordState) error) error {
	c.gameLock.RLock()
	defer c.gameLock.RUnlock()

	f, err := os.Open("var/crossword/game/current.json")
	if err != nil {
		return err
	}
	defer f.Close()

	cw := CrosswordState{}
	if err := json.NewDecoder(f).Decode(&cw); err != nil {
		return err
	}

	return cb(&cw)
}

func (c *Crossword) openCrosswordForWriting(cb func(cw *CrosswordState) *CrosswordState) error {
	c.gameLock.Lock()
	defer c.gameLock.Unlock()

	f, err := os.OpenFile("var/crossword/game/current.json", os.O_RDWR|os.O_EXCL, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	cw := &CrosswordState{}
	if err := json.NewDecoder(f).Decode(cw); err != nil {
		return err
	}

	cw = cb(cw)
	if cw == nil {
		return nil
	}

	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(cw)
}
