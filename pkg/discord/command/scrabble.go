package command

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/warmans/gamesmaster/pkg/discord"
	"github.com/warmans/gamesmaster/pkg/util"
	"github.com/warmans/go-scrabble"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

const scrabbleDescription string = `
[todo]

__COMMANDS__
1. [A|D][CELL ID] [WORD] - Place a word A (across) or D (down), starting at [CELL ID]. Note that if you are overlapping an existing word you must specify the whole word including the existing letters on the board.

__NOTES__
- The '_' character is the blank letter that may stand in for any letter. Just include the letter you want in the word when you submit it and it will remove your _ tile.
- The game end whens when all players have run out of tiles.
`

var submissionRegex = regexp.MustCompile(`([AD][0-9]+)\s([a-zA-Z]+)`)
var validLetters = regexp.MustCompile(`^[A-Za-z]+$`)

type ScrabbleState struct {
	OriginalMessageID      string
	OriginalMessageChannel string
	AnswerThreadID         string
	Game                   *scrabble.Scrabulous
	RoleIDMap              map[string]string
}

func (s *ScrabbleState) roleIDFromName(wantName string) string {
	for name, id := range s.RoleIDMap {
		if wantName == name {
			return id
		}
	}
	return ""
}

const (
	scrabbleCommand = "scrabble"
)

const (
	scrabbleCmdStart string = "start"
)

func NewScrabbleCommand(globalSession *discordgo.Session, wordsFilePath string) (*Scrabble, error) {
	words, err := os.Open(wordsFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open words file: %w", err)
	}
	defer words.Close()

	dict := make(map[string]struct{})
	scanner := bufio.NewScanner(words)
	for scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to scan dictionary: %w", err)
		}
		if validLetters.Match(scanner.Bytes()) {
			dict[strings.ToUpper(scanner.Text())] = struct{}{}
		}
	}
	sc := &Scrabble{globalSession: globalSession, dict: dict}
	go sc.runBackgroundTasks()
	return sc, nil
}

type Scrabble struct {
	gameLock       sync.RWMutex
	answerThreadID string
	globalSession  *discordgo.Session
	dict           map[string]struct{}
}

func (c *Scrabble) Prefix() string {
	return "scr"
}

func (c *Scrabble) RootCommand() string {
	return scrabbleCommand
}

func (c *Scrabble) Description() string {
	return "Scrabble game"
}

func (c *Scrabble) AutoCompleteHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{}
}

func (c *Scrabble) ButtonHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{}
}

func (c *Scrabble) ModalHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{}
}

func (c *Scrabble) CommandHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{
		scrabbleCmdStart: c.startScrabble,
	}
}

func (c *Scrabble) MessageHandlers() discord.MessageHandlers {
	return discord.MessageHandlers{
		func(s *discordgo.Session, m *discordgo.MessageCreate) {
			if m.Flags == discordgo.MessageFlagsEphemeral {
				return
			}
			complete := false
			if c.answerThreadID == "" {
				if err := c.openScrabbleForReading(m.GuildID, func(cw *ScrabbleState) error {
					fmt.Println("Message for thread: ", cw.AnswerThreadID)
					c.answerThreadID = cw.AnswerThreadID
					complete = cw.Game.Complete
					return nil
				}); err != nil {
					fmt.Println("Failed to get current scrabble answer thread ID: ", err.Error())
					return
				}
			}
			if m.ChannelID == c.answerThreadID && !complete {

				// commands are like :skip, :complete
				if strings.HasPrefix(m.Content, ":") {
					ok, err := c.handleTextCommand(s, m.Content, m)
					if err != nil {
						fmt.Println("Failed to handle command: ", err.Error())
						if err := s.MessageReactionAdd(m.ChannelID, m.ID, "🔥"); err != nil {
							fmt.Println("Failed to add reaction: ", err.Error())
							return
						}
					}
					if ok {
						if err := s.MessageReactionAdd(m.ChannelID, m.ID, "👍"); err != nil {
							fmt.Println("Failed to add reaction: ", err.Error())
							return
						}
					}
					return
				}

				matches := submissionRegex.FindStringSubmatch(m.Content)
				if matches == nil || len(matches) != 3 {
					return
				}

				if err := c.handleCheckWordSubmission(
					s,
					m.GuildID,
					strings.ToUpper(strings.TrimSpace(matches[1])),
					strings.ToUpper(strings.TrimSpace(matches[2])),
					m.ChannelID,
					m.ID,
					m.Author,
				); err != nil {
					fmt.Println("Failed to check word: ", err.Error())
					return
				}
			}
		},
	}
}

func (c *Scrabble) runBackgroundTasks() {
	entries, err := os.ReadDir("var/scrabble")
	if err != nil {
		fmt.Printf("Failed to list games: %s\n", err.Error())
	}
	for _, v := range entries {
		if v.IsDir() || !strings.HasSuffix(v.Name(), ".json") {
			continue
		}
		guildID := strings.TrimSuffix(v.Name(), ".json")
		go c.runBackgroundTask(guildID)
	}
}

func (c *Scrabble) handleTextCommand(s *discordgo.Session, command string, m *discordgo.MessageCreate) (bool, error) {
	fmt.Println("handling text command ", command)
	switch command {
	case ":refresh":
		err := c.openScrabbleForWriting(m.GuildID, func(cw *ScrabbleState) (*ScrabbleState, error) {
			return cw, cw.Game.TryPlacePendingWord()
		})
		if err != nil {
			return false, err
		}
		return true, c.refreshGameImage(s, m.GuildID)
	case ":complete":
		// todo: should probably check for the moderator role
		if m.Author.Username != ".warmans" {
			return false, nil
		}
		err := c.openScrabbleForWriting(m.GuildID, func(cw *ScrabbleState) (*ScrabbleState, error) {
			cw.Game.Complete = true
			return cw, nil
		})
		if err != nil {
			return false, err
		}
		return true, c.completeGame(m.GuildID)
	}
	return false, nil
}

func (c *Scrabble) SubCommands() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Name:        scrabbleCmdStart,
			Description: "Start the game (if available).",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
		},
	}
}

func (c *Scrabble) handleCheckWordSubmission(
	s *discordgo.Session,
	guildID string,
	placementStr string,
	word string,
	channelID string,
	messageID string,
	member *discordgo.User,
) error {

	placement, err := scrabble.ParsePlacement(placementStr)
	if err != nil {
		if err := s.MessageReactionAdd(channelID, messageID, "🔥"); err != nil {
			return err
		}
		return nil
	}

	if _, ok := c.dict[word]; !ok {
		if err := s.MessageReactionAdd(channelID, messageID, "👎"); err != nil {
			return err
		}
		return nil
	}

	isAllowedPlayer := true
	err = c.openScrabbleForWriting(guildID, func(cw *ScrabbleState) (*ScrabbleState, error) {
		isAllowedPlayer = cw.Game.IsPlayerAllowed(member.Username)
		return cw, nil
	})
	if err != nil {
		return err
	}

	if !isAllowedPlayer && os.Getenv("DEV") != "true" {
		if err := s.MessageReactionAdd(channelID, messageID, "🙅‍♂️"); err != nil {
			return err
		}
		return nil
	}

	var gameComplete bool = false
	var isFirstPendingWord = false
	err = c.openScrabbleForWriting(guildID, func(sc *ScrabbleState) (*ScrabbleState, error) {

		if len(sc.Game.PendingWords) == 0 {
			isFirstPendingWord = true
		}
		if err := sc.Game.CreatePendingWord(placement, word, member.Username); err != nil {
			if err := s.MessageReactionAdd(channelID, messageID, "❌"); err != nil {
				return sc, err
			}
			return sc, err
		}
		canvas, err := scrabble.RenderScrabulousPNG(sc.Game, 1500, 1000)
		if err != nil {
			return sc, err
		}
		buff := &bytes.Buffer{}
		if err := canvas.EncodePNG(buff); err != nil {
			return sc, err
		}

		_, err = s.ChannelMessageEditComplex(
			&discordgo.MessageEdit{
				Channel: sc.OriginalMessageChannel,
				ID:      sc.OriginalMessageID,
				Content: util.ToPtr(scrabbleDescription),
				Files: []*discordgo.File{
					{
						Name:        "board.png",
						ContentType: "images/png",
						Reader:      buff,
					},
				},
				Attachments: util.ToPtr([]*discordgo.MessageAttachment{}),
			},
		)
		if err != nil {
			return sc, err
		}
		gameComplete = sc.Game.Complete
		return sc, nil
	})
	if err != nil {
		return err
	}

	if isFirstPendingWord {
		go c.runBackgroundTask(guildID)
	}

	// best effort
	if err := s.MessageReactionAdd(channelID, messageID, "✅"); err != nil {
		fmt.Println("failed to add reaction  ", err.Error())
	}

	if gameComplete {
		return c.completeGame(guildID)
	}

	return nil
}

func (c *Scrabble) runBackgroundTask(guildID string) {
	for {
		var stealComplete bool = false
		var nextRefresh time.Duration
		fmt.Println("Running background task")
		if err := c.openScrabbleForWriting(guildID, func(cw *ScrabbleState) (*ScrabbleState, error) {
			if err := cw.Game.TryPlacePendingWord(); err != nil {
				return nil, err
			}
			if cw.Game.GameState == scrabble.StateStealing {
				var myNextRefresh time.Duration
				// overdue
				if time.Until(*cw.Game.PlaceWordAt) < 0 {
					myNextRefresh = time.Second
				} else {
					// some time in the future
					if time.Until(*cw.Game.PlaceWordAt) > time.Minute {
						myNextRefresh = time.Minute
					} else {
						myNextRefresh = time.Until(*cw.Game.PlaceWordAt)
					}
				}
				if nextRefresh == 0 || nextRefresh > myNextRefresh {
					nextRefresh = myNextRefresh
				}
			} else {
				stealComplete = true
			}
			return cw, nil
		}); err != nil {
			fmt.Println("failed to get game state ", err.Error())
			continue
		}
		if stealComplete {
			if err := c.refreshGameImage(c.globalSession, guildID); err != nil {
				fmt.Println("failed to refresh game image ", err.Error())
			}
			fmt.Println("Steal complete")
			return
		}
		fmt.Println("Next refresh ", nextRefresh.String())
		time.Sleep(nextRefresh)
		if err := c.refreshGameImage(c.globalSession, guildID); err != nil {
			fmt.Println("failed to refresh game image ", err.Error())
		}
	}
}

func (c *Scrabble) startScrabble(s *discordgo.Session, i *discordgo.InteractionCreate) error {

	if err := c.createGameIfNoneExists(i.GuildID); err != nil {
		return err
	}

	var cw ScrabbleState
	err := c.openScrabbleForReading(i.GuildID, func(c *ScrabbleState) error {
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
	canvas, err := scrabble.RenderScrabulousPNG(cw.Game, 1500, 1000)
	if err != nil {
		return err
	}
	buff := &bytes.Buffer{}
	if err := canvas.EncodePNG(buff); err != nil {
		return err
	}

	initialMessage, err := s.ChannelMessageSendComplex(i.ChannelID, &discordgo.MessageSend{
		Content: scrabbleDescription,
		Files: []*discordgo.File{
			{
				Name:        "board.png",
				ContentType: "images/png",
				Reader:      buff,
			},
		},
	})
	if err != nil {
		fmt.Printf("Failed to start game: %s\n", err.Error())
		return err
	}

	thread, err := s.MessageThreadStartComplex(initialMessage.ChannelID, initialMessage.ID, &discordgo.ThreadStart{
		Name: "Absolutely Scrabulous",
		Type: discordgo.ChannelTypeGuildPublicThread,
	})
	if err != nil {
		panic(err)
	}
	if err := c.openScrabbleForWriting(i.GuildID, func(cw *ScrabbleState) (*ScrabbleState, error) {
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

func (c *Scrabble) createGameIfNoneExists(guildID string) error {

	_, err := os.Stat(fmt.Sprintf("var/scrabble/%s.json", guildID))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		return nil
	}
	f, err := os.Create(fmt.Sprintf("var/scrabble/%s.json", guildID))
	if err != nil {
		return err
	}

	cw := ScrabbleState{
		Game:      scrabble.NewScrabulousGame(time.Minute * 5),
		RoleIDMap: make(map[string]string),
	}

	cw.Game.ResetLetters()

	if err := json.NewEncoder(f).Encode(cw); err != nil {
		return err
	}
	return nil
}

func (c *Scrabble) openScrabbleForReading(guildID string, cb func(cw *ScrabbleState) error) error {
	c.gameLock.RLock()
	defer c.gameLock.RUnlock()

	f, err := os.Open(fmt.Sprintf("var/scrabble/%s.json", guildID))
	if err != nil {
		return err
	}
	defer f.Close()

	cw := ScrabbleState{}
	if err := json.NewDecoder(f).Decode(&cw); err != nil {
		return err
	}

	return cb(&cw)
}

func (c *Scrabble) openScrabbleForWriting(guildID string, cb func(cw *ScrabbleState) (*ScrabbleState, error)) error {
	c.gameLock.Lock()
	defer c.gameLock.Unlock()

	f, err := os.OpenFile(fmt.Sprintf("var/scrabble/%s.json", guildID), os.O_RDWR|os.O_EXCL, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	cw := &ScrabbleState{}
	if err := json.NewDecoder(f).Decode(cw); err != nil {
		return err
	}
	cw, err = cb(cw)
	if err != nil || cw == nil {
		return err
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

func (c *Scrabble) completeGame(guildId string) error {
	var winner *scrabble.Score
	err := c.openScrabbleForReading(guildId, func(cw *ScrabbleState) error {
		for _, p := range cw.Game.GetScores() {
			if winner == nil || p.Score > winner.Score {
				winner = p
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	return c.sendThreadMessage(guildId, fmt.Sprintf("GAME COMPLETE, %s WINS", winner.PlayerName))
}

func (c *Scrabble) sendThreadMessage(guildId string, message string) error {
	return c.openScrabbleForReading(guildId, func(cw *ScrabbleState) error {
		if _, err := c.globalSession.ChannelMessageSend(
			cw.AnswerThreadID,
			message,
		); err != nil {
			return err
		}

		return nil
	})
}

func (c *Scrabble) refreshGameImage(s *discordgo.Session, guildID string) error {
	return c.openScrabbleForWriting(guildID, func(sc *ScrabbleState) (*ScrabbleState, error) {

		canvas, err := scrabble.RenderScrabulousPNG(sc.Game, 1500, 1000)
		if err != nil {
			return sc, err
		}
		buff := &bytes.Buffer{}
		if err := canvas.EncodePNG(buff); err != nil {
			return sc, err
		}

		_, err = s.ChannelMessageEditComplex(
			&discordgo.MessageEdit{
				Channel: sc.OriginalMessageChannel,
				ID:      sc.OriginalMessageID,
				Content: util.ToPtr(scrabbleDescription),
				Files: []*discordgo.File{
					{
						Name:        "board.png",
						ContentType: "images/png",
						Reader:      buff,
					},
				},
				Attachments: util.ToPtr([]*discordgo.MessageAttachment{}),
			},
		)
		if err != nil {
			return sc, err
		}

		return sc, nil
	})
}
