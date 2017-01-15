package main

import (
	"gopkg.in/urfave/cli.v2"
	"log"
	"os"
	"os/exec"
	"strings"
)

const thePusherVersion = "0.0.2"

var verbose bool
var imageStorage string
var pusherIP string
var imageToUpload string
var staticContentRoot string

func main() {

	// override cli -version shortcut (for -verbose); use -V instead
	cli.VersionFlag = &cli.BoolFlag{
		Name:    "version",
		Aliases: []string{"V"},
		Usage:   "print thePusher version",
	}

	app := &cli.App{
		Version: thePusherVersion,
		Name:    "thePusher",
		Usage:   "makes your linux and mac disk images flow to the masses",
		EnableShellCompletion: true,

		// GLOBAL flags
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "verbose",
				Aliases:     []string{"v"},
				Usage:       "produce verbose output",
				Value:       false,
				Destination: &verbose,
			},
		},

		Commands: []*cli.Command{

			{
				Name:    "master",
				Aliases: []string{"m"},
				Usage:   "run thePusher master server",
				Action: func(c *cli.Context) error {
					if imageStorage == "" {
						print("Missing required parameter -image-storage (-S)")
						return nil
					}
					runWebserver()
					return nil
				},
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "listen-addr",
						Value:       ":8080",
						Aliases:     []string{"l"},
						Usage:       "master http listener address",
						Destination: &listenAddr,
					},
					&cli.StringFlag{
						Name: "image-storage",
						//Value:       "/var/lib/pusher",
						Aliases:     []string{"S"},
						Usage:       "path to master-local image storage",
						Destination: &imageStorage,
					},
					&cli.StringFlag{
						Name:        "static-content",
						Aliases:     []string{"s"},
						Usage:       "(optional) path to folder to be served as /static",
						Destination: &staticContentRoot,
					},
				},
			},

			{
				Name:    "create-nbi",
				Aliases: []string{"n"},
				Usage:   "create netboot image for thePusher",
				Action: func(c *cli.Context) error {
					if pusherIP == "" {
						log.Fatal("-pusher IP required to create netboot image")
					}
					createNBI()
					return nil
				},
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "pusher",
						Aliases:     []string{"p"},
						Usage:       "thePusher master IP address",
						Destination: &pusherIP,
					},
				},
			},

			{
				Name:    "client",
				Aliases: []string{"c"},
				Usage:   "act as thePusher client / image consumer",
				Action: func(c *cli.Context) error {
					runClient()
					return nil
				},
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "pusher",
						Aliases:     []string{"p"},
						Usage:       "thePusher master IP address",
						Destination: &pusherIP,
					},
				},
			},

			{
				Name:    "put-image",
				Aliases: []string{"p"},
				Usage:   "upload new image to master using HTTP PUT",
				Action: func(c *cli.Context) error {
					if pusherIP == "" {
						log.Fatal("-pusher IP required to upload image")
					}
					if imageToUpload == "" {
						log.Fatal("-image-file required to upload image")
					}
					putImage()
					return nil
				},
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "pusher",
						Aliases:     []string{"p"},
						Usage:       "thePusher master IP address",
						Destination: &pusherIP,
					},
					&cli.StringFlag{
						Name:        "image-file",
						Aliases:     []string{"i"},
						Usage:       "image file to upload",
						Destination: &imageToUpload,
					},
				},
			},
		},

		Action: func(c *cli.Context) error {
			// default action for no args: show help
			cli.ShowAppHelp(c)
			return nil
		},
	}

	app.Run(os.Args)
}

func execFatal(cmd string, args ...string) {
	log.Printf("Executing: %s %s", cmd, strings.Join(args, " "))
	out, err := exec.Command(cmd, args...).Output()
	if err != nil {
		log.Fatalf("FAILED: %T", err)
	}
	if len(out) != 0 {
		log.Printf("Output:\n%s", out)
	}
}
