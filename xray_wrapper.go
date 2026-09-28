// libXray is an Xray wrapper focusing on improving the experience of Xray-core mobile development.
package libXray

import (
	"encoding/base64"
	"encoding/json"

	"github.com/xtls/libxray/geo"
	"github.com/xtls/libxray/memory"
	"github.com/xtls/libxray/nodep"
	"github.com/xtls/libxray/xray"
)

// SetTunFd sets the TUN file descriptor.
// Call this BEFORE RunXray/RunXrayFromJSON.
func SetTunFd(fd int32) {
	xray.SetTunFd(fd)
}

// SetMemoryLimitMB sets the Go heap ceiling in megabytes.
// Call this BEFORE RunXray/RunXrayFromJSON.
func SetMemoryLimitMB(mb int64) {
	memory.SetMemoryLimitMB(mb)
}

// FreeOSMemory forces a GC and returns all free Go heap pages to the OS.
// Stops the world briefly; meant for memory emergencies, not the steady state.
func FreeOSMemory() {
	memory.FreeOSMemory()
}

// GoMemoryReport returns a one-line summary of the Go runtime's memory.
func GoMemoryReport() string {
	return memory.Report()
}

// GoroutineSummary groups live goroutines by where they are parked and returns
// the largest groups. Takes a goroutine profile; emergency diagnostics only.
func GoroutineSummary(top int) string {
	return memory.GoroutineSummary(top)
}

type CountGeoDataRequest struct {
	DatDir  string `json:"datDir,omitempty"`
	Name    string `json:"name,omitempty"`
	GeoType string `json:"geoType,omitempty"`
}

// Read geo data and write all codes to text file.
func CountGeoData(base64Text string) string {
	var response nodep.CallResponse[string]
	req, err := base64.StdEncoding.DecodeString(base64Text)
	if err != nil {
		return response.EncodeToBase64("", err)
	}
	var request CountGeoDataRequest
	err = json.Unmarshal(req, &request)
	if err != nil {
		return response.EncodeToBase64("", err)
	}
	err = geo.CountGeoData(request.DatDir, request.Name, request.GeoType)
	return response.EncodeToBase64("", err)
}

type readGeoFilesResponse struct {
	Domain []string `json:"domain,omitempty"`
	IP     []string `json:"ip,omitempty"`
}

// thin geo data
func ReadGeoFiles(base64Text string) string {
	var response nodep.CallResponse[*readGeoFilesResponse]
	xray, err := base64.StdEncoding.DecodeString(base64Text)
	if err != nil {
		return response.EncodeToBase64(nil, err)
	}
	domain, ip := geo.ReadGeoFiles(xray)
	var resp readGeoFilesResponse
	resp.Domain = domain
	resp.IP = ip
	return response.EncodeToBase64(&resp, nil)
}

type pingRequest struct {
	DatDir     string `json:"datDir,omitempty"`
	ConfigPath string `json:"configPath,omitempty"`
	Timeout    int    `json:"timeout,omitempty"`
	Url        string `json:"url,omitempty"`
	Proxy      string `json:"proxy,omitempty"`
}

type pingBatchItemRequest struct {
	XrayJSON    string `json:"xrayJSON,omitempty"`
	OutboundTag string `json:"outboundTag,omitempty"`
}

type pingBatchRequest struct {
	Configs []pingBatchItemRequest `json:"configs,omitempty"`
	Timeout int                    `json:"timeout,omitempty"`
	URL     string                 `json:"url,omitempty"`
}

type pingBatchItemResponse struct {
	Success bool   `json:"success"`
	Delay   int64  `json:"delay"`
	Error   string `json:"error,omitempty"`
}

// Ping Xray config and get the delay of its outbound.
func Ping(base64Text string) string {
	var response nodep.CallResponse[int64]
	req, err := base64.StdEncoding.DecodeString(base64Text)
	if err != nil {
		return response.EncodeToBase64(nodep.PingDelayError, err)
	}
	var request pingRequest
	err = json.Unmarshal(req, &request)
	if err != nil {
		return response.EncodeToBase64(nodep.PingDelayError, err)
	}
	delay, err := xray.Ping(request.DatDir, request.ConfigPath, request.Timeout, request.Url, request.Proxy)
	return response.EncodeToBase64(delay, err)
}

// PingBatch measures up to ten Xray configs in one temporary core instance.
// Keeping the outbounds together avoids Xray-core's process-global dialer state
// being replaced by concurrently-created temporary instances.
func PingBatch(base64Text string) string {
	var response nodep.CallResponse[[]pingBatchItemResponse]
	req, err := base64.StdEncoding.DecodeString(base64Text)
	if err != nil {
		return response.EncodeToBase64(nil, err)
	}
	var request pingBatchRequest
	if err := json.Unmarshal(req, &request); err != nil {
		return response.EncodeToBase64(nil, err)
	}

	items := make([]xray.PingBatchItem, len(request.Configs))
	for index, config := range request.Configs {
		items[index] = xray.PingBatchItem{
			XrayJSON:    config.XrayJSON,
			OutboundTag: config.OutboundTag,
		}
	}
	results, err := xray.PingBatch(items, request.Timeout, request.URL)
	if err != nil {
		return response.EncodeToBase64(nil, err)
	}

	encodedResults := make([]pingBatchItemResponse, len(results))
	for index, result := range results {
		encodedResults[index] = pingBatchItemResponse{
			Success: result.Success,
			Delay:   result.Delay,
			Error:   result.Error,
		}
	}
	return response.EncodeToBase64(encodedResults, nil)
}

// query inbound and outbound stats.
func QueryStats(base64Text string) string {
	var response nodep.CallResponse[string]
	server, err := base64.StdEncoding.DecodeString(base64Text)
	if err != nil {
		return response.EncodeToBase64("", err)
	}

	stats, err := xray.QueryStats(string(server))
	if err != nil {
		return response.EncodeToBase64("", err)
	}
	return response.EncodeToBase64(stats, nil)
}

// Test Xray Config.
func TestXray(base64Text string) string {
	var response nodep.CallResponse[string]
	req, err := base64.StdEncoding.DecodeString(base64Text)
	if err != nil {
		return response.EncodeToBase64("", err)
	}
	var request RunXrayRequest
	err = json.Unmarshal(req, &request)
	if err != nil {
		return response.EncodeToBase64("", err)
	}
	err = xray.TestXray(request.DatDir, request.ConfigPath)
	return response.EncodeToBase64("", err)
}

type RunXrayRequest struct {
	DatDir       string `json:"datDir,omitempty"`
	MphCachePath string `json:"mphCachePath,omitempty"`
	ConfigPath   string `json:"configPath,omitempty"`
}

type RunXrayFromJSONRequest struct {
	DatDir       string `json:"datDir,omitempty"`
	MphCachePath string `json:"mphCachePath,omitempty"`
	ConfigJSON   string `json:"configJSON,omitempty"`
}

// Create Xray Run Request
func NewXrayRunRequest(datDir, mphCachePath, configPath string) (string, error) {
	request := RunXrayRequest{
		DatDir:       datDir,
		MphCachePath: mphCachePath,
		ConfigPath:   configPath,
	}
	requestBytes, err := json.Marshal(&request)
	if err != nil {
		return "", err
	}

	// Encode the JSON bytes to a base64 string
	return base64.StdEncoding.EncodeToString(requestBytes), nil
}

// Create Xray Run From JSON Request
func NewXrayRunFromJSONRequest(datDir, mphCachePath, configJSON string) (string, error) {
	request := RunXrayFromJSONRequest{
		DatDir:       datDir,
		MphCachePath: mphCachePath,
		ConfigJSON:   configJSON,
	}
	requestBytes, err := json.Marshal(&request)
	if err != nil {
		return "", err
	}

	// Encode the JSON bytes to a base64 string
	return base64.StdEncoding.EncodeToString(requestBytes), nil
}

// Run Xray instance.
func RunXray(base64Text string) string {
	var response nodep.CallResponse[string]
	req, err := base64.StdEncoding.DecodeString(base64Text)
	if err != nil {
		return response.EncodeToBase64("", err)
	}
	var request RunXrayRequest
	err = json.Unmarshal(req, &request)
	if err != nil {
		return response.EncodeToBase64("", err)
	}
	err = xray.RunXray(request.DatDir, request.MphCachePath, request.ConfigPath)
	return response.EncodeToBase64("", err)
}

// Run Xray instance with JSON configuration.
func RunXrayFromJSON(base64Text string) string {
	var response nodep.CallResponse[string]
	req, err := base64.StdEncoding.DecodeString(base64Text)
	if err != nil {
		return response.EncodeToBase64("", err)
	}
	var request RunXrayFromJSONRequest
	err = json.Unmarshal(req, &request)
	if err != nil {
		return response.EncodeToBase64("", err)
	}
	err = xray.RunXrayFromJSON(request.DatDir, request.MphCachePath, request.ConfigJSON)
	return response.EncodeToBase64("", err)
}

// Get Xray State
func GetXrayState() bool {
	return xray.GetXrayState()
}

// Stop Xray instance.
func StopXray() string {
	var response nodep.CallResponse[string]
	err := xray.StopXray()
	return response.EncodeToBase64("", err)
}

// Xray's version
func XrayVersion() string {
	var response nodep.CallResponse[string]
	return response.EncodeToBase64(xray.XrayVersion(), nil)
}

// Build Mph Cache
func BuildMphCache(base64Text string) string {
	var response nodep.CallResponse[string]
	req, err := base64.StdEncoding.DecodeString(base64Text)
	if err != nil {
		return response.EncodeToBase64("", err)
	}
	var request RunXrayRequest
	err = json.Unmarshal(req, &request)
	if err != nil {
		return response.EncodeToBase64("", err)
	}
	err = xray.BuildMphCache(request.DatDir, request.MphCachePath, request.ConfigPath)
	return response.EncodeToBase64("", err)
}
