package gateway

import (
 "net/http"
 "net/http/httptest"
 "testing"
)

func TestGatewayHealth(t *testing.T) {
 h:=New(map[string]string{}); r:=httptest.NewRequest(http.MethodGet,"/health",nil); w:=httptest.NewRecorder(); h.ServeHTTP(w,r)
 if w.Code!=http.StatusOK { t.Fatalf("expected 200, got %d",w.Code) }
}
func TestGatewayNotFound(t *testing.T) {
 h:=New(map[string]string{}); r:=httptest.NewRequest(http.MethodGet,"/missing",nil); w:=httptest.NewRecorder(); h.ServeHTTP(w,r)
 if w.Code!=http.StatusNotFound { t.Fatalf("expected 404, got %d",w.Code) }
}
