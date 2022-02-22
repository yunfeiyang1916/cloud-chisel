package chserver

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"github.com/yunfeiyang1916/cloud-chisel/share/settings"
	"golang.org/x/crypto/acme/autocert"
	"io/ioutil"
	"net"
	"os"
	"os/user"
	"path/filepath"
)

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

func (s *Server) listener(host, port string) (net.Listener, error) {
	hasDomains := len(s.config.TLS.Domains) > 0
	hasKeyCert := s.config.TLS.Key != "" && s.config.TLS.Cert != ""
	if hasDomains && hasKeyCert {
		return nil, errors.New("cannot use key/cert and domains")
	}
	var tlsConf *tls.Config
	if hasDomains {
		tlsConf = s.tlsLetsEncrypt(s.config.TLS.Domains)
	}
	extra := ""
	if hasKeyCert {
		c, err := s.tlsKeyCert(s.config.TLS.Key, s.config.TLS.Cert, s.config.TLS.CA)
		if err != nil {
			return nil, err
		}
		tlsConf = c
		if port != "443" && hasDomains {
			extra = " (WARNING: LetsEncrypt will attempt to connect to your domain on port 443)"
		}
	}
	// tcp监听
	l, err := net.Listen("tcp", host+":"+port)
	if err != nil {
		return nil, err
	}
	// 可选的tls封装
	proto:="http"
	if tlsConf!=nil{
		proto+="s"
		l=tls.NewListener(l,tlsConf)
	}
	if err == nil {
		s.Infof("Listening on %s://%s:%s%s", proto, host, port, extra)
	}
	return l, nil
}

// tls加密
func (s *Server) tlsLetsEncrypt(domains []string) *tls.Config {
	// 准备证书管理器
	m := &autocert.Manager{
		Prompt: func(tosURL string) bool {
			s.Infof("Accepting LetsEncrypt TOS and fetching certificate...")
			return true
		},
		Email:      settings.Env("LE_EMAIL"),
		HostPolicy: autocert.HostWhitelist(domains...),
	}
	// 设置文件缓存
	c := settings.Env("LE_CACHE")
	if c == "" {
		h := os.Getenv("HOME")
		if h == "" {
			if u, err := user.Current(); err == nil {
				h = u.HomeDir
			}
		}
		c = filepath.Join(h, ".cache", "chisel")
	}
	if c != "-" {
		s.Infof("LetsEncrypt cache directory %s", c)
		m.Cache = autocert.DirCache(c)
	}
	// 创建tls配置
	return m.TLSConfig()
}

// 从给定的证书路径加载
func (s *Server) tlsKeyCert(key, cert string, ca string) (*tls.Config, error) {
	keypair, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return nil, err
	}
	// 基于文件的TLS配置使用默认TLS
	c := &tls.Config{
		Certificates: []tls.Certificate{keypair},
	}
	if ca != "" {
		if err := addCA(ca, c); err != nil {
			return nil, err
		}
		s.Infof("Loaded CA path: %s", ca)
	}
	return c, nil
}

// 添加根证书
func addCA(ca string, c *tls.Config) error {
	fileInfo, err := os.Stat(ca)
	if err != nil {
		return err
	}
	clientCAPool := x509.NewCertPool()
	if fileInfo.IsDir() {
		// 这是一个存放CA bundle文件的目录
		files, err := ioutil.ReadDir(ca)
		if err != nil {
			return err
		}
		// 从path添加所有的证书文件
		for _, file := range files {
			f := file.Name()
			if err := addPEMFile(filepath.Join(ca, f), clientCAPool); err != nil {
				return err
			}
		}
	} else {
		// this is a CA bundle file
		if err := addPEMFile(ca, clientCAPool); err != nil {
			return err
		}
	}
	// 设置客户端cas并启用证书验证
	c.ClientCAs = clientCAPool
	c.ClientAuth = tls.RequireAndVerifyClientCert
	return nil
}

// 添加pem编码的文件到x509.CertPool
func addPEMFile(path string, pool *x509.CertPool) error {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	if !pool.AppendCertsFromPEM(content) {
		return errors.New("Fail to load certificates from : " + path)
	}
	return nil
}
