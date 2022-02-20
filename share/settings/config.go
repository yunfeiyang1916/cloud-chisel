package settings

import (
	"encoding/json"
	"fmt"
)

// Config 本地与远程服务的映射集合及协议版本号
type Config struct {
	// 版本号
	Version string
	// 本地与远程服务的映射集合
	Remotes
}

// DecodeConfig 解码配置
func DecodeConfig(b []byte) (*Config, error) {
	c := &Config{}
	err := json.Unmarshal(b, c)
	if err != nil {
		return nil, fmt.Errorf("Invalid JSON config")
	}
	return c, nil
}

// EncodeConfig 编码配置
func EncodeConfig(c Config) []byte {
	// 编码不会失败的
	b, _ := json.Marshal(c)
	return b
}
