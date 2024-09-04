package discord

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"log"
	"log/slog"
)

type InteractionHandlers map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate) error
type MessageHandlers []func(s *discordgo.Session, m *discordgo.MessageCreate)

type Registerable interface {
	Prefix() string
	RootCommand() string
	Description() string
	SubCommands() []*discordgo.ApplicationCommandOption
	ButtonHandlers() InteractionHandlers
	ModalHandlers() InteractionHandlers
	CommandHandlers() InteractionHandlers
	AutoCompleteHandlers() InteractionHandlers
	MessageHandlers() MessageHandlers
}

type Command string

const (
	RootCommand Command = "gamesmaster"
)

func NewBot(
	logger *slog.Logger,
	session *discordgo.Session,
	commmands ...Registerable,
) (*Bot, error) {
	session.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsAllWithoutPrivileged | discordgo.IntentMessageContent)
	bot := &Bot{
		logger:  logger,
		session: session,
		commands: []*discordgo.ApplicationCommand{
			{
				Name:        string(RootCommand),
				Description: "Game selection",
				Type:        discordgo.ChatApplicationCommand,
				Options:     make([]*discordgo.ApplicationCommandOption, 0),
			},
		},
		buttonHandlers:  InteractionHandlers{},
		modalHandlers:   InteractionHandlers{},
		commandHandlers: map[string]InteractionHandlers{},
	}
	for _, c := range commmands {
		bot.commands[0].Options = append(bot.commands[0].Options, &discordgo.ApplicationCommandOption{
			Name:        c.RootCommand(),
			Description: c.RootCommand(),
			Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
			Options:     c.SubCommands(),
		})

		for k, v := range c.ButtonHandlers() {
			bot.buttonHandlers[fmt.Sprintf("%s:%s", c.Prefix(), k)] = v
		}
		for k, v := range c.ModalHandlers() {
			bot.modalHandlers[fmt.Sprintf("%s:%s", c.Prefix(), k)] = v
		}
		for k, v := range c.AutoCompleteHandlers() {
			bot.autoCompleteHandlers[fmt.Sprintf("%s:%s", c.Prefix(), k)] = v
		}
		for k, v := range c.CommandHandlers() {
			if bot.commandHandlers[c.RootCommand()] == nil {
				bot.commandHandlers[c.RootCommand()] = InteractionHandlers{}
			}
			bot.commandHandlers[c.RootCommand()][k] = v
		}
		bot.messageHandlers = append(bot.messageHandlers, c.MessageHandlers()...)
	}

	return bot, nil
}

type Bot struct {
	logger               *slog.Logger
	session              *discordgo.Session
	commands             []*discordgo.ApplicationCommand
	commandHandlers      map[string]InteractionHandlers
	autoCompleteHandlers InteractionHandlers
	buttonHandlers       InteractionHandlers
	modalHandlers        InteractionHandlers
	messageHandlers      MessageHandlers
	createdCommands      []*discordgo.ApplicationCommand
}

func (b *Bot) Start() error {
	b.session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
	})
	b.session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {

		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			if err := b.handleRootCommand(s, i); err != nil {
				b.respondError(s, i, err)
			}
			return
		case discordgo.InteractionApplicationCommandAutocomplete:
			if h, ok := b.autoCompleteHandlers[i.ApplicationCommandData().Name]; ok {
				if err := h(s, i); err != nil {
					b.respondError(s, i, err)
				}
				return
			}
			b.respondError(s, i, fmt.Errorf("no handler for autocomplete action: %s", i.ApplicationCommandData().Name))
			return
		case discordgo.InteractionModalSubmit:
			// prefix match buttons to allow additional data in the customID
			for k, h := range b.modalHandlers {
				if i.ModalSubmitData().CustomID == k {
					if err := h(s, i); err != nil {
						b.respondError(s, i, err)
					}
					return
				}
			}
			b.respondError(s, i, fmt.Errorf("no handler for modal action: %s", i.ModalSubmitData().CustomID))
			return
		case discordgo.InteractionMessageComponent:
			// prefix match buttons to allow additional data in the customID
			for k, h := range b.buttonHandlers {
				if i.MessageComponentData().CustomID == k {
					if err := h(s, i); err != nil {
						b.respondError(s, i, err)
					}
					return
				}
			}
			b.respondError(s, i, fmt.Errorf("no handler for button action: %s", i.MessageComponentData().CustomID))
			return
		}
	})
	for _, v := range b.messageHandlers {
		b.session.AddHandler(v)
	}
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

func (b *Bot) handleRootCommand(s *discordgo.Session, i *discordgo.InteractionCreate) error {

	subCommand := i.ApplicationCommandData().Options[0]

	game, ok := b.commandHandlers[subCommand.Name]
	if !ok {
		return fmt.Errorf("unkown game: %s", i.ApplicationCommandData().Options[0].Options[0].Name)
	}
	if commandOption, ok := game[subCommand.Options[0].Name]; ok {
		return commandOption(s, i)
	}
	return fmt.Errorf("unkown option for %s: %s", subCommand.Name, subCommand.Options[0].Name)
}
