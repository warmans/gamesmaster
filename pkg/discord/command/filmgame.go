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
var posterClueRegex = regexp.MustCompile(`[Cc]lue\s([0-9]+)`)
var adminRegex = regexp.MustCompile(`[Aa]dmin\s(.+)`)

const gameDescription = "Guess the film posters by adding a message to the attached thread e.g. `guess 1 fargo`"

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
				if err := c.openFilmgameForReading(func(cw *filmgame.State) error {
					c.answerThreadID = cw.AnswerThreadID
					return nil
				}); err != nil {
					fmt.Println("Failed to get current filmgame answer thread ID: ", err.Error())
					return
				}
			}
			if m.ChannelID == c.answerThreadID {
				clueMatches := posterClueRegex.FindStringSubmatch(m.Content)
				if clueMatches != nil || len(clueMatches) == 2 {
					if err := c.handleRequestClue(s, clueMatches[1], m.ChannelID, m.ID); err != nil {
						fmt.Println("Failed to get clue: ", err.Error())
					}
					return
				}
				if m.Author.Username == ".warmans" {
					adminMatches := adminRegex.FindStringSubmatch(m.Content)
					if adminMatches != nil || len(adminMatches) == 2 {
						if err := c.handleAdminAction(s, adminMatches[1], m.ChannelID, m.ID); err != nil {
							fmt.Println("Admin action failed: ", err.Error())
						}
						return
					}
				}
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
					fmt.Println("Failed to check word: ", err.Error())
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

func (c *Filmgame) handleRequestClue(s *discordgo.Session, clueID string, channelID string, messageID string) error {
	return c.openFilmgameForReading(func(cw *filmgame.State) error {
		numUnsolved := 0
		for _, v := range cw.Posters {
			if !v.Guessed {
				numUnsolved++
			}
		}
		if numUnsolved > 5 {
			if err := s.MessageReactionAdd(channelID, messageID, "👎"); err != nil {
				return err
			}
			return nil
		}
		for k, v := range cw.Posters {
			if fmt.Sprintf("%d", k+1) == clueID {
				if _, err := s.ChannelMessageSend(
					cw.AnswerThreadID,
					fmt.Sprintf("%s starts with: %s", clueID, strings.ToUpper(string(v.Answer[0]))),
				); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (c *Filmgame) handleAdminAction(s *discordgo.Session, action string, channelID string, messageID string) error {
	switch action {
	case "refresh":
		if err := c.openFilmgameForReading(func(cw *filmgame.State) error {
			return c.refreshGameImage(s, cw)
		}); err != nil {
			return err
		}
		return s.MessageReactionAdd(channelID, messageID, "👀")
	default:
		return s.MessageReactionAdd(channelID, messageID, "🤷")
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
	if err := c.openFilmgameForWriting(func(cw *filmgame.State) (*filmgame.State, error) {
		for k, v := range cw.Posters {
			if fmt.Sprintf("%d", k+1) == clueID && strings.EqualFold(simplifyGuess(word), simplifyGuess(v.Answer)) {
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
		err := c.openFilmgameForWriting(func(cw *filmgame.State) (*filmgame.State, error) {

			if err := c.refreshGameImage(s, cw); err != nil {
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

func (c *Filmgame) refreshGameImage(s *discordgo.Session, cw *filmgame.State) error {
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
	return err
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
		if a.score < b.score {
			return 1
		}
		return -1
	})

	sb := &strings.Builder{}
	for k, v := range scoreSlice {
		fmt.Fprintf(sb, "%d. %s: %d\n", k+1, v.userName, v.score)
	}
	return sb.String()
}

func (c *Filmgame) startFilmgame(s *discordgo.Session, i *discordgo.InteractionCreate) error {

	var fgs filmgame.State
	err := c.openFilmgameForReading(func(c *filmgame.State) error {
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
		Name: fmt.Sprintf("%s Answers", fgs.GameTitle),
		Type: discordgo.ChannelTypeGuildPublicThread,
	})
	if err != nil {
		panic(err)
	}
	if err := c.openFilmgameForWriting(func(cw *filmgame.State) (*filmgame.State, error) {
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

func (c *Filmgame) renderBoard(state *filmgame.State) (*bytes.Buffer, error) {
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

func (c *Filmgame) openFilmgameForReading(cb func(cw *filmgame.State) error) error {
	c.gameLock.RLock()
	defer c.gameLock.RUnlock()

	f, err := os.Open("var/filmgame/game/current.json")
	if err != nil {
		return err
	}
	defer f.Close()

	cw := filmgame.State{}
	if err := json.NewDecoder(f).Decode(&cw); err != nil {
		return err
	}

	return cb(&cw)
}

func (c *Filmgame) openFilmgameForWriting(cb func(cw *filmgame.State) (*filmgame.State, error)) error {
	c.gameLock.Lock()
	defer c.gameLock.Unlock()

	f, err := os.OpenFile("var/filmgame/game/current.json", os.O_RDWR|os.O_EXCL, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	cw := &filmgame.State{}
	if err := json.NewDecoder(f).Decode(cw); err != nil {
		return err
	}

	cw, err = cb(cw)
	if err != nil {
		return err
	}

	if err := f.Truncate(0); err != nil {
		// allow recovery of file contents from logs
		fmt.Println("DUMPING STATE...")
		if encerr := json.NewEncoder(os.Stderr).Encode(cw); encerr != nil {
			fmt.Println("ERROR: ", err.Error())
		}
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		fmt.Println("DUMPING STATE...")
		if encerr := json.NewEncoder(os.Stderr).Encode(cw); encerr != nil {
			fmt.Println("ERROR: ", err.Error())
		}
		return err
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cw); err != nil {
		fmt.Println("DUMPING STATE...")
		if encerr := json.NewEncoder(os.Stderr).Encode(cw); encerr != nil {
			fmt.Println("ERROR: ", err.Error())
		}
		return err
	}
	return nil
}

func simplifyGuess(guess string) string {
	return trimAllPrefix(guess, "a ", "the ")
}

func trimAllPrefix(str string, trim ...string) string {
	str = strings.TrimSpace(str)
	for _, v := range trim {
		str = strings.TrimSpace(strings.TrimPrefix(str, v))
	}
	return str
}
