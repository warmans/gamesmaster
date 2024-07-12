package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/warmans/gamesmaster/pkg/crossword"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"
)

type Command string

const (
	RootCommand      Command = "gamesmaster"
	CrosswordCommand Command = "crossword"
)

type Action string

const (
	crosswordAnswerModalOpen Action = "handleAnswerModalOpen"
	crosswordAnswerCheck     Action = "handleCheckWordSubmission"
)

func NewBot(
	logger *slog.Logger,
	session *discordgo.Session,
) (*Bot, error) {

	bot := &Bot{
		logger:  logger,
		session: session,
		commands: []*discordgo.ApplicationCommand{
			{
				Name:        string(RootCommand),
				Description: "Game selection",
				Type:        discordgo.ChatApplicationCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{
						Name:        string(CrosswordCommand),
						Description: "Show crossword game",
						Type:        discordgo.ApplicationCommandOptionSubCommand,
					},
				},
			},
		},
	}
	bot.commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		string(RootCommand): bot.handleGameSubcommand,
	}
	bot.buttonHandlers = map[Action]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		crosswordAnswerModalOpen: bot.handleAnswerModalOpen,
	}
	bot.modalHandlers = map[Action]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		crosswordAnswerCheck: bot.handleCheckWordSubmission,
	}

	return bot, nil
}

type Bot struct {
	logger          *slog.Logger
	session         *discordgo.Session
	commands        []*discordgo.ApplicationCommand
	commandHandlers map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate)
	buttonHandlers  map[Action]func(s *discordgo.Session, i *discordgo.InteractionCreate)
	modalHandlers   map[Action]func(s *discordgo.Session, i *discordgo.InteractionCreate)
	createdCommands []*discordgo.ApplicationCommand
	gameLock        sync.RWMutex
}

func (b *Bot) Start() error {
	b.session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
	})
	b.session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {

		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			// exact match
			if h, ok := b.commandHandlers[i.ApplicationCommandData().Name]; ok {
				h(s, i)
			}
		case discordgo.InteractionApplicationCommandAutocomplete:
			// exact match
			if h, ok := b.commandHandlers[i.ApplicationCommandData().Name]; ok {
				h(s, i)
			}
		case discordgo.InteractionModalSubmit:
			// prefix match buttons to allow additional data in the customID
			for k, h := range b.modalHandlers {
				if Action(i.ModalSubmitData().CustomID) == k {
					h(s, i)
					return
				}
			}
			b.respondError(s, i, fmt.Errorf("unknown customID format: %s", i.MessageComponentData().CustomID))
			return
		case discordgo.InteractionMessageComponent:
			// prefix match buttons to allow additional data in the customID
			for k, h := range b.buttonHandlers {
				if Action(i.MessageComponentData().CustomID) == k {
					h(s, i)
					return
				}
			}
			b.respondError(s, i, fmt.Errorf("unknown customID format: %s", i.MessageComponentData().CustomID))
			return
		}
	})
	if err := b.session.Open(); err != nil {
		return fmt.Errorf("failed to open session: %w", err)
	}
	var err error
	b.createdCommands, err = b.session.ApplicationCommandBulkOverwrite(b.session.State.User.ID, "", b.commands)
	if err != nil {
		return fmt.Errorf("cannot register commands: %w", err)
	}
	return nil
}

func (b *Bot) Close() error {
	// cleanup commands
	for _, cmd := range b.createdCommands {
		err := b.session.ApplicationCommandDelete(b.session.State.User.ID, "", cmd.ID)
		if err != nil {
			return fmt.Errorf("cannot delete %s command: %w", cmd.Name, err)
		}
	}
	return b.session.Close()
}

func (b *Bot) respondError(s *discordgo.Session, i *discordgo.InteractionCreate, err error, logCtx ...any) {
	b.logger.Error("Error response was sent: "+err.Error(), logCtx...)
	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Request failed with error: %s", err.Error()),
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		b.logger.Error("failed to respond", slog.String("err", err.Error()))
		return
	}
}

func (b *Bot) handleGameSubcommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch Command(i.ApplicationCommandData().Options[0].Name) {
	case CrosswordCommand:
		if err := b.showCrossword(s, i); err != nil {
			b.respondError(s, i, err)
		}
	default:
		b.respondError(s, i, fmt.Errorf("unkown game"))
	}
}

func (b *Bot) openCrosswordForReading(cb func(cw *crossword.Crossword) error) error {
	b.gameLock.RLock()
	defer b.gameLock.RUnlock()

	f, err := os.Open("var/crossword/game/current.json")
	if err != nil {
		return err
	}
	defer f.Close()

	cw := crossword.Crossword{}
	if err := json.NewDecoder(f).Decode(&cw); err != nil {
		return err
	}

	return cb(&cw)
}

func (b *Bot) openCrosswordForWriting(cb func(cw *crossword.Crossword) *crossword.Crossword) error {
	b.gameLock.Lock()
	defer b.gameLock.Unlock()

	f, err := os.OpenFile("var/crossword/game/current.json", os.O_RDWR|os.O_EXCL, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	cw := &crossword.Crossword{}
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

func (b *Bot) showCrossword(s *discordgo.Session, i *discordgo.InteractionCreate) error {

	var cw crossword.Crossword
	err := b.openCrosswordForReading(func(c *crossword.Crossword) error {
		cw = *c
		return nil
	})
	if err != nil {
		return err
	}
	canvas, err := cw.Render(1024, 1024)
	if err != nil {
		return err
	}

	content := &bytes.Buffer{}

	solved := []crossword.ActiveWord{}
	unsolved := []crossword.ActiveWord{}
	for _, w := range cw.WordList {
		if w.Solved {
			unsolved = append(unsolved, w)
		} else {
			solved = append(solved, w)
		}
	}

	fmt.Fprintf(content, "# UNSOLVED\n")
	for _, w := range solved {
		fmt.Fprintf(content, "- [%s | %d letters] %s\n", w.String(), len(w.Word.Word), w.Word.Clue)
	}
	fmt.Fprintf(content, "\n# SOLVED\n")
	for _, w := range unsolved {
		fmt.Fprintf(content, "- [%s | %d letters] %s\n", w.String(), len(w.Word.Word), w.Word.Clue)
	}

	buff := &bytes.Buffer{}
	if err := canvas.EncodePNG(buff); err != nil {
		return err
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: "",
			Files: []*discordgo.File{
				{
					Name:        "crossword.png",
					ContentType: "images/png",
					Reader:      buff,
				},
				{
					Name:        "clues.txt",
					ContentType: "text/plain",
					Reader:      content,
				},
			},
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{discordgo.Button{
						Label: "Submit Answer",
						Emoji: &discordgo.ComponentEmoji{
							Name: "✅",
						},
						Style:    discordgo.PrimaryButton,
						Disabled: false,
						CustomID: string(crosswordAnswerModalOpen),
					}},
				},
			},
		},
	})
	return err
}

func (b *Bot) handleAnswerModalOpen(s *discordgo.Session, i *discordgo.InteractionCreate) {

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: string(crosswordAnswerCheck),
			Title:    "Submit Word",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  "id",
							Label:     "Word ID (e.g. 4D)",
							Style:     discordgo.TextInputShort,
							Required:  true,
							MaxLength: 3,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  "word",
							Label:     "Word",
							Style:     discordgo.TextInputShort,
							Required:  true,
							MaxLength: 128,
						},
					},
				},
			},
		},
	})
	if err != nil {
		b.respondError(s, i, err)
	}
}

func (b *Bot) handleCheckWordSubmission(s *discordgo.Session, i *discordgo.InteractionCreate) {
	id := i.Interaction.ModalSubmitData().Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
	word := i.Interaction.ModalSubmitData().Components[1].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value

	alreadySolved := false
	correct := false
	clue := ""
	err := b.openCrosswordForWriting(func(cw *crossword.Crossword) *crossword.Crossword {
		for k, w := range cw.WordList {
			if w.String() != strings.ToUpper(id) {
				continue
			}
			if w.Solved {
				alreadySolved = true
				break
			}
			if strings.TrimSpace(strings.ToUpper(word)) == strings.ToUpper(w.Word.Word) {
				correct = true
				solved := cw.WordList[k]
				clue = cw.WordList[k].Word.Clue
				solved.Solved = true
				cw.WordList[k] = solved
				break
			}
		}
		return cw
	})
	if err != nil {
		b.respondError(s, i, err)
		return
	}

	if correct {
		err := b.openCrosswordForReading(func(cw *crossword.Crossword) error {
			canvas, err := cw.Render(1200, 1200)
			if err != nil {
				return err
			}
			buff := &bytes.Buffer{}
			if err := canvas.EncodePNG(buff); err != nil {
				return err
			}
			return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf(
						"\n> %s\n\n`%s` was solved by %s: `%s`\n",
						clue,
						id,
						i.Interaction.Member.DisplayName(),
						strings.ToUpper(word),
					),
					Files: []*discordgo.File{
						{
							Name:        "crossword.png",
							ContentType: "images/png",
							Reader:      buff,
						},
					},
				},
			})
		})
		if err != nil {
			b.respondError(s, i, err)
			return
		}
	} else {
		if alreadySolved {
			err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("%s has already been solved", id),
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			if err != nil {
				b.logger.Error("failed to respond", slog.String("err", err.Error()))
				return
			}
		} else {
			err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("%s was not correct: %s", id, word),
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			if err != nil {
				b.logger.Error("failed to respond", slog.String("err", err.Error()))
				return
			}
		}

	}
}
