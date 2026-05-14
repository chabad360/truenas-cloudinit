package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"truenas-cloudinit/vmconfig"

	"charm.land/huh/v2"
	"charm.land/huh/v2/spinner"
	"github.com/adrg/xdg"
	"github.com/deevus/truenas-go"
	"github.com/deevus/truenas-go/client"
	"gopkg.in/yaml.v3"
)

type Settings struct {
	ApiToken string `yaml:"api_token"`

	Host string `yaml:"host"`
	Port int    `yaml:"port"`

	ISODir string `yaml:"iso_dir,omitempty"`

	VMDefaults vmconfig.VMConfig `yaml:"vm_defaults,omitempty"`
}

func editSettings(config *Settings, client client.Client) {
	dirRegex := regexp.MustCompile(`^/mnt/[^/]+(/[^/]+)*$`)
	fsService := truenas.NewFilesystemService(client, client.Version())

	port := strconv.Itoa(config.Port)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Edit Configuration").
				Description("Update your TrueNAS connection settings. Changes will be saved to the config file."),

			huh.NewInput().
				Title("TrueNAS Host").
				Placeholder("Enter your TrueNAS host (e.g., truenas.local)").
				Value(&config.Host),
			huh.NewInput().
				Title("TrueNAS Port").
				Placeholder("Enter your TrueNAS port (e.g., 80 or 443)").
				Value(&port).
				Validate(func(value string) error {
					if _, err := strconv.Atoi(value); err != nil {
						return fmt.Errorf("port must be a number")
					}
					return nil
				}),
			huh.NewInput().
				Title("Image Directory").
				Placeholder("Enter the directory where the Cloud Init images will be stored (e.g., /mnt/pool/iso)").
				Value(&config.ISODir).
				SuggestionsFunc(func() []string {
					p := path.Dir(config.ISODir)
					r, err := fsService.ListDir(context.Background(), p, truenas.ListDirOpts{})
					if err != nil {
						return nil
					}
					var suggestions []string
					for _, entry := range r {
						suggestions = append(suggestions, entry.Path)
					}
					return suggestions
				}, &config.ISODir).
				Validate(func(s string) error {
					if !dirRegex.MatchString(s) {
						return fmt.Errorf("Disk directory must start with /mnt/ and be a valid path")
					}
					return nil
				}),
			huh.NewInput().
				Title("Default VM Directory").
				Placeholder("Enter the directory where the VM disks will be stored (e.g., /mnt/pool/vm)").
				Value(&config.VMDefaults.DiskDir).
				SuggestionsFunc(func() []string {
					p := path.Dir(config.VMDefaults.DiskDir)
					r, err := fsService.ListDir(context.Background(), p, truenas.ListDirOpts{})
					if err != nil {
						return nil
					}
					var suggestions []string
					for _, entry := range r {
						suggestions = append(suggestions, entry.Path)
					}
					return suggestions
				}, &config.VMDefaults.DiskDir).
				Validate(func(s string) error {
					if !dirRegex.MatchString(s) {
						return fmt.Errorf("Disk directory must start with /mnt/ and be a valid path")
					}
					return nil
				}),
		),
	)

	err := form.Run()
	if err != nil {
		log.Fatal(err)
	}

	config.Port, err = strconv.Atoi(port)
	if err != nil {
		log.Fatal(fmt.Errorf("invalid port: %w", err))
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		log.Fatal(err)
	}

	configPath, err := xdg.ConfigFile("truenas/config.yaml")
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile(configPath, data, 0o600)
}

func createSettings() (*Settings, error) {
	path, err := xdg.SearchConfigFile("truenas/config.yaml")
	if err != nil {
		return nil, err
	}
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config := &Settings{}
	err = yaml.Unmarshal(file, config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func getApiToken() (*Settings, error) {
	var host, portStr string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Sign in to your TrueNAS account").
				Description("To use this application, you need to sign in to your TrueNAS account and generate an API token."),

			huh.NewInput().
				Title("TrueNAS Host").
				Placeholder("Enter your TrueNAS host (e.g., truenas.local)").
				Value(&host),
			huh.NewInput().
				Title("TrueNAS Port").
				Placeholder("Enter your TrueNAS port (e.g., 80 or 443)").
				Value(&portStr).
				Validate(func(value string) error {
					if _, err := strconv.Atoi(value); err != nil {
						return fmt.Errorf("port must be a number")
					}
					return nil
				}),
		),
	)

	err := form.Run()
	if err != nil {
		return nil, err
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port: %w", err)
	}

	err = spinner.New().
		Title(fmt.Sprintf("Connecting to %s...", host)).
		ActionWithErr(func(ctx context.Context) error {
			r, err := http.Get(fmt.Sprintf("http://%s:%s", host, portStr))
			if err != nil {
				return err
			}
			r.Body.Close()
			return nil
		}).
		Run()

	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", host, err)
	}

	var username, password string

	form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Username").
				Placeholder("Enter your TrueNAS username").
				Value(&username),

			huh.NewInput().
				Title("Password").
				Placeholder("Enter your TrueNAS password").
				Value(&password).EchoMode(huh.EchoModePassword),
		),
	)

	err = form.Run()
	if err != nil {
		return nil, err
	}

	var wsClient *client.WebSocketClient
	err = spinner.New().
		Title("Authenticating...").
		ActionWithErr(func(ctx context.Context) error {
			c, err := client.NewWebSocketClient(client.WebSocketConfig{
				Host:     host,
				Port:     port,
				Username: username,
				Password: password,
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
		return nil, fmt.Errorf("failed to authenticate: %w", err)
	}
	defer wsClient.Close()

	var confirm bool
	err = huh.NewConfirm().Title("Create API Token").Description("Do you want to create an API token?").Value(&confirm).Run()
	if err != nil {
		return nil, err
	}
	if !confirm {
		log.Fatal("API token creation cancelled")
	}

	var token string

	err = spinner.New().
		Title(fmt.Sprintf("Creating token on %s...", host)).
		ActionWithErr(func(ctx context.Context) error {
			hostname, err := os.Hostname()
			if err != nil {
				return err
			}

			req := struct {
				Name     string `json:"name"`
				Username string `json:"username"`
			}{Name: "truenas-cloudinit-" + hostname, Username: username}
			r, err := wsClient.Call(ctx, "api_key.create", []any{req})
			if err != nil {
				return err
			}

			var result struct {
				Key string `json:"key"`
			}
			err = json.Unmarshal(r, &result)
			if err != nil {
				return err
			}
			token = result.Key
			return nil
		}).
		Run()
	if err != nil {
		return nil, fmt.Errorf("failed to create API token: %w", err)
	}

	diskDir := "/mnt/"
	dirRegex := regexp.MustCompile(`^/mnt/[^/]+(/[^/]+)*$`)
	fsService := truenas.NewFilesystemService(wsClient, wsClient.Version())

	form = huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("API Token Created").
				Description("An API token has been created for you. This token will be used to authenticate with the TrueNAS API. Make sure to keep it safe and do not share it with anyone."),
			huh.NewNote().
				Title("Disk Location").
				Description("Please choose the location where ISOs will be stored"),
			huh.NewInput().
				Title("Image Directory").
				Placeholder("Enter the directory where the Cloud Init images will be stored (e.g., /mnt/pool/iso)").
				Value(&diskDir).
				SuggestionsFunc(func() []string {
					p := path.Dir(diskDir)
					r, err := fsService.ListDir(context.Background(), p, truenas.ListDirOpts{})
					if err != nil {
						return nil
					}
					var suggestions []string
					for _, entry := range r {
						suggestions = append(suggestions, entry.Path)
					}
					return suggestions
				}, &diskDir).
				Validate(func(s string) error {
					if !dirRegex.MatchString(s) {
						return fmt.Errorf("Disk directory must start with /mnt/ and be a valid path")
					}
					return nil
				}),
		),
	)

	config := &Settings{
		ApiToken: token,
		Host:     host,
		Port:     port,
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}

	configPath, err := xdg.ConfigFile("truenas/config.yaml")
	if err != nil {
		return nil, err
	}

	err = os.WriteFile(configPath, data, 0o600)
	if err != nil {
		return nil, err
	}

	return config, nil
}
