package xray

import (
	"os"
	"runtime/debug"
	"strconv"

	"github.com/xtls/libxray/memory"
	"github.com/xtls/xray-core/common/cmdarg"
	"github.com/xtls/xray-core/common/platform"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
	_ "github.com/xtls/xray-core/main/distro/all"
)

var (
	coreServer *core.Instance
)

func StartXray(configPath string) (*core.Instance, error) {
	file := cmdarg.Arg{configPath}
	config, err := core.LoadConfig("json", file)
	if err != nil {
		return nil, err
	}

	server, err := core.New(config)
	if err != nil {
		return nil, err
	}

	return server, nil
}

func StartXrayFromJSON(configJSON string) (*core.Instance, error) {
	// Convert JSON string to bytes
	configBytes := []byte(configJSON)

	// Use core.StartInstance which can load configuration directly from bytes
	server, err := core.StartInstance("json", configBytes)
	if err != nil {
		return nil, err
	}

	return server, nil
}

// SetTunFd sets the TUN file descriptor.
// Call this BEFORE RunXray/RunXrayFromJSON.
func SetTunFd(fd int32) {
	os.Setenv(platform.TunFdKey, strconv.Itoa(int(fd)))
}


// mphCachePath is accepted for API compatibility only. Xray-core v26 removed the
// MPH cache (and platform.MphCachePath with it); geo lookups no longer use one.
func InitEnv(datDir string, mphCachePath string) {
	os.Setenv(platform.AssetLocation, datDir)
	os.Setenv(platform.CertLocation, datDir)
}

// Run Xray instance.
// datDir means the dir which geosite.dat and geoip.dat are in.
// mphCachePath means the path of mph cache file. leave it empty if you don't use mph cache.
// configPath means the config.json file path.
func RunXray(datDir string, mphCachePath string, configPath string) (err error) {
	InitEnv(datDir, mphCachePath)
	memory.InitForceFree()
	coreServer, err = StartXray(configPath)
	if err != nil {
		return
	}

	if err = coreServer.Start(); err != nil {
		return
	}

	debug.FreeOSMemory()
	return nil
}

// Run Xray instance with JSON configuration string.
// datDir means the dir which geosite.dat and geoip.dat are in.
// mphCachePath means the path of mph cache file. leave it empty if you don't use mph cache.
// configJSON means the JSON configuration string.
func RunXrayFromJSON(datDir string, mphCachePath string, configJSON string) (err error) {
	InitEnv(datDir, mphCachePath)
	memory.InitForceFree()
	coreServer, err = StartXrayFromJSON(configJSON)
	if err != nil {
		return
	}

	debug.FreeOSMemory()
	return nil
}

// Get Xray State
func GetXrayState() bool {
	return coreServer != nil && coreServer.IsRunning()
}

// Stop Xray instance.
func StopXray() error {
	var err error
	if coreServer != nil {
		err = coreServer.Close()
		coreServer = nil
	}
	// Clear global caches that survive coreServer.Close().
	debug.FreeOSMemory()
	return err
}

// Xray's version
func XrayVersion() string {
	return core.Version()
}

// No-op since Xray-core v26, which removed the MPH cache along with the
// buildmphcache command this mirrored. The function is kept so the gomobile
// binding keeps exporting LibXrayBuildMphCache; callers still get a nil error
// and simply have nothing to build. The config is still decoded so a malformed
// config is reported here rather than silently accepted.
func BuildMphCache(datDir string, mphCachePath string, configPath string) error {
	InitEnv(datDir, "")
	cf, err := os.Open(configPath)
	if err != nil {
		return err
	}
	defer cf.Close()

	if _, err := serial.DecodeJSONConfig(cf); err != nil {
		return err
	}
	return nil
}
