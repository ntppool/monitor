package main

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "monitor-api",
	Short: "API server for the NTP Pool monitor",
}

func init() {
	// rootCmd.PersistentFlags().Bool("viper", true, "use Viper for configuration")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
