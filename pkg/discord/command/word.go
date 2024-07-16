package command

import (
	"errors"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/warmans/gamesmaster/pkg/dictionary"
	"github.com/warmans/gamesmaster/pkg/discord"
	"math/rand/v2"
)

const (
	randomCommand = "random"
)

const (
	wordCommandRandomNoun   string = "noun"
	wordCommandRandomSong   string = "song"
	wordCommandRandomArtist string = "artist"
	wordCommandRandomHost   string = "host"
	wordCommandRandomNumber string = "dice-roll"
)

func NewRandomCommand() *Random {
	return &Random{}
}

type Random struct {
}

func (c *Random) Prefix() string {
	return "wrd"
}

func (c *Random) RootCommand() string {
	return randomCommand
}

func (c *Random) Description() string {
	return "Generate words for word games."
}

func (c *Random) AutoCompleteHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{}
}

func (c *Random) ButtonHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{}
}

func (c *Random) ModalHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{}
}

func (c *Random) CommandHandlers() discord.InteractionHandlers {
	return discord.InteractionHandlers{
		wordCommandRandomNoun:   c.randomWord,
		wordCommandRandomSong:   c.randomSong,
		wordCommandRandomArtist: c.randomArtist,
		wordCommandRandomNumber: c.randomNumber,
		wordCommandRandomHost:   c.randomHost,
	}
}

func (c *Random) SubCommands() []*discordgo.ApplicationCommandOption {
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
		}, {
			Name:        wordCommandRandomHost,
			Description: "Post a random Xfm host",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
		}, {
			Name:        wordCommandRandomNumber,
			Description: "Roll an N-sided dice",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "sides",
					Description: "Number will between 0 and N",
					Required:    true,
				},
			},
		},
	}
}

func (c *Random) randomWord(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Word do you think of: **%s**", dictionary.RandomNoun()),
		},
	})
}

func (c *Random) randomSong(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Song: **%s**", dictionary.RandomSong()),
		},
	})
}

func (c *Random) randomArtist(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Artist: **%s**", dictionary.RandomArtist()),
		},
	})
}

func (c *Random) randomNumber(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	diceSides := i.ApplicationCommandData().Options[0].Options[0].Options[0].IntValue()
	if diceSides == 0 || diceSides > 1000000 {
		return errors.New("dice sides must be between 1 and 1,000,000")
	}
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf(":game_die: **%d**", rand.IntN(int(diceSides))),
		},
	})
}

func (c *Random) randomHost(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	hosts := []string{
		"Karl",
		"Steve",
		"Ricky",
		"Camfield",
		"Claire",
	}
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: hosts[rand.IntN(len(hosts)-1)],
		},
	})
}
