package application

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cloudfoundry/bosh-bootloader/flags"
)

var getwd func() (string, error) = os.Getwd

type CommandLineConfiguration struct {
	Command         string
	SubcommandFlags []string
	StateDir        string
	Debug           bool

	help    bool
	version bool
}

type CommandLineParser struct {
	usage      func()
	commandSet CommandSet
	envGetter  envGetter
}

type envGetter interface {
	Get(name string) string
}

func NewCommandLineParser(usage func(), commandSet CommandSet, envGetter envGetter) CommandLineParser {
	return CommandLineParser{
		usage:      usage,
		commandSet: commandSet,
		envGetter:  envGetter,
	}
}

func (p CommandLineParser) Parse(arguments []string) (CommandLineConfiguration, error) {
	var err error
	commandLineConfiguration := CommandLineConfiguration{}
	var commandNotFoundError error
	commandWasBlank := false

	commandFinderResult := NewCommandFinder().FindCommand(arguments)

	_, ok := p.commandSet[commandFinderResult.Command]
	if !ok {
		if commandFinderResult.Command == "" {
			commandWasBlank = true
		} else {
			commandNotFoundError = fmt.Errorf("Unrecognized command '%s'", commandFinderResult.Command)
		}
	}

	commandLineConfiguration.SubcommandFlags = commandFinderResult.OtherArgs
	commandLineConfiguration, _, err = p.parseGlobalFlags(commandLineConfiguration, commandFinderResult.GlobalFlags)
	if err != nil && commandNotFoundError == nil {
		p.usage()
		return CommandLineConfiguration{}, err
	}

	commandLineConfiguration.Command = commandFinderResult.Command
	if commandLineConfiguration.version {
		commandLineConfiguration.Command = "version"
	} else if commandLineConfiguration.help || commandWasBlank {
		commandLineConfiguration.Command = "help"
		if !commandWasBlank {
			commandLineConfiguration.SubcommandFlags = append([]string{commandFinderResult.Command}, commandLineConfiguration.SubcommandFlags...)
		}
	}
	if commandNotFoundError != nil {
		p.usage()
		return CommandLineConfiguration{}, commandNotFoundError
	}

	commandLineConfiguration, err = p.setDefaultStateDirectory(commandLineConfiguration)
	if err != nil {
		return CommandLineConfiguration{}, err
	}

	return commandLineConfiguration, nil
}

func (c CommandLineParser) parseGlobalFlags(commandLineConfiguration CommandLineConfiguration, arguments []string) (CommandLineConfiguration, []string, error) {
	if err := c.validateGlobalFlags(arguments); err != nil {
		return commandLineConfiguration, []string{}, err
	}

	debugEnv := c.envGetter.Get("BBL_DEBUG")

	globalFlags := flags.New("global")

	globalFlags.String(&commandLineConfiguration.StateDir, "state-dir", "")
	globalFlags.Bool(&commandLineConfiguration.Debug, "d", "debug", (debugEnv == "true"))

	globalFlags.Bool(&commandLineConfiguration.help, "h", "help", false)
	globalFlags.Bool(&commandLineConfiguration.version, "v", "version", false)

	err := globalFlags.Parse(arguments)
	if err != nil {
		return CommandLineConfiguration{}, []string{}, err
	}

	return commandLineConfiguration, globalFlags.Args(), nil
}

func (c CommandLineParser) validateGlobalFlags(arguments []string) error {
	hasStateDir := false
	for _, argument := range arguments {
		name := strings.Split(argument, "=")[0]
		if name == "--state-dir" || name == "-state-dir" {
			if hasStateDir {
				return errors.New("Invalid usage: cannot specify global 'state-dir' flag more than once.")
			}

			hasStateDir = true
		}
	}

	return nil
}

func (c CommandLineParser) parseCommandAndSubcommandFlags(commandLineConfiguration CommandLineConfiguration, remainingArguments []string) (CommandLineConfiguration, error) {
	if len(remainingArguments) == 0 {
		c.usage()
		return CommandLineConfiguration{}, errors.New("unknown command: [EMPTY]")
	}

	commandLineConfiguration.Command = remainingArguments[0]
	commandLineConfiguration.SubcommandFlags = remainingArguments[1:]

	return commandLineConfiguration, nil
}

func (CommandLineParser) setDefaultStateDirectory(commandLineConfiguration CommandLineConfiguration) (CommandLineConfiguration, error) {
	if commandLineConfiguration.StateDir == "" {
		wd, err := getwd()
		if err != nil {
			return CommandLineConfiguration{}, err
		}

		commandLineConfiguration.StateDir = wd
	}

	return commandLineConfiguration, nil
}
