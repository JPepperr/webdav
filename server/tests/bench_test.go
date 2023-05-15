package webdav_testing

import (
	"io"
	webdavvc "jpepper_webdav/webdavvc/lib"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var cfg webdavvc.Config = webdavvc.Config{
	FileSystemRootPath:  "./webdav_fs",
	Port:                4444,
	ReadTimeoutSeconds:  1,
	WriteTimeoutSeconds: 5,
	IdleTimeoutSeconds:  120,
	LogLevel:            "error",
	CacheSize:           1,
}
var client *http.Client = &http.Client{}

func NewServer() *http.Server {
	srv, err := webdavvc.GetServer(cfg)
	if err != nil {
		panic(err)
	}
	return srv
}

func DoRequest(req *http.Request) (*http.Response, error) {
	return client.Do(req)
}

func DoPut(url string, r io.Reader) error {
	req, err := http.NewRequest("PUT", url, r)
	if err != nil {
		return err
	}
	_, err = DoRequest(req)
	if err != nil {
		return err
	}
	return nil
}

func BenchmarkGet(b *testing.B) {
	server := NewServer()

	ts := httptest.NewServer(server.Handler)
	defer ts.Close()

	err := DoPut(ts.URL+"/test.txt", strings.NewReader("test"))
	if err != nil {
		b.Fatalf("request failed: %v", err)
	}

	req, err := http.NewRequest("GET", ts.URL+"/test.txt", nil)
	if err != nil {
		b.Fatalf("can't create get request: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := DoRequest(req)
		b.StopTimer()
		if err != nil || res.StatusCode != 200 {
			b.Fatalf("request failed: %v", err)
		}
		res.Body.Close()
		b.StartTimer()
	}
}

func BenchmarkVersionControlCacheMiss(b *testing.B) {
	server := NewServer()

	ts := httptest.NewServer(server.Handler)
	defer ts.Close()

	err := DoPut(ts.URL+"/test1.txt", strings.NewReader("test"))
	if err != nil {
		b.Fatalf("request failed: %v", err)
	}

	err = DoPut(ts.URL+"/test2.txt", strings.NewReader("test"))
	if err != nil {
		b.Fatalf("request failed: %v", err)
	}

	req1, err := http.NewRequest("VERSION-CONTROL", ts.URL+"/test1.txt", nil)
	if err != nil {
		b.Fatalf("can't create version-control request: %v", err)
	}
	req2, err := http.NewRequest("VERSION-CONTROL", ts.URL+"/test2.txt", nil)
	if err != nil {
		b.Fatalf("can't create version-control request: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := DoRequest(req1)
		b.StopTimer()
		if err != nil || res.StatusCode != 200 {
			b.Fatalf("request failed: %v", err)
		}
		res.Body.Close()
		b.StartTimer()
		res, err = DoRequest(req2)
		b.StopTimer()
		if err != nil || res.StatusCode != 200 {
			b.Fatalf("request failed: %v", err)
		}
		res.Body.Close()
		b.StartTimer()
	}
}
func BenchmarkVersionControlCacheHit(b *testing.B) {
	server := NewServer()

	ts := httptest.NewServer(server.Handler)
	defer ts.Close()

	err := DoPut(ts.URL+"/test.txt", strings.NewReader("test"))
	if err != nil {
		b.Fatalf("request failed: %v", err)
	}

	req, err := http.NewRequest("VERSION-CONTROL", ts.URL+"/test.txt", nil)
	if err != nil {
		b.Fatalf("can't create version-control request: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := DoRequest(req)
		b.StopTimer()
		if err != nil || res.StatusCode != 200 {
			b.Fatalf("request failed: %v", err)
		}
		res.Body.Close()
		b.StartTimer()
	}
}

func BenchmarkCheckin(b *testing.B) {
	server := NewServer()

	ts := httptest.NewServer(server.Handler)
	defer ts.Close()

	err := DoPut(ts.URL+"/test.txt", strings.NewReader("test"))
	if err != nil {
		b.Fatalf("request failed: %v", err)
	}

	req, err := http.NewRequest("CHECKIN", ts.URL+"/test.txt", nil)
	if err != nil {
		b.Fatalf("can't create checkin request: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := DoRequest(req)
		b.StopTimer()
		if err != nil || res.StatusCode != 201 {
			b.Fatalf("request failed: %v", err)
		}
		res.Body.Close()
		b.StartTimer()
	}
}

func BenchmarkCheckout(b *testing.B) {
	server := NewServer()

	ts := httptest.NewServer(server.Handler)
	defer ts.Close()

	err := DoPut(ts.URL+"/test.txt", strings.NewReader("test"))
	if err != nil {
		b.Fatalf("request failed: %v", err)
	}

	req, err := http.NewRequest("VERSION-CONTROL", ts.URL+"/test.txt", nil)
	if err != nil {
		b.Fatalf("can't create version-control request: %v", err)
	}

	res, err := DoRequest(req)
	if err != nil {
		b.Fatalf("request failed: %v", err)
	}
	ver := res.Header.Get("Version")

	req, err = http.NewRequest("CHECKOUT", ts.URL+"/test.txt", nil)
	if err != nil {
		b.Fatalf("can't create checkout request: %v", err)
	}
	req.Header.Add("Version", ver)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := DoRequest(req)
		b.StopTimer()
		if err != nil || res.StatusCode != 200 {
			b.Fatalf("request failed: %v", err)
		}
		res.Body.Close()
		b.StartTimer()
	}
}
