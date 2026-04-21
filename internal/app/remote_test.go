package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func TestFilterEntityCommandsSkipsAltPHPFeatureResume(t *testing.T) {
	t.Parallel()

	commands := []string{
		featureUpdateCommand(),
		"/usr/local/mgr5/sbin/mgrctl -m ispmgr feature.resume sok=ok 'elid=altphp72 altphp83' 'elname=PHP 7.2 Apache module, PHP 7.2 PHP-FPM, PHP 7.2 common'",
		"/usr/local/mgr5/sbin/mgrctl -m ispmgr user.edit name=alice sok=ok",
	}

	filtered := filterEntityCommands(commands)
	if len(filtered) != 1 {
		t.Fatalf("expected only one entity command after filtering, got %#v", filtered)
	}
	if filtered[0] != "/usr/local/mgr5/sbin/mgrctl -m ispmgr user.edit name=alice sok=ok" {
		t.Fatalf("unexpected filtered commands: %#v", filtered)
	}
}

func TestWarnOverwriteBlockAddsBlankLinesAtInfo(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	runner := &remoteRunner{
		cfg: Config{LogLevel: "info"},
		ui:  &UI{out: &out, err: &out},
	}

	runner.warnOverwriteBlock("user www-root already exists on remote side, skipped. Run again with --overwrite to modify it.")

	got := stripANSI(out.String())
	want := "\nuser www-root already exists on remote side, skipped. Run again with --overwrite to modify it.\n\n"
	if got != want {
		t.Fatalf("warnOverwriteBlock() info output = %q, want %q", got, want)
	}
}

func TestWarnOverwriteBlockDoesNotAddBlankLinesAtWarn(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	runner := &remoteRunner{
		cfg: Config{LogLevel: "warn"},
		ui:  &UI{out: &out, err: &out},
	}

	runner.warnOverwriteBlock("user www-root already exists on remote side, skipped. Run again with --overwrite to modify it.")

	got := stripANSI(out.String())
	if strings.HasPrefix(got, "\n") || strings.HasSuffix(strings.TrimSuffix(got, "\n"), "\n") {
		t.Fatalf("warnOverwriteBlock() warn output should not have extra blank lines, got %q", got)
	}
	want := "user www-root already exists on remote side, skipped. Run again with --overwrite to modify it.\n"
	if got != want {
		t.Fatalf("warnOverwriteBlock() warn output = %q, want %q", got, want)
	}
}

func TestWarnPackageWarningAddsTrailingBlankLineAtInfoForDockerSkip(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	runner := &remoteRunner{
		cfg: Config{LogLevel: "info"},
		ui:  &UI{out: &out, err: &out},
	}

	runner.warnPackageWarning(`destination panel edition "ispmanager Lite" does not support Docker, package_docker command was skipped.`)

	got := stripANSI(out.String())
	want := "destination panel edition \"ispmanager Lite\" does not support Docker, package_docker command was skipped.\n\n"
	if got != want {
		t.Fatalf("warnPackageWarning() info output = %q, want %q", got, want)
	}
}

func TestRemoteCommandTimeoutAppliesOnlyToMgrctl(t *testing.T) {
	if remoteCommandTimeout("/usr/local/mgr5/sbin/mgrctl -m ispmgr feature") != 5*time.Minute {
		t.Fatalf("expected mgrctl timeout to be 5 minutes")
	}
	if remoteCommandTimeout("bash -lc 'apt-get update && apt-get install -y wget'") != 0 {
		t.Fatalf("expected non-mgrctl command to have no forced timeout")
	}
}

func TestNormalizeRemoteCommandErrorDetectsTimeoutAndDisconnect(t *testing.T) {
	timeoutErr := normalizeRemoteCommandError("/usr/local/mgr5/sbin/mgrctl -m ispmgr feature", context.DeadlineExceeded)
	if !strings.Contains(timeoutErr.Error(), "timed out or stalled under load") {
		t.Fatalf("expected timeout normalization, got %v", timeoutErr)
	}

	disconnectErr := normalizeRemoteCommandError("cmd", errors.New("read tcp 1.2.3.4: broken pipe"))
	if !strings.Contains(disconnectErr.Error(), "server may have rebooted") {
		t.Fatalf("expected disconnect normalization, got %v", disconnectErr)
	}
}

func TestSanitizeRemoteConsoleOutputSuppressesBenignProcSysGrepWarnings(t *testing.T) {
	input := strings.Join([]string{
		"grep: /proc/sys/net/ipv4/route/flush: Permission denied",
		"grep: /proc/sys/net/ipv6/conf/all/stable_secret: Input/output error",
		"normal line",
	}, "\n")

	cleaned, suppressed := sanitizeRemoteConsoleOutput(input)
	if suppressed != 2 {
		t.Fatalf("expected 2 suppressed lines, got %d", suppressed)
	}
	if strings.Contains(cleaned, "/proc/sys/") {
		t.Fatalf("expected /proc/sys grep warnings to be removed, got %q", cleaned)
	}
	if !strings.Contains(cleaned, "normal line") {
		t.Fatalf("expected regular output to remain, got %q", cleaned)
	}
}

func TestConnectRemoteRunnerUsesHooksAndAnnouncesConnection(t *testing.T) {
	originalConnectSSHHook := connectSSHHook
	originalSFTPClientHook := newSFTPClientHook
	t.Cleanup(func() {
		connectSSHHook = originalConnectSSHHook
		newSFTPClientHook = originalSFTPClientHook
	})

	connectSSHHook = func(cfg Config) (*ssh.Client, error) {
		return nil, nil
	}
	newSFTPClientHook = func(client *ssh.Client, opts ...sftp.ClientOption) (*sftp.Client, error) {
		return nil, nil
	}

	var out bytes.Buffer
	runner, err := connectRemoteRunner(&UI{out: &out, err: &out}, slog.Default(), Config{
		DestHost: "192.0.2.10",
		LogLevel: "info",
	}, true)
	if err != nil {
		t.Fatalf("connectRemoteRunner() returned error: %v", err)
	}
	if runner == nil {
		t.Fatalf("connectRemoteRunner() returned nil runner")
	}

	got := stripANSI(out.String())
	if !strings.Contains(got, "connecting: 192.0.2.10\n") {
		t.Fatalf("expected connecting line, got %q", got)
	}
	if !strings.Contains(got, "connecting: OK\n") {
		t.Fatalf("expected success line, got %q", got)
	}
}

func TestPrintMonitoringCommandAddsReadableInfoOutput(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	runner := &remoteRunner{
		cfg: Config{LogLevel: "info"},
		ui:  &UI{out: &out, err: &out},
	}

	runner.printMonitoringCommand("/usr/local/mgr5/sbin/mgrctl -m ispmgr feature")

	got := stripANSI(out.String())
	want := "monitoring command: /usr/local/mgr5/sbin/mgrctl -m ispmgr feature\n\n"
	if got != want {
		t.Fatalf("printMonitoringCommand() info output = %q, want %q", got, want)
	}
}

func TestPrintMonitoringCommandShowsFilesOnDebug(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	runner := &remoteRunner{
		cfg: Config{LogLevel: "debug"},
		ui:  &UI{out: &out, err: &out},
	}

	runner.printMonitoringCommand("/usr/local/mgr5/sbin/mgrctl -m ispmgr feature")

	got := stripANSI(out.String())
	if !strings.Contains(got, "monitoring command: /usr/local/mgr5/sbin/mgrctl -m ispmgr feature\n") {
		t.Fatalf("expected monitoring command, got %q", got)
	}
	if !strings.Contains(got, "monitoring files: /usr/local/mgr5/var/ispmgr.log, /usr/local/mgr5/var/pkg.log\n") {
		t.Fatalf("expected monitoring files, got %q", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Fatalf("expected trailing blank line, got %q", got)
	}
}

func TestParseRemoteSwapStateIgnoresZRAMAndDetectsSwapfile(t *testing.T) {
	t.Parallel()

	state := parseRemoteSwapState(strings.Join([]string{
		"Filename\t\t\tType\t\tSize\t\tUsed\t\tPriority",
		"/dev/zram0                               partition\t524284\t0\t100",
		"/swapfile                                file\t2097148\t0\t10",
	}, "\n"), "/swapfile                                 none                    swap    sw,pri=10              0 0\n", true)

	if state.HasOtherNonZRAMSwap {
		t.Fatalf("expected zram to be ignored, got %+v", state)
	}
	if !state.SwapfileActive {
		t.Fatalf("expected /swapfile to be active, got %+v", state)
	}
	if !state.FstabHasSwapfile {
		t.Fatalf("expected /swapfile fstab entry, got %+v", state)
	}
	if !state.SwapfileExists {
		t.Fatalf("expected /swapfile existence flag, got %+v", state)
	}
}

func TestHasSwapfileFstabEntryAcceptsExistingSwapfileLineWithDifferentOptions(t *testing.T) {
	t.Parallel()

	line := "/swapfile none swap defaults,pri=5 0 0"
	if !hasSwapfileFstabEntry(line) {
		t.Fatalf("expected swapfile fstab line to be detected: %q", line)
	}
}

func TestAppendSwapfileFstabLineDoesNotDuplicateExistingEntry(t *testing.T) {
	t.Parallel()

	original := "/swapfile none swap defaults,pri=5 0 0\n"
	updated, changed := appendSwapfileFstabLine(original)
	if changed {
		t.Fatalf("expected existing swapfile line not to be duplicated")
	}
	if updated != original {
		t.Fatalf("expected unchanged fstab content, got %q", updated)
	}
}

func TestAppendSwapfileFstabLineAppendsManagedLineOnce(t *testing.T) {
	t.Parallel()

	updated, changed := appendSwapfileFstabLine("UUID=abc / ext4 defaults 0 1\n")
	if !changed {
		t.Fatalf("expected swapfile line to be appended")
	}
	if !strings.Contains(updated, swapfileFstabLine+"\n") {
		t.Fatalf("expected managed swapfile line in output, got %q", updated)
	}
}

func TestWarnOnMemoryMismatchCreatesSwapfileWhenMemoryIsLowAndNoSwapExists(t *testing.T) {
	originalLocalMemoryTotalBytesHook := localMemoryTotalBytesHook
	originalRemoteMemoryTotalBytesHook := remoteMemoryTotalBytesHook
	originalRemoteSwapStateHook := remoteSwapStateHook
	originalEnsureSwapfileHook := ensureSwapfileHook
	t.Cleanup(func() {
		localMemoryTotalBytesHook = originalLocalMemoryTotalBytesHook
		remoteMemoryTotalBytesHook = originalRemoteMemoryTotalBytesHook
		remoteSwapStateHook = originalRemoteSwapStateHook
		ensureSwapfileHook = originalEnsureSwapfileHook
	})

	const gib = int64(1024 * 1024 * 1024)
	localMemoryTotalBytesHook = func() (int64, error) { return 4 * gib, nil }
	remoteMemoryTotalBytesHook = func(r *remoteRunner, ctx context.Context) (int64, error) { return int64(1900 * 1024 * 1024), nil }
	remoteSwapStateHook = func(r *remoteRunner, ctx context.Context) (remoteSwapState, error) {
		return remoteSwapState{}, nil
	}

	called := false
	ensureSwapfileHook = func(r *remoteRunner, ctx context.Context, state remoteSwapState) error {
		called = true
		return nil
	}

	var out bytes.Buffer
	runner := &remoteRunner{
		cfg: Config{LogLevel: "info"},
		ui:  &UI{out: &out, err: &out},
	}

	if err := runner.warnOnMemoryMismatch(context.Background()); err != nil {
		t.Fatalf("warnOnMemoryMismatch() returned error: %v", err)
	}
	if !called {
		t.Fatalf("expected ensureSwapfileHook to be called")
	}
	got := stripANSI(out.String())
	if !strings.Contains(got, "destination memory is low:") {
		t.Fatalf("expected low-memory warning, got %q", got)
	}
	if !strings.Contains(got, "creating swapfile and enabling it: OK") {
		t.Fatalf("expected successful swapfile action, got %q", got)
	}
}

func TestWarnOnMemoryMismatchSkipsSwapCreationWhenOtherSwapExists(t *testing.T) {
	originalLocalMemoryTotalBytesHook := localMemoryTotalBytesHook
	originalRemoteMemoryTotalBytesHook := remoteMemoryTotalBytesHook
	originalRemoteSwapStateHook := remoteSwapStateHook
	originalEnsureSwapfileHook := ensureSwapfileHook
	t.Cleanup(func() {
		localMemoryTotalBytesHook = originalLocalMemoryTotalBytesHook
		remoteMemoryTotalBytesHook = originalRemoteMemoryTotalBytesHook
		remoteSwapStateHook = originalRemoteSwapStateHook
		ensureSwapfileHook = originalEnsureSwapfileHook
	})

	const gib = int64(1024 * 1024 * 1024)
	localMemoryTotalBytesHook = func() (int64, error) { return 4 * gib, nil }
	remoteMemoryTotalBytesHook = func(r *remoteRunner, ctx context.Context) (int64, error) { return int64(1900 * 1024 * 1024), nil }
	remoteSwapStateHook = func(r *remoteRunner, ctx context.Context) (remoteSwapState, error) {
		return remoteSwapState{HasOtherNonZRAMSwap: true}, nil
	}

	called := false
	ensureSwapfileHook = func(r *remoteRunner, ctx context.Context, state remoteSwapState) error {
		called = true
		return nil
	}

	runner := &remoteRunner{
		cfg: Config{LogLevel: "info"},
		ui:  &UI{out: &bytes.Buffer{}, err: &bytes.Buffer{}},
	}

	if err := runner.warnOnMemoryMismatch(context.Background()); err != nil {
		t.Fatalf("warnOnMemoryMismatch() returned error: %v", err)
	}
	if called {
		t.Fatalf("did not expect ensureSwapfileHook to be called when other swap exists")
	}
}

func TestWarnOnMemoryMismatchMentionsEnabledSwapWhenAlreadyPresent(t *testing.T) {
	originalLocalMemoryTotalBytesHook := localMemoryTotalBytesHook
	originalRemoteMemoryTotalBytesHook := remoteMemoryTotalBytesHook
	originalRemoteSwapStateHook := remoteSwapStateHook
	t.Cleanup(func() {
		localMemoryTotalBytesHook = originalLocalMemoryTotalBytesHook
		remoteMemoryTotalBytesHook = originalRemoteMemoryTotalBytesHook
		remoteSwapStateHook = originalRemoteSwapStateHook
	})

	const gib = int64(1024 * 1024 * 1024)
	localMemoryTotalBytesHook = func() (int64, error) { return 4 * gib, nil }
	remoteMemoryTotalBytesHook = func(r *remoteRunner, ctx context.Context) (int64, error) { return int64(1900 * 1024 * 1024), nil }
	remoteSwapStateHook = func(r *remoteRunner, ctx context.Context) (remoteSwapState, error) {
		return remoteSwapState{SwapfileActive: true, FstabHasSwapfile: true}, nil
	}

	var out bytes.Buffer
	runner := &remoteRunner{
		cfg: Config{LogLevel: "info"},
		ui:  &UI{out: &out, err: &out},
	}

	if err := runner.warnOnMemoryMismatch(context.Background()); err != nil {
		t.Fatalf("warnOnMemoryMismatch() returned error: %v", err)
	}
	got := stripANSI(out.String())
	if !strings.Contains(got, "destination memory is low:") || !strings.Contains(got, "(swap already enabled)") {
		t.Fatalf("expected low-memory message to mention enabled swap, got %q", got)
	}
}

func TestAltPHPFeatureIDsFromStepCommandParsesResumeElids(t *testing.T) {
	t.Parallel()

	got := altPHPFeatureIDsFromStepCommand("/usr/local/mgr5/sbin/mgrctl -m ispmgr feature.resume sok=ok 'elid=altphp72, altphp83, altphp85' 'elname=PHP 7.2 Apache module, PHP 7.2 PHP-FPM, PHP 7.2 common'")
	for _, want := range []string{"altphp72", "altphp83", "altphp85"} {
		if _, ok := got[want]; !ok {
			t.Fatalf("expected %q in parsed altphp ids, got %#v", want, got)
		}
	}
}

func TestAltPHPFeaturesBusyDetectsInstallStatusForRequestedVersions(t *testing.T) {
	t.Parallel()

	records := []featureRecord{
		{Name: "altphp72", Status: ""},
		{Name: "altphp83", Status: "install"},
	}
	if !altPHPFeaturesBusy(records, map[string]struct{}{"altphp83": {}}) {
		t.Fatalf("expected requested altphp feature in install state to be detected as busy")
	}
	if altPHPFeaturesBusy(records, map[string]struct{}{"altphp72": {}}) {
		t.Fatalf("did not expect completed altphp feature to be detected as busy")
	}
}
