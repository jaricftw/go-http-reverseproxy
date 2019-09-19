package stream

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	http1Port      = 6666
	http1ProxyPort = 16666
)

func TestHTTP1(t *testing.T) {

	tests := []struct {
		msg                string
		port               int
		abortServerHandler bool
		wantErr            string
	}{
		{
			msg:  "direct",
			port: http1Port,
		},
		{
			msg:  "via proxy",
			port: http1ProxyPort,
		},
		{
			msg:                "direct, server panic",
			port:               http1Port,
			abortServerHandler: true,
			wantErr:            "EOF",
		},
		{
			msg:                "via proxy, server panic",
			port:               http1ProxyPort,
			abortServerHandler: true,
			wantErr:            "EOF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			s := startHTTPServer(t, tt.abortServerHandler)
			defer s.Close()

			req, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d", tt.port), nil)
			require.NoError(t, err)

			proxyServer := startHTTP1ReverseProxy(t)
			defer proxyServer.Close()

			resp, err := http.DefaultClient.Do(req)
			t.Logf("got err: %v", err)
			t.Logf("got resp: %v", resp)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Contains(t, getResponseBody(t, resp), "hello world HTTP/1.1")
			defer resp.Body.Close()
		})
	}
}

func startHTTPServer(t *testing.T, abortHandler bool) *http.Server {
	h1Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "hello world "+r.Proto)
		if abortHandler {
			panic(http.ErrAbortHandler)
		}
	})

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", http1Port))
	require.NoError(t, err)

	server := &http.Server{
		Handler: h1Handler,
	}

	go server.Serve(ln)

	return server
}

func startHTTP1ReverseProxy(t *testing.T) *http.Server {
	rpURL, err := url.Parse(fmt.Sprintf("http://localhost:%d", http1Port))
	require.NoError(t, err)

	proxy := httputil.NewSingleHostReverseProxy(rpURL)
	proxy.Transport = &http.Transport{}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", http1ProxyPort))
	require.NoError(t, err)

	proxyServer := &http.Server{
		Handler: proxy,
	}

	go proxyServer.Serve(ln)

	return proxyServer
}
