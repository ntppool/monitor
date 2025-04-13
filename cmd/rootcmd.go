package rootcmd

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
)

func Run(cmd any, name, description string) {
	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM,
	)
	defer cancel()

	parser, err := kong.New(cmd,
		kong.Name(name),
		kong.Description(description),
		kong.BindTo(ctx, (*context.Context)(nil)),
		kong.ConfigureHelp(kong.HelpOptions{
			Tree: true,
		}),
		kong.UsageOnError(),
	)
	if err != nil {
		log.Printf("error: %v", err)
		os.Exit(1)
	}

	kctx, err := parser.Parse(os.Args[1:])
	if err != nil {
		parser.FatalIfErrorf(err)
	}

	err = kctx.Run()
	parser.FatalIfErrorf(err)
}
