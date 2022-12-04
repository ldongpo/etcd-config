package etcdconfig

import (
	"fmt"
	"testing"
)

func TestClient(t *testing.T) {
	c, _ := NewClient([]string{"http://27.0.0.1:2379"}, "", "test", "json")

	_ = c.SetWatcher()
	fmt.Println(c.GetString("test"))

	_ = c.EtcdPut(`{"test":123}`, "")

	fmt.Println(c.GetString("test"))
}
