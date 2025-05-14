package command

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/warmans/gamesmaster/pkg/discord"
	"github.com/warmans/gamesmaster/pkg/scores"
	"github.com/warmans/gamesmaster/pkg/util"
	"github.com/warmans/go-crossword"
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
	Scores                 *scores.Tiered
	Complete               bool
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
				// is the message an admin command?
				if m.Author.Username == ".warmans" {
					adminMatches := adminRegex.FindStringSubmatch(m.Content)
					if adminMatches != nil || len(adminMatches) == 2 {
						if err := c.handleAdminAction(s, adminMatches[1], m.GuildID, m.ChannelID, m.ID); err != nil {
							fmt.Printf("Admin action failed: %s\n", err.Error())
						}
						return
					}
				}

				matches := answerRegex.FindStringSubmatch(m.Content)
				if matches == nil || len(matches) != 3 {
					return
				}
				if err := c.handleCheckWordSubmission(s, matches[1], matches[2], m.ChannelID, m.ID, m.Author.Username); err != nil {
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

func (c *Crossword) handleCheckWordSubmission(s *discordgo.Session, clueID string, word string, channelID string, messageID string, username string) error {
	alreadySolved := false
	correct := false
	err := c.openCrosswordForWriting(func(cw *CrosswordState) (*CrosswordState, error) {
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

				cw.Scores.Add(username)
				break
			}
		}
		unsolved := 0
		for _, w := range cw.Game.Words {
			if !w.Solved {
				unsolved++
			}
		}
		if unsolved == 0 && !cw.Complete {
			cw.Complete = true
			if _, err := s.ChannelMessageSend(
				cw.AnswerThreadID,
				fmt.Sprintf("Game completed!\n\nScores:\n%s", cw.Scores.Render()),
			); err != nil {
				fmt.Println("Failed to send game completion message: ", err.Error())
				// don't fail as it'll stop it writing the game complete flag etc.
				return cw, nil
			}
		}
		return cw, nil
	})
	if err != nil {
		return err
	}

	if correct {
		if err := s.MessageReactionAdd(channelID, messageID, "✅"); err != nil {
			return err
		}
		return c.refreshCrossword(s)
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

func (c *Crossword) refreshCrossword(s *discordgo.Session) error {
	return c.openCrosswordForReading(func(cw *CrosswordState) error {

		files, content, err := c.renderBoard(cw)
		if err != nil {
			return err
		}

		_, err = s.ChannelMessageEditComplex(
			&discordgo.MessageEdit{
				Channel:     cw.OriginalMessageChannel,
				ID:          cw.OriginalMessageID,
				Content:     util.ToPtr(content),
				Files:       files,
				Attachments: util.ToPtr([]*discordgo.MessageAttachment{}),
			},
		)
		return err
	})
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
			},
		})
	}

	files, content, err := c.renderBoard(&cw)
	if err != nil {
		return err
	}
	initialMessage, err := s.ChannelMessageSendComplex(i.ChannelID, &discordgo.MessageSend{
		Content: content,
		Files:   files,
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
		return err
	}
	if err := c.openCrosswordForWriting(func(cw *CrosswordState) (*CrosswordState, error) {
		cw.AnswerThreadID = thread.ID
		c.answerThreadID = thread.ID

		cw.OriginalMessageID = initialMessage.ID
		cw.OriginalMessageChannel = initialMessage.ChannelID

		fmt.Printf("starting game. ThreadID: %s OriginalMessageID: %s OriginalMessageChannel: %s", cw.AnswerThreadID, cw.OriginalMessageID, cw.OriginalMessageChannel)
		return cw, nil
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

func (c *Crossword) renderBoard(cw *CrosswordState) ([]*discordgo.File, string, error) {
	canvas, err := crossword.RenderPNG(cw.Game, 1600, 800, crossword.WithClues(true))
	if err != nil {
		return nil, "", err
	}
	board := &bytes.Buffer{}
	if err := canvas.EncodePNG(board); err != nil {
		return nil, "", err
	}

	_, unsolvedClues := c.renderClues(*cw.Game)

	var messageBody string
	if unsolvedClues.Len() < 2000 {
		messageBody = unsolvedClues.String()
	}

	return []*discordgo.File{
		{
			Name:        "crossword.png",
			ContentType: "images/png",
			Reader:      board,
		},
		//{
		//	Name:        "clues.txt",
		//	ContentType: "text/plain",
		//	Reader:      io.MultiReader(unsolvedClues, solvedClues),
		//},
	}, messageBody, nil

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
		_, _ = fmt.Fprintf(unsolvedClues, "**DOWN**")
		for _, w := range unsolvedDown {
			_, _ = fmt.Fprintf(unsolvedClues, "\n`[%s | %d letters]` %s", w.ClueID(), len(w.Word.Word), w.Word.Clue)
		}
	}
	if len(unsolvedAcross) > 0 {
		_, _ = fmt.Fprintf(unsolvedClues, "\n\n**ACROSS**")
		for _, w := range unsolvedAcross {
			_, _ = fmt.Fprintf(unsolvedClues, "\n`[%s | %d letters]` %s", w.ClueID(), len(w.Word.Word), w.Word.Clue)
		}
	}

	solvedClues := &bytes.Buffer{}
	if len(solved) > 0 {
		_, _ = fmt.Fprintf(solvedClues, "\n\n**SOLVED**")
		for _, w := range solved {
			_, _ = fmt.Fprintf(solvedClues, "\n `[%s | %d letters]` %s", w.ClueID(), len(w.Word.Word), w.Word.Clue)
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

func (c *Crossword) handleAdminAction(s *discordgo.Session, action string, guildID string, channelID string, messageID string) error {
	switch action {
	case "refresh":
		if err := c.refreshCrossword(s); err != nil {
			return err
		}
		return s.MessageReactionAdd(channelID, messageID, "👀")
	case "complete":
		if err := c.openCrosswordForWriting(func(cw *CrosswordState) (*CrosswordState, error) {
			//leave one unsolved
			oneLeft := false
			for k, v := range cw.Game.Words {
				if v.Solved == false && !oneLeft {
					oneLeft = true
					continue
				}
				solved := cw.Game.Words[k]
				solved.Solved = true
				cw.Game.Words[k] = solved
			}
			return cw, nil
		}); err != nil {
			return err
		}
		if err := c.refreshCrossword(s); err != nil {
			return err
		}
		return s.MessageReactionAdd(channelID, messageID, "👀")
	case "reset":
		if err := c.openCrosswordForWriting(func(cw *CrosswordState) (*CrosswordState, error) {
			for k := range cw.Game.Words {
				solved := cw.Game.Words[k]
				solved.Solved = false
				cw.Game.Words[k] = solved
				cw.Scores.Scores = make(map[string]*scores.Score)
				cw.Scores.LastUser = ""
				cw.Complete = false
			}
			return cw, nil
		}); err != nil {
			return err
		}
		if err := c.refreshCrossword(s); err != nil {
			return err
		}
		return s.MessageReactionAdd(channelID, messageID, "👀")
	default:
		return s.MessageReactionAdd(channelID, messageID, "🤷")
	}
}

func (c *Crossword) openCrosswordForWriting(cb func(cw *CrosswordState) (*CrosswordState, error)) error {
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

	cw, err = cb(cw)
	if err != nil {
		return err
	}
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
