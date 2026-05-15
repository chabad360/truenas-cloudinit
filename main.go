package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"charm.land/huh/v2"
	"charm.land/huh/v2/spinner"
	"github.com/deevus/truenas-go/client"
)

var settings *Settings

func main() {
	var err error
	settings, err = createSettings()
	if err != nil {
		settings, err = getApiToken()
		if err != nil {
			log.Fatal(err)
		}
	}

	var wsClient *client.WebSocketClient
	err = spinner.New().
		Title(fmt.Sprintf("Connecting to %s...", settings.Host)).
		ActionWithErr(func(ctx context.Context) error {
			c, err := client.NewWebSocketClient(client.WebSocketConfig{
				Host:     settings.Host,
				Port:     settings.Port,
				Username: "admin",
				APIKey:   settings.ApiToken,
			})
			if err != nil {
				return err
			}
			err = c.Connect(ctx)
			if err != nil {
				return err
			}

			wsClient = c
			return nil
		}).
		Run()
	if err != nil {
		log.Fatal(err)
	}
	defer wsClient.Close()

	fmt.Println()

	var action string
	huh.NewSelect[string]().
		Title("Choose an action").
		Options(
			huh.NewOption("Create a VM", "create_vm"),
			huh.NewOption("Edit Settings", "edit_settings"),
			huh.NewOption("Exit", "exit"),
		).
		Value(&action).
		Run()

	switch action {
	case "create_vm":
		createVM(wsClient)
	case "edit_settings":
		editSettings(settings, wsClient)
	case "exit":
		fmt.Println("Goodbye!")
		os.Exit(0)
	}

}
