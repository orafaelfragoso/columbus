package cli

import (
	"bufio"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/orafaelfragoso/columbus/internal/config"
	"github.com/orafaelfragoso/columbus/internal/contract"
)

// projectLocation resolves the on-disk locations a destructive command acts on,
// without opening the store. It returns NOT_INITIALIZED when no project exists.
type projectLocation struct {
	ConfigPath string
	ProjectID  string
	Paths      config.Paths
}

func (env *Env) projectLocation() (projectLocation, error) {
	configPath := filepath.Join(env.WorkDir, config.FileName)
	loaded, err := config.Load(configPath)
	if err != nil {
		return projectLocation{}, err
	}
	dataDir, err := config.DataDir(env.Getenv)
	if err != nil {
		return projectLocation{}, err
	}
	return projectLocation{
		ConfigPath: configPath,
		ProjectID:  loaded.Config.ProjectID,
		Paths:      config.ProjectPaths(dataDir, loaded.Config.ProjectID),
	}, nil
}

// confirmDestructive gates an irreversible action. With --yes it proceeds. On a
// TTY it prompts; otherwise it refuses and tells the caller to pass --yes. The
// prompt names what is removed and suggests exporting first.
func confirmDestructive(env *Env, yes bool, what string) error {
	if yes {
		return nil
	}
	if !isTTY(env.Stdout) || env.Stdin == nil {
		return contract.Errorf(contract.CodeUsage,
			"%s — this is irreversible; re-run with --yes to confirm (export first with `columbus export`)", what)
	}
	fmt.Fprintf(env.Stderr, "%s\nThis cannot be undone; export first with `columbus export`.\nType 'yes' to continue: ", what)
	line, _ := bufio.NewReader(env.Stdin).ReadString('\n')
	if strings.TrimSpace(line) != "yes" {
		return contract.Errorf(contract.CodeUsage, "aborted")
	}
	return nil
}
