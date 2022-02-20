package chserver

import "time"

// Config server配置
type Config struct {
	KeySeed string
	// 一个可选的user.json路径。这个文件应该是一个对象，如下定义：{"<user:pass>": ["<addr-regex>","<addr-regex>"]}
	// 当使用<user>连接时，<pass>将被验证，然后每个远程地址将与列表进行正则匹配
	// 普通远程地址形式：<remote-host>:<remote-port>
	// 用于反向端口转发远程地址形式：R:<local-interface>:<local-port>
	// 用于反向端口转发远程
	AuthFile string
	// 形式为：<user:pass>，可选。
	// 等价于authfile {"<user:pass>": [""]},如果未设置，则将使用AUTH环境变量
	Auth string
	//
	Proxy string
	// 是否允许客户端访问内部的SOCKS5代理
	Socks5 bool
	// 反向代理，是否允许客户端指定反向端口转发远程服务
	Reverse   bool
	// 可选的保活间隔。 由于底层传输是HTTP，在许多情况下我们将遍历代理，这些代理通常会关闭空闲连接。
	// 您必须使用单位指定时间，例如“5s”或“2m”。 默认为“25s”（设置为 0s 以禁用）。
	KeepAlive time.Duration
	// 传输层安全协议的设置
	TLS       TLSConfig
}
