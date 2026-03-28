package pkg

import (
	"fmt"
	"os"
	"os/exec"
)

// Installer is the interface for OS package managers.
type Installer interface {
	// Install installs the named package.
	Install(pkg string) error
	// IsInstalled returns true if the named command exists in PATH.
	IsInstalled(cmd string) bool
}

// NewInstaller returns the appropriate Installer for the given OS ID.
func NewInstaller(osID string) Installer {
	switch osID {
	case "debian", "ubuntu":
		return &aptInstaller{}
	case "centos", "rhel", "rocky", "almalinux", "fedora":
		return &yumInstaller{}
	case "alpine":
		return &apkInstaller{}
	default:
		return &aptInstaller{} // best guess
	}
}

// ---------------------------------------------------------------------------
// apt (Debian / Ubuntu)
// ---------------------------------------------------------------------------

type aptInstaller struct{}

func (a *aptInstaller) IsInstalled(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func (a *aptInstaller) Install(pkg string) error {
	// Add Fluent Bit GPG key if not present
	keyPath := "/usr/share/keyrings/fluentbit-keyring.gpg"
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		if err := addFluentBitGPGKey(keyPath); err != nil {
			return fmt.Errorf("apt: add gpg key: %w", err)
		}
	}

	if err := runCmd("apt-get", "update", "-qq"); err != nil {
		return fmt.Errorf("apt: update: %w", err)
	}
	if err := runCmd("apt-get", "install", "-y", "-qq", pkg); err != nil {
		return fmt.Errorf("apt: install %s: %w", pkg, err)
	}
	return nil
}

// addFluentBitGPGKey fetches the Fluent Bit GPG key and saves it as a keyring.
func addFluentBitGPGKey(keyPath string) error {
	// Download with curl and dearmor with gpg
	curlCmd := exec.Command("curl", "-fsSL", "https://packages.fluentbit.io/fluentbit.key")
	gpgCmd := exec.Command("gpg", "--dearmor", "-o", keyPath)

	curlOut, err := curlCmd.StdoutPipe()
	if err != nil {
		return err
	}
	gpgCmd.Stdin = curlOut

	if err := curlCmd.Start(); err != nil {
		return fmt.Errorf("curl start: %w", err)
	}
	if err := gpgCmd.Run(); err != nil {
		return fmt.Errorf("gpg dearmor: %w", err)
	}
	return curlCmd.Wait()
}

// AddAptRepo writes the Fluent Bit apt source list entry.
func AddAptRepo(osID, repoCodename string) error {
	content := fmt.Sprintf(
		"deb [signed-by=/usr/share/keyrings/fluentbit-keyring.gpg] https://packages.fluentbit.io/%s/%s %s main\n",
		osID, repoCodename, repoCodename,
	)
	return os.WriteFile("/etc/apt/sources.list.d/fluent-bit.list", []byte(content), 0644)
}

// ---------------------------------------------------------------------------
// yum (CentOS / RHEL / Rocky / Alma / Fedora)
// ---------------------------------------------------------------------------

type yumInstaller struct{}

func (y *yumInstaller) IsInstalled(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func (y *yumInstaller) Install(pkg string) error {
	// Write repo file if not present
	repoPath := "/etc/yum.repos.d/fluent-bit.repo"
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		repoContent := `[fluent-bit]
name=Fluent Bit
baseurl=https://packages.fluentbit.io/centos/$releasever/$basearch/
gpgcheck=1
gpgkey=https://packages.fluentbit.io/fluentbit.key
enabled=1
`
		if err := os.WriteFile(repoPath, []byte(repoContent), 0644); err != nil {
			return fmt.Errorf("yum: write repo file: %w", err)
		}
	}
	if err := runCmd("yum", "install", "-y", pkg); err != nil {
		return fmt.Errorf("yum: install %s: %w", pkg, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// apk (Alpine)
// ---------------------------------------------------------------------------

type apkInstaller struct{}

func (a *apkInstaller) IsInstalled(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func (a *apkInstaller) Install(pkg string) error {
	if err := runCmd("apk", "add", "--no-cache", pkg); err != nil {
		return fmt.Errorf("apk: install %s: %w", pkg, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
