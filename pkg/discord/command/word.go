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
	wordCommandRandomNoun   string = "noun"
	wordCommandRandomSong   string = "song"
	wordCommandRandomArtist string = "artist"
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
		wordCommandRandomNoun:   c.randomWord,
		wordCommandRandomSong:   c.randomSong,
		wordCommandRandomArtist: c.randomArtist,
	}
}

func (c *Word) SubCommands() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Name:        wordCommandRandomNoun,
			Description: "Post a random noun",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
		}, {
			Name:        wordCommandRandomSong,
			Description: "Post a random song played on Xfm",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
		}, {
			Name:        wordCommandRandomArtist,
			Description: "Post a random artist played on Xfm",
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

func (c *Word) randomSong(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Song: **%s**", dictionary.RandomSong()),
		},
	})
}

func (c *Word) randomArtist(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Artist: **%s**", dictionary.RandomArtist()),
		},
	})
}
