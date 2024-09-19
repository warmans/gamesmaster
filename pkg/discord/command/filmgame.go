package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/warmans/gamesmaster/pkg/discord"
	"github.com/warmans/gamesmaster/pkg/filmgame"
	"github.com/warmans/gamesmaster/pkg/util"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

var posterGuessRegex = regexp.MustCompile(`[Gg]uess\s([0-9]+)\s(.+)`)
var posterClueRegex = regexp.MustCompile(`[Cc]lue\s([0-9]+)`)
var adminRegex = regexp.MustCompile(`[Aa]dmin\s(.+)`)

const (
	filmgameCommand = "filmgame"
	gameDuration    = time.Hour * 24
)

const (
	FilmgameCmdStart string = "start"
)

func NewFilmgameCommand(logger *slog.Logger, globalSession *discordgo.Session) *Filmgame {
	f := &Filmgame{globalSession: globalSession, logger: logger}
	go func() {
		if err := f.start(); err != nil {
			panic(err)
		}
	}()
	return f
}

type Filmgame struct {
	logger         *slog.Logger
	globalSession  *discordgo.Session
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

func (c *Filmgame) SubCommands() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Name:        FilmgameCmdStart,
			Description: "Start the game (if available).",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
		},
	}
}

func (c *Filmgame) MessageHandlers() discord.MessageHandlers {
	return discord.MessageHandlers{
		func(s *discordgo.Session, m *discordgo.MessageCreate) {
			if c.answerThreadID == "" {
				if err := c.openFilmgameForReading(func(cw filmgame.State) error {
					c.answerThreadID = cw.AnswerThreadID
					return nil
				}); err != nil {
					c.logger.Error("Failed to get current filmgame answer thread ID", slog.String("err", err.Error()))
					return
				}
			}
			if m.ChannelID == c.answerThreadID {

				// is the message a request for a clue?
				clueMatches := posterClueRegex.FindStringSubmatch(m.Content)
				if clueMatches != nil || len(clueMatches) == 2 {
					if err := c.handleRequestClue(s, clueMatches[1], m.ChannelID, m.ID); err != nil {
						c.logger.Error("Failed to get clue", slog.String("err", err.Error()))
					}
					return
				}

				// is the message an admin command?
				if m.Author.Username == ".warmans" {
					adminMatches := adminRegex.FindStringSubmatch(m.Content)
					if adminMatches != nil || len(adminMatches) == 2 {
						if err := c.handleAdminAction(s, adminMatches[1], m.ChannelID, m.ID); err != nil {
							c.logger.Error("Admin action failed", slog.String("err", err.Error()))
						}
						return
					}
				}

				// is the message a guess?
				guessMatches := posterGuessRegex.FindStringSubmatch(m.Content)
				if guessMatches == nil || len(guessMatches) != 3 {
					return
				}
				if err := c.handleCheckWordSubmission(
					s,
					guessMatches[1],
					guessMatches[2],
					m.ChannelID,
					m.ID,
					m.Author.Username,
				); err != nil {
					c.logger.Error("Failed to check word", slog.String("err", err.Error()))
					return
				}
			}
		},
	}
}

func (c *Filmgame) handleRequestClue(s *discordgo.Session, clueID string, channelID string, messageID string) error {
	cw, err := c.getGameSnapshot()
	if err != nil {
		return err
	}
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
}

func (c *Filmgame) handleAdminAction(s *discordgo.Session, action string, channelID string, messageID string) error {
	switch action {
	case "refresh":
		if err := c.openFilmgameForReading(func(cw filmgame.State) error {
			return c.refreshGameImage(s, cw)
		}); err != nil {
			return err
		}
		return s.MessageReactionAdd(channelID, messageID, "👀")
	case "complete":
		return c.forceCompleteGame("admin action")
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
		gameComplete := false
		err := c.openFilmgameForWriting(func(cw *filmgame.State) (*filmgame.State, error) {
			numCompleted := 0
			for _, v := range cw.Posters {
				if v.Guessed {
					numCompleted++
				}
			}
			if err := c.refreshGameImage(s, *cw); err != nil {
				return cw, err
			}

			if err := s.MessageReactionAdd(channelID, messageID, "✅"); err != nil {
				return cw, err
			}

			// increment scores
			cw.Scores.Add(userName)

			for _, v := range cw.Posters {
				if !v.Guessed {
					return cw, nil
				}
			}
			gameComplete = true
			return cw, nil
		})
		if err != nil {
			return err
		}
		if gameComplete {
			return c.forceCompleteGame("All items have been solved.")
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

func (c *Filmgame) refreshGameImage(s *discordgo.Session, cw filmgame.State) error {
	buff, err := c.renderBoard(cw)
	if err != nil {
		return err
	}

	_, err = s.ChannelMessageEditComplex(
		&discordgo.MessageEdit{
			Channel: cw.OriginalMessageChannel,
			ID:      cw.OriginalMessageID,
			Content: util.ToPtr(gameDescription(gameDuration - time.Since(cw.StartedAt))),
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

func (c *Filmgame) startFilmgame(s *discordgo.Session, i *discordgo.InteractionCreate) error {

	var fgs filmgame.State
	fgs, err := c.getGameSnapshot()
	if err != nil {
		return err
	}
	if fgs.AnswerThreadID != "" {
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Flags:   discordgo.MessageFlagsEphemeral,
				Content: "Game already started",
			},
		})
	}

	board, err := c.renderBoard(fgs)
	if err != nil {
		return err
	}

	initialMessage, err := s.ChannelMessageSendComplex(i.ChannelID, &discordgo.MessageSend{
		Content: gameDescription(gameDuration),
		Files: []*discordgo.File{
			{
				Name:        "Filmgame.png",
				ContentType: "images/png",
				Reader:      board,
			},
		},
	})
	if err != nil {
		c.logger.Error("Failed to start game", slog.String("err", err.Error()))
		return err
	}

	thread, err := s.MessageThreadStartComplex(initialMessage.ChannelID, initialMessage.ID, &discordgo.ThreadStart{
		Name: fmt.Sprintf("%s Answers", fgs.GameTitle),
		Type: discordgo.ChannelTypeGuildPublicThread,
	})
	if err != nil {
		if err := s.ChannelMessageDelete(initialMessage.ChannelID, initialMessage.ID); err != nil {
			c.logger.Error("Failed to initial delete message after failed game start", slog.String("err", err.Error()))
		}
		return err
	}
	if err := c.openFilmgameForWriting(func(cw *filmgame.State) (*filmgame.State, error) {
		cw.AnswerThreadID = thread.ID
		c.answerThreadID = thread.ID

		cw.StartedAt = time.Now()
		cw.OriginalMessageID = initialMessage.ID
		cw.OriginalMessageChannel = initialMessage.ChannelID

		c.logger.Info("Starting game...",
			slog.String("thread_id", cw.AnswerThreadID),
			slog.String("original_message_id", cw.OriginalMessageID),
			slog.String("original_message_channel", cw.OriginalMessageChannel),
		)
		return cw, nil
	}); err != nil {
		c.logger.Error("Failed to store answer thread ID", slog.String("err", err.Error()))
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

func (c *Filmgame) renderBoard(state filmgame.State) (*bytes.Buffer, error) {
	buff := &bytes.Buffer{}
	canvas, err := filmgame.Render("./var/filmgame/game/images", &state)
	if err != nil {
		return nil, err
	}
	if err := canvas.EncodePNG(buff); err != nil {
		return nil, err
	}
	return buff, nil
}

func (c *Filmgame) openFilmgameForReading(cb func(cw filmgame.State) error) error {
	c.gameLock.RLock()
	defer c.gameLock.RUnlock()

	//todo: prefix directory with server ID
	f, err := os.Open("var/filmgame/game/current.json")
	if err != nil {
		return err
	}
	defer f.Close()

	cw := filmgame.State{}
	if err := json.NewDecoder(f).Decode(&cw); err != nil {
		return err
	}

	return cb(cw)
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
		return c.dumpState(cw, err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return c.dumpState(cw, err)
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cw); err != nil {
		return c.dumpState(cw, err)
	}
	return nil
}
func (c *Filmgame) getGameSnapshot() (filmgame.State, error) {
	var snapshot filmgame.State
	err := c.openFilmgameForReading(func(cw filmgame.State) error {
		snapshot = cw
		return nil
	})
	return snapshot, err
}

func (c *Filmgame) dumpState(cw *filmgame.State, err error) error {
	c.logger.Info("Dumping state...")
	if encerr := json.NewEncoder(os.Stderr).Encode(cw); encerr != nil {
		c.logger.Error("failed to dump state", slog.String("err", err.Error()))
	}
	return err
}

func (c *Filmgame) start() error {
	minutely := time.NewTicker(time.Minute)
	hourly := time.NewTicker(time.Hour)
	defer minutely.Stop()
	for {
		select {
		case <-hourly.C:
			if err := c.openFilmgameForReading(func(cw filmgame.State) error {
				return c.refreshGameImage(c.globalSession, cw)
			}); err != nil {
				c.logger.Error("Failed hourly image refresh", slog.String("err", err.Error()))
			}
		case <-minutely.C:
			triggerCompletion := false
			if err := c.openFilmgameForReading(func(cw filmgame.State) error {
				if cw.StartedAt.IsZero() {
					return nil
				}
				unguessed := 0
				for _, v := range cw.Posters {
					if !v.Guessed {
						unguessed++
					}
				}
				if time.Since(cw.StartedAt) >= time.Hour*24 && unguessed > 0 {
					triggerCompletion = true
				}
				return nil
			}); err != nil {
				c.logger.Error("Failed minutely game check", slog.String("err", err.Error()))
			}
			if triggerCompletion {
				if err := c.forceCompleteGame("Ran out of time."); err != nil {
					c.logger.Error("Failed to complete game", slog.String("err", err.Error()))
				}
			}
		}
	}
}

func (c *Filmgame) forceCompleteGame(reason string) error {
	return c.openFilmgameForWriting(func(cw *filmgame.State) (*filmgame.State, error) {
		for k := range cw.Posters {
			cw.Posters[k].Guessed = true
		}
		if _, err := c.globalSession.ChannelMessageSend(
			cw.AnswerThreadID,
			fmt.Sprintf("Game completed in %s!\n%s\n\nScores:\n%s", time.Since(cw.StartedAt), reason, cw.Scores.Render()),
		); err != nil {
			return cw, err
		}
		if err := c.refreshGameImage(c.globalSession, *cw); err != nil {
			return cw, err
		}
		return cw, nil
	})
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

func gameDescription(timeLeft time.Duration) string {
	return fmt.Sprintf(
		"Guess the posters by adding a message to the attached thread e.g. `guess 1 fargo`. You have %s to complete the puzzle.",
		timeLeft.Truncate(time.Minute).String(),
	)
}
