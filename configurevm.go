package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"time"
	"truenas-cloudinit/vmconfig"

	"charm.land/huh/v2"
	"github.com/deevus/truenas-go/client"
	"gopkg.in/yaml.v3"
)

func configureVM(wsClient client.Client) (*vmconfig.CreateVMConfig, error) {
	action := ""

	err := huh.NewSelect[string]().
		Title("Create a VM").
		Description("How would you like to create your VM?").
		Options(
			huh.NewOption("From Scratch", "from_scratch"),
			huh.NewOption("Modify From Default Template", "from_template"),
			huh.NewOption("From Existing Config", "from_config"),
			huh.NewOption("Cancel", "cancel"),
		).
		Value(&action).
		Run()
	if err != nil {
		log.Fatal(err)
	}

	cfg := emptyConfig()
	switch action {
	case "from_scratch":
		cfg, err = vmconfig.EditConfig(cfg, wsClient, false)
		if err != nil {
			log.Fatal(err)
		}
	case "from_template":
		cfg = loadTemplate()
		cfg, err = vmconfig.EditConfig(cfg, wsClient, false)
		if err != nil {
			log.Fatal(err)
		}
	case "from_config":
		cfg = loadConfig()
	case "cancel":
		fmt.Println("VM creation cancelled")
		return nil, nil
	}

	err = reviewConfig(cfg, wsClient)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	return cfg, nil
}

func reviewConfig(cfg *vmconfig.CreateVMConfig, c client.Client) error {
	var action string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Review Configuration").
				Description("Review the VM and Cloud-Init configuration before creating the VM."),

			huh.NewNote().
				Title("VM Details").
				Description(fmt.Sprintf(`%s: %s, 
 - %d VCPUs, 
 - %d Cores, 
 - %d Threads, 

 - %d MiB Memory, 
 - %d MiB Min Memory, 

 - %d GiB Disk`,
					cfg.VM.Name, cfg.VM.ImageId, cfg.VM.VCPUs, cfg.VM.Cores, cfg.VM.Threads, cfg.VM.Memory, cfg.VM.MinMemory, cfg.VM.DiskSize)),

			huh.NewNote().
				Title("Cloud-Init Details").
				Description(fmt.Sprintf(`Hostname: %s
Timezone: %s
Packages: %v`,
					cfg.CloudInit.Hostname, cfg.CloudInit.Timezone, cfg.CloudInit.Packages)),

			huh.NewSelect[string]().
				Title("Action").
				Options(
					huh.NewOption("Create VM", "create"),
					huh.NewOption("Edit Config", "edit"),
					huh.NewOption("Save Config as YAML", "save"),
					huh.NewOption("Cancel", "cancel"),
				).
				Value(&action),
		),
	)

	err := form.Run()
	if err != nil {
		return err
	}

	switch action {
	case "edit":
		newCfg, err := vmconfig.EditConfig(cfg, c, false)
		if err != nil {
			return err
		}
		return reviewConfig(newCfg, c)
	case "save":
		saveCloudInitConfigAsYAML(cfg)
		os.Exit(0)
	case "cancel":
		fmt.Println("VM creation cancelled")
		os.Exit(0)
	}

	return nil
}

func emptyConfig() *vmconfig.CreateVMConfig {
	return &vmconfig.CreateVMConfig{
		VM: vmconfig.VMConfig{
			Name:    "",
			ImageId: "",

			VCPUs:   1,
			Cores:   1,
			Threads: 1,

			Memory: 512,

			DiskSize: 10,
		},
		CloudInit: vmconfig.CloudConfig{
			Packages: []string{"qemu-guest-agent"},
			Timezone: time.Now().Location().String(),
		},
	}
}

func loadTemplate() *vmconfig.CreateVMConfig {
	template := &vmconfig.CreateVMConfig{
		VM: vmconfig.VMConfig{
			Name:    "",
			ImageId: settings.VMDefaults.ImageId,

			VCPUs:   settings.VMDefaults.VCPUs,
			Cores:   settings.VMDefaults.Cores,
			Threads: settings.VMDefaults.Threads,

			Memory:    settings.VMDefaults.Memory,
			MinMemory: settings.VMDefaults.MinMemory,

			DiskDir:  settings.VMDefaults.DiskDir,
			DiskSize: settings.VMDefaults.DiskSize,
		},
		CloudInit: vmconfig.CloudConfig{
			Packages: []string{"qemu-guest-agent"},
		},
	}

	return template
}

func loadConfig() *vmconfig.CreateVMConfig {
	p, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	err = huh.NewInput().
		Title("Config Location").
		Description("Enter the location where the VM configuration YAML file is located.").
		Value(&p).
		SuggestionsFunc(func() []string {
			files, err := os.ReadDir(p)
			if err != nil {
				return nil
			}
			var suggestions []string
			for _, file := range files {
				if file.IsDir() || path.Ext(file.Name()) == ".yaml" || path.Ext(file.Name()) == ".yml" {
					suggestions = append(suggestions, path.Join(p, file.Name()))
				}
			}
			return suggestions
		}, &p).
		Validate(func(s string) error {
			if path.Ext(p) != ".yaml" && path.Ext(p) != ".yml" {
				return fmt.Errorf("file extension must be .yaml or .yml")
			}
			if _, err := os.Stat(p); os.IsNotExist(err) {
				return fmt.Errorf("file does not exist")
			}
			return nil
		}).Run()
	if err != nil {
		log.Fatal(err)
	}

	data, err := os.ReadFile(p)
	if err != nil {
		log.Fatal(err)
	}

	var cfg vmconfig.CreateVMConfig
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		log.Fatal(err)
	}

	return &cfg
}

func saveCloudInitConfigAsYAML(cfg *vmconfig.CreateVMConfig) {
	p, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	err = huh.NewInput().
		Title("Save Location").
		Description("Enter the location where the VM configuration YAML file will be saved.").
		Value(&p).
		SuggestionsFunc(func() []string {
			files, err := os.ReadDir(p)
			if err != nil {
				return nil
			}
			var suggestions []string
			for _, file := range files {
				if file.IsDir() {
					suggestions = append(suggestions, p+"/"+file.Name())
				}
			}
			return suggestions
		}, &p).
		Validate(func(s string) error {
			if _, err := os.Stat(path.Dir(p)); os.IsNotExist(err) {
				return fmt.Errorf("directory does not exist")
			}
			if path.Ext(p) != ".yaml" && path.Ext(p) != ".yml" {
				return fmt.Errorf("file extension must be .yaml or .yml")
			}
			return nil
		}).Run()
	if err != nil {
		log.Fatal(err)
	}

	if _, err := os.Stat(p); err == nil {
		var overwrite bool
		err = huh.NewConfirm().
			Title("File already exists").
			Description("A file already exists at the specified location. Do you want to overwrite it?").
			Value(&overwrite).
			Run()
		if err != nil {
			log.Fatal(err)
		}
		if !overwrite {
			fmt.Println("Save cancelled")
			return
		}
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile(p, data, 0o600)

}
