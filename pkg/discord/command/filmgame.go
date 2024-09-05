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
	"slices"
	"strings"
	"sync"
)

var posterGuessRegex = regexp.MustCompile(`[Gg]uess\s([0-9]+)\s(.+)`)

type FilmgameState struct {
	OriginalMessageID      string
	OriginalMessageChannel string
	AnswerThreadID         string
	Posters                []*filmgame.Poster
	Scores                 map[string]int
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
				if err := c.handleCheckWordSubmission(
					s,
					matches[1],
					matches[2],
					m.ChannelID,
					m.ID,
					m.Author.Username,
				); err != nil {
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

func (c *Filmgame) handleCheckWordSubmission(
	s *discordgo.Session,
	clueID string,
	word string,
	channelID string,
	messageID string,
	userName string,
) error {
	var alreadySolved = false
	var correct = false
	if err := c.openFilmgameForWriting(func(cw *FilmgameState) (*FilmgameState, error) {
		for k, v := range cw.Posters {
			if fmt.Sprintf("%d", k+1) == clueID && strings.EqualFold(word, v.Answer) {
				if v.Guessed {
					alreadySolved = true
					return cw, nil
				}
				cw.Posters[k].Guessed = true
				correct = true
				return cw, nil
			}
		}
		return cw, nil
	}); err != nil {
		return err
	}

	if correct {
		err := c.openFilmgameForWriting(func(cw *FilmgameState) (*FilmgameState, error) {

			buff, err := c.renderBoard(cw)
			if err != nil {
				return cw, err
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
				return cw, err
			}
			if err := s.MessageReactionAdd(channelID, messageID, "✅"); err != nil {
				return cw, err
			}
			if cw.Scores == nil {
				cw.Scores = make(map[string]int)
			}
			if _, exists := cw.Scores[userName]; !exists {
				cw.Scores[userName] = 1
			} else {
				cw.Scores[userName]++
			}
			for _, v := range cw.Posters {
				if !v.Guessed {
					return cw, nil
				}
			}
			if _, err := s.ChannelMessageSend(
				cw.AnswerThreadID,
				fmt.Sprintf("Game complete!\nScores:\n%s", renderScores(cw.Scores)),
			); err != nil {
				return cw, err
			}
			return cw, nil
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

func renderScores(scores map[string]int) string {

	var scoreSlice []struct {
		score    int
		userName string
	}
	for userName, score := range scores {
		scoreSlice = append(scoreSlice, struct {
			score    int
			userName string
		}{score: score, userName: userName})
	}

	slices.SortFunc(scoreSlice, func(a, b struct {
		score    int
		userName string
	}) int {
		if a.score > b.score {
			return 1
		}
		return -1
	})

	sb := &strings.Builder{}
	for k, v := range scoreSlice {
		fmt.Fprintf(sb, "#%d %s: %d", k+1, v.userName, v.score)
	}
	return sb.String()
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
	if err := c.openFilmgameForWriting(func(cw *FilmgameState) (*FilmgameState, error) {
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

func (c *Filmgame) openFilmgameForWriting(cb func(cw *FilmgameState) (*FilmgameState, error)) error {
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

	cw, err = cb(cw)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(cw)
}
