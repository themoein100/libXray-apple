//go:build !ios

package memory

func SetMemoryLimitMB(mb int64) {}
func InitForceFree()            {}
func FreeOSMemory()             {}
func Report() string            { return "" }
