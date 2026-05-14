package vmconfig

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"slices"
	"sort"
	"time"

	"charm.land/huh/v2"
	tz "github.com/tkuchiki/go-timezone"
	cloudinit "go.pilab.hu/cloud/cloud-init"
	"golang.org/x/crypto/ssh"
)

func createCloudInitConfig(defaultConfig cloudinit.CloudConfig) (*cloudinit.CloudConfig, error) {
	cloudInitConfig := defaultConfig
	if cloudInitConfig.Timezone == "" {
		cloudInitConfig.Timezone = time.Now().Location().String()
	}

	rootUser := &cloudinit.User{
		Name:           "root",
		AuthorizedKeys: []string{},
	}
	if cloudInitConfig.Users != nil {
		for i := range cloudInitConfig.Users {
			if cloudInitConfig.Users[i].Name == "root" {
				rootUser = cloudInitConfig.Users[i]
				break
			}
		}
	}

	tzdb := tz.New()
	tzs := tzdb.TzInfos()

	keys, err := getSSHKeys()
	if err != nil {
		return nil, err
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Cloud-Init Configuration").
				Description("Fill in the details for your cloud-init configuration."),

			huh.NewInput().
				Title("Hostname").
				Value(&cloudInitConfig.Hostname),
		),

		huh.NewGroup(
			huh.NewNote().
				Title("Root SSH Configuration").
				Description("Configure the root user for the VM."),

			huh.NewInput().
				Title("Root Password").
				Description("The password for the root user, if empty password login is disabled").
				EchoMode(huh.EchoModePassword).
				Value(&rootUser.Password),

			huh.NewMultiSelect[string]().
				Title("SSH Authorized Keys").
				Description("Public SSH keys to add to the root user's authorized_keys file").
				Height(12).
				Options(func() []huh.Option[string] {
					keys := append(keys, rootUser.AuthorizedKeys...)
					slices.Sort(keys)
					keys = slices.Compact(keys)
					options := []huh.Option[string]{}
					for _, key := range keys {
						k, name, _, _, err := ssh.ParseAuthorizedKey([]byte(key))
						if err != nil {
							continue
						}
						options = append(options, huh.NewOption(fmt.Sprintf("%s (%s)", ssh.FingerprintSHA256(k), name), key))
					}

					return options
				}()...).
				Validate(func(selected []string) error {
					if len(selected) == 0 && rootUser.Password == "" {
						return fmt.Errorf("at least one SSH key must be selected")
					}
					return nil
				}).
				Filterable(true).
				Value(&rootUser.AuthorizedKeys),
		),

		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Timezone").
				Options(func() []huh.Option[string] {
					options := make([]huh.Option[string], len(tzs))
					i := 0
					for k := range tzs {
						options[i] = huh.NewOption(k, k)
						i++
					}
					sort.Slice(options, func(i, j int) bool {
						return options[i].Value < options[j].Value
					})
					return options
				}()...).
				Value(&cloudInitConfig.Timezone),
		),
	)

	err = form.Run()
	if err != nil {
		return nil, err
	}

	if rootUser.Password != "" {
		rootUser.EnableSSHPasswordAuth = true
		rootUser.Password = cloudinit.EncryptPassword(rootUser.Password)
		cloudInitConfig.EnableSSHPasswordAuth = true
	}

	if cloudInitConfig.Users == nil {
		cloudInitConfig.Users = []*cloudinit.User{rootUser}
	} else {
		found := false
		for i := range cloudInitConfig.Users {
			if cloudInitConfig.Users[i].Name == "root" {
				cloudInitConfig.Users[i] = rootUser
				found = true
				break
			}
		}
		if !found {
			cloudInitConfig.Users = append(cloudInitConfig.Users, rootUser)
		}
	}

	return &cloudInitConfig, nil
}

func getSSHKeys() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	sshDir := path.Join(home, ".ssh")
	dir, err := os.ReadDir(sshDir)
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, file := range dir {
		if file.IsDir() {
			continue
		}
		if !file.Type().IsRegular() {
			continue
		}
		if path.Ext(file.Name()) == ".pub" {
			keyData, err := os.ReadFile(path.Join(sshDir, file.Name()))
			if err != nil {
				continue
			}
			keys = append(keys, string(keyData))
		}
	}
	akFile, err := os.Open(path.Join(sshDir, "authorized_keys"))
	if err != nil {
		return keys, nil
	}
	defer akFile.Close()
	akScanner := bufio.NewScanner(akFile)
	for akScanner.Scan() {
		keys = append(keys, akScanner.Text())
	}
	return keys, nil
}
