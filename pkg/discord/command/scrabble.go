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
	"math/rand"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
)

const scrabbleDescription string = `
Any member of the role that is currently active can submit a word or skip.

__COMMANDS__
1. /gamesmaster scrabble letters - show your role's letters
2. :skip - skip your role's turn 
3. [A|D][CELL ID] [WORD] - Place a word A (across) or D (down), starting at [CELL ID]. Note that if you are overlapping an existing word you must specify the whole word including the existing letters on the board.

__NOTES__
- The '_' character is the blank letter that may stand in for any letter.
- The game end whens when all players have run out of tiles.
`

var submissionRegex = regexp.MustCompile(`([AD][0-9]+)\s([a-zA-Z]+)`)
var validLetters = regexp.MustCompile(`^[A-Za-z]+$`)

type ScrabbleState struct {
	OriginalMessageID      string
	OriginalMessageChannel string
	AnswerThreadID         string
	Game                   *scrabble.Game
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
	scrabbleCmdStart   string = "start"
	scrabbleCmdLetters string = "letters"
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
	return &Scrabble{globalSession: globalSession, dict: dict}, nil
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
		scrabbleCmdStart:   c.startScrabble,
		scrabbleCmdLetters: c.showLetters,
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
					if err := c.handleTextCommand(s, m.Content, m); err != nil {
						fmt.Println("Failed to handle command: ", err.Error())
						if err := s.MessageReactionAdd(m.ChannelID, m.ID, "🔥"); err != nil {
							fmt.Println("Failed to add reaction: ", err.Error())
							return
						}
					}
					if err := s.MessageReactionAdd(m.ChannelID, m.ID, "👍"); err != nil {
						fmt.Println("Failed to add reaction: ", err.Error())
						return
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
					m.Member,
				); err != nil {
					fmt.Println("Failed to check word: ", err.Error())
					return
				}
			}
		},
	}
}

func (c *Scrabble) handleTextCommand(s *discordgo.Session, command string, m *discordgo.MessageCreate) error {

	isCurrentPlayer := false
	err := c.openScrabbleForReading(m.GuildID, func(cw *ScrabbleState) error {
		currentPlayer, err := cw.Game.GetCurrentPlayer()
		if err != nil {
			return err
		}
		if slices.Contains(m.Member.Roles, cw.roleIDFromName(fmt.Sprintf("%s:%s", m.GuildID, currentPlayer.Name))) {
			isCurrentPlayer = true
		}
		return nil
	})
	if err != nil {
		return err
	}

	switch command {
	case ":skip":
		if !isCurrentPlayer {
			return nil
		}
		err := c.openScrabbleForWriting(m.GuildID, func(cw *ScrabbleState) (*ScrabbleState, error) {
			cw.Game.NextPlayer()
			return cw, nil
		})
		if err != nil {
			return err
		}
		return c.refreshGameImage(s, m.GuildID)
	case ":complete":
		// todo: should probably check for the moderator role
		if m.Member.User.Username != "warmans" {
			return nil
		}
		err := c.openScrabbleForWriting(m.GuildID, func(cw *ScrabbleState) (*ScrabbleState, error) {
			cw.Game.Complete = true
			return cw, nil
		})
		if err != nil {
			return err
		}
		return c.completeGame(m.GuildID)
	}
	return nil
}

func (c *Scrabble) SubCommands() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Name:        scrabbleCmdStart,
			Description: "Start the game (if available).",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
		},
		{
			Name:        scrabbleCmdLetters,
			Description: "Show your team's letters",
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
	member *discordgo.Member,
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

	isCurrentPlayer := false
	correct := false
	err = c.openScrabbleForWriting(guildID, func(cw *ScrabbleState) (*ScrabbleState, error) {

		// nothing to do
		if cw.Game.Complete {
			return cw, nil
		}

		currentPlayer, err := cw.Game.GetCurrentPlayer()
		if err != nil {
			return nil, err
		}

		if slices.Contains(member.Roles, cw.roleIDFromName(fmt.Sprintf("%s:%s", guildID, currentPlayer.Name))) {
			isCurrentPlayer = true
		}
		correct = true
		return cw, nil
	})
	if err != nil {
		return err
	}

	if !isCurrentPlayer {
		if err := s.MessageReactionAdd(channelID, messageID, "🙅‍♂️"); err != nil {
			return err
		}
		return nil
	}
	if !correct {
		if err := s.MessageReactionAdd(channelID, messageID, "❌"); err != nil {
			return err
		}
		return nil
	}

	var gameComplete bool = false
	err = c.openScrabbleForWriting(guildID, func(sc *ScrabbleState) (*ScrabbleState, error) {
		if err := sc.Game.PlaceWord(placement, word); err != nil {
			if err := s.MessageReactionAdd(channelID, messageID, "❌"); err != nil {
				return sc, err
			}
			return sc, err
		}
		canvas, err := scrabble.RenderPNG(sc.Game, 1500, 1000)
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

	// best effort
	s.MessageReactionAdd(channelID, messageID, "✅")

	if gameComplete {
		return c.completeGame(guildID)
	}

	return nil
}

func (c *Scrabble) showLetters(s *discordgo.Session, i *discordgo.InteractionCreate) error {

	return c.openScrabbleForReading(i.Interaction.GuildID, func(cw *ScrabbleState) error {

		str := &strings.Builder{}
		str.WriteString("Your Letters are: \n")
		for _, v := range i.Member.Roles {
			for _, p := range cw.Game.Players {
				if roleID, ok := cw.RoleIDMap[fmt.Sprintf("%s:%s", i.Interaction.GuildID, p.Name)]; ok && roleID == v {
					str.WriteString(fmt.Sprintf("%s: `%s`\n", p.Name, strings.Join(util.RunesToStrings(p.Letters), ", ")))
				}
			}
		}

		return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Flags:   discordgo.MessageFlagsEphemeral,
				Content: str.String(),
			},
		})
	})
}

func (c *Scrabble) startScrabble(s *discordgo.Session, i *discordgo.InteractionCreate) error {

	roles, err := s.GuildRoles(i.Interaction.GuildID)
	if err != nil {
		return err
	}

	if err := c.createGameIfNoneExists(i.GuildID, roles); err != nil {
		return err
	}

	var cw ScrabbleState
	err = c.openScrabbleForReading(i.GuildID, func(c *ScrabbleState) error {
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
	canvas, err := scrabble.RenderPNG(cw.Game, 1500, 1000)
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

func (c *Scrabble) createGameIfNoneExists(guildID string, roles discordgo.Roles) error {

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

	// this whole role situation is a bit messed up. Really the user should select roles when creating
	// the game

	game := scrabble.NewGame()
	roleNames := []string{"SAUCER DRINKER", "TEAM GERV", "TEAM SMERCH", "TEAM PILK", "MODS"}
	slices.SortFunc(roleNames, func(a, b string) int {
		return rand.Int() - rand.Int()
	})
	for _, role := range roleNames {
		if err := game.AddPlayer(role); err != nil {
			return err
		}
	}
	cw := ScrabbleState{
		Game:      game,
		RoleIDMap: make(map[string]string),
	}
	for _, v := range roles {
		cw.RoleIDMap[fmt.Sprintf("%s:%s", guildID, v.Name)] = v.ID
	}
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

func (c *Scrabble) completeGame(guildId string) error {
	var winner *scrabble.Player
	err := c.openScrabbleForReading(guildId, func(cw *ScrabbleState) error {
		for _, p := range cw.Game.Players {
			if winner == nil || p.Score > winner.Score {
				winner = p
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	return c.sendThreadMessage(guildId, fmt.Sprintf("GAME COMPLETE, %s WINS", winner.Name))
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

		canvas, err := scrabble.RenderPNG(sc.Game, 1500, 1000)
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
