package command

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/warmans/gamesmaster/pkg/dictionary"
	"github.com/warmans/gamesmaster/pkg/discord"
)

const (
	wordCommand = "word"
)

const (
	wordCommandRandomNoun string = "noun"
)

func NewWordCommand() *Word {
	return &Word{}
}

type Word struct {
}

func (c *Word) Prefix() string {
	return "wrd"
}

func (c *Word) RootCommand() string {
	return wordCommand
}

func (c *Word) Description() string {
	return "Generate words for word games."
}

func (c *Word) AutoCompleteHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{}
}

func (c *Word) ButtonHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{}
}

func (c *Word) ModalHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{}
}

func (c *Word) CommandHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{
		wordCommandRandomNoun: c.randomWord,
	}
}

func (c *Word) SubCommands() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Name:        wordCommandRandomNoun,
			Description: "Post a random noun",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
		},
	}
}

func (c *Word) randomWord(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Word do you think of: **%s**", dictionary.RandomNoun()),
		},
	})
}
