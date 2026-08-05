package common

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"maps"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/bitrise-io/go-utils/v2/log"
)

type CacheConfigMetadata struct {
	CIProvider   string
	CLIVersion   string
	RedactedEnvs map[string]string
	HostMetadata HostMetadata
	GitMetadata  GitMetadata
	// BitriseCI specific
	BitriseAppID           string
	BitriseWorkflowName    string
	BitriseBuildID         string
	BitriseStepExecutionID string
	Datacenter             string
	// External CI identifiers (non-Bitrise CI providers)
	ExternalAppID        string
	ExternalBuildID      string
	ExternalWorkflowName string
	// BenchmarkPhase is the benchmark phase (baseline, warmup, etc.)
	BenchmarkPhase string
}

const (
	// CIProviderBitrise ...
	CIProviderBitrise = "bitrise"
	// CIProviderCircleCI ...
	CIProviderCircleCI = "circle-ci"
	// CIProviderGitHubActions ...
	CIProviderGitHubActions = "github-actions"
	// CIProviderGitLabCI ...
	CIProviderGitLabCI = "gitlab-ci"

	RedactorSeed = "BitriseBuildCacheRedactor"
)

type CommandFunc func(string, ...string) (string, error)

// DetectCIProvider inspects environment variables to identify which CI provider
// (if any) the current build is running on. Returns "" when no CI is detected.
func DetectCIProvider(envs map[string]string) string {
	// Check other CI providers first, so that Build Hub builds
	// (which also set BITRISE_IO) detect the original CI provider.
	if envs["CIRCLECI"] != "" {
		// https://circleci.com/docs/variables/#built-in-environment-variables
		return CIProviderCircleCI
	}
	if envs["GITHUB_ACTIONS"] != "" {
		// https://docs.github.com/en/actions/learn-github-actions/variables#default-environment-variables
		return CIProviderGitHubActions
	}
	if envs["GITLAB_CI"] == "true" {
		// https://docs.gitlab.com/ci/variables/predefined_variables/
		return CIProviderGitLabCI
	}
	if envs["BITRISE_IO"] != "" && envs["BITRISE_BUILD_SLUG"] != "" {
		// https://devcenter.bitrise.io/en/references/available-environment-variables.html
		// Build Hub sets BITRISE_IO but not BITRISE_BUILD_SLUG
		return CIProviderBitrise
	}

	return ""
}

func detectExternalIDs(provider string, envs map[string]string) (string, string, string) {
	switch provider {
	case CIProviderCircleCI:
		return envs["CIRCLE_PROJECT_REPONAME"], envs["CIRCLE_WORKFLOW_ID"], envs["CIRCLE_JOB"]
	case CIProviderGitHubActions:
		return envs["GITHUB_REPOSITORY"], envs["GITHUB_RUN_ID"], envs["GITHUB_JOB"]
	case CIProviderGitLabCI:
		return envs["CI_PROJECT_PATH"], envs["CI_PIPELINE_ID"], envs["CI_JOB_NAME"]
	default:
		return "", "", ""
	}
}

// HostMetadata contains metadata about the local environment. Only used for Bazel to
// generate bazelrc. For Gradle, it's done by the plugin dynamically.
type HostMetadata struct {
	OS             string
	CPUCores       int
	MemSize        int64
	Locale         string
	DefaultCharset string
	Hostname       string
	Username       string
}

type GitMetadata struct {
	RepoURL     string
	CommitHash  string
	Branch      string
	CommitEmail string
}

// NewMetadata creates a new CacheConfigMetadata instance based on the environment variables.
func NewMetadata(envs map[string]string, commandFunc CommandFunc, logger log.Logger) CacheConfigMetadata {
	hostMetadata := generateHostMetadata(envs, commandFunc, logger)
	git := generateGitMetadata(logger, commandFunc, envs)

	cliVersion := GetCLIVersion(logger)

	provider := DetectCIProvider(envs)

	redactedEnvs := maps.Clone(envs)
	redactBitriseEnvs(redactedEnvs)

	if provider == CIProviderBitrise {
		return CacheConfigMetadata{
			RedactedEnvs:           redactedEnvs,
			CIProvider:             provider,
			CLIVersion:             cliVersion,
			GitMetadata:            git,
			HostMetadata:           hostMetadata,
			BitriseAppID:           envs["BITRISE_APP_SLUG"],
			BitriseWorkflowName:    envs["BITRISE_TRIGGERED_WORKFLOW_ID"],
			BitriseBuildID:         envs["BITRISE_BUILD_SLUG"],
			BitriseStepExecutionID: envs["BITRISE_STEP_EXECUTION_ID"],
			Datacenter:             envs[datacenterEnvKey],
		}
	}

	externalAppID, externalBuildID, externalWorkflowName := detectExternalIDs(provider, envs)

	return CacheConfigMetadata{
		RedactedEnvs:         redactedEnvs,
		CIProvider:           provider,
		CLIVersion:           cliVersion,
		GitMetadata:          git,
		HostMetadata:         hostMetadata,
		ExternalAppID:        externalAppID,
		ExternalBuildID:      externalBuildID,
		ExternalWorkflowName: externalWorkflowName,
	}
}

func hashKeyValue(hasher hash.Hash, key, value string) []byte {
	hasher.Reset()
	hasher.Write([]byte(RedactorSeed))
	hasher.Write([]byte(key))
	hasher.Write([]byte(value))
	hasher.Write([]byte(RedactorSeed))

	return hasher.Sum(nil)
}

// alwaysRedactKeys are env vars that carry a Bitrise credential regardless of
// whether BITRISE_SECRET_ENV_KEY_LIST names them. Keep in sync with any new
// token-bearing env var the CLI or step layer introduces.
var alwaysRedactKeys = []string{ //nolint:gochecknoglobals
	EnvAuthToken, // BITRISE_BUILD_CACHE_AUTH_TOKEN
	EnvJWT,       // BITRISEIO_BITRISE_SERVICES_ACCESS_TOKEN
	"BITRISE_API_TOKEN",
	"BUILD_TRIGGER_TOKEN",
}

// tokenValuePrefixes are value prefixes that identify a Bitrise credential —
// used as a safety net so a value never surfaces cleartext even if its env-var
// key isn't in the explicit lists above.
var tokenValuePrefixes = []string{"bitpat_", "bitwat_"} //nolint:gochecknoglobals

func redactBitriseEnvs(envs map[string]string) {
	hasher := sha256.New()
	redact := func(key, value string) {
		envs[key] = fmt.Sprintf("<sha256@%x>", hashKeyValue(hasher, key, value)[:4])
	}

	secretKeys := envs["BITRISE_SECRET_ENV_KEY_LIST"]
	for key := range strings.SplitSeq(secretKeys, ",") {
		if key == "" {
			continue
		}
		if envValue, ok := envs[key]; ok {
			redact(key, envValue)
		}
	}

	for _, key := range alwaysRedactKeys {
		if envValue, ok := envs[key]; ok {
			redact(key, envValue)
		}
	}

	for key, value := range envs {
		if !hasTokenPrefix(value) {
			continue
		}
		redact(key, value)
	}
}

func hasTokenPrefix(value string) bool {
	for _, p := range tokenValuePrefixes {
		if strings.HasPrefix(value, p) {
			return true
		}
	}

	return false
}

func generateGitMetadata(logger log.Logger, commandFunc CommandFunc, envs map[string]string) GitMetadata {
	gitMetadata := GitMetadata{}

	// Repo URL
	repoURL, err := commandFunc("git", "config", "--get", "remote.origin.url")
	if err != nil {
		logger.Debugf("Error in get git repo URL: %v", err)
		repoURL = envs["GIT_REPOSITORY_URL"]
	}
	gitMetadata.RepoURL = strings.TrimSpace(repoURL)

	// Commit hash
	commitHash, err := commandFunc("git", "rev-parse", "HEAD")
	if err != nil {
		logger.Debugf("Error in get git commit hash: %v", err)
		commitHash = envs["GIT_CLONE_COMMIT_HASH"]
	}
	gitMetadata.CommitHash = strings.TrimSpace(commitHash)

	// Branch
	branch := envs["BITRISE_GIT_BRANCH"]
	if branch == "" {
		branch, err = commandFunc("git", "branch", "--show-current")
		if err != nil {
			logger.Debugf("Error in get git branch: %v", err)
		}
	}
	if branch == "" {
		logger.Debugf("HEAD is probably detached, finding matching branches...")
		branch, err = commandFunc("sh", "-c", "git branch --contains HEAD --format='%(refname:short)' | grep -v HEAD")
		if err != nil {
			logger.Debugf("Error in get git branch (2nd attempt): %v", err)
		} else {
			matchingBranches := strings.Split(branch, "\n")
			if len(matchingBranches) == 1 {
				// Only if there's a single matching branch, otherwise leave empty to avoid confusion
				branch = matchingBranches[0]
			}
		}
	}
	branch = strings.TrimSpace(branch)
	if branch != "" {
		logger.Debugf("Detected git branch: %s", branch)
	}
	gitMetadata.Branch = branch

	// Commit email
	commitEmail, err := commandFunc("git", "show", "-s", "--format=%ae", gitMetadata.CommitHash)
	if err != nil {
		logger.Debugf("Error in get git commit email: %v", err)
		commitEmail = envs["GIT_CLONE_COMMIT_AUTHOR_EMAIL"]
	}
	gitMetadata.CommitEmail = strings.TrimSpace(commitEmail)

	return gitMetadata
}

// nolint: funlen, nestif
func generateHostMetadata(envs map[string]string, commandFunc CommandFunc, logger log.Logger) HostMetadata {
	metadata := HostMetadata{}

	// OS
	detectedOS, err := commandFunc("uname", "-a")
	if err != nil {
		logger.Errorf("Error in get OS: %v", err)
	}
	logger.Debugf("OS: %s", detectedOS)
	metadata.OS = strings.TrimSpace(detectedOS)

	// CPU cores
	var ncpu int
	if runtime.GOOS == "darwin" {
		output, err := commandFunc("sysctl", "-n", "hw.ncpu")
		if err != nil {
			logger.Errorf("Error in get cpu cores: %v", err)
		} else if len(output) > 0 {
			ncpu, err = strconv.Atoi(strings.TrimSpace(output))
			if err != nil {
				logger.Errorf("Error in parse cpu cores: %v", err)
			}
		}
	} else {
		output, err := commandFunc("nproc")
		if err != nil {
			logger.Errorf("Error in get cpu cores: %v", err)
		} else if len(output) > 0 {
			ncpu, err = strconv.Atoi(strings.TrimSpace(output))
			if err != nil {
				logger.Errorf("Error in parse cpu cores: %v", err)
			}
		}
	}
	logger.Debugf("CPU cores: %d", ncpu)
	metadata.CPUCores = ncpu

	// RAM
	var memSize int64
	if runtime.GOOS == "darwin" {
		output, err := commandFunc("sysctl", "-n", "hw.memsize")
		if err != nil {
			logger.Errorf("Error in get mem size: %v", err)
		} else if len(output) > 0 {
			memSize, err = strconv.ParseInt(strings.TrimSpace(output), 10, 64)
			if err != nil {
				logger.Errorf("Error in parse mem size: %v", err)
			}
		}
	} else {
		output, err := commandFunc("sh", "-c",
			"grep MemTotal /proc/meminfo | tr -s ' ' | cut -d ' ' -f 2")
		if err != nil {
			logger.Errorf("Error in get mem size: %v", err)
		} else if len(output) > 0 {
			memSizeStr := strings.TrimSpace(output)
			memSize, err = strconv.ParseInt(memSizeStr, 10, 64)
			if err != nil {
				logger.Errorf("Error in parse mem size: %v", err)
			} else {
				memSize *= 1024 // Convert from KB to Bytes
			}
		}
	}
	logger.Debugf("Memory size: %d", memSize)
	metadata.MemSize = memSize

	// Locale
	localeRaw := getLocale(envs)
	localeComps := strings.Split(localeRaw, ".")
	if len(localeComps) == 2 {
		metadata.Locale = localeComps[0]
		metadata.DefaultCharset = localeComps[1]
	}
	logger.Debugf("Locale: %s", metadata.Locale)
	logger.Debugf("Default charset: %s", metadata.DefaultCharset)

	// Hostname
	hostname, err := os.Hostname()
	if err != nil {
		logger.Errorf("Error in get hostname: %v", err)
	}
	metadata.Hostname = strings.TrimSpace(hostname)

	resolved, src := ResolveUsername(envs)
	if src == UsernameSourceNone {
		logger.Debugf("Could not resolve local invocation username from any source (env, keychain, os/user); leaving empty.")
	}
	metadata.Username = strings.TrimSpace(resolved)

	return metadata
}

func getLocale(envs map[string]string) string {
	// Check various environment variables for locale information
	lang := envs["LANG"]
	lcAll := envs["LC_ALL"]
	if lcAll != "" {
		return lcAll
	}
	if lang != "" {
		return lang
	}

	return ""
}
