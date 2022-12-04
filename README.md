# etcd-config
基于etcd的配置客户端

使用方法：


`go get github.com/ldongpo/etcd-config`


## 客户端监听服务端数据变化

```
c, _ := etcdconfig.NewClient([]string{"http://27.0.0.1:2379"}, "", "group", "json")

// 客户端需要监听的时候需要这行代码
_ = c.SetWatcher()

//c.GetString 需要提前监听 c.SetWatcher()，如果客户端只是写入功能无需监听
fmt.Println(c.GetString("test"))
```

NewClient 接受etcd的连接地址，配置的group，和配置的类型，目前有 json 和 yaml 两种格式


需要我们在往etcd put 数据的时候按规定的格式写入

## 客户端写入数据实例：
EtcdPut 第二个参数是 group ，不传默认group

```
// put 一个json 数据
_ = c.EtcdPut(`{"test":123}`,"")
```

```
c, _ := etcdconfig.NewClient([]string{"http://27.0.0.1:2379"}, "", "group", "yaml")
// put 一个yaml 数据
yaml := `
test:
    123abc
    `
_ = c.EtcdPut(yaml,"")

fmt.Println(c.GetString("test"))
//输出：123abc
```

除了 c.GetString 方法外，还有GetInt ，GetInt32 等，具体可参考 ：github.com/spf13/viper