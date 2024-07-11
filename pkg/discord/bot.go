package discord

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"log"
	"log/slog"
	"strings"
)

type Command string

const BotUsername string = "Gamesmaster"

const (
	RootCommand Command = "gamesmaster"
	GameCommand Command = "game"
)

type Action string

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
				Description: "TODO",
				Type:        discordgo.ChatApplicationCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{
						Name:        string(GameCommand),
						Description: "Top-level subcommand",
						Type:        discordgo.ApplicationCommandOptionSubCommand,
					},
				},
			},
		},
	}
	bot.commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		string(GameCommand): bot.helpText,
	}
	bot.buttonHandlers = map[Action]func(s *discordgo.Session, i *discordgo.InteractionCreate, suffix string){}
	bot.modalHandlers = map[Action]func(s *discordgo.Session, i *discordgo.InteractionCreate, suffix string){}

	return bot, nil
}

type Bot struct {
	logger          *slog.Logger
	session         *discordgo.Session
	commands        []*discordgo.ApplicationCommand
	commandHandlers map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate)
	buttonHandlers  map[Action]func(s *discordgo.Session, i *discordgo.InteractionCreate, customIdPayload string)
	modalHandlers   map[Action]func(s *discordgo.Session, i *discordgo.InteractionCreate, customIdPayload string)
	createdCommands []*discordgo.ApplicationCommand
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
				actionPrefix := fmt.Sprintf("%s:", k)
				if strings.HasPrefix(i.ModalSubmitData().CustomID, actionPrefix) {
					h(s, i, strings.TrimPrefix(i.ModalSubmitData().CustomID, actionPrefix))
					return
				}
			}
			b.respondError(s, i, fmt.Errorf("unknown customID format: %s", i.MessageComponentData().CustomID))
			return
		case discordgo.InteractionMessageComponent:
			// prefix match buttons to allow additional data in the customID
			for k, h := range b.buttonHandlers {
				actionPrefix := fmt.Sprintf("%s:", k)
				if strings.HasPrefix(i.MessageComponentData().CustomID, actionPrefix) {
					h(s, i, strings.TrimPrefix(i.MessageComponentData().CustomID, actionPrefix))
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

func (b *Bot) helpText(s *discordgo.Session, i *discordgo.InteractionCreate) {

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: "foo",
		},
	})
	if err != nil {
		b.respondError(s, i, err)
		return
	}
}
