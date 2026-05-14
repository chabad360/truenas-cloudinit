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
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/ssh"
)

type User struct {
	// Name is the username.
	Name string `yaml:"name"`
	// Groups is a comma separated list of groups the user should be added to.
	Groups string `yaml:"groups,omitempty"`
	// Shell is the shell the user should use.
	Shell string `yaml:"shell,omitempty"`
	// Sudo is a list of sudo rules for the user.
	Sudo string `yaml:"sudo,omitempty"`
	// AuthorizedKeys is a list of SSH public keys for the user.
	AuthorizedKeys []string `yaml:"ssh_authorized_keys,omitempty"`
	// Password is the password hash for the user.
	Password string `yaml:"passwd,omitempty"`
	// LockPassword is a flag to lock the password.
	LockPassword bool `yaml:"lock_passwd"`
}

// PasswordChange holds the configuration for password change on first boot.
type PasswordChange struct {
	// Expire is a flag to force password change on first boot.
	Expire bool `yaml:"expire"`
	// List is a list of users to force password change.
	Users []UserPasswordChange `yaml:"users"`
}

type UserPasswordChange struct {
	// Name is the username.
	Name string `yaml:"name"`
	// Password is the password hash for the user.
	Password string `yaml:"password,omitempty"`
}

// CloudConfig holds the configuration for cloud-init.
//
//nolint:tagliatelle // This format is required by the cloud-init metadata.
type CloudConfig struct {
	// Users is a list of users to create.
	Users []*User `yaml:"users,omitempty"`
	// PasswordChange is the configuration for password change on first boot.
	PasswordChange PasswordChange `yaml:"chpasswd,omitempty"`
	// PackageUpdate is a flag to update the package list.
	PackageUpdate bool `yaml:"package_update,omitempty"`
	// PackageUpgrade is a flag to upgrade the packages.
	PackageUpgrade bool `yaml:"package_upgrade,omitempty"`
	// RunCommands is a list of commands to run.
	RunCommands []string `yaml:"runcmd,omitempty"`
	// TODO: write_files
	// Packages is a list of packages to install.
	Packages []string `yaml:"packages,omitempty"`
	// Timezone is the timezone to set.
	Timezone string `yaml:"timezone,omitempty"`
	// EnableSSHPasswordAuth is a flag to enable SSH password authentication for the root user.
	EnableSSHPasswordAuth bool `yaml:"ssh_pwauth,omitempty"`
	// Hostname is the hostname to set.
	Hostname string `yaml:"hostname,omitempty"`
	// Locale is the locale to set.
	Locale string `yaml:"locale,omitempty"`
	// Mounts is a list of mounts to configure.
	Mounts [][]string `yaml:"mounts,omitempty"`
	// DisableRoot is a flag to disable root login.
	DisableRoot bool `yaml:"disable_root"`
	// Growpart is the configuration for growpart.
	Growpart *GrowpartConfig `yaml:"growpart,omitempty"`
}

// Metadata holds the metadata for cloud-init.
//
//nolint:tagliatelle // This format is required by the cloud-init metadata.
type Metadata struct {
	// InstanceID is the instance ID.
	InstanceID string `yaml:"instance-id"`
	// LocalHostname is the hostname of the instance.
	LocalHostname string `yaml:"local-hostname"`
}

type GrowpartConfig struct {
	Mode    string   `yaml:"mode,omitempty"`
	Devices []string `yaml:"devices,omitempty"`
}

func createCloudInitConfig(defaultConfig CloudConfig) (*CloudConfig, error) {
	cloudInitConfig := defaultConfig
	if cloudInitConfig.Timezone == "" {
		cloudInitConfig.Timezone = time.Now().Location().String()
	}

	rootUser := &User{
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
		cloudInitConfig.DisableRoot = false
		chUser := UserPasswordChange{
			Name:     "root",
			Password: EncryptPassword(rootUser.Password),
		}
		cloudInitConfig.PasswordChange.Expire = false
		cloudInitConfig.PasswordChange.Users = append(cloudInitConfig.PasswordChange.Users, chUser)
		rootUser.Password = ""

		cloudInitConfig.EnableSSHPasswordAuth = true
	}

	if cloudInitConfig.Users == nil {
		cloudInitConfig.Users = []*User{rootUser}
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

func EncryptPassword(password string) string {
	// Generate a salt and hash the password using bcrypt.
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "" //, fmt.Errorf("failed to hash password: %w", err)
	}

	// Return the hash as a string suitable for /etc/shadow.
	return fmt.Sprintf("%s", hash)
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
