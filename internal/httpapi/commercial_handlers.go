package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"ai-token/internal/auth"
	"ai-token/internal/billing"
	"ai-token/internal/enterprise"
	"ai-token/internal/payments"
)

func enterpriseCurrentHandler(service enterprise.ConsoleService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ENTERPRISE_UNAVAILABLE"})
			return
		}
		principal, ok := principalFromRequest(r)
		if !ok || principal.TenantID == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "RESOURCE_NOT_FOUND"})
			return
		}
		item, err := service.GetCurrent(r.Context(), principal.TenantID, principal.ID)
		if errors.Is(err, enterprise.ErrNotFound) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "not_submitted"})
			return
		}
		if err != nil {
			writeEnterpriseError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
}

func enterpriseSubmitHandler(service enterprise.ConsoleService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ENTERPRISE_UNAVAILABLE"})
			return
		}
		principal, ok := principalFromRequest(r)
		if !ok || principal.TenantID == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "RESOURCE_NOT_FOUND"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, enterprise.MaxLicenseSize+1<<20)
		if err := r.ParseMultipartForm(enterprise.MaxLicenseSize + 1<<20); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_ENTERPRISE_REQUEST"})
			return
		}
		file, header, err := r.FormFile("license")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "LICENSE_REQUIRED"})
			return
		}
		defer file.Close()
		content, err := io.ReadAll(io.LimitReader(file, enterprise.MaxLicenseSize+1))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "LICENSE_INVALID"})
			return
		}
		if len(content) > enterprise.MaxLicenseSize {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "LICENSE_TOO_LARGE"})
			return
		}
		item, err := service.Submit(r.Context(), principal.ID, principal.TenantID, enterprise.SubmitRequest{
			EnterpriseName: r.FormValue("enterprise_name"), UnifiedCreditCode: r.FormValue("unified_credit_code"),
			LicenseFilename: filepath.Base(header.Filename), LicenseType: header.Header.Get("Content-Type"), License: content,
			BankAccountName: r.FormValue("bank_account_name"), BankName: r.FormValue("bank_name"), BankAccount: r.FormValue("bank_account"),
		})
		if err != nil {
			writeEnterpriseError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	})
}

func enterpriseAdminListHandler(service enterprise.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ENTERPRISE_UNAVAILABLE"})
			return
		}
		items, err := service.List(r.Context(), r.URL.Query().Get("status"))
		if err != nil {
			writeEnterpriseError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"submissions": items})
	})
}

func enterpriseAdminGetHandler(service enterprise.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ENTERPRISE_UNAVAILABLE"})
			return
		}
		item, err := service.Get(r.Context(), r.PathValue("verificationID"))
		if err != nil {
			writeEnterpriseError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
}

func enterpriseAdminLicenseHandler(service enterprise.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ENTERPRISE_UNAVAILABLE"})
			return
		}
		document, err := service.License(r.Context(), r.PathValue("verificationID"))
		if err != nil {
			writeEnterpriseError(w, err)
			return
		}
		w.Header().Set("Content-Type", document.ContentType)
		filename := filepath.Base(document.Filename)
		filename = strings.Map(func(value rune) rune {
			if value == '"' || value == '\r' || value == '\n' || value < 0x20 || value == 0x7f {
				return -1
			}
			return value
		}, filename)
		if filename == "" || filename == "." || filename == ".." {
			filename = "business-license"
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.Header().Set("X-Content-SHA256", document.SHA256)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(document.Content)
	})
}

func enterpriseAdminReviewHandler(service enterprise.AdminService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ENTERPRISE_UNAVAILABLE"})
			return
		}
		var payload struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		}
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_ENTERPRISE_REVIEW"})
			return
		}
		principal, ok := principalFromRequest(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		item, err := service.Review(r.Context(), principal.ID, r.PathValue("verificationID"), payload.Status, payload.Reason)
		if err != nil {
			writeEnterpriseError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
}

func paymentAdminListHandler(service payments.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PAYMENTS_UNAVAILABLE"})
			return
		}
		items, err := service.AdminList(r.Context())
		if err != nil {
			writePaymentError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"providers": items})
	})
}

func paymentPublicProvidersHandler(service payments.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, ok := service.(interface {
			PublicList(context.Context) ([]payments.PublicProvider, error)
		})
		if !ok || reader == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PAYMENTS_UNAVAILABLE"})
			return
		}
		items, err := reader.PublicList(r.Context())
		if err != nil {
			writePaymentError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"providers": items})
	})
}

func paymentAdminUpdateHandler(service payments.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PAYMENTS_UNAVAILABLE"})
			return
		}
		var payload payments.ConfigUpdate
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_PAYMENT_CONFIG"})
			return
		}
		principal, ok := principalFromRequest(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "AUTH_REQUIRED"})
			return
		}
		item, err := service.AdminUpdate(r.Context(), principal.ID, r.PathValue("provider"), payload)
		if err != nil {
			writePaymentError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
}

func paymentCreateOrderHandler(service payments.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PAYMENTS_UNAVAILABLE"})
			return
		}
		principal, ok := principalFromRequest(r)
		if !ok || principal.TenantID == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "RESOURCE_NOT_FOUND"})
			return
		}
		var payload struct {
			Provider  string `json:"provider"`
			Amount    string `json:"amount"`
			Currency  string `json:"currency"`
			ReturnURL string `json:"return_url"`
		}
		if err := decodeJSON(w, r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_PAYMENT_REQUEST"})
			return
		}
		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if key == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "IDEMPOTENCY_KEY_REQUIRED"})
			return
		}
		returnURL := strings.TrimSpace(payload.ReturnURL)
		if returnURL == "" {
			returnURL = strings.TrimSpace(r.Header.Get("Origin"))
		}
		if !safeReturnURL(returnURL, r) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_RETURN_URL"})
			return
		}
		result, err := service.CreateOrder(r.Context(), payments.CreateRequest{TenantID: principal.TenantID, UserID: principal.ID, Provider: payload.Provider, Amount: payload.Amount, Currency: payload.Currency, IdempotencyKey: key, ReturnURL: returnURL})
		if err != nil {
			log.Printf("payment create order failed request_id=%s tenant_id=%s provider=%s error=%v", r.Header.Get("X-Request-ID"), principal.TenantID, strings.TrimSpace(payload.Provider), err)
			writePaymentError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, result)
	})
}

func paymentGetOrderHandler(service payments.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PAYMENTS_UNAVAILABLE"})
			return
		}
		principal, ok := principalFromRequest(r)
		if !ok || principal.TenantID == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "RESOURCE_NOT_FOUND"})
			return
		}
		order, err := service.GetOrder(r.Context(), principal.TenantID, r.PathValue("orderID"))
		if err != nil {
			writePaymentError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, order)
	})
}

func paymentListOrdersHandler(service payments.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PAYMENTS_UNAVAILABLE"})
			return
		}
		principal, ok := principalFromRequest(r)
		if !ok || principal.TenantID == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "RESOURCE_NOT_FOUND"})
			return
		}
		reader, ok := service.(payments.OrderLister)
		if !ok {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PAYMENTS_UNAVAILABLE"})
			return
		}
		limit, offset, err := reportPagination(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_PAYMENT_REQUEST"})
			return
		}
		orders, err := reader.ListOrders(r.Context(), principal.TenantID, payments.OrderQuery{Limit: limit, Offset: offset})
		if err != nil {
			writePaymentError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, orders)
	})
}

func paymentPayPalCaptureHandler(service payments.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PAYMENTS_UNAVAILABLE"})
			return
		}
		principal, ok := principalFromRequest(r)
		if !ok || principal.TenantID == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "RESOURCE_NOT_FOUND"})
			return
		}
		if err := service.CapturePayPal(r.Context(), principal.TenantID, r.PathValue("orderID")); err != nil {
			writePaymentError(w, err)
			return
		}
		order, err := service.GetOrder(r.Context(), principal.TenantID, r.PathValue("orderID"))
		if err != nil {
			writePaymentError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, order)
	})
}

func paymentWebhookHandler(service payments.Service, provider string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PAYMENTS_UNAVAILABLE"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_PAYMENT_CALLBACK"})
			return
		}
		if err := service.HandleWebhook(r.Context(), provider, r.Header, body); err != nil {
			writePaymentError(w, err)
			return
		}
		if provider == payments.ProviderWechat {
			writeJSON(w, http.StatusOK, map[string]string{"code": "SUCCESS", "message": "成功"})
			return
		}
		if provider == payments.ProviderAlipay {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("success"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
}

func principalFromRequest(r *http.Request) (*auth.Principal, bool) {
	return auth.PrincipalFromContext(r.Context())
}

func safeReturnURL(value string, r *http.Request) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || (parsed.Scheme != "https" && !isLocalDevelopmentHost(parsed.Hostname())) {
		return false
	}
	targetOrigin := parsed.Scheme + "://" + parsed.Host
	allowedOrigins := []string{targetOriginForRequest(r)}
	if origin := normalizeOrigin(r.Header.Get("Origin")); origin != "" {
		allowedOrigins = append(allowedOrigins, origin)
	}
	for _, raw := range []string{os.Getenv("PUBLIC_BASE_URL"), os.Getenv("CORS_ALLOWED_ORIGINS")} {
		for _, item := range strings.Split(raw, ",") {
			if origin := normalizeOrigin(item); origin != "" {
				allowedOrigins = append(allowedOrigins, origin)
			}
		}
	}
	for _, allowed := range allowedOrigins {
		if strings.EqualFold(allowed, targetOrigin) {
			return true
		}
	}
	return false
}

func targetOriginForRequest(r *http.Request) string {
	if r == nil || strings.TrimSpace(r.Host) == "" {
		return ""
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func normalizeOrigin(value string) string {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(value), "/"))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}
	if parsed.Scheme != "https" && !isLocalDevelopmentHost(parsed.Hostname()) {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func isLocalDevelopmentHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func writeEnterpriseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, enterprise.ErrInvalidRequest), errors.Is(err, enterprise.ErrReviewInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_ENTERPRISE_REQUEST"})
	case errors.Is(err, enterprise.ErrDocumentTooLarge):
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "LICENSE_TOO_LARGE"})
	case errors.Is(err, enterprise.ErrDocumentType), errors.Is(err, enterprise.ErrDocumentInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "LICENSE_INVALID"})
	case errors.Is(err, enterprise.ErrPending):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "ENTERPRISE_PENDING"})
	case errors.Is(err, enterprise.ErrAlreadyApproved):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "ENTERPRISE_APPROVED"})
	case errors.Is(err, enterprise.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "ENTERPRISE_NOT_FOUND"})
	case errors.Is(err, enterprise.ErrUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ENTERPRISE_UNAVAILABLE"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ENTERPRISE_UNAVAILABLE"})
	}
}

func writePaymentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, payments.ErrInvalidRequest), errors.Is(err, payments.ErrCallbackInvalid), errors.Is(err, payments.ErrAmountMismatch), errors.Is(err, payments.ErrCurrencyMismatch):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "INVALID_PAYMENT_REQUEST"})
	case errors.Is(err, payments.ErrProviderDisabled):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "PAYMENT_PROVIDER_DISABLED"})
	case errors.Is(err, payments.ErrProviderUnconfig):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "PAYMENT_PROVIDER_UNCONFIGURED"})
	case errors.Is(err, payments.ErrCallbackUntrusted):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "PAYMENT_CALLBACK_UNTRUSTED"})
	case errors.Is(err, payments.ErrOrderNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "PAYMENT_ORDER_NOT_FOUND"})
	case errors.Is(err, payments.ErrOrderClosed):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "PAYMENT_ORDER_CLOSED"})
	case errors.Is(err, payments.ErrUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "PAYMENTS_UNAVAILABLE"})
	case errors.Is(err, billing.ErrAccountNotFound):
		writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": "BILLING_ACCOUNT_NOT_FOUND"})
	default:
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "PAYMENT_PROVIDER_FAILED"})
	}
}
