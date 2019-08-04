package stream

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

const (
	http2Port = 7777
	proxyPort = 17777
)

func TestHTTP2(t *testing.T) {
	server := startHTTP2Server(t)
	defer server.Close()
	proxyServer := startHTTP2ReverseProxy(t)
	defer proxyServer.Close()

	client := &http.Client{
		Timeout: time.Second,
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(netw, addr string, cfg *tls.Config) (net.Conn, error) {
				return net.Dial(netw, addr)
			},
		},
	}
	defer client.CloseIdleConnections()

	tests := []struct {
		msg  string
		port int
	}{
		{
			"direct",
			http2Port,
		},
		{
			"via proxy",
			proxyPort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			req, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d", tt.port), nil)
			require.NoError(t, err)

			resp, err := client.Do(req)
			require.NoError(t, err)
			assert.Contains(t, getResponseBody(t, resp), "hello world HTTP/2.0")
		})
	}
}

func startHTTP2Server(t *testing.T) *http.Server {
	h1Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "hello world "+r.Proto)
	})

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", http2Port))
	require.NoError(t, err)

	server := &http.Server{
		Handler: h2c.NewHandler(h1Handler, &http2.Server{}),
	}

	go server.Serve(ln)

	return server
}

func startHTTP2ReverseProxy(t *testing.T) *http.Server {
	rpURL, err := url.Parse(fmt.Sprintf("http://localhost:%d", http2Port))
	require.NoError(t, err)

	proxy := httputil.NewSingleHostReverseProxy(rpURL)
	proxy.Transport = &http2.Transport{
		AllowHTTP: true,
		DialTLS: func(netw, addr string, _ *tls.Config) (net.Conn, error) {
			ta, err := net.ResolveTCPAddr(netw, addr)
			if err != nil {
				return nil, err
			}
			return net.DialTCP(netw, nil, ta)
		},
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", proxyPort))
	require.NoError(t, err)

	proxyServer := &http.Server{
		Handler: proxy,
	}

	go proxyServer.Serve(ln)

	return proxyServer
}

func getResponseBody(t *testing.T, resp *http.Response) string {
	b, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(b)
}
