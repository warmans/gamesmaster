package bot

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/spf13/cobra"
	"github.com/warmans/gamesmaster/pkg/discord"
	"github.com/warmans/gamesmaster/pkg/discord/command"
	"github.com/warmans/gamesmaster/pkg/discord/command/crossfilm"
	"github.com/warmans/gamesmaster/pkg/flag"

	"log"
	"log/slog"
	"os"
	"os/signal"
)

func NewBotCommand(logger *slog.Logger) *cobra.Command {

	var discordToken string
	var botName string
	var wordsFilePath string

	cmd := &cobra.Command{
		Use:   "bot",
		Short: "start the discord bot",
		RunE: func(cmd *cobra.Command, args []string) error {

			logger.Info("Creating discord session...")
			if discordToken == "" {
				return fmt.Errorf("discord token is required")
			}
			session, err := discordgo.New("Bot " + discordToken)
			if err != nil {
				return fmt.Errorf("failed to create discord session: %w", err)
			}

			scrabble, err := command.NewScrabbleCommand(session, wordsFilePath)
			if err != nil {
				return err
			}

			logger.Info("Starting bot...")
			bot, err := discord.NewBot(
				botName,
				logger,
				session,
				command.NewCrosswordCommand(),
				command.NewRandomCommand(),
				command.NewFilmgameCommand(logger, session),
				crossfilm.NewCrossfilmCommand(logger, session),
				scrabble,
			)
			if err != nil {
				return fmt.Errorf("failed to create bot: %w", err)
			}

			if err = bot.Start(); err != nil {
				return fmt.Errorf("failed to start bot: %w", err)
			}
			stop := make(chan os.Signal, 1)
			signal.Notify(stop, os.Interrupt)
			<-stop

			log.Println("Gracefully shutting down")
			if err = bot.Close(); err != nil {
				return fmt.Errorf("failed to gracefully shutdown bot: %w", err)
			}
			return nil
		},
	}

	flag.StringVarEnv(cmd.Flags(), &discordToken, "", "discord-token", "", "discord auth token")
	flag.StringVarEnv(cmd.Flags(), &botName, "", "bot-name", "gamesmaster", "root command of the bot")
	flag.StringVarEnv(cmd.Flags(), &wordsFilePath, "", "words-path", "/usr/share/dict/words", "Path to words list of valid dictionary words")

	flag.Parse()

	return cmd
}
