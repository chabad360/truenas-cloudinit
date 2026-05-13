package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"regexp"
	"strconv"

	"charm.land/huh/v2"
	"github.com/deevus/truenas-go"
	"github.com/deevus/truenas-go/client"
	"gopkg.in/yaml.v3"
)

type VMConfig struct {
	Name    string `yaml:"name,omitempty"`
	ImageId string ` yaml:"image_id,omitempty"`

	VCPUs   int `yaml:"vcpus,omitempty"`
	Cores   int `yaml:"cores,omitempty"`
	Threads int `yaml:"threads,omitempty"`

	Memory    int `yaml:"memory,omitempty"`
	MinMemory int `yaml:"min_memory,omitempty"`

	DiskDir  string `yaml:"disk_dir,omitempty"`
	DiskSize int    `yaml:"disk_size,omitempty"`
}

func saveVMConfigAsYAML(vmConfig *VMConfig) {
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

	cfg := CreateVMConfig{
		VM: *vmConfig,
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile(p, data, 0o600)

}

func createVMConfig(defaultConfig VMConfig, client client.Client) (*VMConfig, error) {
	vmConfig := defaultConfig

	var (
		VCPUs     = strconv.Itoa(vmConfig.VCPUs)
		Cores     = strconv.Itoa(vmConfig.Cores)
		Threads   = strconv.Itoa(vmConfig.Threads)
		Memory    = strconv.Itoa(vmConfig.Memory)
		MinMemory = strconv.Itoa(vmConfig.MinMemory)
		DiskSize  = strconv.Itoa(vmConfig.DiskSize)
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
				Value(&vmConfig.Name).
				Validate(func(s string) error {
					if !vmNameRegex.MatchString(s) {
						return fmt.Errorf("VM name must contain only letters, numbers, or underscores")
					}
					return nil
				}),

			huh.NewSelect[string]().
				Title("Image").
				OptionsFunc(func() []huh.Option[string] {
					opts := []huh.Option[string]{}
					for k, img := range images {
						opts = append(opts, huh.Option[string]{Value: k, Key: fmt.Sprintf("%s (%.2f GB)", img.Name, float64(img.Size)/(1000*1000*1000))})
					}
					return opts
				}, &vmConfig.ImageId).
				Value(&vmConfig.ImageId),
		),
		huh.NewGroup(
			huh.NewNote().
				Title("CPU Configuration").
				Description("Specify the CPU configuration for your VM."),

			huh.NewInput().
				Title("VCPU Count").
				Placeholder("Enter the number of VCPUs").
				Value(&VCPUs).
				Validate(func(s string) error {
					if _, err := strconv.Atoi(s); err != nil {
						return fmt.Errorf("VCPU count must be a number")
					}
					return nil
				}),
			huh.NewInput().
				Title("Core Count").
				Placeholder("Enter the number of Cores").
				Value(&Cores).
				Validate(func(s string) error {
					if _, err := strconv.Atoi(s); err != nil {
						return fmt.Errorf("CPU count must be a number")
					}
					return nil
				}),
			huh.NewInput().
				Title("Thread Count").
				Placeholder("Enter the number of Threads").
				Value(&Threads).
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
				Value(&Memory).
				Validate(func(s string) error {
					if _, err := strconv.Atoi(s); err != nil {
						return fmt.Errorf("Memory must be a number")
					}
					return nil
				}),
			huh.NewInput().
				Title("Minimum Memory (MiB)").
				Placeholder("Enter the minimum amount of memory in MiB (Optional)").
				Value(&MinMemory).
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
				Value(&vmConfig.DiskDir).
				SuggestionsFunc(func() []string {
					p := path.Dir(vmConfig.DiskDir)
					r, err := fsService.ListDir(context.Background(), p, truenas.ListDirOpts{})
					if err != nil {
						return nil
					}
					var suggestions []string
					for _, entry := range r {
						suggestions = append(suggestions, entry.Path)
					}
					return suggestions
				}, &vmConfig.DiskDir).
				Validate(func(s string) error {
					if !dirRegex.MatchString(s) {
						return fmt.Errorf("Disk directory must start with /mnt/ and be a valid path")
					}
					return nil
				}),

			huh.NewInput().
				Title("Disk Size").
				Placeholder("Enter the size of the VM disk in GiB (e.g., 20)").
				Value(&DiskSize).
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
		return nil, err
	}

	vmConfig.VCPUs, _ = strconv.Atoi(VCPUs)
	vmConfig.Cores, _ = strconv.Atoi(Cores)
	vmConfig.Threads, _ = strconv.Atoi(Threads)
	vmConfig.Memory, _ = strconv.Atoi(Memory)
	if MinMemory != "" {
		vmConfig.MinMemory, _ = strconv.Atoi(MinMemory)
	} else {
		vmConfig.MinMemory = 0
	}
	vmConfig.DiskSize, _ = strconv.Atoi(DiskSize)

	return &vmConfig, nil
}
