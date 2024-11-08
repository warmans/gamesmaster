package crossfilm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/warmans/gamesmaster/pkg/crossfilm"
	"github.com/warmans/gamesmaster/pkg/discord"
	"github.com/warmans/gamesmaster/pkg/util"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

var posterGuessRegex = regexp.MustCompile(`[Gg]uess\s([ADad0-9]+)\s(.+)`)
var whitespaceRegex = regexp.MustCompile(`\s+`)

const (
	crossfilmCommand = "crossfilm"
	gameDuration     = time.Hour * 24
)

const (
	crossfilmCmdStart string = "start"
)

func NewCrossfilmCommand(logger *slog.Logger, globalSession *discordgo.Session) *Crossfilm {
	f := &Crossfilm{globalSession: globalSession, logger: logger}
	go func() {
		if err := f.start(); err != nil {
			panic(err)
		}
	}()
	return f
}

type Crossfilm struct {
	logger         *slog.Logger
	globalSession  *discordgo.Session
	gameLock       sync.RWMutex
	answerThreadID string
}

func (c *Crossfilm) Prefix() string {
	return "flm"
}

func (c *Crossfilm) RootCommand() string {
	return crossfilmCommand
}

func (c *Crossfilm) Description() string {
	return "Crossword/Filmposter game"
}

func (c *Crossfilm) AutoCompleteHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{}
}

func (c *Crossfilm) ButtonHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{}
}

func (c *Crossfilm) ModalHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{}
}

func (c *Crossfilm) CommandHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{
		crossfilmCmdStart: c.startcrossfilm,
	}
}

func (c *Crossfilm) SubCommands() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Name:        crossfilmCmdStart,
			Description: "Start the game (if available).",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
		},
	}
}

func (c *Crossfilm) MessageHandlers() discord.MessageHandlers {
	return discord.MessageHandlers{
		func(s *discordgo.Session, m *discordgo.MessageCreate) {
			if c.answerThreadID == "" {
				if err := c.opencrossfilmForReading(func(cw crossfilm.State) error {
					c.answerThreadID = cw.AnswerThreadID
					return nil
				}); err != nil {
					c.logger.Error("Failed to get current crossfilm answer thread ID", slog.String("err", err.Error()))
					return
				}
			}
			if m.ChannelID == c.answerThreadID {
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

func (c *Crossfilm) handleCheckWordSubmission(
	s *discordgo.Session,
	clueID string,
	word string,
	channelID string,
	messageID string,
	userName string,
) error {
	var alreadySolved = false
	var correct = false
	if err := c.opencrossfilmForWriting(func(cw *crossfilm.State) (*crossfilm.State, error) {
		wordId := strings.TrimLeft(clueID, "AD")
		for k, v := range cw.FilmgameState {
			if fmt.Sprintf("%d", k+1) == wordId && strings.EqualFold(simplifyGuess(word), simplifyGuess(v.Answer)) {
				if v.Guessed {
					alreadySolved = true
					break
				}
				cw.FilmgameState[k].Guessed = true
				correct = true

				//update cw state
				for k, v := range cw.CrosswordState.Words {
					// use the label to avoid having to strip spaces from the crossword answers
					if *v.Word.Label == wordId {
						cw.CrosswordState.Words[k].Solved = true
					}
				}
				return cw, nil
			}
		}
		return cw, nil
	}); err != nil {
		return err
	}

	if correct {
		if err := s.MessageReactionAdd(channelID, messageID, "✅"); err != nil {
			return err
		}
		gameComplete := false
		err := c.opencrossfilmForWriting(func(cw *crossfilm.State) (*crossfilm.State, error) {

			if err := c.refreshGameImage(s, *cw); err != nil {
				return cw, err
			}

			numCompleted := 0
			for _, v := range cw.FilmgameState {
				if v.Guessed {
					numCompleted++
				}
			}
			if numCompleted == len(cw.FilmgameState) {
				gameComplete = true
			}

			// increment scores
			cw.Scores.Add(userName)

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

func (c *Crossfilm) refreshGameImage(s *discordgo.Session, cw crossfilm.State) error {
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
					Name:        "Crossfilm.png",
					ContentType: "images/png",
					Reader:      buff,
				},
			},
			Attachments: util.ToPtr([]*discordgo.MessageAttachment{}),
		},
	)
	return err
}

func (c *Crossfilm) startcrossfilm(s *discordgo.Session, i *discordgo.InteractionCreate) error {

	var fgs crossfilm.State
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
				Name:        "Crossfilm.png",
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
	if err := c.opencrossfilmForWriting(func(cw *crossfilm.State) (*crossfilm.State, error) {
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

func (c *Crossfilm) renderBoard(state crossfilm.State) (*bytes.Buffer, error) {
	buff := &bytes.Buffer{}
	canvas, err := crossfilm.Render("./var/crossfilm/game/images", state)
	if err != nil {
		return nil, err
	}
	if err := canvas.EncodePNG(buff); err != nil {
		return nil, err
	}
	return buff, nil
}

func (c *Crossfilm) opencrossfilmForReading(cb func(cw crossfilm.State) error) error {
	c.gameLock.RLock()
	defer c.gameLock.RUnlock()

	f, err := os.Open("var/crossfilm/game/current.json")
	if err != nil {
		return err
	}
	defer f.Close()

	cw := crossfilm.State{}
	if err := json.NewDecoder(f).Decode(&cw); err != nil {
		return err
	}

	return cb(cw)
}

func (c *Crossfilm) opencrossfilmForWriting(cb func(cw *crossfilm.State) (*crossfilm.State, error)) error {
	c.gameLock.Lock()
	defer c.gameLock.Unlock()

	f, err := os.OpenFile("var/crossfilm/game/current.json", os.O_RDWR|os.O_EXCL, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	cw := &crossfilm.State{}
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
func (c *Crossfilm) getGameSnapshot() (crossfilm.State, error) {
	var snapshot crossfilm.State
	err := c.opencrossfilmForReading(func(cw crossfilm.State) error {
		snapshot = cw
		return nil
	})
	return snapshot, err
}

func (c *Crossfilm) dumpState(cw *crossfilm.State, err error) error {
	c.logger.Info("Dumping state...")
	if encerr := json.NewEncoder(os.Stderr).Encode(cw); encerr != nil {
		c.logger.Error("failed to dump state", slog.String("err", err.Error()))
	}
	return err
}

func (c *Crossfilm) start() error {
	minutely := time.NewTicker(time.Minute)
	hourly := time.NewTicker(time.Hour)
	defer minutely.Stop()
	for {
		select {
		case <-hourly.C:
			if err := c.opencrossfilmForReading(func(cw crossfilm.State) error {
				return c.refreshGameImage(c.globalSession, cw)
			}); err != nil {
				c.logger.Error("Failed hourly image refresh", slog.String("err", err.Error()))
			}
		case <-minutely.C:
			triggerCompletion := false
			if err := c.opencrossfilmForReading(func(cw crossfilm.State) error {
				if cw.StartedAt.IsZero() {
					return nil
				}
				unguessed := 0
				for _, v := range cw.FilmgameState {
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

func (c *Crossfilm) forceCompleteGame(reason string) error {
	return c.opencrossfilmForWriting(func(cw *crossfilm.State) (*crossfilm.State, error) {
		for k := range cw.FilmgameState {
			cw.FilmgameState[k].Guessed = true
		}
		for k := range cw.CrosswordState.Words {
			cw.CrosswordState.Words[k].Solved = true
		}
		if _, err := c.globalSession.ChannelMessageSend(
			cw.AnswerThreadID,
			fmt.Sprintf("Game completed in %s!\n%s\n%s\n", time.Since(cw.StartedAt).Truncate(time.Minute), reason, cw.Scores.Render()),
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
	return whitespaceRegex.ReplaceAllString(trimAllPrefix(guess, "a ", "the "), "")
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
