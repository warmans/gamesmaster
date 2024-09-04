package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/warmans/gamesmaster/pkg/discord"
	"github.com/warmans/gamesmaster/pkg/filmgame"
	"github.com/warmans/gamesmaster/pkg/util"
	"os"
	"regexp"
	"strings"
	"sync"
)

var posterGuessRegex = regexp.MustCompile("[Gg]uess\\s([0-9]+)\\s(.+)")

type FilmgameState struct {
	OriginalMessageID      string
	OriginalMessageChannel string
	AnswerThreadID         string
	Posters                []*filmgame.Poster
}

const gameDescription = "Guess the flm posters in the game thread with e.g. `guess 1 fargo`"

const (
	filmgameCommand = "filmgame"
)

const (
	FilmgameCmdStart string = "start"
)

func NewFilmgameCommand() *Filmgame {
	return &Filmgame{}
}

type Filmgame struct {
	gameLock       sync.RWMutex
	answerThreadID string
}

func (c *Filmgame) Prefix() string {
	return "flm"
}

func (c *Filmgame) RootCommand() string {
	return filmgameCommand
}

func (c *Filmgame) Description() string {
	return "Film poster game"
}

func (c *Filmgame) AutoCompleteHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{}
}

func (c *Filmgame) ButtonHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{}
}

func (c *Filmgame) ModalHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{}
}

func (c *Filmgame) CommandHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{
		FilmgameCmdStart: c.startFilmgame,
	}
}

func (c *Filmgame) MessageHandlers() discord.MessageHandlers {
	return discord.MessageHandlers{
		func(s *discordgo.Session, m *discordgo.MessageCreate) {
			if c.answerThreadID == "" {
				if err := c.openFilmgameForReading(func(cw *FilmgameState) error {
					c.answerThreadID = cw.AnswerThreadID
					return nil
				}); err != nil {
					fmt.Println("Failed to get current filmgame answer thread ID: ", err.Error())
					return
				}
			}
			if m.ChannelID == c.answerThreadID {
				matches := posterGuessRegex.FindStringSubmatch(m.Content)
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

func (c *Filmgame) SubCommands() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Name:        FilmgameCmdStart,
			Description: "Start the game (if available).",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
		},
	}
}

func (c *Filmgame) handleCheckWordSubmission(s *discordgo.Session, clueID string, word string, channelID string, messageID string) error {

	var alreadySolved = false
	var correct = false
	if err := c.openFilmgameForWriting(func(cw *FilmgameState) *FilmgameState {
		for k, v := range cw.Posters {
			if fmt.Sprintf("%d", k+1) == clueID && strings.ToLower(word) == strings.ToLower(v.Answer) {
				if v.Guessed == true {
					alreadySolved = true
					return cw
				}
				cw.Posters[k].Guessed = true
				correct = true
				return cw
			}
		}
		return cw
	}); err != nil {
		return err
	}

	if correct {
		fmt.Println("Correct!")
		err := c.openFilmgameForReading(func(cw *FilmgameState) error {

			buff, err := c.renderBoard(cw)
			if err != nil {
				return err
			}

			_, err = s.ChannelMessageEditComplex(
				&discordgo.MessageEdit{
					Channel: cw.OriginalMessageChannel,
					ID:      cw.OriginalMessageID,
					Content: util.ToPtr(gameDescription),
					Files: []*discordgo.File{
						{
							Name:        "Filmgame.png",
							ContentType: "images/png",
							Reader:      buff,
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

func (c *Filmgame) startFilmgame(s *discordgo.Session, i *discordgo.InteractionCreate) error {

	var fgs FilmgameState
	err := c.openFilmgameForReading(func(c *FilmgameState) error {
		fgs = *c
		return nil
	})
	if err != nil {
		return err
	}

	if fgs.AnswerThreadID != "" {
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Flags:   discordgo.MessageFlagsEphemeral,
				Content: "Game already started",
				//todo: how to add link to existing thread.
			},
		})
	}

	board, err := c.renderBoard(&fgs)
	if err != nil {
		return err
	}

	initialMessage, err := s.ChannelMessageSendComplex(i.ChannelID, &discordgo.MessageSend{
		Content: gameDescription,
		Files: []*discordgo.File{
			{
				Name:        "Filmgame.png",
				ContentType: "images/png",
				Reader:      board,
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
	if err := c.openFilmgameForWriting(func(cw *FilmgameState) *FilmgameState {
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

func (c *Filmgame) renderBoard(state *FilmgameState) (*bytes.Buffer, error) {
	buff := &bytes.Buffer{}
	canvas, err := filmgame.Render("./var/filmgame/game/images", state.Posters)
	if err != nil {
		return nil, err
	}
	if err := canvas.EncodePNG(buff); err != nil {
		return nil, err
	}
	return buff, nil
}

func (c *Filmgame) openFilmgameForReading(cb func(cw *FilmgameState) error) error {
	c.gameLock.RLock()
	defer c.gameLock.RUnlock()

	f, err := os.Open("var/filmgame/game/current.json")
	if err != nil {
		return err
	}
	defer f.Close()

	cw := FilmgameState{}
	if err := json.NewDecoder(f).Decode(&cw); err != nil {
		return err
	}

	return cb(&cw)
}

func (c *Filmgame) openFilmgameForWriting(cb func(cw *FilmgameState) *FilmgameState) error {
	c.gameLock.Lock()
	defer c.gameLock.Unlock()

	f, err := os.OpenFile("var/filmgame/game/current.json", os.O_RDWR|os.O_EXCL, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	cw := &FilmgameState{}
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

func (c *Filmgame) withPrefix(id string) string {
	return fmt.Sprintf("%s:%s", c.Prefix(), id)
}
