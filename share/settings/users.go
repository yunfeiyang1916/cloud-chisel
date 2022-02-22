package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/yunfeiyang1916/cloud-chisel/share/cio"
	"io/ioutil"
	"regexp"
	"sync"
)

type Users struct {
	sync.RWMutex
	inner map[string]*User
}

func NewUsers() *Users {
	return &Users{inner: map[string]*User{}}
}

// Len 返回users数量
func (u *Users) Len() int {
	u.RLock()
	l := len(u.inner)
	u.RUnlock()
	return l
}

// Get user from the index by key
func (u *Users) Get(key string) (*User, bool) {
	u.RLock()
	user, found := u.inner[key]
	u.RUnlock()
	return user, found
}

// Set a users into the list by specific key
func (u *Users) Set(key string, user *User) {
	u.Lock()
	u.inner[key] = user
	u.Unlock()
}

// Del ete a users from the list
func (u *Users) Del(key string) {
	u.Lock()
	delete(u.inner, key)
	u.Unlock()
}

// AddUser adds a users to the set
func (u *Users) AddUser(user *User) {
	u.Set(user.Name, user)
}

// Reset 重置，如果值为nil表示清除所有
func (u *Users) Reset(users []*User) {
	m := map[string]*User{}
	for _, u := range users {
		m[u.Name] = u
	}
	u.Lock()
	u.inner = m
	u.Unlock()
}

// UserIndex 可重载的用户源配置
type UserIndex struct {
	*cio.Logger
	*Users
	configFile string
}

// NewUserIndex 创建
func NewUserIndex(logger *cio.Logger) *UserIndex {
	return &UserIndex{
		Logger: logger.Fork("users"),
		Users:  NewUsers(),
	}
}

// LoadUsers 从给定的文件路径加载，默认为authfile指定的文件路径
func (u *UserIndex) LoadUsers(configFile string) error {
	u.configFile = configFile
	u.Infof("Loading configuration file %s", configFile)
	if err := u.loadUserIndex(); err != nil {
		return err
	}
	if err := u.addWatchEvents(); err != nil {
		return err
	}
	return nil
}

// 负责监视文件的更新和重新加载
func (u *UserIndex) addWatchEvents() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := watcher.Add(u.configFile); err != nil {
		return err
	}
	go func() {
		for e := range watcher.Events {
			if e.Op&fsnotify.Write != fsnotify.Write {
				continue
			}
			if err := u.loadUserIndex(); err != nil {
				u.Infof("Failed to reload the users configuration: %s", err)
			} else {
				u.Debugf("Users configuration successfully reloaded from: %s", u.configFile)
			}
		}
	}()
	return nil
}

// 加载用户配置
func (u *UserIndex) loadUserIndex() error {
	if u.configFile == "" {
		return errors.New("configuration file not set")
	}
	b, err := ioutil.ReadFile(u.configFile)
	if err != nil {
		return fmt.Errorf("Failed to read auth file: %s, error: %s", u.configFile, err)
	}
	var raw map[string][]string
	if err := json.Unmarshal(b, &raw); err != nil {
		return errors.New("Invalid JSON: " + err.Error())
	}
	users := []*User{}
	for auth, remotes := range raw {
		user := &User{}
		user.Name, user.Pass = ParseAuth(auth)
		if user.Name == "" {
			return errors.New("Invalid user:pass string")
		}
		for _, r := range remotes {
			if r == "" || r == "*" {
				user.Addrs = append(user.Addrs, UserAllowAll)
			} else {
				re, err := regexp.Compile(r)
				if err != nil {
					return errors.New("Invalid address regex")
				}
				user.Addrs = append(user.Addrs, re)
			}
		}
		users = append(users, user)
	}
	//swap
	u.Reset(users)
	return nil
}
