package payments

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

func parseRSAPrivateKey(value string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(value))
	if block == nil {
		return nil, errors.New("private key PEM is invalid")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("RSA private key is invalid")
}

func parseRSAPublicKey(value string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(value))
	if block == nil {
		return nil, errors.New("public key PEM is invalid")
	}
	if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
		if key, ok := cert.PublicKey.(*rsa.PublicKey); ok {
			return key, nil
		}
	}
	if key, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PublicKey); ok {
			return rsaKey, nil
		}
	}
	if key, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("RSA public key is invalid")
}

func (s *SQLService) createWechatOrder(ctx context.Context, order Order, config map[string]string) (providerOrder, error) {
	privateKey, err := parseRSAPrivateKey(config["private_key_pem"])
	if err != nil {
		return providerOrder{}, ErrProviderUnconfig
	}
	nonce := randomText(32)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	payload := map[string]any{"appid": config["app_id"], "mchid": config["mch_id"], "description": "AI Token Gateway balance recharge", "out_trade_no": order.MerchantOrderNo, "notify_url": config["notify_url"], "amount": map[string]any{"total": mustMinor(order.Amount, order.Currency), "currency": order.Currency}}
	encoded, _ := json.Marshal(payload)
	endpoint := strings.TrimRight(config["api_base_url"], "/")
	if endpoint == "" {
		endpoint = "https://api.mch.weixin.qq.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/v3/pay/transactions/native", bytes.NewReader(encoded))
	if err != nil {
		return providerOrder{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", wechatAuthorization("POST", "/v3/pay/transactions/native", timestamp, nonce, encoded, config["mch_id"], config["serial_no"], privateKey))
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return providerOrder{}, &ProviderError{Provider: ProviderWechat, Detail: err.Error()}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return providerOrder{}, &ProviderError{Provider: ProviderWechat, StatusCode: resp.StatusCode, Detail: providerErrorSummary(raw)}
	}
	var result struct {
		CodeURL string `json:"code_url"`
	}
	if json.Unmarshal(raw, &result) != nil || result.CodeURL == "" {
		return providerOrder{}, &ProviderError{Provider: ProviderWechat, StatusCode: resp.StatusCode, Detail: providerErrorSummary(raw)}
	}
	return providerOrder{QRCode: result.CodeURL, Raw: raw}, nil
}

func mustMinor(amount, currency string) int64 {
	value, _ := minorAmount(amount, currency)
	parsed, _ := strconv.ParseInt(value, 10, 64)
	return parsed
}

func wechatAuthorization(method, path, timestamp, nonce string, body []byte, mchID, serial string, key *rsa.PrivateKey) string {
	message := method + "\n" + path + "\n" + timestamp + "\n" + nonce + "\n" + string(body) + "\n"
	digest := sha256.Sum256([]byte(message))
	signature, _ := rsa.SignPKCS1v15(rand.Reader, key, cryptoHashSHA256, digest[:])
	return fmt.Sprintf(`WECHATPAY2-SHA256-RSA2048 mchid="%s",nonce_str="%s",timestamp="%s",serial_no="%s",signature="%s"`, mchID, nonce, timestamp, serial, base64.StdEncoding.EncodeToString(signature))
}

// cryptoHashSHA256 is kept as a package constant so the signing code remains
// obvious at call sites without introducing an SDK-specific dependency.
var cryptoHashSHA256 = crypto.SHA256

func verifyWechatWebhook(config map[string]string, headers http.Header, body []byte) (callbackPayment, error) {
	timestamp, nonce, signature := headers.Get("Wechatpay-Timestamp"), headers.Get("Wechatpay-Nonce"), headers.Get("Wechatpay-Signature")
	serial := headers.Get("Wechatpay-Serial")
	if timestamp == "" || nonce == "" || signature == "" || serial == "" {
		return callbackPayment{}, ErrCallbackUntrusted
	}
	if !strings.EqualFold(strings.TrimSpace(serial), strings.TrimSpace(config["platform_certificate_serial_no"])) {
		return callbackPayment{}, ErrCallbackUntrusted
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Since(time.Unix(seconds, 0)) > 5*time.Minute || time.Until(time.Unix(seconds, 0)) > 5*time.Minute {
		return callbackPayment{}, ErrCallbackUntrusted
	}
	publicKey, err := parseRSAPublicKey(config["platform_certificate_pem"])
	if err != nil {
		return callbackPayment{}, ErrProviderUnconfig
	}
	message := timestamp + "\n" + nonce + "\n" + string(body) + "\n"
	digest := sha256.Sum256([]byte(message))
	decoded, err := base64.StdEncoding.DecodeString(signature)
	if err != nil || rsa.VerifyPKCS1v15(publicKey, cryptoHashSHA256, digest[:], decoded) != nil {
		return callbackPayment{}, ErrCallbackUntrusted
	}
	var envelope struct {
		Resource struct {
			Algorithm      string `json:"algorithm"`
			Ciphertext     string `json:"ciphertext"`
			Nonce          string `json:"nonce"`
			AssociatedData string `json:"associated_data"`
		} `json:"resource"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return callbackPayment{}, ErrCallbackInvalid
	}
	if envelope.Resource.Algorithm != "AEAD_AES_256_GCM" {
		return callbackPayment{}, ErrCallbackUntrusted
	}
	plain, err := decryptWechat(envelope.Resource.Ciphertext, envelope.Resource.Nonce, envelope.Resource.AssociatedData, config["api_v3_key"])
	if err != nil {
		return callbackPayment{}, ErrCallbackUntrusted
	}
	var result struct {
		OutTradeNo    string `json:"out_trade_no"`
		TransactionID string `json:"transaction_id"`
		TradeState    string `json:"trade_state"`
		Amount        struct {
			Total    int64  `json:"total"`
			Currency string `json:"currency"`
		} `json:"amount"`
		AppID string `json:"appid"`
		MchID string `json:"mchid"`
	}
	if json.Unmarshal(plain, &result) != nil || result.OutTradeNo == "" || result.TransactionID == "" {
		return callbackPayment{}, ErrCallbackInvalid
	}
	if result.AppID != config["app_id"] || result.MchID != config["mch_id"] {
		return callbackPayment{}, ErrCallbackUntrusted
	}
	return callbackPayment{MerchantOrderNo: result.OutTradeNo, ProviderOrderID: result.TransactionID, Currency: result.Amount.Currency, Amount: minorToAmount(result.Amount.Total, result.Amount.Currency), Paid: result.TradeState == "SUCCESS"}, nil
}

func decryptWechat(ciphertext, nonce, associatedData, key string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, errors.New("wechat callback nonce has invalid length")
	}
	return gcm.Open(nil, []byte(nonce), raw, []byte(associatedData))
}

func (s *SQLService) createAlipayOrder(ctx context.Context, order Order, config map[string]string) (providerOrder, error) {
	privateKey, err := parseRSAPrivateKey(config["private_key_pem"])
	if err != nil {
		return providerOrder{}, ErrProviderUnconfig
	}
	gateway := config["gateway"]
	if gateway == "" {
		gateway = "https://openapi.alipay.com/gateway.do"
	}
	biz, _ := json.Marshal(map[string]any{"out_trade_no": order.MerchantOrderNo, "total_amount": order.Amount, "subject": "AI Token Gateway balance recharge", "product_code": "FACE_TO_FACE_PAYMENT"})
	params := map[string]string{"app_id": config["app_id"], "method": "alipay.trade.precreate", "format": "JSON", "charset": "utf-8", "sign_type": "RSA2", "timestamp": time.Now().Format("2006-01-02 15:04:05"), "version": "1.0", "biz_content": string(biz), "notify_url": config["notify_url"]}
	params["sign"] = alipaySign(params, privateKey)
	form := url.Values{}
	for key, value := range params {
		form.Set(key, value)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gateway, strings.NewReader(form.Encode()))
	if err != nil {
		return providerOrder{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return providerOrder{}, &ProviderError{Provider: ProviderAlipay, Detail: err.Error()}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return providerOrder{}, &ProviderError{Provider: ProviderAlipay, StatusCode: resp.StatusCode, Detail: providerErrorSummary(raw)}
	}
	var result struct {
		Response struct {
			Code   string `json:"code"`
			Msg    string `json:"msg"`
			QRCode string `json:"qr_code"`
		} `json:"alipay_trade_precreate_response"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Response.Code != "10000" || result.Response.QRCode == "" {
		return providerOrder{}, &ProviderError{Provider: ProviderAlipay, StatusCode: resp.StatusCode, Detail: providerErrorSummary(raw)}
	}
	return providerOrder{QRCode: result.Response.QRCode, Raw: raw}, nil
}

func alipaySign(params map[string]string, key *rsa.PrivateKey) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		if key != "sign" && key != "sign_type" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+params[key])
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "&")))
	signature, _ := rsa.SignPKCS1v15(rand.Reader, key, cryptoHashSHA256, digest[:])
	return base64.StdEncoding.EncodeToString(signature)
}

func verifyAlipayWebhook(config map[string]string, body []byte) (callbackPayment, error) {
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return callbackPayment{}, ErrCallbackInvalid
	}
	signature := values.Get("sign")
	if signature == "" {
		return callbackPayment{}, ErrCallbackUntrusted
	}
	if values.Get("sign_type") != "RSA2" || values.Get("app_id") != config["app_id"] {
		return callbackPayment{}, ErrCallbackUntrusted
	}
	if expectedSeller := strings.TrimSpace(config["seller_id"]); expectedSeller != "" && values.Get("seller_id") != expectedSeller {
		return callbackPayment{}, ErrCallbackUntrusted
	}
	publicKey, err := parseRSAPublicKey(config["alipay_public_key_pem"])
	if err != nil {
		return callbackPayment{}, ErrProviderUnconfig
	}
	params := map[string]string{}
	for key, list := range values {
		if key != "sign" && key != "sign_type" && len(list) > 0 {
			params[key] = list[0]
		}
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+params[key])
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "&")))
	decoded, err := base64.StdEncoding.DecodeString(signature)
	if err != nil || rsa.VerifyPKCS1v15(publicKey, cryptoHashSHA256, digest[:], decoded) != nil {
		return callbackPayment{}, ErrCallbackUntrusted
	}
	if values.Get("out_trade_no") == "" || values.Get("trade_no") == "" || values.Get("total_amount") == "" {
		return callbackPayment{}, ErrCallbackInvalid
	}
	return callbackPayment{MerchantOrderNo: values.Get("out_trade_no"), ProviderOrderID: values.Get("trade_no"), Amount: values.Get("total_amount"), Paid: values.Get("trade_status") == "TRADE_SUCCESS"}, nil
}

func randomText(length int) string {
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw)[:length]
}

var _ = bytes.Equal
