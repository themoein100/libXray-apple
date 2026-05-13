//go:build !ios

package memory

func SetMemoryLimitMB(mb int64) {}
func InitForceFree()             {}
