package cmd

import (
	"github.com/spf13/cobra"
	"github.com/warmans/gamesmaster/cmd/cmd/bot"
	"github.com/warmans/gamesmaster/cmd/cmd/crossfilm"
	"github.com/warmans/gamesmaster/cmd/cmd/crossword"
	"github.com/warmans/gamesmaster/cmd/cmd/filmgame"
	"github.com/warmans/gamesmaster/cmd/cmd/imagegame"
	"log/slog"
)

var (
	rootCmd = &cobra.Command{
		Use:   "gamesmaster",
		Short: "",
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
	rootCmd.AddCommand(crossfilm.NewInitCommand(logger))
	rootCmd.AddCommand(imagegame.NewInitCommand(logger))
	return rootCmd.Execute()
}
