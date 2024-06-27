package etcdconfig

import (
	"context"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	client "go.etcd.io/etcd/client/v3"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	FN   = "application" //生成配置文件的名称
	PATH = "HOME"        //生成配置文件的根目录
)

type EtcdConfig struct {
	client      *client.Client
	lock        sync.RWMutex
	keyName     string
	group       string   // group
	configType  string   //配置的类型，目前支持：json、yaml
	endpoints   []string //etcd 地址配置
	password    string   // 连接密码
	lastSentRev int64
	v           *viper.Viper
	path        string //缓存文件的路径
	keyPrefix   string //key 前缀
}

// NewClient
// @Author mail@liangdongpo.com
// @Description create client
// @Date 8:38 PM 2022/12/4
// @Param endpoints etcd 地址，password 密码，group ：配置group， configType：json or yaml,prefix:key prefix
// @return
func NewClient(endpoints []string, password string, group, configType string, prefix ...string) (*EtcdConfig, error) {
	e := &EtcdConfig{}
	e.endpoints = endpoints
	e.configType = configType
	e.group = group
	if password != "" {
		e.password = password
	}
	cfg := client.Config{
		Endpoints: e.endpoints,
		// set timeout per request to fail fast when the target endpoints is unavailable
		DialKeepAliveTimeout: time.Second * 10,
		DialTimeout:          time.Second * 30,
		Password:             e.password,
	}
	c, err := client.New(cfg)
	if err != nil {
		return nil, err
	}
	e.client = c
	//前缀，没有设置给个默认
	if len(prefix) == 0 {
		e.keyPrefix = "/hub-config/etcd"
	} else {
		e.keyPrefix = prefix[0]
	}
	e.keyName = e.keyPrefix + "/" + e.group + "/" + e.configType

	return e, nil
}

// SetWatcher
// @Author mail@liangdongpo.com
// @Description 设置监听，etcd Watch 和 Viper 文件变动监听
// @Date 10:59 PM 2022/12/4
// @Param
// @return
func (e *EtcdConfig) SetWatcher() error {
	e.path = filepath.Join(os.Getenv(PATH), e.keyName)
	//创建缓存目录
	err := os.MkdirAll(e.path, 0755)
	if err != nil {
		return err
	}
	//监听etcd
	go func() {
		_ = e.startWatch()
	}()
	content, err := e.EtcdGet()
	//第一次获取错误,获取没有值的话，给一个空值，创建空文件
	if err != nil || len(content) == 0 {
		if e.configType == "json" {
			//json 格式的需要创建一个空json
			content = []byte("{}")
		} else {
			content = []byte("")
		}
	}
	//先写一次数据到缓存文件
	//fmt.Println(filepath.Join(e.path, fmt.Sprintf("%s.%s", FN, e.configType)))
	err = e.WriteCache(content)
	if err != nil {
		return err
	}
	err = e.NewViper()
	if err != nil {
		return err
	}
	return nil
}

// WriteCache
// @Author mail@liangdongpo.com
// @Description 写入缓存文件
// @Date 10:43 PM 2022/12/4
// @Param
// @return
func (e *EtcdConfig) WriteCache(content []byte) error {
	err := ioutil.WriteFile(filepath.Join(e.path, fmt.Sprintf("%s.%s", FN, e.configType)), content, 0644)
	if err != nil {
		log.Printf("GetConfig err: %v", err)
		return err
	}
	return nil
}

// NewViper
// @Author mail@liangdongpo.com
// @Description 创建 Viper
// @Date 10:44 PM 2022/12/4
// @Param
// @return
func (e *EtcdConfig) NewViper() error {
	v := viper.New()
	v.SetConfigName(FN)
	v.SetConfigType(e.configType)
	v.AddConfigPath(e.path)
	err := v.ReadInConfig()
	if err != nil {
		log.Printf(err.Error())
		return err
	}
	v.WatchConfig()
	v.OnConfigChange(func(in fsnotify.Event) {
		log.Printf("Config file change: %s op: %d\n", in.Name, in.Op)
	})
	//赋值
	e.v = v
	return nil
}

// startWatch
// @Author mail@liangdongpo.com
// @Description 创建etcd 监听
// @Date 10:45 PM 2022/12/4
// @Param
// @return
func (e *EtcdConfig) startWatch() error {
	watcher := e.client.Watch(context.Background(), e.keyName)
	for res := range watcher {
		for _, ev := range res.Events {
			if ev.IsCreate() || ev.IsModify() {
				e.lock.RLock()
				//fmt.Printf("Type: %s Key:%s Value:%s\n", ev.Type, ev.Kv.Key, ev.Kv.Value)
				_ = e.WriteCache(ev.Kv.Value)
				e.lock.RUnlock()
			} else {
				//fmt.Println("不是创建，也不是更新")
			}
		}
	}
	return nil
}

// EtcdPut
// @Author mail@liangdongpo.com
// @Description  put 数据
// @Date 10:45 PM 2022/12/4
// @Param
// @return
func (e *EtcdConfig) EtcdPut(val string, group string, configType string) error {
	e.lock.Lock()
	defer e.lock.Unlock()
	if configType == "" {
		configType = e.configType
	}
	if group == "" {
		group = e.group
	}
	//keyName重新赋值
	e.keyName = e.keyPrefix + "/" + group + "/" + configType
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	_, err := e.client.Put(ctx, e.keyName, val)
	//if err == nil {
	//	e.lastSentRev = resp.Header.GetRevision()
	//}
	cancel()
	return err
}

// EtcdGet
// @Author 东坡
// @Description get 数据
// @Date 10:46 PM 2022/12/4
// @Param
// @return
func (e *EtcdConfig) EtcdGet() ([]byte, error) {
	getResp, err := e.client.Get(context.Background(), e.keyName)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	if len(getResp.Kvs) == 0 {
		return []byte(""), nil
	}
	return getResp.Kvs[0].Value, nil
}

// GetDiscoverServices
// @Author 东坡
// @Description 获取服务注册的服务列表，目前只简单获取了服务列表
// @Date 16:01 2024/6/27
// @Param
// @return
func (e *EtcdConfig) GetDiscoverServices() (map[string][]string, error) {
	servicesMap := make(map[string][]string)
	resp, err := e.client.Get(context.Background(), e.keyPrefix, client.WithPrefix())
	if err != nil {
		return nil, err
	}
	for _, kv := range resp.Kvs {
		// 这里目前只提供简单的服务发现功能，可以根据需要扩展
		//sd.lock.Lock()
		//defer sd.lock.Unlock()
		// 提取服务名称和 Pod ID
		key := string(kv.Key)
		value := string(kv.Value)
		parts := strings.Split(key, "/")
		if len(parts) >= 2 {
			// 获取服务名称（假设它是路径的最后一个部分）
			serviceName := parts[len(parts)-2]
			servicesMap[serviceName] = append(servicesMap[serviceName], value)
		}
	}
	return servicesMap, nil
}

// Get
// @Author  mail@liangdongpo.com
// @Description 获取某个缓存，只支持默认参数下
// @Date 2:25 下午 2021/11/18
// @Param
// @return
func (e *EtcdConfig) Get(key string) interface{} {
	return e.v.Get(key)
}

// GetString
// @Author  mail@liangdongpo.com
// @Description 获取string类型的缓存，只支持默认参数下
// @Date 2:30 下午 2021/11/18
// @Param
// @return
func (e *EtcdConfig) GetString(key string) string {
	return e.v.GetString(key)
}

// GetBool 获取bool 类型 支持默认参数
func (e *EtcdConfig) GetBool(key string) bool {
	return e.v.GetBool(key)
}

// GetInt 获取int 类型 支持默认参数
func (e *EtcdConfig) GetInt(key string) int {
	return e.v.GetInt(key)
}

func (e *EtcdConfig) GetInt32(key string) int32 {
	return e.v.GetInt32(key)
}

func (e *EtcdConfig) GetInt64(key string) int64 {
	return e.v.GetInt64(key)
}

func (e *EtcdConfig) GetUint(key string) uint {
	return e.v.GetUint(key)
}

func (e *EtcdConfig) GetUint32(key string) uint32 {
	return e.v.GetUint32(key)
}

func (e *EtcdConfig) GetUint64(key string) uint64 {
	return e.v.GetUint64(key)
}

func (e *EtcdConfig) GetFloat64(key string) float64 {
	return e.v.GetFloat64(key)
}

func (e *EtcdConfig) GetTime(key string) time.Time {
	return e.v.GetTime(key)
}

func (e *EtcdConfig) GetDuration(key string) time.Duration {
	return e.v.GetDuration(key)
}

func (e *EtcdConfig) GetIntSlice(key string) []int {
	return e.v.GetIntSlice(key)
}

func (e *EtcdConfig) GetStringSlice(key string) []string {
	return e.v.GetStringSlice(key)
}

func (e *EtcdConfig) GetStringMap(key string) map[string]interface{} {
	return e.v.GetStringMap(key)
}

func (e *EtcdConfig) GetStringMapString(key string) map[string]string {
	return e.v.GetStringMapString(key)
}

func (e *EtcdConfig) GetStringMapStringSlice(key string) map[string][]string {
	return e.v.GetStringMapStringSlice(key)
}

func (e *EtcdConfig) GetSizeInBytes(key string) uint {
	return e.v.GetSizeInBytes(key)
}

func (e *EtcdConfig) AllSettings() map[string]interface{} {
	return e.v.AllSettings()
}
