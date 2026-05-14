package vmconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strconv"

	"charm.land/huh/v2"
	"github.com/deevus/truenas-go"
	"github.com/deevus/truenas-go/client"
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

	NIC        string `yaml:"nic,omitempty"`
	MACAddress string `yaml:"mac_address,omitempty"`
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

	r, err := client.Call(context.Background(), "vm.random_mac", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random MAC address: %w", err)
	}
	if err := json.Unmarshal(r, &vmConfig.MACAddress); err != nil {
		return nil, fmt.Errorf("failed to parse random MAC address: %w", err)
	}

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
					for k, img := range Images {
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
				Placeholder("Enter the directory where the VM ZVol will be stored (e.g., /mnt/pool/vm-disks)").
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

		huh.NewGroup(
			huh.NewNote().
				Title("NIC Selection").
				Description("Specify the network device configuration for the VM."),

			huh.NewInput().
				Title("MAC Address").
				Placeholder("Enter the MAC address for the VM's NIC (e.g., 02:00:00:00:00:01)").
				Value(&vmConfig.MACAddress).
				Validate(func(s string) error {
					macRegex := regexp.MustCompile(`^([0-9A-Fa-f]{2}[:]){5}([0-9A-Fa-f]{2})$`)
					if !macRegex.MatchString(s) {
						return fmt.Errorf("Invalid MAC address format")
					}
					return nil
				}),
			huh.NewSelect[string]().
				Title("NIC").
				Description("Select the NIC the virtual device will bind to").
				Options(func() []huh.Option[string] {
					r, err := client.Call(context.Background(), "vm.device.nic_attach_choices", nil)
					if err != nil {
						return nil
					}
					var results map[string]string
					if err := json.Unmarshal(r, &results); err != nil {
						return nil
					}
					var options []huh.Option[string]
					for k, v := range results {
						options = append(options, huh.NewOption(k, v))
					}
					return options
				}()...).
				Value(&vmConfig.NIC),
		),
	)

	err = form.Run()
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
