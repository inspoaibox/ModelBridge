package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type ProviderError struct {
	Provider   string
	StatusCode int
	Detail     string
}

func (e *ProviderError) Error() string {
	if e == nil {
		return "payment provider failed"
	}
	detail := strings.TrimSpace(e.Detail)
	if e.StatusCode > 0 {
		if detail == "" {
			return fmt.Sprintf("%s provider returned HTTP %d", e.Provider, e.StatusCode)
		}
		return fmt.Sprintf("%s provider returned HTTP %d: %s", e.Provider, e.StatusCode, detail)
	}
	if detail == "" {
		return e.Provider + " provider request failed"
	}
	return e.Provider + " provider request failed: " + detail
}

func (s *SQLService) createStripeOrder(ctx context.Context, order Order, config map[string]string, returnURL string) (providerOrder, error) {
	minor, err := minorAmount(order.Amount, order.Currency)
	if err != nil {
		return providerOrder{}, err
	}
	successURL, err := paymentReturnURL(returnURL, url.Values{
		"payment_order_id": {order.ID},
		"provider":         {"stripe"},
	})
	if err != nil {
		return providerOrder{}, err
	}
	cancelURL, err := paymentReturnURL(returnURL, url.Values{
		"payment_order_id": {order.ID},
		"provider":         {"stripe"},
		"cancelled":        {"1"},
	})
	if err != nil {
		return providerOrder{}, err
	}
	form := url.Values{}
	form.Set("mode", "payment")
	embedded := strings.HasPrefix(strings.TrimSpace(config["publishable_key"]), "pk_")
	if embedded {
		form.Set("ui_mode", "embedded")
		form.Set("return_url", successURL)
	} else {
		form.Set("success_url", successURL)
		form.Set("cancel_url", cancelURL)
	}
	form.Set("client_reference_id", order.MerchantOrderNo)
	form.Set("metadata[order_id]", order.ID)
	form.Set("metadata[merchant_order_no]", order.MerchantOrderNo)
	form.Set("line_items[0][price_data][currency]", strings.ToLower(order.Currency))
	form.Set("line_items[0][price_data][product_data][name]", "AI Token Gateway balance recharge")
	form.Set("line_items[0][price_data][unit_amount]", minor)
	form.Set("line_items[0][quantity]", "1")
	methods, err := normalizeStripePaymentMethods(config["payment_method_types"])
	if err != nil {
		return providerOrder{}, err
	}
	if methods != "" {
		for index, method := range strings.Split(methods, ",") {
			form.Set(fmt.Sprintf("payment_method_types[%d]", index), method)
		}
	} else {
		// Stripe selects eligible methods from its Dashboard and account
		// capabilities when no explicit allow-list is configured.
		form.Set("automatic_payment_methods[enabled]", "true")
	}
	endpoint := stripeAPIBaseURL(config["api_base_url"])
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/v1/checkout/sessions", strings.NewReader(form.Encode()))
	if err != nil {
		return providerOrder{}, err
	}
	req.SetBasicAuth(config["secret_key"], "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return providerOrder{}, &ProviderError{Provider: ProviderStripe, Detail: err.Error()}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return providerOrder{}, &ProviderError{Provider: ProviderStripe, StatusCode: resp.StatusCode, Detail: stripeErrorSummary(raw)}
	}
	var payload struct {
		ID           string `json:"id"`
		URL          string `json:"url"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.ID == "" || (!embedded && payload.URL == "") || (embedded && payload.ClientSecret == "") {
		return providerOrder{}, &ProviderError{Provider: ProviderStripe, StatusCode: resp.StatusCode, Detail: "Stripe returned an invalid Checkout Session response"}
	}
	return providerOrder{ProviderOrderID: payload.ID, CheckoutURL: payload.URL, CheckoutClientSecret: payload.ClientSecret, Raw: raw}, nil
}

func stripeErrorSummary(raw []byte) string {
	var payload struct {
		Error struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &payload) == nil {
		parts := make([]string, 0, 3)
		if value := strings.TrimSpace(payload.Error.Type); value != "" {
			parts = append(parts, value)
		}
		if value := strings.TrimSpace(payload.Error.Code); value != "" {
			parts = append(parts, value)
		}
		if value := strings.TrimSpace(payload.Error.Message); value != "" {
			parts = append(parts, value)
		}
		if len(parts) > 0 {
			return strings.Join(parts, ": ")
		}
	}
	value := strings.Join(strings.Fields(string(raw)), " ")
	if len(value) > 240 {
		value = value[:240]
	}
	if value == "" {
		return "empty response"
	}
	return value
}

// stripeAPIBaseURL accepts both the documented API root and the common value
// copied from Stripe's API documentation (which already includes /v1). The
// checkout path below owns the version segment and must add it exactly once.
func stripeAPIBaseURL(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
		return "https://api.stripe.com"
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return value
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(parsed.Path, "/v1") {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/v1")
		parsed.RawPath = ""
	}
	return strings.TrimRight(parsed.String(), "/")
}

func verifyStripeWebhook(config map[string]string, headers http.Header, body []byte) (callbackPayment, error) {
	secret := strings.TrimSpace(config["webhook_secret"])
	if secret == "" {
		return callbackPayment{}, ErrProviderUnconfig
	}
	header := strings.TrimSpace(headers.Get("Stripe-Signature"))
	var timestamp string
	var signatures []string
	for _, part := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch key {
		case "t":
			timestamp = value
		case "v1":
			signatures = append(signatures, value)
		}
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || timestamp == "" {
		return callbackPayment{}, ErrCallbackUntrusted
	}
	if time.Since(time.Unix(seconds, 0)) > 5*time.Minute || time.Until(time.Unix(seconds, 0)) > 5*time.Minute {
		return callbackPayment{}, ErrCallbackUntrusted
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "." + string(body)))
	expected := hex.EncodeToString(mac.Sum(nil))
	valid := false
	for _, candidate := range signatures {
		if hmac.Equal([]byte(expected), []byte(candidate)) {
			valid = true
			break
		}
	}
	if !valid {
		return callbackPayment{}, ErrCallbackUntrusted
	}
	var event struct {
		Type string `json:"type"`
		Data struct {
			Object struct {
				ID                string            `json:"id"`
				ClientReferenceID string            `json:"client_reference_id"`
				Metadata          map[string]string `json:"metadata"`
				AmountTotal       int64             `json:"amount_total"`
				Currency          string            `json:"currency"`
				PaymentStatus     string            `json:"payment_status"`
			} `json:"object"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &event) != nil {
		return callbackPayment{}, ErrCallbackInvalid
	}
	if event.Type != "checkout.session.completed" && event.Type != "checkout.session.async_payment_succeeded" {
		return callbackPayment{}, ErrCallbackInvalid
	}
	value := event.Data.Object
	merchant := value.ClientReferenceID
	if merchant == "" {
		merchant = value.Metadata["merchant_order_no"]
	}
	if merchant == "" {
		return callbackPayment{}, ErrCallbackInvalid
	}
	return callbackPayment{MerchantOrderNo: merchant, ProviderOrderID: value.ID, Currency: strings.ToUpper(value.Currency), Amount: minorToAmount(value.AmountTotal, value.Currency), Paid: value.PaymentStatus == "paid" || event.Type == "checkout.session.async_payment_succeeded"}, nil
}

func minorToAmount(value int64, currency string) string {
	places := decimalPlaces(currency)
	if places == 0 {
		return strconv.FormatInt(value, 10)
	}
	divisor := int64(1)
	for i := 0; i < places; i++ {
		divisor *= 10
	}
	return fmt.Sprintf("%d.%0*d", value/divisor, places, value%divisor)
}

func paypalBaseURL(config map[string]string) string {
	if strings.EqualFold(strings.TrimSpace(config["environment"]), "live") {
		return "https://api-m.paypal.com"
	}
	return "https://api-m.sandbox.paypal.com"
}

func paypalAccessToken(ctx context.Context, client *http.Client, config map[string]string) (string, error) {
	form := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, paypalBaseURL(config)+"/v1/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(config["client_id"], config["client_secret"])
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("paypal oauth returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if json.Unmarshal(raw, &payload) != nil || payload.AccessToken == "" {
		return "", ErrProviderUnconfig
	}
	return payload.AccessToken, nil
}

func (s *SQLService) createPayPalOrder(ctx context.Context, order Order, config map[string]string, returnURL string) (providerOrder, error) {
	token, err := paypalAccessToken(ctx, s.httpClient(), config)
	if err != nil {
		return providerOrder{}, err
	}
	returnURLValue, err := paymentReturnURL(returnURL, url.Values{
		"payment_order_id": {order.ID},
		"provider":         {"paypal"},
	})
	if err != nil {
		return providerOrder{}, err
	}
	cancelURL, err := paymentReturnURL(returnURL, url.Values{
		"payment_order_id": {order.ID},
		"provider":         {"paypal"},
		"cancelled":        {"1"},
	})
	if err != nil {
		return providerOrder{}, err
	}
	payload := map[string]any{"intent": "CAPTURE", "purchase_units": []any{map[string]any{"reference_id": order.MerchantOrderNo, "custom_id": order.MerchantOrderNo, "amount": map[string]string{"currency_code": order.Currency, "value": order.Amount}}}, "application_context": map[string]string{"return_url": returnURLValue, "cancel_url": cancelURL}}
	encoded, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, paypalBaseURL(config)+"/v2/checkout/orders", bytesReader(encoded))
	if err != nil {
		return providerOrder{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("PayPal-Request-Id", order.MerchantOrderNo)
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return providerOrder{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return providerOrder{}, fmt.Errorf("paypal create order returned HTTP %d", resp.StatusCode)
	}
	var result struct {
		ID    string `json:"id"`
		Links []struct {
			Href string `json:"href"`
			Rel  string `json:"rel"`
		} `json:"links"`
	}
	if json.Unmarshal(raw, &result) != nil || result.ID == "" {
		return providerOrder{}, ErrCallbackInvalid
	}
	checkout := ""
	for _, link := range result.Links {
		if link.Rel == "approve" {
			checkout = link.Href
			break
		}
	}
	if checkout == "" {
		return providerOrder{}, ErrCallbackInvalid
	}
	return providerOrder{ProviderOrderID: result.ID, CheckoutURL: checkout, Raw: raw}, nil
}

func (s *SQLService) verifyPayPalWebhook(ctx context.Context, config map[string]string, headers http.Header, body []byte) (callbackPayment, error) {
	if strings.TrimSpace(config["webhook_id"]) == "" {
		return callbackPayment{}, ErrProviderUnconfig
	}
	for _, name := range []string{"PAYPAL-AUTH-ALGO", "PAYPAL-CERT-URL", "PAYPAL-TRANSMISSION-ID", "PAYPAL-TRANSMISSION-SIG", "PAYPAL-TRANSMISSION-TIME"} {
		if strings.TrimSpace(headers.Get(name)) == "" {
			return callbackPayment{}, ErrCallbackUntrusted
		}
	}
	token, err := paypalAccessToken(ctx, s.httpClient(), config)
	if err != nil {
		return callbackPayment{}, err
	}
	payload := map[string]string{
		"auth_algo":         headers.Get("PAYPAL-AUTH-ALGO"),
		"cert_url":          headers.Get("PAYPAL-CERT-URL"),
		"transmission_id":   headers.Get("PAYPAL-TRANSMISSION-ID"),
		"transmission_sig":  headers.Get("PAYPAL-TRANSMISSION-SIG"),
		"transmission_time": headers.Get("PAYPAL-TRANSMISSION-TIME"),
		"webhook_id":        config["webhook_id"],
		"webhook_event":     string(body),
	}
	encoded, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, paypalBaseURL(config)+"/v1/notifications/verify-webhook-signature", bytesReader(encoded))
	if err != nil {
		return callbackPayment{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return callbackPayment{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return callbackPayment{}, ErrCallbackUntrusted
	}
	var verification struct {
		VerificationStatus string `json:"verification_status"`
	}
	if json.Unmarshal(raw, &verification) != nil || verification.VerificationStatus != "SUCCESS" {
		return callbackPayment{}, ErrCallbackUntrusted
	}
	return parsePayPalWebhookEvent(body)
}

func parsePayPalWebhookEvent(body []byte) (callbackPayment, error) {
	var event struct {
		EventType string `json:"event_type"`
		Resource  struct {
			ID                string `json:"id"`
			CustomID          string `json:"custom_id"`
			SupplementaryData struct {
				RelatedIDs struct {
					OrderID string `json:"order_id"`
				} `json:"related_ids"`
			} `json:"supplementary_data"`
			Amount struct {
				Value        string `json:"value"`
				CurrencyCode string `json:"currency_code"`
			} `json:"amount"`
			PurchaseUnits []struct {
				CustomID string `json:"custom_id"`
				Amount   struct {
					Value        string `json:"value"`
					CurrencyCode string `json:"currency_code"`
				} `json:"amount"`
			} `json:"purchase_units"`
		} `json:"resource"`
	}
	if json.Unmarshal(body, &event) != nil {
		return callbackPayment{}, ErrCallbackInvalid
	}
	if event.EventType != "PAYMENT.CAPTURE.COMPLETED" {
		return callbackPayment{}, ErrCallbackInvalid
	}
	merchant, amount, currency := event.Resource.CustomID, event.Resource.Amount.Value, event.Resource.Amount.CurrencyCode
	providerOrderID := event.Resource.SupplementaryData.RelatedIDs.OrderID
	if len(event.Resource.PurchaseUnits) > 0 {
		if merchant == "" {
			merchant = event.Resource.PurchaseUnits[0].CustomID
		}
		if amount == "" {
			amount = event.Resource.PurchaseUnits[0].Amount.Value
		}
		if currency == "" {
			currency = event.Resource.PurchaseUnits[0].Amount.CurrencyCode
		}
	}
	if merchant == "" && providerOrderID == "" {
		return callbackPayment{}, ErrCallbackInvalid
	}
	return callbackPayment{MerchantOrderNo: merchant, ProviderOrderID: providerOrderID, Currency: currency, Amount: amount, Paid: true}, nil
}

func bytesReader(value []byte) *bytes.Reader { return bytes.NewReader(value) }

func paymentReturnURL(base string, params url.Values) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return "", ErrInvalidRequest
	}
	query := parsed.Query()
	for key, values := range params {
		query.Del(key)
		for _, value := range values {
			query.Add(key, value)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
