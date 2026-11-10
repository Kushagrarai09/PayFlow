package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Claims struct { UserID string `json:"sub"`; Email string `json:"email"`; Expires int64 `json:"exp"` }
func secret() []byte { s:=os.Getenv("JWT_SECRET"); if s=="" { s="unsafe-development-secret-change-me" }; return []byte(s) }
func sign(data string) string { m:=hmac.New(sha256.New,secret()); m.Write([]byte(data)); return base64.RawURLEncoding.EncodeToString(m.Sum(nil)) }
func makeToken(userID,email string) string {
	h:=base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	p,_:=json.Marshal(Claims{UserID:userID,Email:email,Expires:time.Now().Add(24*time.Hour).Unix()})
	body:=h+"."+base64.RawURLEncoding.EncodeToString(p)
	return body+"."+sign(body)
}
func auth(r *http.Request) (Claims,error) {
	var c Claims
	h:=strings.TrimPrefix(r.Header.Get("Authorization"),"Bearer ")
	parts:=strings.Split(h,".")
	if len(parts)!=3 { return c,errors.New("missing bearer token") }
	body:=parts[0]+"."+parts[1]
	if !hmac.Equal([]byte(parts[2]),[]byte(sign(body))) { return c,errors.New("invalid token") }
	b,err:=base64.RawURLEncoding.DecodeString(parts[1]); if err!=nil{return c,err}
	if err=json.Unmarshal(b,&c); err!=nil{return c,err}
	if c.UserID=="" || c.Expires<time.Now().Unix(){return c,errors.New("expired token")}
	return c,nil
}
func db() (*pgxpool.Pool,error) {
	dsn:=os.Getenv("DATABASE_URL"); if dsn=="" { return nil,fmt.Errorf("DATABASE_URL is required") }
	p,err:=pgxpool.New(context.Background(),dsn); if err!=nil{return nil,err}
	if err=p.Ping(context.Background()); err!=nil {p.Close(); return nil,err}
	return p,nil
}
func jsonOut(w http.ResponseWriter,status int,v any){w.Header().Set("Content-Type","application/json");w.WriteHeader(status);_ = json.NewEncoder(w).Encode(v)}
func jsonIn(r *http.Request,v any) error { return json.NewDecoder(http.MaxBytesReader(nil,r.Body,1<<20)).Decode(v) }
func fail(w http.ResponseWriter,status int,msg string){jsonOut(w,status,map[string]string{"error":msg})}
func amountOK(n int64) bool { return n>0 && n<=1_000_000_000_000 }
