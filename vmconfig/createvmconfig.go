package vmconfig

import (
	"fmt"
	"log"

	"charm.land/huh/v2"
	"github.com/deevus/truenas-go/client"
	"golang.org/x/crypto/ssh"
)

type CreateVMConfig struct {
	VM        VMConfig    `yaml:"vm,omitempty"`
	CloudInit CloudConfig `yaml:"cloud_init,omitempty"`
}

func EditConfig(config *CreateVMConfig, client client.Client, skipvm bool) (*CreateVMConfig, error) {
	if !skipvm {
		vmConfig, err := createVMConfig(config.VM, client)
		if err != nil {
			return nil, err
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
						huh.NewOption("Cancel", "cancel"),
					).
					Value(&action),
			),
		)

		err = form.Run()
		if err != nil {
			log.Fatal(err)
		}

		config.VM = *vmConfig

		switch action {
		case "edit":
			return EditConfig(config, client, false)
		case "cancel":
			fmt.Println("VM creation cancelled")
			return nil, nil
		}
	}

	if config.CloudInit.Hostname == "" {
		config.CloudInit.Hostname = config.VM.Name
	}

	cloudInitConfig, err := createCloudInitConfig(config.CloudInit)
	if err != nil {
		return nil, err
	}

	var rootUser *User
	if cloudInitConfig.Users != nil {
		for i := range cloudInitConfig.Users {
			if cloudInitConfig.Users[i].Name == "root" {
				rootUser = cloudInitConfig.Users[i]
				break
			}
		}
	}

	ciStr := fmt.Sprintf(`Hostname: %s
Timezone: %s
Root Password Login Enabled: %t
Root SSH Keys:
%s`,
		cloudInitConfig.Hostname,
		cloudInitConfig.Timezone,
		rootUser.Password != "",
		func() string {
			if rootUser == nil || len(rootUser.AuthorizedKeys) == 0 {
				return "None"
			}
			keys := ""
			for _, key := range rootUser.AuthorizedKeys {
				k, name, _, _, err := ssh.ParseAuthorizedKey([]byte(key))
				if err != nil {
					continue
				}
				keys += fmt.Sprintf("- %s (%s)\n", ssh.FingerprintSHA256(k), name)
			}
			return keys
		}(),
	)

	var action string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Cloud-Init Configuration").
				Description("Review the cloud-init configuration before creating the VM."),

			huh.NewNote().
				Title("Details").
				Description(ciStr),

			huh.NewSelect[string]().
				Title("Action").
				Options(
					huh.NewOption("Continue", "create"),
					huh.NewOption("Edit Config", "edit"),
					huh.NewOption("Cancel", "cancel"),
				).
				Value(&action),
		),
	)

	err = form.Run()
	if err != nil {
		log.Fatal(err)
	}

	config.CloudInit = *cloudInitConfig

	switch action {
	case "edit":
		return EditConfig(config, client, true)
	case "cancel":
		fmt.Println("VM creation cancelled")
		return nil, nil
	}

	return config, nil
}
