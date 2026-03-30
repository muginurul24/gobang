package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type server struct {
	mu sync.Mutex

	agentCode  string
	agentToken string
	client     string
	clientKey  string
	globalUUID string
	nexusDelay atomic.Int64
	qrisDelay  atomic.Int64

	users         map[string]float64
	transfers     map[string]transferRecord
	payments      map[string]paymentRecord
	disbursements map[string]disbursementRecord
	inquiries     map[int64]inquiryRecord

	nextPaymentID      atomic.Int64
	nextPartnerRefNo   atomic.Int64
	nextInquiryID      atomic.Int64
	nextVendorRefNo    atomic.Int64
	nextTransferStatus atomic.Int64
}

type transferRecord struct {
	UserCode    string
	Amount      float64
	Type        string
	UserBalance float64
}

type paymentRecord struct {
	Username   string
	Amount     int64
	Status     string
	CustomRef  string
	CreatedAt  time.Time
	FinishedAt *time.Time
}

type inquiryRecord struct {
	Amount        int64
	BankCode      string
	AccountNumber string
	Fee           int64
	PartnerRefNo  string
}

type disbursementRecord struct {
	PartnerRefNo string
	Amount       int64
	Fee          int64
	Status       string
}

func main() {
	defaultDelay := envDuration("MOCK_UPSTREAM_DELAY", 0)
	srv := &server{
		agentCode:     envString("MOCK_NEXUSGGR_AGENT_CODE", "demo-agent"),
		agentToken:    envString("MOCK_NEXUSGGR_AGENT_TOKEN", "demo-token"),
		client:        envString("MOCK_QRIS_CLIENT", "demo-client"),
		clientKey:     envString("MOCK_QRIS_CLIENT_KEY", "demo-key"),
		globalUUID:    envString("MOCK_QRIS_GLOBAL_UUID", "demo-uuid"),
		users:         map[string]float64{},
		transfers:     map[string]transferRecord{},
		payments:      map[string]paymentRecord{},
		disbursements: map[string]disbursementRecord{},
		inquiries:     map[int64]inquiryRecord{},
	}
	srv.nextPaymentID.Store(1000)
	srv.nextPartnerRefNo.Store(9000)
	srv.nextInquiryID.Store(2949800)
	srv.nextVendorRefNo.Store(7000)
	srv.nexusDelay.Store(defaultDelay.Milliseconds())
	srv.qrisDelay.Store(defaultDelay.Milliseconds())

	mux := http.NewServeMux()
	mux.HandleFunc("POST /", srv.handleNexus)
	mux.HandleFunc("POST /api/generate", srv.handleGenerate)
	mux.HandleFunc("POST /api/checkstatus/v2/{trxID}", srv.handleCheckStatus)
	mux.HandleFunc("POST /api/inquiry", srv.handleInquiry)
	mux.HandleFunc("POST /api/transfer", srv.handleTransfer)
	mux.HandleFunc("POST /api/disbursement/check-status/{partnerRefNo}", srv.handleDisbursementStatus)
	mux.HandleFunc("GET /_admin/state", srv.handleAdminState)
	mux.HandleFunc("POST /_admin/nexus/delay", srv.handleAdminNexusDelay)
	mux.HandleFunc("POST /_admin/qris/delay", srv.handleAdminQRISDelay)
	mux.HandleFunc("POST /_admin/qris/payments/{trxID}", srv.handleAdminPaymentStatus)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})

	address := envString("MOCK_UPSTREAM_ADDRESS", ":18081")
	log.Printf("mock upstream listening on %s", address)
	if err := http.ListenAndServe(address, mux); err != nil {
		log.Fatal(err)
	}
}

func (s *server) handleNexus(w http.ResponseWriter, r *http.Request) {
	s.sleepNexus()

	var request struct {
		Method       string      `json:"method"`
		AgentCode    string      `json:"agent_code"`
		AgentToken   string      `json:"agent_token"`
		UserCode     string      `json:"user_code"`
		ProviderCode string      `json:"provider_code"`
		GameCode     string      `json:"game_code"`
		Lang         string      `json:"lang"`
		AgentSign    string      `json:"agent_sign"`
		Amount       json.Number `json:"amount"`
		AllUsers     bool        `json:"all_users"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": 0, "msg": "INVALID_REQUEST"})
		return
	}

	if request.AgentCode != s.agentCode || request.AgentToken != s.agentToken {
		writeJSON(w, http.StatusOK, map[string]any{"status": 0, "msg": "Invalid Agent."})
		return
	}

	switch strings.TrimSpace(request.Method) {
	case "provider_list":
		writeJSON(w, http.StatusOK, map[string]any{
			"status": 1,
			"msg":    "SUCCESS",
			"providers": []map[string]any{
				{"code": "PRAGMATIC", "name": "Pragmatic Play", "status": 1},
				{"code": "HACKSAW", "name": "Hacksaw Gaming", "status": 1},
			},
		})
	case "game_list":
		games := []map[string]any{}
		switch strings.TrimSpace(request.ProviderCode) {
		case "PRAGMATIC":
			games = append(games, map[string]any{
				"game_code": "vs20doghouse",
				"game_name": "The Dog House",
				"banner":    "https://mock.local/banner/pragmatic/vs20doghouse.png",
				"status":    1,
			})
		case "HACKSAW":
			games = append(games, map[string]any{
				"game_code": "wanteddead",
				"game_name": "Wanted Dead or a Wild",
				"banner":    "https://mock.local/banner/hacksaw/wanteddead.png",
				"status":    1,
			})
		default:
			writeJSON(w, http.StatusOK, map[string]any{"status": 0, "msg": "Invalid Provider"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status": 1,
			"msg":    "SUCCESS",
			"games":  games,
		})
	case "game_launch":
		if strings.TrimSpace(request.UserCode) == "" || strings.TrimSpace(request.ProviderCode) == "" || strings.TrimSpace(request.Lang) == "" {
			writeJSON(w, http.StatusOK, map[string]any{"status": 0, "msg": "INVALID_PARAMETER"})
			return
		}
		s.ensureUser(strings.TrimSpace(request.UserCode))
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     1,
			"msg":        "SUCCESS",
			"launch_url": "https://mock.nexusggr.local/launch/" + strings.TrimSpace(request.ProviderCode) + "/" + strings.TrimSpace(request.GameCode),
		})
	case "money_info":
		if request.AllUsers {
			s.mu.Lock()
			users := make([]map[string]any, 0, len(s.users))
			for code, balance := range s.users {
				users = append(users, map[string]any{
					"user_code": code,
					"balance":   balance,
				})
			}
			s.mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{
				"status":    1,
				"msg":       "SUCCESS",
				"agent":     map[string]any{"agent_code": s.agentCode, "balance": 1000000},
				"user_list": users,
			})
			return
		}

		userCode := strings.TrimSpace(request.UserCode)
		if userCode == "" {
			writeJSON(w, http.StatusOK, map[string]any{"status": 0, "msg": "INVALID_PARAMETER"})
			return
		}
		balance := s.ensureUser(userCode)
		writeJSON(w, http.StatusOK, map[string]any{
			"status": 1,
			"msg":    "SUCCESS",
			"agent":  map[string]any{"agent_code": s.agentCode, "balance": 1000000},
			"user":   map[string]any{"user_code": userCode, "balance": balance},
		})
	case "user_create":
		userCode := strings.TrimSpace(request.UserCode)
		if userCode == "" {
			writeJSON(w, http.StatusOK, map[string]any{"status": 0, "msg": "INVALID_PARAMETER"})
			return
		}
		balance := s.ensureUser(userCode)
		writeJSON(w, http.StatusOK, map[string]any{
			"status":       1,
			"msg":          "SUCCESS",
			"user_code":    userCode,
			"user_balance": balance,
		})
	case "user_deposit":
		userCode := strings.TrimSpace(request.UserCode)
		agentSign := strings.TrimSpace(request.AgentSign)
		amount, err := request.Amount.Float64()
		if userCode == "" || agentSign == "" || err != nil || amount <= 0 {
			writeJSON(w, http.StatusOK, map[string]any{"status": 0, "msg": "INVALID_PARAMETER"})
			return
		}
		userBalance := s.adjustUserBalance(userCode, amount)
		s.storeTransfer(agentSign, transferRecord{
			UserCode:    userCode,
			Amount:      amount,
			Type:        "user_deposit",
			UserBalance: userBalance,
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"status":        1,
			"msg":           "SUCCESS",
			"agent_balance": 1000000 - amount,
			"user_balance":  userBalance,
		})
	case "user_withdraw":
		userCode := strings.TrimSpace(request.UserCode)
		agentSign := strings.TrimSpace(request.AgentSign)
		amount, err := request.Amount.Float64()
		if userCode == "" || agentSign == "" || err != nil || amount <= 0 {
			writeJSON(w, http.StatusOK, map[string]any{"status": 0, "msg": "INVALID_PARAMETER"})
			return
		}

		s.mu.Lock()
		current := s.users[userCode]
		if current < amount {
			s.mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{"status": 0, "msg": "Insufficient Balance"})
			return
		}
		current -= amount
		s.users[userCode] = current
		s.transfers[agentSign] = transferRecord{
			UserCode:    userCode,
			Amount:      amount,
			Type:        "user_withdraw",
			UserBalance: current,
		}
		s.mu.Unlock()

		writeJSON(w, http.StatusOK, map[string]any{
			"status":        1,
			"msg":           "SUCCESS",
			"agent_balance": 1000000 + amount,
			"user_balance":  current,
		})
	case "user_withdraw_reset":
		userCode := strings.TrimSpace(request.UserCode)
		if userCode == "" && !request.AllUsers {
			writeJSON(w, http.StatusOK, map[string]any{"status": 0, "msg": "INVALID_PARAMETER"})
			return
		}

		if request.AllUsers {
			s.mu.Lock()
			users := make([]map[string]any, 0, len(s.users))
			for code, balance := range s.users {
				users = append(users, map[string]any{
					"user_code":       code,
					"withdraw_amount": 0,
					"balance":         balance,
				})
			}
			s.mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{
				"status":    1,
				"msg":       "SUCCESS",
				"agent":     map[string]any{"agent_code": s.agentCode, "balance": 1000000},
				"user_list": users,
			})
			return
		}

		balance := s.ensureUser(userCode)
		writeJSON(w, http.StatusOK, map[string]any{
			"status": 1,
			"msg":    "SUCCESS",
			"agent":  map[string]any{"agent_code": s.agentCode, "balance": 1000000},
			"user": map[string]any{
				"user_code":       userCode,
				"withdraw_amount": 0,
				"balance":         balance,
			},
		})
	case "transfer_status":
		userCode := strings.TrimSpace(request.UserCode)
		agentSign := strings.TrimSpace(request.AgentSign)
		if userCode == "" || agentSign == "" {
			writeJSON(w, http.StatusOK, map[string]any{"status": 0, "msg": "INVALID_PARAMETER"})
			return
		}
		s.mu.Lock()
		record, ok := s.transfers[agentSign]
		if !ok {
			s.mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{"status": 0, "msg": "TRANSACTION_NOT_FOUND"})
			return
		}
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"status":        1,
			"msg":           "SUCCESS",
			"amount":        record.Amount,
			"agent_balance": 1000000,
			"user_balance":  record.UserBalance,
			"type":          record.Type,
		})
	default:
		writeJSON(w, http.StatusOK, map[string]any{"status": 0, "msg": "METHOD_NOT_FOUND"})
	}
}

func (s *server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	s.sleepQRIS()

	var request struct {
		Username  string `json:"username"`
		Amount    int64  `json:"amount"`
		UUID      string `json:"uuid"`
		Expire    int    `json:"expire"`
		CustomRef string `json:"custom_ref"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": false, "error": "INVALID_REQUEST"})
		return
	}
	if strings.TrimSpace(request.Username) == "" || request.Amount <= 0 || strings.TrimSpace(request.UUID) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"status": false, "error": "INVALID_REQUEST"})
		return
	}

	id := s.nextPaymentID.Add(1)
	trxID := "mock-qris-" + strconv.FormatInt(id, 10)
	expireSeconds := request.Expire
	if expireSeconds <= 0 {
		expireSeconds = 300
	}
	expiresAt := time.Now().UTC().Add(time.Duration(expireSeconds) * time.Second)

	s.mu.Lock()
	s.payments[trxID] = paymentRecord{
		Username:  strings.TrimSpace(request.Username),
		Amount:    request.Amount,
		Status:    "pending",
		CustomRef: strings.TrimSpace(request.CustomRef),
		CreatedAt: time.Now().UTC(),
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"status":     true,
		"data":       "00020101021226MOCK" + strconv.FormatInt(id, 10),
		"trx_id":     trxID,
		"expired_at": expiresAt.Unix(),
	})
}

func (s *server) handleCheckStatus(w http.ResponseWriter, r *http.Request) {
	s.sleepQRIS()

	trxID := strings.TrimSpace(r.PathValue("trxID"))
	if trxID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"status": false, "error": "INVALID_REQUEST"})
		return
	}

	s.mu.Lock()
	record, ok := s.payments[trxID]
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"status": false, "error": "Transaction not found"})
		return
	}

	payload := map[string]any{
		"amount":      record.Amount,
		"merchant_id": s.globalUUID,
		"trx_id":      trxID,
		"status":      record.Status,
		"created_at":  record.CreatedAt.Format(time.RFC3339),
		"custom_ref":  record.CustomRef,
		"vendor":      "mock-qris",
	}
	if record.FinishedAt != nil {
		payload["finish_at"] = record.FinishedAt.Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, payload)
}

func (s *server) handleInquiry(w http.ResponseWriter, r *http.Request) {
	s.sleepQRIS()

	var request struct {
		Client        string `json:"client"`
		ClientKey     string `json:"client_key"`
		UUID          string `json:"uuid"`
		Amount        int64  `json:"amount"`
		BankCode      string `json:"bank_code"`
		AccountNumber string `json:"account_number"`
		TransferType  int    `json:"type"`
		ClientRefID   string `json:"client_ref_id"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": false, "error": "INVALID_REQUEST"})
		return
	}
	if request.Client != s.client || request.ClientKey != s.clientKey || request.UUID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"status": false, "error": "Invalid client"})
		return
	}
	if request.Amount <= 0 || strings.TrimSpace(request.BankCode) == "" || strings.TrimSpace(request.AccountNumber) == "" || request.TransferType <= 0 {
		writeJSON(w, http.StatusOK, map[string]any{"status": false, "error": "INVALID_REQUEST"})
		return
	}

	inquiryID := s.nextInquiryID.Add(1)
	partnerRefNo := "partner-" + strconv.FormatInt(s.nextPartnerRefNo.Add(1), 10)
	vendorRefNo := "vendor-" + strconv.FormatInt(s.nextVendorRefNo.Add(1), 10)
	fee := int64(1800)

	s.mu.Lock()
	s.inquiries[inquiryID] = inquiryRecord{
		Amount:        request.Amount,
		BankCode:      strings.TrimSpace(request.BankCode),
		AccountNumber: strings.TrimSpace(request.AccountNumber),
		Fee:           fee,
		PartnerRefNo:  partnerRefNo,
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"status": true,
		"data": map[string]any{
			"account_number": request.AccountNumber,
			"account_name":   "DEMO ACCOUNT 9749",
			"bank_code":      request.BankCode,
			"bank_name":      bankName(request.BankCode),
			"partner_ref_no": partnerRefNo,
			"vendor_ref_no":  vendorRefNo,
			"amount":         request.Amount,
			"fee":            fee,
			"inquiry_id":     inquiryID,
		},
	})
}

func (s *server) handleTransfer(w http.ResponseWriter, r *http.Request) {
	s.sleepQRIS()

	var request struct {
		Client        string `json:"client"`
		ClientKey     string `json:"client_key"`
		UUID          string `json:"uuid"`
		Amount        int64  `json:"amount"`
		BankCode      string `json:"bank_code"`
		AccountNumber string `json:"account_number"`
		TransferType  int    `json:"type"`
		InquiryID     int64  `json:"inquiry_id"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": false, "error": "INVALID_REQUEST"})
		return
	}
	if request.Client != s.client || request.ClientKey != s.clientKey || request.UUID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"status": false, "error": "Invalid client"})
		return
	}
	if request.Amount <= 0 || request.InquiryID <= 0 {
		writeJSON(w, http.StatusOK, map[string]any{"status": false, "error": "INVALID_REQUEST"})
		return
	}

	s.mu.Lock()
	inquiry, ok := s.inquiries[request.InquiryID]
	if !ok {
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"status": false, "error": "Inquiry not found"})
		return
	}
	s.disbursements[inquiry.PartnerRefNo] = disbursementRecord{
		PartnerRefNo: inquiry.PartnerRefNo,
		Amount:       request.Amount,
		Fee:          inquiry.Fee,
		Status:       "success",
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{"status": true})
}

func (s *server) handleDisbursementStatus(w http.ResponseWriter, r *http.Request) {
	s.sleepQRIS()

	partnerRefNo := strings.TrimSpace(r.PathValue("partnerRefNo"))
	if partnerRefNo == "" {
		writeJSON(w, http.StatusOK, map[string]any{"status": false, "error": "INVALID_REQUEST"})
		return
	}

	s.mu.Lock()
	record, ok := s.disbursements[partnerRefNo]
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"status": false, "error": "Transaction not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"amount":         record.Amount,
		"fee":            record.Fee,
		"partner_ref_no": record.PartnerRefNo,
		"merchant_uuid":  s.globalUUID,
		"status":         record.Status,
	})
}

func (s *server) ensureUser(userCode string) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	balance, ok := s.users[userCode]
	if !ok {
		s.users[userCode] = 0
		return 0
	}

	return balance
}

func (s *server) adjustUserBalance(userCode string, delta float64) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := s.users[userCode] + delta
	s.users[userCode] = next
	return next
}

func (s *server) storeTransfer(agentSign string, record transferRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transfers[agentSign] = record
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return errors.New("unexpected trailing json")
	}

	return nil
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func bankName(bankCode string) string {
	if strings.TrimSpace(bankCode) == "542" {
		return "PT. BANK ARTOS INDONESIA (Bank Jago)"
	}

	return "Mock Bank"
}

func envString(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func (s *server) sleepNexus() {
	delay := time.Duration(s.nexusDelay.Load()) * time.Millisecond
	if delay > 0 {
		time.Sleep(delay)
	}
}

func (s *server) sleepQRIS() {
	delay := time.Duration(s.qrisDelay.Load()) * time.Millisecond
	if delay > 0 {
		time.Sleep(delay)
	}
}

func (s *server) handleAdminState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"delays": map[string]any{
			"nexus_ms": s.nexusDelay.Load(),
			"qris_ms":  s.qrisDelay.Load(),
		},
	})
}

func (s *server) handleAdminNexusDelay(w http.ResponseWriter, r *http.Request) {
	s.handleAdminDelay(w, r, &s.nexusDelay)
}

func (s *server) handleAdminQRISDelay(w http.ResponseWriter, r *http.Request) {
	s.handleAdminDelay(w, r, &s.qrisDelay)
}

func (s *server) handleAdminDelay(w http.ResponseWriter, r *http.Request, target *atomic.Int64) {
	var request struct {
		Milliseconds int64 `json:"milliseconds"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": "invalid_request"})
		return
	}
	if request.Milliseconds < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": "invalid_delay"})
		return
	}

	target.Store(request.Milliseconds)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"milliseconds": request.Milliseconds,
	})
}

func (s *server) handleAdminPaymentStatus(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": "invalid_request"})
		return
	}

	trxID := strings.TrimSpace(r.PathValue("trxID"))
	status := strings.ToLower(strings.TrimSpace(request.Status))
	if trxID == "" || status == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": "invalid_request"})
		return
	}

	s.mu.Lock()
	record, ok := s.payments[trxID]
	if !ok {
		s.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]any{"status": "not_found"})
		return
	}

	record.Status = status
	now := time.Now().UTC()
	record.FinishedAt = &now
	s.payments[trxID] = record
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"trx_id":  trxID,
		"payment": record,
	})
}
