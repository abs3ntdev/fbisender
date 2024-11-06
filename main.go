package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"git.asdf.cafe/abs3nt/fbisender/src/sender"
	"github.com/urfave/cli/v3"
)

func main() {
	log.SetFlags(0)

	app := &cli.Command{
		Name:      "fbisender",
		Usage:     "send files to FBI over network with an HTTP server",
		UsageText: "fbisender [global options] <target file or directory>",
		Action: func(ctx context.Context, c *cli.Command) error {
			if !c.Args().Present() {
				return fmt.Errorf("target file or directory is required as an argument")
			}
			return sender.SendFiles(ctx, c.Args().First())
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
