package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct { name string; pool *pgxpool.Pool; client *http.Client }

func New(name string) (http.Handler,error) {
	p,err:=db(); if err!=nil{return nil,err}
	a:=&App{name:name,pool:p,client:&http.Client{Timeout:5*time.Second}}
	if err=a.migrate(context.Background());err!=nil{return nil,err}
	mux:=http.NewServeMux()
	mux.HandleFunc("GET /health",func(w http.ResponseWriter,r *http.Request){jsonOut(w,200,map[string]string{"status":"ok","service":name})})
	switch name {
	case "auth": mux.HandleFunc("POST /register",a.register); mux.HandleFunc("POST /login",a.login)
	case "account": mux.HandleFunc("POST /accounts",a.createAccount); mux.HandleFunc("GET /accounts",a.listAccounts); mux.HandleFunc("GET /accounts/{id}",a.getAccount); mux.HandleFunc("POST /internal/adjust",a.adjust)
	case "ledger": mux.HandleFunc("POST /internal/journals",a.createJournal); mux.HandleFunc("GET /ledger/{account_id}",a.getLedger)
	case "transaction": mux.HandleFunc("POST /transactions/deposit",a.deposit); mux.HandleFunc("POST /transactions/withdraw",a.withdraw); mux.HandleFunc("POST /transactions/transfer",a.transfer); mux.HandleFunc("GET /transactions",a.listTransactions)
	default: return nil,fmt.Errorf("unknown SERVICE %q",name)
	}
	return cors(mux),nil
}
func cors(next http.Handler)http.Handler{return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.Header().Set("Access-Control-Allow-Origin","*");w.Header().Set("Access-Control-Allow-Headers","Authorization,Content-Type,Idempotency-Key");w.Header().Set("Access-Control-Allow-Methods","GET,POST,OPTIONS");if r.Method=="OPTIONS"{w.WriteHeader(204);return};next.ServeHTTP(w,r)})}
func(a *App)migrate(ctx context.Context)error{
	var q string
	switch a.name {
	case "auth": q=`CREATE EXTENSION IF NOT EXISTS pgcrypto; CREATE TABLE IF NOT EXISTS users(id UUID PRIMARY KEY DEFAULT gen_random_uuid(),name TEXT NOT NULL,email TEXT UNIQUE NOT NULL,password_hash TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT now());`
	case "account": q=`CREATE EXTENSION IF NOT EXISTS pgcrypto; CREATE TABLE IF NOT EXISTS accounts(id UUID PRIMARY KEY DEFAULT gen_random_uuid(),user_id UUID NOT NULL,currency CHAR(3) NOT NULL DEFAULT 'INR',balance BIGINT NOT NULL DEFAULT 0 CHECK(balance>=0),status TEXT NOT NULL DEFAULT 'active',created_at TIMESTAMPTZ NOT NULL DEFAULT now()); CREATE TABLE IF NOT EXISTS adjustments(id UUID PRIMARY KEY DEFAULT gen_random_uuid(),idempotency_key TEXT UNIQUE NOT NULL,account_id UUID NOT NULL,delta BIGINT NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT now());`
	case "ledger": q=`CREATE EXTENSION IF NOT EXISTS pgcrypto; CREATE TABLE IF NOT EXISTS journals(id UUID PRIMARY KEY DEFAULT gen_random_uuid(),reference TEXT UNIQUE NOT NULL,description TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT now()); CREATE TABLE IF NOT EXISTS ledger_entries(id UUID PRIMARY KEY DEFAULT gen_random_uuid(),journal_id UUID NOT NULL REFERENCES journals(id),account_id UUID NOT NULL,side TEXT NOT NULL CHECK(side IN ('debit','credit')),amount BIGINT NOT NULL CHECK(amount>0),created_at TIMESTAMPTZ NOT NULL DEFAULT now());`
	case "transaction": q=`CREATE EXTENSION IF NOT EXISTS pgcrypto; CREATE TABLE IF NOT EXISTS transactions(id UUID PRIMARY KEY DEFAULT gen_random_uuid(),idempotency_key TEXT UNIQUE NOT NULL,owner_id UUID NOT NULL,kind TEXT NOT NULL,from_account UUID,to_account UUID,amount BIGINT NOT NULL,status TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT now());`
	}
	_,err:=a.pool.Exec(ctx,q);return err
}
func(a *App)register(w http.ResponseWriter,r *http.Request){
	var in struct{Name string `json:"name"`;Email string `json:"email"`;Password string `json:"password"`}
	if jsonIn(r,&in)!=nil||len(in.Password)<8||in.Email==""{fail(w,400,"name, valid email and password (8+ characters) are required");return}
	var hash string;err:=a.pool.QueryRow(r.Context(),`SELECT crypt($1, gen_salt('bf'))`,in.Password).Scan(&hash);if err!=nil{fail(w,500,"password hashing failed");return}
	var id string;err=a.pool.QueryRow(r.Context(),`INSERT INTO users(name,email,password_hash) VALUES($1,$2,$3) RETURNING id`,in.Name,strings.ToLower(in.Email),hash).Scan(&id)
	if err!=nil{fail(w,409,"email may already be registered");return};jsonOut(w,201,map[string]any{"id":id,"name":in.Name,"email":strings.ToLower(in.Email)})
}
func(a *App)login(w http.ResponseWriter,r *http.Request){
	var in struct{Email string `json:"email"`;Password string `json:"password"`};if jsonIn(r,&in)!=nil{fail(w,400,"invalid request");return}
	var id,name,email string;var valid bool;err:=a.pool.QueryRow(r.Context(),`SELECT id,name,email,(password_hash=crypt($2,password_hash)) FROM users WHERE email=$1`,strings.ToLower(in.Email),in.Password).Scan(&id,&name,&email,&valid)
	if err!=nil||!valid{fail(w,401,"invalid email or password");return}
	jsonOut(w,200,map[string]any{"access_token":makeToken(id,email),"token_type":"Bearer","user":map[string]string{"id":id,"name":name,"email":email}})
}
func(a *App)createAccount(w http.ResponseWriter,r *http.Request){
	c,e:=auth(r);if e!=nil{fail(w,401,e.Error());return};var in struct{Currency string `json:"currency"`};_ = jsonIn(r,&in);if in.Currency==""{in.Currency="INR"};if len(in.Currency)!=3{fail(w,400,"currency must be a 3-letter code");return}
	var id string;e=a.pool.QueryRow(r.Context(),`INSERT INTO accounts(user_id,currency) VALUES($1,$2) RETURNING id`,c.UserID,strings.ToUpper(in.Currency)).Scan(&id);if e!=nil{fail(w,500,"could not create account");return};jsonOut(w,201,map[string]any{"id":id,"currency":strings.ToUpper(in.Currency),"balance":0,"status":"active"})
}
func(a *App)listAccounts(w http.ResponseWriter,r *http.Request){c,e:=auth(r);if e!=nil{fail(w,401,e.Error());return};rows,e:=a.pool.Query(r.Context(),`SELECT id,currency,balance,status,created_at FROM accounts WHERE user_id=$1 ORDER BY created_at DESC`,c.UserID);if e!=nil{fail(w,500,"query failed");return};defer rows.Close();out:=[]map[string]any{};for rows.Next(){var id,cur,status string;var bal int64;var at time.Time;if rows.Scan(&id,&cur,&bal,&status,&at)==nil{out=append(out,map[string]any{"id":id,"currency":strings.TrimSpace(cur),"balance":bal,"status":status,"created_at":at})}};jsonOut(w,200,out)}
func(a *App)getAccount(w http.ResponseWriter,r *http.Request){c,e:=auth(r);if e!=nil{fail(w,401,e.Error());return};var id,cur,status string;var bal int64;e=a.pool.QueryRow(r.Context(),`SELECT id,currency,balance,status FROM accounts WHERE id=$1 AND user_id=$2`,r.PathValue("id"),c.UserID).Scan(&id,&cur,&bal,&status);if e!=nil{fail(w,404,"account not found");return};jsonOut(w,200,map[string]any{"id":id,"currency":strings.TrimSpace(cur),"balance":bal,"status":status})}
func(a *App)adjust(w http.ResponseWriter,r *http.Request){
	var in struct{AccountID string `json:"account_id"`;Delta int64 `json:"delta"`;Key string `json:"idempotency_key"`;OwnerID string `json:"owner_id"`}
	if jsonIn(r,&in)!=nil||in.AccountID==""||in.Delta==0||in.Key==""{fail(w,400,"account_id, non-zero delta and idempotency_key required");return}
	tx,e:=a.pool.Begin(r.Context());if e!=nil{fail(w,500,"database error");return};defer tx.Rollback(r.Context())
	var prior string;e=tx.QueryRow(r.Context(),`SELECT id FROM adjustments WHERE idempotency_key=$1`,in.Key).Scan(&prior);if e==nil{jsonOut(w,200,map[string]any{"status":"already_applied"});return}
	var balance int64
	if in.Delta<0 {e=tx.QueryRow(r.Context(),`SELECT balance FROM accounts WHERE id=$1 AND user_id=$2 AND status='active' FOR UPDATE`,in.AccountID,in.OwnerID).Scan(&balance)} else {e=tx.QueryRow(r.Context(),`SELECT balance FROM accounts WHERE id=$1 AND status='active' FOR UPDATE`,in.AccountID).Scan(&balance)}
	if e!=nil{fail(w,404,"active account not found");return};if balance+in.Delta<0{fail(w,409,"insufficient funds");return}
	_,e=tx.Exec(r.Context(),`UPDATE accounts SET balance=balance+$1 WHERE id=$2`,in.Delta,in.AccountID);if e!=nil{fail(w,500,"balance update failed");return}
	_,e=tx.Exec(r.Context(),`INSERT INTO adjustments(idempotency_key,account_id,delta) VALUES($1,$2,$3)`,in.Key,in.AccountID,in.Delta);if e!=nil{fail(w,409,"adjustment key conflict");return}
	if e=tx.Commit(r.Context());e!=nil{fail(w,500,"commit failed");return};jsonOut(w,200,map[string]any{"status":"applied","balance":balance+in.Delta})
}
func(a *App)createJournal(w http.ResponseWriter,r *http.Request){
	var in struct{Reference string `json:"reference"`;Description string `json:"description"`;Entries []struct{AccountID string `json:"account_id"`;Side string `json:"side"`;Amount int64 `json:"amount"`} `json:"entries"`}
	if jsonIn(r,&in)!=nil||in.Reference==""||len(in.Entries)<2{fail(w,400,"reference and at least two entries required");return}
	var deb,cred int64;for _,x:=range in.Entries{if x.Amount<=0||(x.Side!="debit"&&x.Side!="credit"){fail(w,400,"invalid ledger entry");return};if x.Side=="debit"{deb+=x.Amount}else{cred+=x.Amount}}
	if deb!=cred{fail(w,422,"unbalanced journal: debits must equal credits");return}
	tx,e:=a.pool.Begin(r.Context());if e!=nil{fail(w,500,"database error");return};defer tx.Rollback(r.Context())
	var jid string;e=tx.QueryRow(r.Context(),`INSERT INTO journals(reference,description) VALUES($1,$2) RETURNING id`,in.Reference,in.Description).Scan(&jid);if e!=nil{fail(w,409,"journal reference already exists");return}
	for _,x:=range in.Entries{_,e=tx.Exec(r.Context(),`INSERT INTO ledger_entries(journal_id,account_id,side,amount) VALUES($1,$2,$3,$4)`,jid,x.AccountID,x.Side,x.Amount);if e!=nil{fail(w,500,"ledger insert failed");return}}
	if e=tx.Commit(r.Context());e!=nil{fail(w,500,"ledger commit failed");return};jsonOut(w,201,map[string]any{"journal_id":jid,"debits":deb,"credits":cred,"status":"posted"})
}
func(a *App)getLedger(w http.ResponseWriter,r *http.Request){c,e:=auth(r);if e!=nil{fail(w,401,e.Error());return};_ = c
	rows,e:=a.pool.Query(r.Context(),`SELECT j.reference,j.description,e.side,e.amount,e.created_at FROM ledger_entries e JOIN journals j ON j.id=e.journal_id WHERE e.account_id=$1 ORDER BY e.created_at DESC LIMIT 100`,r.PathValue("account_id"));if e!=nil{fail(w,500,"query failed");return};defer rows.Close();out:=[]map[string]any{};for rows.Next(){var ref,desc,side string;var amount int64;var at time.Time;if rows.Scan(&ref,&desc,&side,&amount,&at)==nil{out=append(out,map[string]any{"reference":ref,"description":desc,"side":side,"amount":amount,"created_at":at})}};jsonOut(w,200,out)
}
func(a *App)recordTx(ctx context.Context,key,owner,kind,from,to string,amount int64,status string)error{_,e:=a.pool.Exec(ctx,`INSERT INTO transactions(idempotency_key,owner_id,kind,from_account,to_account,amount,status) VALUES($1,$2,$3,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,$6,$7)`,key,owner,kind,from,to,amount,status);return e}
func(a *App)listTransactions(w http.ResponseWriter,r *http.Request){c,e:=auth(r);if e!=nil{fail(w,401,e.Error());return};rows,e:=a.pool.Query(r.Context(),`SELECT id,kind,from_account,to_account,amount,status,created_at FROM transactions WHERE owner_id=$1 ORDER BY created_at DESC LIMIT 100`,c.UserID);if e!=nil{fail(w,500,"query failed");return};defer rows.Close();out:=[]map[string]any{};for rows.Next(){var id,kind,status string;var from,to *string;var amount int64;var at time.Time;if rows.Scan(&id,&kind,&from,&to,&amount,&status,&at)==nil{out=append(out,map[string]any{"id":id,"kind":kind,"from_account":from,"to_account":to,"amount":amount,"status":status,"created_at":at})}};jsonOut(w,200,out)}
func(a *App)deposit(w http.ResponseWriter,r *http.Request){a.moneyOperation(w,r,"deposit")}
func(a *App)withdraw(w http.ResponseWriter,r *http.Request){a.moneyOperation(w,r,"withdraw")}
func(a *App)transfer(w http.ResponseWriter,r *http.Request){a.moneyOperation(w,r,"transfer")}
func(a *App)moneyOperation(w http.ResponseWriter,r *http.Request,kind string){
	c,e:=auth(r);if e!=nil{fail(w,401,e.Error());return}
	var in struct{AccountID string `json:"account_id"`;FromAccount string `json:"from_account_id"`;ToAccount string `json:"to_account_id"`;Amount int64 `json:"amount"`};if jsonIn(r,&in)!=nil||!amountOK(in.Amount){fail(w,400,"amount must be positive integer minor units");return}
	key:=r.Header.Get("Idempotency-Key");if key==""{fail(w,400,"Idempotency-Key header required");return}
	from,to:=in.AccountID,"";if kind=="transfer"{from,to=in.FromAccount,in.ToAccount};if kind=="withdraw"{from=in.AccountID};if kind=="deposit"{to=in.AccountID}
	if (kind=="transfer"&&(from==""||to==""||from==to))||((kind!="transfer")&&in.AccountID==""){fail(w,400,"valid account id(s) required");return}
	var existing string;e=a.pool.QueryRow(r.Context(),`SELECT id FROM transactions WHERE idempotency_key=$1`,key).Scan(&existing);if e==nil{jsonOut(w,200,map[string]any{"status":"already_processed","transaction_id":existing});return}
	if kind=="deposit"{e=a.adjust(r.Context(),to,in.Amount,key+"-credit",c.UserID)}else{
		e=a.adjust(r.Context(),from,-in.Amount,key+"-debit",c.UserID);if e!=nil{fail(w,409,e.Error());return}
		if kind=="transfer"{e=a.adjust(r.Context(),to,in.Amount,key+"-credit",c.UserID);if e!=nil{_ = a.adjust(r.Context(),from,in.Amount,key+"-compensate",c.UserID);fail(w,409,"recipient credit failed; sender debit compensated");return}}
	}
	if e!=nil{fail(w,409,e.Error());return}
	entries:=[]map[string]any{}
	if kind=="deposit"{entries=[]map[string]any{{"account_id":to,"side":"debit","amount":in.Amount},{"account_id":to,"side":"credit","amount":in.Amount}}}else if kind=="withdraw"{entries=[]map[string]any{{"account_id":from,"side":"debit","amount":in.Amount},{"account_id":from,"side":"credit","amount":in.Amount}}}else{entries=[]map[string]any{{"account_id":from,"side":"debit","amount":in.Amount},{"account_id":to,"side":"credit","amount":in.Amount}}}
	if e=a.postJournal(r.Context(),key,kind,entries);e!=nil{
		if kind=="deposit"{_ = a.adjust(r.Context(),to,-in.Amount,key+"-compensate",c.UserID)}else if kind=="withdraw"{_ = a.adjust(r.Context(),from,in.Amount,key+"-compensate",c.UserID)}else{_ = a.adjust(r.Context(),to,-in.Amount,key+"-compensate",c.UserID);_ = a.adjust(r.Context(),from,in.Amount,key+"-compensate",c.UserID)}
		fail(w,502,"ledger unavailable; compensation attempted");return
	}
	e=a.recordTx(r.Context(),key,c.UserID,kind,from,to,in.Amount,"completed");if e!=nil{fail(w,409,"transaction applied but record could not be saved; reconcile before retry");return}
	jsonOut(w,201,map[string]any{"status":"completed","kind":kind,"amount":in.Amount,"from_account_id":from,"to_account_id":to,"idempotency_key":key})
}
func(a *App)adjust(ctx context.Context,account string,delta int64,key,ownerID string)error{
	body,_:=json.Marshal(map[string]any{"account_id":account,"delta":delta,"idempotency_key":key,"owner_id":ownerID})
	req,_:=http.NewRequestWithContext(ctx,"POST",envURL("ACCOUNT_URL","http://localhost:8082")+"/internal/adjust",strings.NewReader(string(body)));req.Header.Set("Content-Type","application/json")
	resp,e:=a.client.Do(req);if e!=nil{return e};defer resp.Body.Close();if resp.StatusCode>=300{return fmt.Errorf("account adjustment rejected (%d)",resp.StatusCode)};return nil
}
func(a *App)postJournal(ctx context.Context,ref,desc string,entries []map[string]any)error{
	body,_:=json.Marshal(map[string]any{"reference":ref,"description":desc,"entries":entries})
	req,_:=http.NewRequestWithContext(ctx,"POST",envURL("LEDGER_URL","http://localhost:8083")+"/internal/journals",strings.NewReader(string(body)));req.Header.Set("Content-Type","application/json")
	resp,e:=a.client.Do(req);if e!=nil{return e};defer resp.Body.Close();if resp.StatusCode>=300{return fmt.Errorf("journal rejected (%d)",resp.StatusCode)};return nil
}
func envURL(k,d string)string{if v:=strings.TrimRight(os.Getenv(k),"/");v!=""{return v};return d}
