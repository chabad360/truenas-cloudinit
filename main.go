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

	"charm.land/huh/v2"
	"charm.land/huh/v2/spinner"
	"github.com/adrg/xdg"
	"github.com/deevus/truenas-go"
	"github.com/deevus/truenas-go/client"
	"gopkg.in/yaml.v3"
)

type Config struct {
	ApiToken string `yaml:"api_token"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	ISODir   string `yaml:"iso_dir,omitempty"`
}

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
		createVM(wsClient)
	case "edit_config":
		fmt.Println("Edit config not implemented yet")
	case "exit":
		fmt.Println("Goodbye!")
		os.Exit(0)
	}

}

func createVM(client client.Client) {
	var (
		vmName  string
		imageId int

		vcpus   = "1"
		cpus    = "1"
		threads = "1"

		memory    = "512"
		minMemory string

		diskDir  = "/mnt/"
		diskSize = "20"
	)
	vmNameRegex := regexp.MustCompile("^[A-Za-z0-9_]+$")
	dirRegex := regexp.MustCompile(`^/mnt/[^/]+(/[^/]+)*$`)
	fsService := truenas.NewFilesystemService(client, client.Version())

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Create a new VM").
				Description("Fill out the form to create a new VM on your TrueNAS server."),

			huh.NewInput().
				Title("VM Name").
				Placeholder("Enter a name for your VM").
				Value(&vmName).
				Validate(func(s string) error {
					if !vmNameRegex.MatchString(s) {
						return fmt.Errorf("VM name must contain only letters, numbers, or underscores")
					}
					return nil
				}),

			huh.NewSelect[int]().
				Title("Image").
				OptionsFunc(func() []huh.Option[int] {
					opts := make([]huh.Option[int], len(images))
					for i, img := range images {
						opts[i] = huh.Option[int]{Value: i, Key: fmt.Sprintf("%s (%.2f GB)", img.Name, float64(img.Size)/(1000*1000*1000))}
					}
					return opts
				}, &imageId),
		),
		huh.NewGroup(
			huh.NewNote().
				Title("CPU Configuration").
				Description("Specify the CPU configuration for your VM."),

			huh.NewInput().
				Title("VCPU Count").
				Placeholder("Enter the number of VCPUs").
				Value(&vcpus).
				Validate(func(s string) error {
					if _, err := strconv.Atoi(s); err != nil {
						return fmt.Errorf("VCPU count must be a number")
					}
					return nil
				}),
			huh.NewInput().
				Title("Core Count").
				Placeholder("Enter the number of Cores").
				Value(&cpus).
				Validate(func(s string) error {
					if _, err := strconv.Atoi(s); err != nil {
						return fmt.Errorf("CPU count must be a number")
					}
					return nil
				}),
			huh.NewInput().
				Title("Thread Count").
				Placeholder("Enter the number of Threads").
				Value(&threads).
				Validate(func(s string) error {
					if _, err := strconv.Atoi(s); err != nil {
						return fmt.Errorf("Thread count must be a number")
					}
					return nil
				}),
		),
		huh.NewGroup(
			huh.NewNote().
				Title("Memory Configuration").
				Description("Specify the memory configuration for your VM. You can set a minimum memory value that allows the VM to start even if the full amount of memory is not available."),

			huh.NewInput().
				Title("Memory (MiB)").
				Placeholder("Enter the amount of memory in MiB").
				Value(&memory).
				Validate(func(s string) error {
					if _, err := strconv.Atoi(s); err != nil {
						return fmt.Errorf("Memory must be a number")
					}
					return nil
				}),
			huh.NewInput().
				Title("Minimum Memory (MiB)").
				Placeholder("Enter the minimum amount of memory in MiB (Optional)").
				Value(&minMemory).
				Validate(func(s string) error {
					if s == "" {
						return nil // Allow empty value for minimum memory
					}
					if _, err := strconv.Atoi(s); err != nil {
						return fmt.Errorf("Minimum Memory must be a number")
					}
					return nil
				}),
		),
		huh.NewGroup(
			huh.NewNote().
				Title("Disk Configuration").
				Description("The selected image will be used to initialize the VM, please choose a directory where the VM disk will be stored. The image will be copied to this location and used as the VM's disk."),

			huh.NewInput().
				Title("Disk Directory").
				Placeholder("Enter the directory where the VM disk will be stored (e.g., /mnt/pool/vm-disks)").
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

			huh.NewInput().
				Title("Disk Size").
				Placeholder("Enter the size of the VM disk in GiB (e.g., 20)").
				Value(&diskSize).
				Validate(func(s string) error {
					if _, err := strconv.Atoi(s); err != nil {
						return fmt.Errorf("Disk size must be a number")
					}
					return nil
				}),
		),
	)

	err := form.Run()
	if err != nil {
		log.Fatal(err)
	}
}

func getConfig() (*Config, error) {
	path, err := xdg.SearchConfigFile("truenas/config.yaml")
	if err != nil {
		return nil, err
	}
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config := &Config{}
	err = yaml.Unmarshal(file, config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func getApiToken() (*Config, error) {
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
				Title("Disk Directory").
				Placeholder("Enter the directory where the VM disk will be stored (e.g., /mnt/pool/vm-disks)").
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

	config := &Config{
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
