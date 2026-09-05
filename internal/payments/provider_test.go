package payments

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNormalizeAmountUsesCurrencyPrecision(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		currency string
		want     string
		wantErr  bool
	}{
		{name: "trim usd zeros", value: "10.00", currency: "USD", want: "10"},
		{name: "usd cents", value: "10.25", currency: "USD", want: "10.25"},
		{name: "reject usd fractions below cents", value: "10.001", currency: "USD", wantErr: true},
		{name: "zero decimal currency", value: "1000", currency: "JPY", want: "1000"},
		{name: "reject jpy fraction", value: "1000.1", currency: "JPY", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeAmount(test.value, test.currency)
			if test.wantErr {
				if !errors.Is(err, ErrInvalidRequest) {
					t.Fatalf("expected invalid request, got %v", err)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("normalizeAmount(%q, %q) = %q, %v; want %q", test.value, test.currency, got, err, test.want)
			}
		})
	}
}

func TestRechargeRateCalculatesCreditedAmount(t *testing.T) {
	got, err := applyRechargeRate("100", "0.98", "USD")
	if err != nil || got != "98" {
		t.Fatalf("applyRechargeRate() = %q, %v; want 98", got, err)
	}
	if _, err := normalizeRechargeRate("0"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("zero recharge rate must be rejected, got %v", err)
	}
	if _, err := applyRechargeRate("10", "1.0001", "USD"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("sub-cent credited amount must be rejected, got %v", err)
	}
}

func TestRechargePresetsNormalizeAndValidate(t *testing.T) {
	got, err := normalizeRechargePresetsValue("10.00, 50, 10, 100.50")
	if err != nil || got != "10,50,100.5" {
		t.Fatalf("normalizeRechargePresetsValue() = %q, %v; want 10,50,100.5", got, err)
	}
	if _, err := normalizeRechargePresetsValue("10.001"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("sub-cent preset must be rejected, got %v", err)
	}
	if _, err := normalizeRechargePresetsValue("0,50"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("zero preset must be rejected, got %v", err)
	}
	if got := normalizedRechargePresets("10,50"); len(got) != 2 || got[0] != "10" || got[1] != "50" {
		t.Fatalf("normalizedRechargePresets() = %#v", got)
	}
}

func TestDisabledProviderCanBeSavedWithIncompleteConfiguration(t *testing.T) {
	if err := validateProviderConfig(ProviderWechat, map[string]string{}); err != nil {
		t.Fatalf("disabling an incomplete WeChat provider must remain possible: %v", err)
	}
	if err := validateProviderConfig(ProviderWechat, map[string]string{"api_v3_key": "short"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("malformed configured API v3 key must be rejected, got %v", err)
	}
}

func TestStripePaymentMethodConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
		err   bool
	}{
		{name: "empty uses dashboard dynamic methods", value: "", want: ""},
		{name: "normalizes values", value: " card, alipay, wechat_pay ", want: "card,alipay,wechat_pay"},
		{name: "rejects unknown method", value: "card,bank_transfer", err: true},
		{name: "rejects duplicate method", value: "card,card", err: true},
		{name: "rejects empty item", value: "card,,alipay", err: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeStripePaymentMethods(test.value)
			if test.err {
				if !errors.Is(err, ErrInvalidRequest) {
					t.Fatalf("expected invalid request, got %v", err)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("normalizeStripePaymentMethods(%q) = %q, %v; want %q", test.value, got, err, test.want)
			}
		})
	}
}

func TestPaymentReturnURLKeepsHashRouteAfterAddingProviderParameters(t *testing.T) {
	got, err := paymentReturnURL("https://gateway.example.com/#console/billing", url.Values{
		"payment_order_id": {"order-123"},
		"provider":         {"stripe"},
	})
	if err != nil {
		t.Fatalf("paymentReturnURL returned error: %v", err)
	}
	if !strings.Contains(got, "?payment_order_id=order-123&provider=stripe#console/billing") {
		t.Fatalf("paymentReturnURL(%q) = %q; query must be before hash", "https://gateway.example.com/#console/billing", got)
	}
}

func TestProviderCurrencyRulesPreventDomesticCurrencyMismatch(t *testing.T) {
	if providerSupportsCurrency(ProviderWechat, "USD") || providerSupportsCurrency(ProviderAlipay, "USD") {
		t.Fatal("domestic payment providers must not accept USD orders")
	}
	if !providerSupportsCurrency(ProviderWechat, "CNY") || !providerSupportsCurrency(ProviderAlipay, "CNY") {
		t.Fatal("domestic payment providers must accept CNY orders")
	}
	if !providerSupportsCurrency(ProviderStripe, "USD") || !providerSupportsCurrency(ProviderPayPal, "EUR") {
		t.Fatal("international providers should use the tenant account currency")
	}
}

func TestVerifyAlipayWebhookRejectsForeignAppID(t *testing.T) {
	config := map[string]string{"app_id": "expected-app"}
	form := url.Values{
		"app_id":    {"other-app"},
		"sign_type": {"RSA2"},
		"sign":      {"not-a-signature"},
	}
	_, err := verifyAlipayWebhook(config, []byte(form.Encode()))
	if !errors.Is(err, ErrCallbackUntrusted) {
		t.Fatalf("foreign Alipay app ID must be rejected before signature processing, got %v", err)
	}
}

func TestVerifyWechatWebhookRejectsWrongCertificateSerial(t *testing.T) {
	headers := http.Header{}
	headers.Set("Wechatpay-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	headers.Set("Wechatpay-Nonce", "nonce")
	headers.Set("Wechatpay-Signature", "invalid")
	headers.Set("Wechatpay-Serial", "wrong-serial")
	_, err := verifyWechatWebhook(map[string]string{
		"platform_certificate_pem":       "invalid",
		"platform_certificate_serial_no": "expected-serial",
	}, headers, []byte(`{}`))
	if !errors.Is(err, ErrCallbackUntrusted) {
		t.Fatalf("wrong WeChat serial must be rejected as untrusted, got %v", err)
	}
}

func TestVerifyWechatWebhookRejectsUnsupportedAlgorithm(t *testing.T) {
	privateKey, certificatePEM := testWechatCertificate(t)
	body := []byte(`{"resource":{"algorithm":"SM4","ciphertext":"","nonce":"","associated_data":""}}`)
	headers := http.Header{}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := "nonce"
	digest := sha256.Sum256([]byte(timestamp + "\n" + nonce + "\n" + string(body) + "\n"))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	headers.Set("Wechatpay-Timestamp", timestamp)
	headers.Set("Wechatpay-Nonce", nonce)
	headers.Set("Wechatpay-Signature", base64.StdEncoding.EncodeToString(signature))
	headers.Set("Wechatpay-Serial", "expected-serial")
	_, err = verifyWechatWebhook(map[string]string{
		"platform_certificate_pem":       string(certificatePEM),
		"platform_certificate_serial_no": "expected-serial",
	}, headers, body)
	if !errors.Is(err, ErrCallbackUntrusted) {
		t.Fatalf("unsupported WeChat callback algorithm must be rejected, got %v", err)
	}
}

func TestVerifyWechatWebhookDecryptsStringNonce(t *testing.T) {
	privateKey, certificatePEM := testWechatCertificate(t)
	apiV3Key := "01234567890123456789012345678901"
	nonce := "012345678901"
	associatedData := "transaction"
	plain := []byte(`{"out_trade_no":"recharge_123","transaction_id":"wx_transaction","trade_state":"SUCCESS","amount":{"total":1000,"currency":"CNY"},"appid":"wx-app","mchid":"merchant"}`)
	block, err := aes.NewCipher([]byte(apiV3Key))
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext := base64.StdEncoding.EncodeToString(gcm.Seal(nil, []byte(nonce), plain, []byte(associatedData)))
	body, err := json.Marshal(map[string]any{"resource": map[string]string{
		"algorithm":       "AEAD_AES_256_GCM",
		"ciphertext":      ciphertext,
		"nonce":           nonce,
		"associated_data": associatedData,
	}})
	if err != nil {
		t.Fatal(err)
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	headers := http.Header{}
	headers.Set("Wechatpay-Timestamp", timestamp)
	headers.Set("Wechatpay-Nonce", "header-nonce")
	headers.Set("Wechatpay-Serial", "expected-serial")
	digest := sha256.Sum256([]byte(timestamp + "\nheader-nonce\n" + string(body) + "\n"))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	headers.Set("Wechatpay-Signature", base64.StdEncoding.EncodeToString(signature))

	payment, err := verifyWechatWebhook(map[string]string{
		"app_id":                         "wx-app",
		"mch_id":                         "merchant",
		"api_v3_key":                     apiV3Key,
		"platform_certificate_pem":       string(certificatePEM),
		"platform_certificate_serial_no": "expected-serial",
	}, headers, body)
	if err != nil {
		t.Fatalf("valid WeChat callback should be accepted, got %v", err)
	}
	if payment.MerchantOrderNo != "recharge_123" || payment.ProviderOrderID != "wx_transaction" || payment.Amount != "10.00" || payment.Currency != "CNY" || !payment.Paid {
		t.Fatalf("unexpected callback payment: %+v", payment)
	}
}

func TestPayPalCheckoutCompletionIsNotPaymentCapture(t *testing.T) {
	body := []byte(`{"event_type":"CHECKOUT.ORDER.COMPLETED","resource":{"custom_id":"recharge_order","amount":{"value":"10.00","currency_code":"USD"}}}`)
	_, err := parsePayPalWebhookEvent(body)
	if !errors.Is(err, ErrCallbackInvalid) {
		t.Fatalf("PayPal checkout completion must not settle a balance, got %v", err)
	}
}

func testWechatCertificate(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-wechat-platform"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return privateKey, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
