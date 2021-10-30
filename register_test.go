package discovery

import (
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestRegister(t *testing.T) {
	info := ServiceInfo{
		Name:    "etcd-g.srv.mail",
		Addr:    "127.0.0.1:8748",
		Version: "1.0",
		Weight:  0,
	}
	addrs := []string{"127.0.0.1:123", "127.0.0.1:22379", "127.0.0.1:32379"}
	register := NewRegister(addrs, zap.NewNop())

	err := register.Start(info, 1)
	if err != nil {
		t.Fatal(err)
	}

	serverInfo, err := register.GetServerInfo()
	if err != nil {
		t.Fatal(err)
	}
	log.Println(serverInfo, err)

	// set serverInfo.weight = 3
	req, err := http.NewRequest("GET", "/weight?weight=3", nil)
	if err != nil {
		t.Fatalf("init request failed: %v", err)
	}

	recorder := httptest.NewRecorder()
	register.UpdateHandler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Errorf("return wrong http status code, got: %v want: %v", recorder.Code, http.StatusOK)
	}

	info, err = register.GetServerInfo()
	if err != nil {
		t.Fatalf("get info failed %v", err)
	}
	log.Println(info)
	if info.Weight != 3 {
		t.Fatal("update weight error")
	}
}
