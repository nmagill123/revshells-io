package agent

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/noahmagill/webhook-rev-shell/internal/protocol"
)

func CollectSystemInfo() protocol.SystemInfo {
	si := protocol.SystemInfo{
		Arch: runtime.GOARCH,
	}
	if h, err := os.Hostname(); err == nil {
		si.Hostname = h
	}
	if runtime.GOOS == "linux" {
		collectLinuxProc(&si)
	} else if runtime.GOOS == "darwin" {
		collectDarwin(&si)
	} else {
		si.OSName = runtime.GOOS
	}
	if si.OSName == "" {
		si.OSName = runtime.GOOS
	}
	if out, err := exec.Command("uname", "-r").Output(); err == nil {
		if si.Kernel == "" {
			si.Kernel = strings.TrimSpace(string(out))
		}
	}
	if out, err := exec.Command("uname", "-m").Output(); err == nil {
		si.Arch = strings.TrimSpace(string(out))
	}
	return si
}

func collectLinuxProc(si *protocol.SystemInfo) {
	if data, err := os.ReadFile("/proc/version"); err == nil {
		si.Kernel = parseProcVersion(string(data))
	}
	readOSRelease(si)
	if si.OSName == "" {
		if data, err := os.ReadFile("/proc/sys/kernel/ostype"); err == nil {
			si.OSName = strings.TrimSpace(string(data))
		}
	}
}

func parseProcVersion(s string) string {
	// Linux version 6.1.0-xxx (user@host) (gcc ...) #1 SMP ...
	s = strings.TrimPrefix(s, "Linux version ")
	if idx := strings.Index(s, " ("); idx > 0 {
		return strings.TrimSpace(s[:idx])
	}
	if idx := strings.IndexByte(s, ' '); idx > 0 {
		return strings.TrimSpace(s[:idx])
	}
	return strings.TrimSpace(s)
}

func readOSRelease(si *protocol.SystemInfo) {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return
	}
	defer f.Close()
	vals := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, "\"")
		vals[k] = v
	}
	if n := vals["PRETTY_NAME"]; n != "" {
		si.OSName = n
	} else if n := vals["NAME"]; n != "" {
		si.OSName = n
		if vals["VERSION_ID"] != "" {
			si.OSVersion = vals["VERSION_ID"]
		}
	}
	if si.OSVersion == "" {
		si.OSVersion = vals["VERSION_ID"]
	}
	si.IDLike = vals["ID_LIKE"]
	if si.IDLike == "" {
		si.IDLike = vals["ID"]
	}
}

func collectDarwin(si *protocol.SystemInfo) {
	si.OSName = "macOS"
	if out, err := exec.Command("sw_vers", "-productVersion").Output(); err == nil {
		si.OSVersion = strings.TrimSpace(string(out))
	}
	if data, err := exec.Command("uname", "-v").Output(); err == nil {
		si.Kernel = strings.TrimSpace(string(data))
	}
}

