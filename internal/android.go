package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/browser"
)

func startDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
	return nil
}

type AndroidPlayerProbeResult struct {
	PackageFound      bool
	ActivityValidated bool
	Capability        string
	SocketPath        string
	Warnings          []string
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func termuxToolAvailable(name string) bool {
	if !hasCommand(name) {
		return false
	}

	cmd := exec.Command(name, "--help")
	return cmd.Run() == nil
}

func OpenURL(target string, config *CurdConfig) error {
	if config != nil && config.IsAndroid() {
		if config.AndroidUseTermuxAPI && hasCommand("termux-open-url") {
			return startDetached("termux-open-url", target)
		}

		if hasCommand("am") {
			action := "android.intent.action.VIEW"
			if strings.TrimSpace(config.AndroidOpenLinksWith) != "" && config.AndroidOpenLinksWith != "termux-open-url" {
				action = config.AndroidOpenLinksWith
			}
			return startDetached("am", "start", "-a", action, "-d", target)
		}
	}

	return browser.OpenURL(target)
}

func Notify(message string) {
	config := GetGlobalConfig()
	if config == nil || !config.IsAndroid() || !config.AndroidNotifications {
		return
	}

	if config.AndroidUseTermuxAPI && hasCommand("termux-toast") {
		if err := exec.Command("termux-toast", message).Run(); err == nil {
			return
		}
	}

	if config.AndroidUseTermuxAPI && hasCommand("termux-notification") {
		_ = exec.Command("termux-notification", "--title", "Curd", "--content", message).Run()
	}
}

func AcquireWakeLock(config *CurdConfig) {
	if config == nil || !config.IsAndroid() || !config.AndroidWakeLock || !config.AndroidUseTermuxAPI {
		return
	}
	if hasCommand("termux-wake-lock") {
		_ = exec.Command("termux-wake-lock").Run()
	}
}

func ReleaseWakeLock(config *CurdConfig) {
	if config == nil || !config.IsAndroid() || !config.AndroidWakeLock || !config.AndroidUseTermuxAPI {
		return
	}
	if hasCommand("termux-wake-unlock") {
		_ = exec.Command("termux-wake-unlock").Run()
	}
}

func buildAndroidSocketPath(config *CurdConfig) string {
	if config != nil && strings.TrimSpace(config.AndroidPlayerSocketPath) != "" {
		return config.AndroidPlayerSocketPath
	}

	return filepath.Join(os.TempDir(), fmt.Sprintf("curd_mpvsocket_%d", time.Now().UnixNano()))
}

func hasCleanPathPrefix(path string, prefix string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	prefix = filepath.Clean(strings.TrimSpace(prefix))
	if path == "" || prefix == "" {
		return false
	}
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, prefix+string(os.PathSeparator))
}

type androidSocketPathAssessment struct {
	Supported bool
	Warnings  []string
}

func assessAndroidSocketPath(socketPath string) androidSocketPathAssessment {
	socketPath = strings.TrimSpace(socketPath)
	result := androidSocketPathAssessment{Supported: true}
	if socketPath == "" {
		return result
	}

	cleanPath := filepath.Clean(socketPath)
	lowerPath := strings.ToLower(cleanPath)

	termuxRoots := []string{
		"/data/data/com.termux",
		"/data/user/0/com.termux",
	}
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		termuxRoots = append(termuxRoots, filepath.Clean(home))
	}
	if prefix := strings.TrimSpace(os.Getenv("PREFIX")); prefix != "" {
		termuxRoots = append(termuxRoots, filepath.Clean(prefix))
	}

	for _, root := range termuxRoots {
		if hasCleanPathPrefix(cleanPath, root) {
			result.Supported = false
			result.Warnings = append(result.Warnings, fmt.Sprintf("Android player apps cannot create Unix sockets inside Termux private storage: %s", cleanPath))
			return result
		}
	}

	for _, root := range []string{"/sdcard", "/storage", "/mnt/media_rw"} {
		if hasCleanPathPrefix(cleanPath, root) {
			result.Supported = false
			result.Warnings = append(result.Warnings, fmt.Sprintf("shared storage is not a reliable location for Unix socket files on Android: %s", cleanPath))
			return result
		}
	}

	if (strings.HasPrefix(lowerPath, "/data/data/") || strings.HasPrefix(lowerPath, "/data/user/0/")) &&
		!hasCleanPathPrefix(cleanPath, "/data/data/com.termux") &&
		!hasCleanPathPrefix(cleanPath, "/data/user/0/com.termux") {
		result.Supported = false
		result.Warnings = append(result.Warnings, fmt.Sprintf("socket path points to another app's private storage, which curd in Termux cannot access: %s", cleanPath))
	}

	return result
}

func androidIntentUsesSocketPlaceholder(config *CurdConfig) bool {
	if config == nil {
		return false
	}

	return strings.Contains(strings.Join(config.AndroidExtraIntentArgs, " "), "{socket}")
}

func androidConfiguredSocketPath(config *CurdConfig) string {
	if config == nil {
		return ""
	}

	socketPath := strings.TrimSpace(config.AndroidPlayerSocketPath)
	if socketPath != "" {
		return socketPath
	}

	if androidIntentUsesSocketPlaceholder(config) {
		return buildAndroidSocketPath(config)
	}

	return ""
}

func BuildAndroidIntentCommand(config *CurdConfig, link string, title string, socketPath string) []string {
	action := "android.intent.action.VIEW"
	if config != nil && strings.TrimSpace(config.AndroidPlayerIntentAction) != "" {
		action = strings.TrimSpace(config.AndroidPlayerIntentAction)
	}

	args := []string{"start", "--user", "0", "-a", action}
	if strings.TrimSpace(link) != "" {
		args = append(args, "-d", link)
	}

	component := strings.TrimSpace(strings.TrimSpace(config.AndroidPlayerPackage) + "/" + strings.TrimSpace(config.AndroidPlayerActivity))
	if config != nil && strings.TrimSpace(component) != "/" {
		args = append(args, "-n", component)
	}

	placeholders := map[string]string{
		"{url}":    link,
		"{title}":  title,
		"{socket}": socketPath,
	}

	if config != nil {
		for _, extra := range config.AndroidExtraIntentArgs {
			value := extra
			for placeholder, replacement := range placeholders {
				value = strings.ReplaceAll(value, placeholder, replacement)
			}
			args = append(args, value)
		}
	}

	return args
}

func packageExists(packageName string) bool {
	packageName = strings.TrimSpace(packageName)
	if packageName == "" || !hasCommand("pm") {
		return false
	}

	return exec.Command("pm", "path", packageName).Run() == nil
}

func validateAndroidActivity(config *CurdConfig) bool {
	component := strings.TrimSpace(config.AndroidPlayerPackage) + "/" + strings.TrimSpace(config.AndroidPlayerActivity)
	if component == "/" {
		return false
	}

	if hasCommand("am") {
		output, err := runAndroidIntentProbe(config, "https://example.com", "")
		if err == nil && !strings.Contains(string(output), "Error:") {
			return true
		}
	}

	return packageExists(config.AndroidPlayerPackage)
}

func runAndroidIntentProbe(config *CurdConfig, link string, socketPath string) ([]byte, error) {
	probeArgs := BuildAndroidIntentCommand(config, link, "Curd Android Setup", socketPath)
	return exec.Command("am", append([]string{"start", "-W"}, probeArgs[1:]...)...).CombinedOutput()
}

func waitForAndroidSocket(socketPath string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := connectToPipe(socketPath)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}

	return false
}

func ProbeAndroidPlayer(config *CurdConfig) AndroidPlayerProbeResult {
	result := AndroidPlayerProbeResult{
		Capability: "unsupported",
	}

	if config == nil {
		result.Warnings = append(result.Warnings, "missing Android config")
		return result
	}

	result.PackageFound = packageExists(config.AndroidPlayerPackage)
	if !result.PackageFound {
		result.Warnings = append(result.Warnings, "configured Android player package was not found")
		return result
	}

	result.ActivityValidated = validateAndroidActivity(config)
	if !result.ActivityValidated {
		result.Warnings = append(result.Warnings, "configured Android player activity could not be resolved")
		return result
	}

	result.Capability = "intent"
	if strings.EqualFold(config.AndroidPlayerMode, "intent") {
		return result
	}

	socketPath := androidConfiguredSocketPath(config)
	result.SocketPath = socketPath
	if socketPath == "" {
		result.Warnings = append(result.Warnings, "Android IPC mode needs either AndroidPlayerSocketPath or AndroidExtraIntentArgs with {socket}")
		return result
	}

	socketAssessment := assessAndroidSocketPath(socketPath)
	result.Warnings = append(result.Warnings, socketAssessment.Warnings...)
	if !socketAssessment.Supported {
		if strings.Contains(strings.ToLower(config.AndroidPlayerPackage), "mpvex") {
			result.Warnings = append(result.Warnings, "mpvEx loads mpv.conf into its own app sandbox, but curd in Termux cannot attach to a Unix socket inside another Android app's private storage")
			result.Warnings = append(result.Warnings, "use mpvEx in intent mode unless it adds an exported control bridge that curd can reach from Termux")
		}
		return result
	}

	cmdArgs := BuildAndroidIntentCommand(config, "https://example.com", "Curd Android Setup", socketPath)
	cmd := exec.Command("am", cmdArgs...)
	if err := cmd.Start(); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("failed to launch Android player probe: %v", err))
		return result
	}

	if waitForAndroidSocket(socketPath, 5*time.Second) {
		result.Capability = "ipc"
		_ = ExitMPV(socketPath)
		return result
	}

	result.Warnings = append(result.Warnings, fmt.Sprintf("player launched but no IPC socket appeared at %s; falling back to intent mode", socketPath))
	if strings.Contains(strings.ToLower(config.AndroidPlayerPackage), "mpvex") {
		result.Warnings = append(result.Warnings, "mpvEx does not appear to accept arbitrary MPV options through VIEW intent extras; configure input-ipc-server in mpvEx's own MPV config and set the same AndroidPlayerSocketPath in curd")
	}
	return result
}

func RunAndroidSetup(configPath string, config *CurdConfig) error {
	if DetectPlatform() != PlatformAndroid {
		return fmt.Errorf("--android-setup must be run from Termux/Android")
	}

	if config == nil {
		return fmt.Errorf("missing config")
	}

	config.Platform = string(PlatformAndroid)

	CurdOut("Running Android setup...")
	if config.AndroidUseTermuxAPI {
		missing := make([]string, 0)
		for _, tool := range []string{"termux-open-url", "termux-toast", "termux-wake-lock", "termux-wake-unlock"} {
			if !hasCommand(tool) {
				missing = append(missing, tool)
			}
		}
		if len(missing) > 0 {
			CurdOut(fmt.Sprintf("Termux API tools not found: %s", strings.Join(missing, ", ")))
		}
	}

	probe := ProbeAndroidPlayer(config)
	for _, warning := range probe.Warnings {
		CurdOut("Android setup: " + warning)
	}

	if !probe.PackageFound {
		return fmt.Errorf("Android player package %q was not found", config.AndroidPlayerPackage)
	}
	if !probe.ActivityValidated {
		return fmt.Errorf("Android player activity %q/%q could not be resolved", config.AndroidPlayerPackage, config.AndroidPlayerActivity)
	}

	config.AndroidDetectedCapability = probe.Capability
	if probe.Capability != "ipc" {
		config.AndroidPlayerMode = "intent"
	}

	configMap := map[string]string{
		"StoragePath":               config.StoragePath,
		"AnimeNameLanguage":         config.AnimeNameLanguage,
		"SubsLanguage":              config.SubsLanguage,
		"MenuOrder":                 config.MenuOrder,
		"SubOrDub":                  config.SubOrDub,
		"PercentageToMarkComplete":  fmt.Sprintf("%d", config.PercentageToMarkComplete),
		"NextEpisodePrompt":         fmt.Sprintf("%t", config.NextEpisodePrompt),
		"SkipOp":                    fmt.Sprintf("%t", config.SkipOp),
		"SkipEd":                    fmt.Sprintf("%t", config.SkipEd),
		"SkipFiller":                fmt.Sprintf("%t", config.SkipFiller),
		"SkipRecap":                 fmt.Sprintf("%t", config.SkipRecap),
		"RofiSelection":             fmt.Sprintf("%t", config.RofiSelection),
		"ImagePreview":              fmt.Sprintf("%t", config.ImagePreview),
		"CurrentCategory":           fmt.Sprintf("%t", config.CurrentCategory),
		"ScoreOnCompletion":         fmt.Sprintf("%t", config.ScoreOnCompletion),
		"SaveMpvSpeed":              fmt.Sprintf("%t", config.SaveMpvSpeed),
		"AddMissingOptions":         fmt.Sprintf("%t", config.AddMissingOptions),
		"AlternateScreen":           fmt.Sprintf("%t", config.AlternateScreen),
		"DiscordPresence":           fmt.Sprintf("%t", config.DiscordPresence),
		"AndroidPlayerPackage":      config.AndroidPlayerPackage,
		"AndroidPlayerActivity":     config.AndroidPlayerActivity,
		"AndroidPlayerMode":         config.AndroidPlayerMode,
		"AndroidPlayerIntentAction": config.AndroidPlayerIntentAction,
		"AndroidUseTermuxAPI":       fmt.Sprintf("%t", config.AndroidUseTermuxAPI),
		"AndroidOpenLinksWith":      config.AndroidOpenLinksWith,
		"AndroidNotifications":      fmt.Sprintf("%t", config.AndroidNotifications),
		"AndroidWakeLock":           fmt.Sprintf("%t", config.AndroidWakeLock),
		"AndroidExtraIntentArgs":    fmt.Sprintf("[%s]", strings.Join(config.AndroidExtraIntentArgs, ",")),
		"AndroidPlayerSocketPath":   config.AndroidPlayerSocketPath,
		"AndroidDetectedCapability": config.AndroidDetectedCapability,
	}

	if err := SaveConfigToFileWithSchema(configPath, configMap, configSchemaForPlatform(PlatformAndroid)); err != nil {
		return err
	}

	CurdOut(fmt.Sprintf("Android setup complete. Player capability: %s", probe.Capability))
	return nil
}
