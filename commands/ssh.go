package commands

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cloudfoundry/bosh-bootloader/fileio"
	"github.com/cloudfoundry/bosh-bootloader/flags"
	"github.com/cloudfoundry/bosh-bootloader/storage"
)

type SSH struct {
	cmd           sshCmd
	keyGetter     sshKeyGetter
	tempDirWriter tempDirWriter
	randomPort    randomPort
}

type sshCmd interface {
	Run([]string) error
}

type randomPort interface {
	GetPort() (string, error)
}

type tempDirWriter interface {
	fileio.FileWriter
	fileio.TempDirer
}

func NewSSH(sshCmd sshCmd, sshKeyGetter sshKeyGetter, tempDirWriter tempDirWriter, randomPort randomPort) SSH {
	return SSH{
		cmd:           sshCmd,
		keyGetter:     sshKeyGetter,
		tempDirWriter: tempDirWriter,
		randomPort:    randomPort,
	}
}

func (s SSH) CheckFastFails(subcommandFlags []string, state storage.State) error {
	if len(state.Jumpbox.URL) == 0 {
		return errors.New("Invalid bbl state for bbl ssh.")
	}

	return nil
}

func (s SSH) Execute(args []string, state storage.State) error {
	var (
		jumpbox  bool
		director bool
	)
	sshFlags := flags.New("ssh")
	sshFlags.Bool(&jumpbox, "jumpbox")
	sshFlags.Bool(&director, "director")
	err := sshFlags.Parse(args)
	if err != nil {
		return err
	}

	if !jumpbox && !director {
		return fmt.Errorf("This command requires the --jumpbox or --director flag.")
	}

	tempDir, err := s.tempDirWriter.TempDir("", "")
	if err != nil {
		return fmt.Errorf("Create temp directory: %s", err)
	}

	jumpboxKey, err := s.keyGetter.Get("jumpbox")
	if err != nil {
		return fmt.Errorf("Get jumpbox private key: %s", err)
	}

	jumpboxKeyPath := filepath.Join(tempDir, "jumpbox-private-key")

	err = s.tempDirWriter.WriteFile(jumpboxKeyPath, []byte(jumpboxKey), 0600)
	if err != nil {
		return fmt.Errorf("Write private key file: %s", err)
	}

	jumpboxURL := strings.Split(state.Jumpbox.URL, ":")[0]

	if jumpbox {
		return s.cmd.Run([]string{"-o StrictHostKeyChecking=no -o ServerAliveInterval=300", fmt.Sprintf("jumpbox@%s", jumpboxURL), "-i", jumpboxKeyPath})
	}

	directorPrivateKey, err := s.keyGetter.Get("director")
	if err != nil {
		return fmt.Errorf("Get director private key: %s", err)
	}

	directorKeyPath := filepath.Join(tempDir, "director-private-key")

	err = s.tempDirWriter.WriteFile(directorKeyPath, []byte(directorPrivateKey), 0600)
	if err != nil {
		return fmt.Errorf("Write private key file: %s", err)
	}

	port, err := s.randomPort.GetPort()
	if err != nil {
		return fmt.Errorf("Open proxy port: %s", err)
	}

	err = s.cmd.Run([]string{"-4 -D", port, "-fNC", fmt.Sprintf("jumpbox@%s", jumpboxURL), "-i", jumpboxKeyPath})
	if err != nil {
		return fmt.Errorf("Open tunnel to jumpbox: %s", err)
	}

	ip := strings.Split(strings.TrimPrefix(state.BOSH.DirectorAddress, "https://"), ":")[0]

	return s.cmd.Run([]string{fmt.Sprintf("-o ProxyCommand=nc -x localhost:%s %s", port, "%h %p"), "-i", directorKeyPath, fmt.Sprintf("jumpbox@%s", ip)})
}