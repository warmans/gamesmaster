package cmd

import (
	"github.com/spf13/cobra"
	"github.com/warmans/gamesmaster/cmd/cmd/bot"
	"github.com/warmans/gamesmaster/cmd/cmd/crossword"
	"github.com/warmans/gamesmaster/cmd/cmd/filmgame"
	"log/slog"
)

var (
	rootCmd = &cobra.Command{
		Use:   "tvgif",
		Short: "Discord bot for posting TV show gifs",
	}
)

func init() {

}

// Execute executes the root command.
func Execute(logger *slog.Logger) error {
	rootCmd.AddCommand(bot.NewBotCommand(logger))
	rootCmd.AddCommand(crossword.NewInitCommand(logger))
	rootCmd.AddCommand(crossword.NewRandomWordListCommand(logger))
	rootCmd.AddCommand(crossword.NewLoadCommand(logger))
	rootCmd.AddCommand(filmgame.NewInitCommand(logger))
	return rootCmd.Execute()
}
