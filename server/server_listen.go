package chserver

// TLSConfig Transport Layer Security 传输层安全协议的设置
type TLSConfig struct {
	// 启用TLS，并提供pem编码的TLS私钥的可选路径。
	// 设置此标志时，还必须设置——tls-cert，并且不能设置tls-domain
	Key string
	// 启用TLS，并为pem编码的TLS证书提供可选路径。
	// 设置此标志时，还必须设置tls-key，并且不能设置tls-domain
	Cert string
	// 启用TLS，并使用LetsEncypt自动获取TLS密钥和证书。需要指定端口443。
	// 你可以指定多个tls-domain标志来服务多个域。
	// 生成的文件缓存在$HOME/.cache/chisel目录中。
	// 可以通过设置CHISEL_LE_CACHE环境变量来修改该路径，
	// 或者通过将这个变量设置为"-"来禁用缓存。通过设置CHISEL_LE_EMAIL，您可以选择提供证书通知电子邮件
	Domains []string
	// 一个PEM编码的CA证书包的路径，或者一个存放多个PEM编码CA证书包文件的目录，用于验证客户端连接。
	// 提供的CA证书将代替系统根证书。这通常用于实现mutual-TLS
	CA string
}
