package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"charm.land/huh/v2"
	"charm.land/huh/v2/spinner"
	"github.com/adrg/xdg"
	"github.com/deevus/truenas-go/client"
	cloudinit "go.pilab.hu/cloud/cloud-init"
	"gopkg.in/yaml.v3"
)

func main() {
	config, err := getConfig()
	if err != nil {
		config, err = getApiToken()
		if err != nil {
			log.Fatal(err)
		}
	}

	var wsClient *client.WebSocketClient
	spinner.New().
		Title(fmt.Sprintf("Connecting to %s...", config.Host)).
		ActionWithErr(func(ctx context.Context) error {
			c, err := client.NewWebSocketClient(client.WebSocketConfig{
				Host:   config.Host,
				Port:   config.Port,
				APIKey: config.ApiToken,
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
			huh.NewOption("Edit Config", "edit_config"),
			huh.NewOption("Exit", "exit"),
		).
		Value(&action).
		Run()

	switch action {
	case "create_vm":
		createVM(config, wsClient)
	case "edit_config":
		editConfig(config, wsClient)
	case "exit":
		fmt.Println("Goodbye!")
		os.Exit(0)
	}

}

func createVM(config *Config, client client.Client) {
	vmConfig, err := createVMConfig(config.VMDefaults, client)
	if err != nil {
		log.Fatal(err)
	}

	vmStr := fmt.Sprintf(`%s: %s, 
 - %d VCPUs, 
 - %d Cores, 
 - %d Threads, 
 
 - %d MiB Memory, 
 - %d MiB Min Memory, 
 
 - %d GiB Disk`,
		vmConfig.Name, vmConfig.ImageId, vmConfig.VCPUs, vmConfig.Cores, vmConfig.Threads, vmConfig.Memory, vmConfig.MinMemory, vmConfig.DiskSize)

	var action string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("VM Configuration").
				Description("Review the VM configuration before creating the VM."),

			huh.NewNote().
				Title("Details").
				Description(vmStr),

			huh.NewSelect[string]().
				Title("Action").
				Options(
					huh.NewOption("Continue", "create"),
					huh.NewOption("Edit Config", "edit"),
					huh.NewOption("Save as YAML", "save_yaml"),
					huh.NewOption("Save as Default", "save"),
					huh.NewOption("Cancel", "cancel"),
				).
				Value(&action),
		),
	)

	err = form.Run()
	if err != nil {
		log.Fatal(err)
	}

	switch action {
	case "edit":
		createVM(config, client)
	case "save_yaml":
		saveVMConfigAsYAML(vmConfig)
	case "save":
		config.VMDefaults = *vmConfig
		data, err := yaml.Marshal(config)
		if err != nil {
			log.Fatal(err)
		}

		configPath, err := xdg.ConfigFile("truenas/config.yaml")
		if err != nil {
			log.Fatal(err)
		}

		err = os.WriteFile(configPath, data, 0o600)
		if err != nil {
			log.Fatal(err)
		}
	case "cancel":
		fmt.Println("VM creation cancelled")
		return
	}
}

type CreateVMConfig struct {
	VM        VMConfig              `yaml:"vm,omitempty"`
	CloudInit cloudinit.CloudConfig `yaml:"cloud_init,omitempty"`
}
