package settings

import (
	"os"
	"strconv"
	"time"
)

// Env 前缀为 CHISEL_ 的环境变量
func Env(name string) string {
	return os.Getenv("CHISEL_" + name)
}

// EnvInt 前缀为 CHISEL_ 的整型环境变量
func EnvInt(name string, def int) int {
	if n, err := strconv.Atoi(Env(name)); err == nil {
		return n
	}
	return def
}

// EnvDuration 前缀为 CHISEL_ 的时间环境变量
func EnvDuration(name string, def time.Duration) time.Duration {
	if n, err := time.ParseDuration(Env(name)); err == nil {
		return n
	}
	return def
}
