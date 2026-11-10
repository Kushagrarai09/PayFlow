package gateway

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

type route struct { prefix string; proxy *httputil.ReverseProxy }

func New(targets map[string]string) http.Handler {
	routes := make([]route, 0, len(targets))
	for prefix, raw := range targets {
		u, err := url.Parse(raw)
		if err != nil { panic(err) }
		p := httputil.NewSingleHostReverseProxy(u)
		original := p.Director
		p.Director = func(req *http.Request) {
			original(req)
			req.URL.Path = strings.TrimPrefix(req.URL.Path, prefix)
			if req.URL.Path == "" { req.URL.Path = "/" }
		}
		routes = append(routes, route{prefix:prefix, proxy:p})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.Header().Set("Content-Type","application/json")
			_, _ = w.Write([]byte(`{"status":"ok","service":"gateway"}`))
			return
		}
		best := -1
		var selected *httputil.ReverseProxy
		for _, candidate := range routes {
			prefix := candidate.prefix
			match := strings.HasSuffix(prefix, "/") && strings.HasPrefix(r.URL.Path, prefix) ||
				!strings.HasSuffix(prefix, "/") && (r.URL.Path == prefix || strings.HasPrefix(r.URL.Path, prefix+"/"))
			if match && len(prefix) > best { best, selected = len(prefix), candidate.proxy }
		}
		if selected == nil { http.NotFound(w,r); return }
		selected.ServeHTTP(w,r)
	})
}
