package filesystem

import (
	"path/filepath"
	"testing"
)

func TestPathsAreAbsolute(t *testing.T) {
	paths := []string{
		ConfigDir,
		ServicesDir,
		NftablesDir,
		AgentConfigDir,
		BackupDir,
		StateFile,
		ComponentsDir,
		LockFilePath,
		SystemdDir,
		LogDir,
		RunDir,
		DefaultCertPath,
		DefaultKeyPath,
	}

	for _, p := range paths {
		if !filepath.IsAbs(p) {
			t.Errorf("%s is not an absolute path", p)
		}
	}
}

func TestAllowedWritePrefixesNotEmpty(t *testing.T) {
	if len(AllowedWritePrefixes) == 0 {
		t.Error("AllowedWritePrefixes is empty — must list writable paths")
	}
}

func TestDefaltPathsUnderExpectedRoots(t *testing.T) {
	if filepath.Dir(DefaultCertPath) != AgentConfigDir {
		t.Errorf("DefaultCertPath %s should be under %s", DefaultCertPath, AgentConfigDir)
	}
	if filepath.Dir(DefaultKeyPath) != AgentConfigDir {
		t.Errorf("DefaultKeyPath %s should be under %s", DefaultKeyPath, AgentConfigDir)
	}
}
