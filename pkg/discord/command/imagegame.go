package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/warmans/gamesmaster/pkg/discord"
	"github.com/warmans/gamesmaster/pkg/imagegame"
	"github.com/warmans/gamesmaster/pkg/util"
	"log/slog"
	"os"
	"path"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	imageGameCommand       = "imagegame"
	imageGameDuration      = time.Hour * 24 * 7
	imageGameClueThreshold = 5
)

const (
	ImageGameCmdStart string = "start"
)

func NewImageGameCommand(logger *slog.Logger, globalSession *discordgo.Session) *ImageGame {
	f := &ImageGame{globalSession: globalSession, logger: logger}
	go func() {
		if err := f.start(); err != nil {
			panic(err)
		}
	}()
	return f
}

type ImageGame struct {
	logger         *slog.Logger
	globalSession  *discordgo.Session
	gameLock       sync.RWMutex
	answerThreadID string
}

func (c *ImageGame) Prefix() string {
	return "flm"
}

func (c *ImageGame) RootCommand() string {
	return imageGameCommand
}

func (c *ImageGame) Description() string {
	return "Image Game"
}

func (c *ImageGame) AutoCompleteHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{}
}

func (c *ImageGame) ButtonHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{}
}

func (c *ImageGame) ModalHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{}
}

func (c *ImageGame) CommandHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{
		ImageGameCmdStart: c.startImageGame,
	}
}

func (c *ImageGame) SubCommands() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Name:        ImageGameCmdStart,
			Description: "Start the game (if available).",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
		},
	}
}

func (c *ImageGame) MessageHandlers() discord.MessageHandlers {
	return discord.MessageHandlers{
		func(s *discordgo.Session, m *discordgo.MessageCreate) {
			if c.answerThreadID == "" {
				if err := c.openImageGameForReading(m.GuildID, func(cw imagegame.State) error {
					c.answerThreadID = cw.AnswerThreadID
					return nil
				}); err != nil {
					c.logger.Error("Failed to get current ImageGame answer thread ID", slog.String("err", err.Error()))
					return
				}
			}
			if m.ChannelID == c.answerThreadID {

				// is the message a request for a clue?
				clueMatches := posterClueRegex.FindStringSubmatch(m.Content)
				if clueMatches != nil || len(clueMatches) == 2 {
					if err := c.handleRequestClue(s, m.GuildID, clueMatches[1], m.ChannelID, m.ID); err != nil {
						c.logger.Error("Failed to get clue", slog.String("err", err.Error()))
					}
					return
				}

				// is the message an admin command?
				if m.Author.Username == ".warmans" {
					adminMatches := adminRegex.FindStringSubmatch(m.Content)
					if adminMatches != nil || len(adminMatches) == 2 {
						if err := c.handleAdminAction(s, adminMatches[1], m.GuildID, m.ChannelID, m.ID); err != nil {
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
					m.GuildID,
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

func (c *ImageGame) handleRequestClue(s *discordgo.Session, guildID string, clueID string, channelID string, messageID string) error {
	cw, err := c.getGameSnapshot(guildID)
	if err != nil {
		return err
	}

	if cw.NumUnsolved() > imageGameClueThreshold {
		if err := s.MessageReactionAdd(channelID, messageID, "👎"); err != nil {
			return err
		}
		return nil
	}
	for k, v := range cw.Posters {
		if fmt.Sprintf("%d", k+1) == clueID {
			if _, err := s.ChannelMessageSend(
				cw.AnswerThreadID,
				c.getClueText(clueID, v.Answer, time.Since(cw.StartedAt)),
			); err != nil {
				return err
			}
			return nil
		}
	}
	return nil
}

func (c *ImageGame) getClueText(clueID string, answer string, gameDuration time.Duration) string {
	//if gameDuration < time.Hour*12 {
	//	return fmt.Sprintf("%s starts with: %s", clueID, strings.ToUpper(string(answer[0])))
	//}
	initials := ""
	for _, w := range strings.Split(answer, " ") {
		if len(w) > 0 && !unicode.IsNumber(rune(w[0])) {
			initials += strings.ToUpper(string(w[0]))
		}
	}
	return fmt.Sprintf("%s initials: %s", clueID, initials)
}

func (c *ImageGame) handleAdminAction(s *discordgo.Session, action string, guildID string, channelID string, messageID string) error {
	switch action {
	case "refresh":
		if err := c.openImageGameForReading(guildID, func(cw imagegame.State) error {
			return c.refreshGameImage(s, cw)
		}); err != nil {
			return err
		}
		return s.MessageReactionAdd(channelID, messageID, "👀")
	case "complete":
		return c.forceCompleteGame(guildID, "admin action")
	default:
		return s.MessageReactionAdd(channelID, messageID, "🤷")
	}
}

func (c *ImageGame) handleCheckWordSubmission(
	s *discordgo.Session,
	guildID string,
	clueID string,
	word string,
	channelID string,
	messageID string,
	userName string,
) error {
	var alreadySolved = false
	var correct = false
	var guessAllowed = true
	var gameComplete = true

	if err := c.openImageGameForWriting(guildID, func(cw *imagegame.State) (*imagegame.State, error) {

		if cw.Cfg.RequireAlternatingUsers && cw.Scores.LastUser == userName && cw.NumUnsolved() > 3 {
			// don't let the same user answer many in a row
			guessAllowed = false
			// return immediately if the guess isn't allowed
			return cw, nil
		}

		// check if the answer is correct (and if the game is complete)
		for k, v := range cw.Posters {
			if fmt.Sprintf("%d", k+1) == clueID && util.GuessRoughlyMatchesAnswer(word, v.Answer) {
				if v.Guessed {
					alreadySolved = true
					return cw, nil
				}
				cw.Posters[k].Guessed = true
				correct = true
			}
			// check if any are unguessed
			if !cw.Posters[k].Guessed {
				gameComplete = false
			}
		}
		if correct {
			// increment scores
			cw.Scores.Add(userName)
		}
		return cw, nil
	}); err != nil {
		return err
	}

	if !guessAllowed {
		if err := s.MessageReactionAdd(channelID, messageID, "🙅‍♂️"); err != nil {
			return err
		}
		return nil
	}

	if correct {
		if err := s.MessageReactionAdd(channelID, messageID, "✅"); err != nil {
			return err
		}
		err := c.openImageGameForReading(guildID, func(cw imagegame.State) error {
			if err := c.refreshGameImage(s, cw); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return err
		}
		if gameComplete {
			return c.forceCompleteGame(guildID, "All items have been solved.")
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

func (c *ImageGame) refreshGameImage(s *discordgo.Session, cw imagegame.State) error {
	buff, err := c.renderBoard(cw)
	if err != nil {
		return err
	}

	_, err = s.ChannelMessageEditComplex(
		&discordgo.MessageEdit{
			Channel: cw.OriginalMessageChannel,
			ID:      cw.OriginalMessageID,
			Content: util.ToPtr(imageGameDescription(imageGameDuration-time.Since(cw.StartedAt), cw.Cfg.RequireAlternatingUsers)),
			Files: []*discordgo.File{
				{
					Name:        "imagegame.png",
					ContentType: "images/png",
					Reader:      buff,
				},
			},
			Attachments: util.ToPtr([]*discordgo.MessageAttachment{}),
		},
	)
	return err
}

func (c *ImageGame) startImageGame(s *discordgo.Session, i *discordgo.InteractionCreate) error {

	var gameState imagegame.State
	gameState, err := c.getGameSnapshot(i.GuildID)
	if err != nil {
		return err
	}
	if gameState.AnswerThreadID != "" {
		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Flags:   discordgo.MessageFlagsEphemeral,
				Content: "Game already started",
			},
		})
	}

	board, err := c.renderBoard(gameState)
	if err != nil {
		return err
	}

	initialMessage, err := s.ChannelMessageSendComplex(i.ChannelID, &discordgo.MessageSend{
		Content: imageGameDescription(imageGameDuration, gameState.Cfg.RequireAlternatingUsers),
		Files: []*discordgo.File{
			{
				Name:        "imagegame.png",
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
		Name: gameState.GameTitle,
		Type: discordgo.ChannelTypeGuildPublicThread,
	})
	if err != nil {
		if err := s.ChannelMessageDelete(initialMessage.ChannelID, initialMessage.ID); err != nil {
			c.logger.Error("Failed to initial delete message after failed game start", slog.String("err", err.Error()))
		}
		return err
	}
	if err := c.openImageGameForWriting(i.GuildID, func(cw *imagegame.State) (*imagegame.State, error) {
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

func (c *ImageGame) renderBoard(state imagegame.State) (*bytes.Buffer, error) {
	buff := &bytes.Buffer{}
	canvas, err := imagegame.Render("./var/imagegame/game/images", &state)
	if err != nil {
		return nil, err
	}
	if err := canvas.EncodePNG(buff); err != nil {
		return nil, err
	}
	return buff, nil
}

func (c *ImageGame) openImageGameForReading(guildID string, cb func(cw imagegame.State) error) error {
	c.gameLock.RLock()
	defer c.gameLock.RUnlock()

	f, err := os.Open(fmt.Sprintf("var/imagegame/game/%s.json", guildID))
	if err != nil {
		return err
	}
	defer f.Close()

	cw := imagegame.State{}
	if err := json.NewDecoder(f).Decode(&cw); err != nil {
		return err
	}

	return cb(cw)
}

func (c *ImageGame) openImageGameForWriting(guildID string, cb func(cw *imagegame.State) (*imagegame.State, error)) error {
	c.gameLock.Lock()
	defer c.gameLock.Unlock()

	f, err := os.OpenFile(path.Join(c.gameDir(), fmt.Sprintf("%s.json", guildID)), os.O_RDWR|os.O_EXCL, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	cw := &imagegame.State{}
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
func (c *ImageGame) getGameSnapshot(guildID string) (imagegame.State, error) {
	var snapshot imagegame.State
	err := c.openImageGameForReading(guildID, func(cw imagegame.State) error {
		snapshot = cw
		return nil
	})
	return snapshot, err
}

func (c *ImageGame) dumpState(cw *imagegame.State, err error) error {
	c.logger.Info("Dumping state...")
	if encerr := json.NewEncoder(os.Stderr).Encode(cw); encerr != nil {
		c.logger.Error("failed to dump state", slog.String("err", err.Error()))
	}
	return err
}

func (c *ImageGame) gameDir() string {
	return "var/imagegame/game"
}

func (c *ImageGame) activeGuildIDs() ([]string, error) {

	entries, err := os.ReadDir(c.gameDir())
	if err != nil {
		return nil, err
	}
	out := make([]string, 0)
	for _, v := range entries {
		if v.IsDir() || !strings.HasSuffix(v.Name(), ".json") {
			continue
		}
		out = append(out, strings.TrimSuffix(v.Name(), ".json"))
	}
	return out, nil
}

func (c *ImageGame) start() error {
	minutely := time.NewTicker(time.Minute)
	hourly := time.NewTicker(time.Hour)
	defer minutely.Stop()
	for {
		select {
		case <-hourly.C:
			activeGuilds, err := c.activeGuildIDs()
			if err != nil {
				c.logger.Error("Failed to get active guilds", slog.String("err", err.Error()))
				continue
			}
			for _, guildID := range activeGuilds {
				if err := c.openImageGameForReading(guildID, func(cw imagegame.State) error {
					return c.refreshGameImage(c.globalSession, cw)
				}); err != nil {
					c.logger.Error("Failed hourly image refresh", slog.String("err", err.Error()))
				}
			}

		case <-minutely.C:
			activeGuilds, err := c.activeGuildIDs()
			if err != nil {
				c.logger.Error("Failed to get active guilds", slog.String("err", err.Error()))
				continue
			}
			for _, guildID := range activeGuilds {
				triggerCompletion := false
				if err := c.openImageGameForReading(guildID, func(cw imagegame.State) error {
					if cw.StartedAt.IsZero() {
						return nil
					}
					unguessed := 0
					for _, v := range cw.Posters {
						if !v.Guessed {
							unguessed++
						}
					}
					if time.Since(cw.StartedAt) >= imageGameDuration && unguessed > 0 {
						triggerCompletion = true
					}
					return nil
				}); err != nil {
					c.logger.Error("Failed minutely game check", slog.String("err", err.Error()))
				}
				if triggerCompletion {
					if err := c.forceCompleteGame(guildID, "Ran out of time."); err != nil {
						c.logger.Error("Failed to complete game", slog.String("err", err.Error()))
					}
				}
			}
		}
	}
}

func (c *ImageGame) forceCompleteGame(guildID, reason string) error {
	return c.openImageGameForWriting(guildID, func(cw *imagegame.State) (*imagegame.State, error) {
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

func imageGameDescription(timeLeft time.Duration, requireAlternatingUsers bool) string {
	extraRulesText := ""
	if requireAlternatingUsers {
		extraRulesText = "\nExtra rules: Guessing must alternate between users. You cannot submit multiple guesses in a row." +
			" The bot will respond :man_gesturing_no: if you guess while not allowed.\n\n"
	}
	return fmt.Sprintf(
		"Guess the posters by adding a message to the attached thread: \n"+
			"- `guess` e.g. `guess 1 fargo` - submit an answer. \n"+
			"- `clue` e.g. `clue 1` - get a clue about the panel (only available for the final %d panels). \n\n"+
			"You have %s remaining to complete the puzzle.\n%s",
		imageGameClueThreshold,
		timeLeft.Truncate(time.Minute).String(),
		extraRulesText,
	)
}
