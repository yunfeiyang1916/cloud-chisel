package settings

// Config 设置中的配置
type Config struct {
	// 版本号
	Version string
	// 本地与远程服务的映射集合
	Remotes
}
