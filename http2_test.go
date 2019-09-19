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
	http2Port      = 7777
	http2ProxyPort = 17777
)

func TestHTTP2(t *testing.T) {

	tests := []struct {
		msg                string
		port               int
		abortServerHandler bool
		wantErr            string
	}{
		{
			msg:  "direct, success",
			port: http2Port,
		},
		{
			msg:  "via proxy, success",
			port: http2ProxyPort,
		},
		{
			msg:                "direct, server panic",
			port:               http2Port,
			abortServerHandler: true,
			wantErr:            "stream error",
		},
		{
			msg:                "via proxy, server panic",
			port:               http2ProxyPort,
			abortServerHandler: true,
			wantErr:            "stream error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
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

			proxyServer := startHTTP2ReverseProxy(t)
			defer proxyServer.Close()

			server := startHTTP2Server(t, tt.abortServerHandler)
			defer server.Close()

			req, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d", tt.port), nil)
			require.NoError(t, err)

			resp, err := client.Do(req)
			t.Logf("got err: %v", err)
			t.Logf("got resp: %v", resp)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Contains(t, getResponseBody(t, resp), "hello world HTTP/2.0")
			defer resp.Body.Close()
		})
	}
}

func startHTTP2Server(t *testing.T, abortHandler bool) *http.Server {
	h1Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "hello world "+r.Proto)
		if abortHandler {
			panic(http.ErrAbortHandler)
		}
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

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", http2ProxyPort))
	require.NoError(t, err)

	proxyServer := &http.Server{
		Handler: h2c.NewHandler(proxy, &http2.Server{}),
	}

	go proxyServer.Serve(ln)

	return proxyServer
}

func getResponseBody(t *testing.T, resp *http.Response) string {
	b, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(b)
}
