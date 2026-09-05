package payments

// This package contains the payment boundary for customer balance top-ups.
// Provider credentials are encrypted at rest and provider callbacks are the
// only source of truth used to credit a tenant account.

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ai-token/internal/billing"
	"ai-token/internal/ids"
	"ai-token/internal/mfa"
)

const (
	ProviderWechat = "wechat"
	ProviderAlipay = "alipay"
	ProviderStripe = "stripe"
	ProviderPayPal = "paypal"
	orderLifetime  = 30 * time.Minute
)

var (
	ErrUnavailable       = errors.New("payment service is unavailable")
	ErrInvalidRequest    = errors.New("invalid payment request")
	ErrProviderDisabled  = errors.New("payment provider is disabled")
	ErrProviderUnconfig  = errors.New("payment provider is not configured")
	ErrOrderNotFound     = errors.New("payment order is not found")
	ErrOrderClosed       = errors.New("payment order is closed")
	ErrCallbackInvalid   = errors.New("payment callback is invalid")
	ErrCallbackUntrusted = errors.New("payment callback could not be verified")
	ErrAmountMismatch    = errors.New("payment amount does not match order")
	ErrCurrencyMismatch  = errors.New("payment currency does not match order")
)

type ProviderConfig struct {
	Provider     string            `json:"provider"`
	Enabled      bool              `json:"enabled"`
	Values       map[string]string `json:"values"`
	Configured   bool              `json:"configured"`
	SecretFields []string          `json:"secret_fields"`
	WebhookURL   string            `json:"webhook_url,omitempty"`
	UpdatedAt    time.Time         `json:"updated_at,omitempty"`
	UpdatedBy    string            `json:"updated_by,omitempty"`
}

type PublicProvider struct {
	Provider         string            `json:"provider"`
	Enabled          bool              `json:"enabled"`
	RechargeRate     string            `json:"recharge_rate,omitempty"`
	RechargePresets  []string          `json:"recharge_presets,omitempty"`
	RechargePackages []RechargePackage `json:"recharge_packages,omitempty"`
	PaymentMethods   []string          `json:"payment_method_types,omitempty"`
	PublishableKey   string            `json:"publishable_key,omitempty"`
}

type RechargePackage struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Description          string `json:"description,omitempty"`
	Kind                 string `json:"kind"`
	Currency             string `json:"currency"`
	Amount               string `json:"amount"`
	CreditedAmount       string `json:"credited_amount"`
	BonusAmount          string `json:"bonus_amount"`
	ValidityDays         int    `json:"validity_days"`
	StartsAt             string `json:"starts_at,omitempty"`
	EndsAt               string `json:"ends_at,omitempty"`
	SubscriptionPlanCode string `json:"subscription_plan_code,omitempty"`
	Enabled              bool   `json:"enabled"`
}

type RechargePackages struct {
	Packages        []RechargePackage `json:"packages"`
	RechargePresets []string          `json:"recharge_presets"`
}

type OrderQuery struct {
	Limit  int
	Offset int
}

type OrderList struct {
	Orders []Order `json:"orders"`
	Total  int64   `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

type OrderLister interface {
	ListOrders(context.Context, string, OrderQuery) (OrderList, error)
}

// ConfigUpdate accepts only named string fields. Secret values are omitted by
// the UI to preserve the current value, or listed in Clear to remove them.
type ConfigUpdate struct {
	Enabled bool              `json:"enabled"`
	Values  map[string]string `json:"values"`
	Clear   []string          `json:"clear"`
}

type CreateRequest struct {
	TenantID       string
	UserID         string
	Provider       string
	Amount         string
	Currency       string
	IdempotencyKey string
	ReturnURL      string
	PackageID      string
}

type Order struct {
	ID                   string     `json:"id"`
	TenantID             string     `json:"tenant_id"`
	UserID               string     `json:"user_id"`
	Provider             string     `json:"provider"`
	MerchantOrderNo      string     `json:"merchant_order_no"`
	ProviderOrderID      string     `json:"provider_order_id,omitempty"`
	Amount               string     `json:"amount"`
	CreditedAmount       string     `json:"credited_amount"`
	PackageID            string     `json:"package_id,omitempty"`
	BonusAmount          string     `json:"bonus_amount,omitempty"`
	ValidUntil           *time.Time `json:"valid_until,omitempty"`
	RechargeRate         string     `json:"recharge_rate"`
	Currency             string     `json:"currency"`
	Status               string     `json:"status"`
	CheckoutURL          string     `json:"checkout_url,omitempty"`
	CheckoutClientSecret string     `json:"checkout_client_secret,omitempty"`
	QRCode               string     `json:"qr_code,omitempty"`
	FailureReason        string     `json:"failure_reason,omitempty"`
	PaidAt               *time.Time `json:"paid_at,omitempty"`
	ExpiresAt            time.Time  `json:"expires_at"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type CreateResult struct {
	Order
	Message string `json:"message,omitempty"`
}

type Service interface {
	AdminList(context.Context) ([]ProviderConfig, error)
	AdminUpdate(context.Context, string, string, ConfigUpdate) (ProviderConfig, error)
	CreateOrder(context.Context, CreateRequest) (CreateResult, error)
	GetOrder(context.Context, string, string) (Order, error)
	HandleWebhook(context.Context, string, http.Header, []byte) error
	CapturePayPal(context.Context, string, string) error
}

type SQLService struct {
	db     *sql.DB
	box    *mfa.SecretBox
	biller billing.AdminService
	client *http.Client
	now    func() time.Time
}

func NewSQLService(db *sql.DB, box *mfa.SecretBox, biller billing.AdminService) (*SQLService, error) {
	if db == nil || box == nil || biller == nil {
		return nil, errors.New("database, secret box and billing service are required")
	}
	return &SQLService{db: db, box: box, biller: biller, client: &http.Client{Timeout: 20 * time.Second}, now: time.Now}, nil
}

func (s *SQLService) httpClient() *http.Client {
	if s.client != nil {
		return s.client
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func supportedProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case ProviderWechat, ProviderAlipay, ProviderStripe, ProviderPayPal:
		return true
	default:
		return false
	}
}

var secretFieldsByProvider = map[string][]string{
	ProviderWechat: {"private_key_pem", "api_v3_key", "platform_certificate_pem"},
	ProviderAlipay: {"private_key_pem", "alipay_public_key_pem"},
	ProviderStripe: {"secret_key", "webhook_secret"},
	ProviderPayPal: {"client_secret"},
}

var requiredFieldsByProvider = map[string][]string{
	ProviderWechat: {"app_id", "mch_id", "serial_no", "platform_certificate_serial_no", "private_key_pem", "api_v3_key", "platform_certificate_pem", "notify_url"},
	ProviderAlipay: {"app_id", "private_key_pem", "alipay_public_key_pem", "notify_url"},
	ProviderStripe: {"secret_key", "webhook_secret"},
	ProviderPayPal: {"client_id", "client_secret", "webhook_id"},
}

var allowedFieldsByProvider = map[string]map[string]struct{}{
	ProviderWechat: {
		"app_id": {}, "mch_id": {}, "serial_no": {}, "platform_certificate_serial_no": {},
		"private_key_pem": {}, "api_v3_key": {}, "platform_certificate_pem": {},
		"notify_url": {}, "api_base_url": {}, "recharge_rate": {}, "recharge_presets": {},
	},
	ProviderAlipay: {
		"app_id": {}, "seller_id": {}, "private_key_pem": {}, "alipay_public_key_pem": {},
		"notify_url": {}, "gateway": {}, "recharge_rate": {}, "recharge_presets": {},
	},
	ProviderStripe: {
		"secret_key": {}, "publishable_key": {}, "webhook_secret": {}, "api_base_url": {}, "payment_method_types": {}, "recharge_rate": {}, "recharge_presets": {},
	},
	ProviderPayPal: {
		"client_id": {}, "client_secret": {}, "webhook_id": {}, "environment": {}, "recharge_rate": {}, "recharge_presets": {},
	},
}

func (s *SQLService) AdminList(ctx context.Context) ([]ProviderConfig, error) {
	if s == nil || s.db == nil || s.box == nil {
		return nil, ErrUnavailable
	}
	items := make([]ProviderConfig, 0, len(secretFieldsByProvider))
	for _, provider := range []string{ProviderWechat, ProviderAlipay, ProviderStripe, ProviderPayPal} {
		item, err := s.readConfig(ctx, provider)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *SQLService) PublicList(ctx context.Context) ([]PublicProvider, error) {
	if s == nil || s.db == nil || s.box == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `SELECT provider, enabled FROM payment_provider_configs WHERE enabled = true ORDER BY provider`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	globalPackages, globalConfigured, err := s.globalRechargePackages(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]PublicProvider, 0, 4)
	for rows.Next() {
		var item PublicProvider
		if err := rows.Scan(&item.Provider, &item.Enabled); err != nil {
			return nil, err
		}
		values, err := s.readRawConfig(ctx, item.Provider)
		if err != nil {
			return nil, err
		}
		if !hasRequiredFields(item.Provider, values) {
			continue
		}
		item.RechargeRate = normalizedRechargeRate(values["recharge_rate"])
		item.RechargePackages = publicRechargePackages(globalPackages, time.Now())
		item.RechargePresets = packageAmounts(item.RechargePackages)
		if !globalConfigured {
			// Keep installations upgraded from the provider-scoped setting usable
			// until an administrator saves the new global package configuration.
			item.RechargePackages = legacyRechargePackages(normalizedRechargePresets(values["recharge_presets"]))
			item.RechargePresets = packageAmounts(item.RechargePackages)
		}
		item.PublishableKey = strings.TrimSpace(values["publishable_key"])
		if item.Provider == ProviderStripe {
			if methods, normalizeErr := normalizeStripePaymentMethods(values["payment_method_types"]); normalizeErr == nil && methods != "" {
				item.PaymentMethods = strings.Split(methods, ",")
			}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLService) GetRechargePresets(ctx context.Context) ([]string, error) {
	packages, _, err := s.globalRechargePackages(ctx)
	return packageAmounts(packages), err
}

func (s *SQLService) GetRechargePackages(ctx context.Context) ([]RechargePackage, error) {
	packages, _, err := s.globalRechargePackages(ctx)
	return packages, err
}

func (s *SQLService) globalRechargePackages(ctx context.Context) ([]RechargePackage, bool, error) {
	if s == nil || s.db == nil {
		return nil, false, ErrUnavailable
	}
	var encoded string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM platform_settings WHERE key = 'payment_recharge_packages'`).Scan(&encoded)
	if err == nil {
		var packages []RechargePackage
		if json.Unmarshal([]byte(encoded), &packages) != nil {
			return nil, true, ErrInvalidRequest
		}
		return normalizeRechargePackages(packages)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	// Older deployments stored this value alongside each payment provider.
	// Read the first configured value as a one-time compatibility fallback;
	// saving the global setting removes this dependency.
	for _, provider := range []string{ProviderStripe, ProviderPayPal, ProviderWechat, ProviderAlipay} {
		values, readErr := s.readRawConfig(ctx, provider)
		if readErr != nil {
			return nil, false, readErr
		}
		if presets := normalizedRechargePresets(values["recharge_presets"]); len(presets) > 0 {
			return legacyRechargePackages(presets), false, nil
		}
	}
	return nil, false, nil
}

func normalizeRechargePackages(input []RechargePackage) ([]RechargePackage, bool, error) {
	result := make([]RechargePackage, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for index, item := range input {
		item.ID = strings.TrimSpace(item.ID)
		if item.ID == "" {
			// Keep IDs stable when reading a legacy or hand-written JSON setting.
			// The same package must validate on the public-list request and the
			// subsequent create-order request.
			item.ID = generatedPackageID(item, index)
		}
		if !ids.Valid(item.ID) || item.Name == "" || len(item.Name) > 128 || len(item.Description) > 500 {
			return nil, true, ErrInvalidRequest
		}
		if _, exists := seen[item.ID]; exists {
			return nil, true, ErrInvalidRequest
		}
		seen[item.ID] = struct{}{}
		item.Name = strings.TrimSpace(item.Name)
		item.Description = strings.TrimSpace(item.Description)
		item.Kind = strings.ToLower(strings.TrimSpace(item.Kind))
		if item.Kind == "" {
			item.Kind = "recharge"
		}
		if item.Kind != "recharge" && item.Kind != "subscription" {
			return nil, true, ErrInvalidRequest
		}
		item.Currency = strings.ToUpper(strings.TrimSpace(item.Currency))
		if len(item.Currency) != 3 {
			return nil, true, ErrInvalidRequest
		}
		amount, err := normalizeAmount(item.Amount, item.Currency)
		if err != nil {
			return nil, true, err
		}
		bonus := strings.TrimSpace(item.BonusAmount)
		if bonus == "" {
			bonus = "0"
		} else if bonus, err = normalizeNonNegativeAmount(bonus, item.Currency); err != nil {
			return nil, true, err
		}
		credited := strings.TrimSpace(item.CreditedAmount)
		computed := addAmounts(amount, bonus)
		if credited == "" {
			credited = computed
		} else if credited, err = normalizeAmount(credited, item.Currency); err != nil {
			return nil, true, err
		} else if credited != computed {
			return nil, true, ErrInvalidRequest
		}
		item.Amount, item.BonusAmount, item.CreditedAmount = amount, bonus, credited
		if item.ValidityDays < 0 || item.ValidityDays > 36500 {
			return nil, true, ErrInvalidRequest
		}
		for _, value := range []string{item.StartsAt, item.EndsAt} {
			if value != "" {
				if _, err := time.Parse(time.RFC3339, strings.TrimSpace(value)); err != nil {
					return nil, true, ErrInvalidRequest
				}
			}
		}
		if item.StartsAt != "" && item.EndsAt != "" {
			start, _ := time.Parse(time.RFC3339, item.StartsAt)
			end, _ := time.Parse(time.RFC3339, item.EndsAt)
			if !end.After(start) {
				return nil, true, ErrInvalidRequest
			}
		}
		if item.Kind == "subscription" && len(item.SubscriptionPlanCode) > 128 {
			return nil, true, ErrInvalidRequest
		}
		result = append(result, item)
	}
	return result, true, nil
}

func generatedPackageID(item RechargePackage, index int) string {
	payload := strings.Join([]string{item.Kind, item.Currency, item.Name, item.Amount, item.BonusAmount, fmt.Sprint(index)}, "\x00")
	sum := md5.Sum([]byte(payload))
	sum[6] = (sum[6] & 0x0f) | 0x30
	sum[8] = (sum[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

func normalizeNonNegativeAmount(value, currency string) (string, error) {
	rat, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	if !ok || rat.Sign() < 0 {
		return "", ErrInvalidRequest
	}
	if rat.Sign() == 0 {
		return "0", nil
	}
	return normalizeAmount(value, currency)
}

func addAmounts(left, right string) string {
	a, _ := new(big.Rat).SetString(left)
	b, _ := new(big.Rat).SetString(right)
	return trimDecimal(new(big.Rat).Add(a, b).FloatString(12))
}

func legacyRechargePackages(presets []string) []RechargePackage {
	result := make([]RechargePackage, 0, len(presets))
	for _, preset := range presets {
		amount := strings.TrimSpace(preset)
		if amount == "" {
			continue
		}
		result = append(result, RechargePackage{ID: legacyPackageID(amount, "USD"), Name: "USD " + amount, Kind: "recharge", Currency: "USD", Amount: amount, CreditedAmount: amount, BonusAmount: "0", Enabled: true})
	}
	return result
}

func packageAmounts(packages []RechargePackage) []string {
	result := make([]string, 0, len(packages))
	seen := map[string]struct{}{}
	for _, item := range packages {
		if !item.Enabled || item.Kind != "recharge" {
			continue
		}
		if _, ok := seen[item.Amount]; ok {
			continue
		}
		seen[item.Amount] = struct{}{}
		result = append(result, item.Amount)
	}
	return result
}

func publicRechargePackages(packages []RechargePackage, now time.Time) []RechargePackage {
	result := make([]RechargePackage, 0, len(packages))
	for _, item := range packages {
		if !item.Enabled || item.Kind != "recharge" {
			continue
		}
		if item.StartsAt != "" {
			start, _ := time.Parse(time.RFC3339, item.StartsAt)
			if now.Before(start) {
				continue
			}
		}
		if item.EndsAt != "" {
			end, _ := time.Parse(time.RFC3339, item.EndsAt)
			if !now.Before(end) {
				continue
			}
		}
		result = append(result, item)
	}
	return result
}

func isRechargePackageActive(item RechargePackage, now time.Time) bool {
	if item.StartsAt != "" {
		start, err := time.Parse(time.RFC3339, item.StartsAt)
		if err != nil || now.Before(start) {
			return false
		}
	}
	if item.EndsAt != "" {
		end, err := time.Parse(time.RFC3339, item.EndsAt)
		if err != nil || !now.Before(end) {
			return false
		}
	}
	return true
}

func legacyPackageID(amount, currency string) string {
	sum := md5.Sum([]byte(strings.ToUpper(strings.TrimSpace(currency)) + ":" + strings.TrimSpace(amount)))
	sum[6] = (sum[6] & 0x0f) | 0x30
	sum[8] = (sum[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

func (s *SQLService) UpdateRechargePresets(ctx context.Context, actorID string, presets []string) ([]string, error) {
	packages := legacyRechargePackages(presets)
	updated, err := s.UpdateRechargePackages(ctx, actorID, packages)
	return packageAmounts(updated), err
}

func (s *SQLService) UpdateRechargePackages(ctx context.Context, actorID string, packages []RechargePackage) ([]RechargePackage, error) {
	if s == nil || s.db == nil || strings.TrimSpace(actorID) == "" {
		return nil, ErrInvalidRequest
	}
	normalizedPackages, _, err := normalizeRechargePackages(packages)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(normalizedPackages)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO platform_settings (key, value, updated_by, updated_at)
		VALUES ('payment_recharge_packages', $1, $2::uuid, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()
	`, string(encoded), actorID); err != nil {
		return nil, err
	}
	return normalizedPackages, nil
}

func (s *SQLService) AdminUpdate(ctx context.Context, actorID, provider string, request ConfigUpdate) (ProviderConfig, error) {
	if s == nil || s.db == nil || s.box == nil || strings.TrimSpace(actorID) == "" {
		return ProviderConfig{}, ErrInvalidRequest
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if !supportedProvider(provider) || request.Values == nil {
		return ProviderConfig{}, ErrInvalidRequest
	}
	current, err := s.readRawConfig(ctx, provider)
	if err != nil {
		return ProviderConfig{}, err
	}
	for key, value := range request.Values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !validConfigKey(provider, key) || len(value) > 100000 {
			return ProviderConfig{}, ErrInvalidRequest
		}
		if _, secret := secretSet(provider, key); secret && value == "" {
			continue
		}
		if provider == ProviderStripe && key == "payment_method_types" {
			value, err = normalizeStripePaymentMethods(value)
			if err != nil {
				return ProviderConfig{}, err
			}
		}
		if key == "recharge_rate" {
			value, err = normalizeRechargeRate(value)
			if err != nil {
				return ProviderConfig{}, err
			}
		}
		if key == "recharge_presets" {
			value, err = normalizeRechargePresetsValue(value)
			if err != nil {
				return ProviderConfig{}, err
			}
		}
		if value == "" {
			delete(current, key)
		} else {
			current[key] = value
		}
	}
	for _, key := range request.Clear {
		key = strings.TrimSpace(key)
		if _, ok := secretSet(provider, key); !ok {
			return ProviderConfig{}, ErrInvalidRequest
		}
		delete(current, key)
	}
	if request.Enabled && !hasRequiredFields(provider, current) {
		return ProviderConfig{}, ErrProviderUnconfig
	}
	if err := validateProviderConfig(provider, current); err != nil {
		return ProviderConfig{}, err
	}
	raw, err := json.Marshal(current)
	if err != nil {
		return ProviderConfig{}, err
	}
	sealed, err := s.box.Seal(raw)
	if err != nil {
		return ProviderConfig{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO payment_provider_configs (provider, enabled, config_ciphertext, updated_by, updated_at)
		VALUES ($1, $2, $3, $4::uuid, now())
		ON CONFLICT (provider) DO UPDATE SET enabled = EXCLUDED.enabled,
		 config_ciphertext = EXCLUDED.config_ciphertext, updated_by = EXCLUDED.updated_by, updated_at = now()
	`, provider, request.Enabled, []byte(sealed), actorID)
	if err != nil {
		return ProviderConfig{}, err
	}
	return s.readConfig(ctx, provider)
}

func (s *SQLService) readRawConfig(ctx context.Context, provider string) (map[string]string, error) {
	var encoded []byte
	if err := s.db.QueryRowContext(ctx, `SELECT config_ciphertext FROM payment_provider_configs WHERE provider = $1`, provider).Scan(&encoded); errors.Is(err, sql.ErrNoRows) {
		return map[string]string{}, nil
	} else if err != nil {
		return nil, err
	}
	if len(encoded) == 0 {
		return map[string]string{}, nil
	}
	plain, err := s.box.Open(string(encoded))
	if err != nil {
		return nil, ErrUnavailable
	}
	values := map[string]string{}
	if err := json.Unmarshal(plain, &values); err != nil {
		return nil, ErrUnavailable
	}
	return values, nil
}

func (s *SQLService) readConfig(ctx context.Context, provider string) (ProviderConfig, error) {
	values, err := s.readRawConfig(ctx, provider)
	if err != nil {
		return ProviderConfig{}, err
	}
	var enabled bool
	var updatedAt sql.NullTime
	var updatedBy sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT enabled, updated_at, COALESCE(updated_by::text, '') FROM payment_provider_configs WHERE provider = $1`, provider).Scan(&enabled, &updatedAt, &updatedBy); errors.Is(err, sql.ErrNoRows) {
		enabled = false
	} else if err != nil {
		return ProviderConfig{}, err
	}
	public := map[string]string{}
	for key, value := range values {
		if _, secret := secretSet(provider, key); !secret {
			public[key] = value
		}
	}
	item := ProviderConfig{Provider: provider, Enabled: enabled, Values: public, Configured: hasRequiredFields(provider, values), SecretFields: append([]string(nil), secretFieldsByProvider[provider]...)}
	item.WebhookURL = providerWebhookURL(provider)
	if updatedAt.Valid {
		item.UpdatedAt = updatedAt.Time
	}
	item.UpdatedBy = updatedBy.String
	for _, key := range item.SecretFields {
		if _, ok := values[key]; ok {
			if item.Values == nil {
				item.Values = map[string]string{}
			}
			item.Values[key+"_configured"] = "true"
		}
	}
	return item, nil
}

func secretSet(provider, key string) (string, bool) {
	for _, candidate := range secretFieldsByProvider[provider] {
		if candidate == key {
			return candidate, true
		}
	}
	return "", false
}

func validConfigKey(provider, key string) bool {
	known := map[string]struct{}{
		"app_id": {}, "mch_id": {}, "serial_no": {}, "platform_certificate_serial_no": {}, "private_key_pem": {}, "api_v3_key": {}, "platform_certificate_pem": {}, "notify_url": {}, "api_base_url": {},
		"gateway": {}, "seller_id": {}, "alipay_public_key_pem": {}, "secret_key": {}, "publishable_key": {}, "webhook_secret": {}, "payment_method_types": {}, "client_id": {}, "client_secret": {}, "environment": {}, "webhook_id": {},
		"recharge_rate": {}, "recharge_presets": {},
	}
	if _, ok := known[key]; !ok || !supportedProvider(provider) {
		return false
	}
	_, ok := allowedFieldsByProvider[provider][key]
	return ok
}

func providerWebhookURL(provider string) string {
	switch provider {
	case ProviderWechat:
		return "/payments/webhooks/wechat"
	case ProviderAlipay:
		return "/payments/webhooks/alipay"
	case ProviderStripe:
		return "/payments/webhooks/stripe"
	case ProviderPayPal:
		return "/payments/webhooks/paypal"
	default:
		return ""
	}
}

func hasRequiredFields(provider string, values map[string]string) bool {
	for _, key := range requiredFieldsByProvider[provider] {
		if strings.TrimSpace(values[key]) == "" {
			return false
		}
	}
	return true
}

func validateProviderConfig(provider string, values map[string]string) error {
	if values["notify_url"] != "" {
		u, err := url.Parse(values["notify_url"])
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return ErrInvalidRequest
		}
	}
	for _, key := range []string{"api_base_url", "gateway"} {
		if value := strings.TrimSpace(values[key]); value != "" {
			u, err := url.Parse(value)
			if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return ErrInvalidRequest
			}
		}
	}
	if provider == ProviderPayPal && values["environment"] != "" && values["environment"] != "sandbox" && values["environment"] != "live" {
		return ErrInvalidRequest
	}
	if provider == ProviderWechat && strings.TrimSpace(values["api_v3_key"]) != "" && len(values["api_v3_key"]) != 32 {
		return ErrInvalidRequest
	}
	if provider == ProviderStripe {
		if _, err := normalizeStripePaymentMethods(values["payment_method_types"]); err != nil {
			return err
		}
	}
	if _, err := normalizeRechargeRate(values["recharge_rate"]); err != nil {
		return err
	}
	if _, err := normalizeRechargePresetsValue(values["recharge_presets"]); err != nil {
		return err
	}
	return nil
}

var stripePaymentMethods = map[string]struct{}{
	"card":       {},
	"alipay":     {},
	"wechat_pay": {},
}

func normalizeStripePaymentMethods(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parts := strings.Split(value, ",")
	normalized := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		method := strings.ToLower(strings.TrimSpace(part))
		if method == "" {
			return "", ErrInvalidRequest
		}
		if _, ok := stripePaymentMethods[method]; !ok {
			return "", ErrInvalidRequest
		}
		if _, duplicate := seen[method]; duplicate {
			return "", ErrInvalidRequest
		}
		seen[method] = struct{}{}
		normalized = append(normalized, method)
	}
	return strings.Join(normalized, ","), nil
}

func normalizeRechargeRate(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "1", nil
	}
	rat, ok := new(big.Rat).SetString(value)
	if !ok || rat.Sign() <= 0 || rat.Cmp(new(big.Rat).SetInt64(1000000)) > 0 {
		return "", ErrInvalidRequest
	}
	return strings.TrimRight(strings.TrimRight(rat.FloatString(12), "0"), "."), nil
}

func normalizedRechargeRate(value string) string {
	normalized, err := normalizeRechargeRate(value)
	if err != nil || normalized == "" {
		return "1"
	}
	return normalized
}

func normalizeRechargePresetsValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		normalized, err := normalizeAmount(strings.TrimSpace(part), "USD")
		if err != nil {
			return "", ErrInvalidRequest
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		values = append(values, normalized)
		if len(values) > 24 {
			return "", ErrInvalidRequest
		}
	}
	return strings.Join(values, ","), nil
}

func normalizedRechargePresets(value string) []string {
	normalized, err := normalizeRechargePresetsValue(value)
	if err != nil || normalized == "" {
		return nil
	}
	return strings.Split(normalized, ",")
}

func applyRechargeRate(amount, rate, currency string) (string, error) {
	base, err := normalizeAmount(amount, currency)
	if err != nil {
		return "", err
	}
	rateValue, ok := new(big.Rat).SetString(normalizedRechargeRate(rate))
	if !ok || rateValue.Sign() <= 0 {
		return "", ErrInvalidRequest
	}
	value, _ := new(big.Rat).SetString(base)
	value.Mul(value, rateValue)
	decimals := decimalPlaces(currency)
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	scaled := new(big.Rat).Mul(value, new(big.Rat).SetInt(scale))
	if scaled.Denom().Cmp(big.NewInt(1)) != 0 {
		return "", ErrInvalidRequest
	}
	return trimDecimal(value.FloatString(12)), nil
}

func (s *SQLService) CreateOrder(ctx context.Context, request CreateRequest) (CreateResult, error) {
	if s == nil || s.db == nil || s.biller == nil {
		return CreateResult{}, ErrUnavailable
	}
	request.TenantID, request.UserID, request.Provider = strings.TrimSpace(request.TenantID), strings.TrimSpace(request.UserID), strings.ToLower(strings.TrimSpace(request.Provider))
	request.Amount, request.Currency, request.IdempotencyKey, request.ReturnURL = strings.TrimSpace(request.Amount), strings.ToUpper(strings.TrimSpace(request.Currency)), strings.TrimSpace(request.IdempotencyKey), strings.TrimSpace(request.ReturnURL)
	request.PackageID = strings.TrimSpace(request.PackageID)
	if !ids.Valid(request.TenantID) || !ids.Valid(request.UserID) || !supportedProvider(request.Provider) || request.IdempotencyKey == "" || len(request.IdempotencyKey) > 128 || !validHTTPSURL(request.ReturnURL) {
		return CreateResult{}, ErrInvalidRequest
	}
	paidAmount, err := normalizeAmount(request.Amount, request.Currency)
	if err != nil {
		return CreateResult{}, err
	}
	if !providerSupportsCurrency(request.Provider, request.Currency) {
		return CreateResult{}, ErrCurrencyMismatch
	}
	account, err := s.biller.GetPrepaidAccount(ctx, request.TenantID, request.Currency)
	if err != nil {
		return CreateResult{}, err
	}
	if account.Currency != request.Currency {
		return CreateResult{}, ErrCurrencyMismatch
	}
	config, err := s.readRawConfig(ctx, request.Provider)
	if err != nil {
		return CreateResult{}, err
	}
	var enabled bool
	if err := s.db.QueryRowContext(ctx, `SELECT enabled FROM payment_provider_configs WHERE provider = $1`, request.Provider).Scan(&enabled); errors.Is(err, sql.ErrNoRows) {
		return CreateResult{}, ErrProviderDisabled
	} else if err != nil {
		return CreateResult{}, err
	}
	if !enabled {
		return CreateResult{}, ErrProviderDisabled
	}
	if !hasRequiredFields(request.Provider, config) {
		return CreateResult{}, ErrProviderUnconfig
	}
	var selectedPackage *RechargePackage
	if request.PackageID != "" {
		packages, packageErr := s.GetRechargePackages(ctx)
		if packageErr != nil {
			return CreateResult{}, packageErr
		}
		for index := range packages {
			candidate := packages[index]
			if candidate.ID == request.PackageID && candidate.Enabled && candidate.Kind == "recharge" && strings.EqualFold(candidate.Currency, request.Currency) && isRechargePackageActive(candidate, s.clock()) {
				selectedPackage = &candidate
				break
			}
		}
		if selectedPackage == nil || !sameAmount(paidAmount, selectedPackage.Amount, request.Currency) {
			return CreateResult{}, ErrInvalidRequest
		}
	}
	rate := normalizedRechargeRate(config["recharge_rate"])
	creditBasis := paidAmount
	bonusAmount := "0"
	validUntil := (*time.Time)(nil)
	if selectedPackage != nil {
		creditBasis = selectedPackage.CreditedAmount
		bonusAmount = selectedPackage.BonusAmount
		if selectedPackage.ValidityDays > 0 {
			value := s.clock().Add(time.Duration(selectedPackage.ValidityDays) * 24 * time.Hour)
			validUntil = &value
		}
	}
	creditedAmount, err := applyRechargeRate(creditBasis, rate, request.Currency)
	if err != nil {
		return CreateResult{}, err
	}
	var existing Order
	if err := s.scanOrder(s.db.QueryRowContext(ctx, orderSelect+` WHERE po.idempotency_key = $1 AND po.tenant_id = $2::uuid`, request.IdempotencyKey, request.TenantID), &existing); err == nil {
		return CreateResult{Order: existing}, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return CreateResult{}, err
	}
	id, err := ids.New()
	if err != nil {
		return CreateResult{}, err
	}
	merchant := "recharge_" + strings.ReplaceAll(id, "-", "")
	expires := s.clock().Add(orderLifetime)
	packageID, packageBonus := "", "0"
	if selectedPackage != nil {
		packageID, packageBonus = selectedPackage.ID, bonusAmount
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO payment_orders (id, tenant_id, user_id, provider, merchant_order_no, amount, credited_amount, recharge_rate, currency, package_id, bonus_amount, valid_until, expires_at, idempotency_key) VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6::numeric, $7::numeric, $8::numeric, $9, NULLIF($10, ''), $11::numeric, $12, $13, $14)`, id, request.TenantID, request.UserID, request.Provider, merchant, paidAmount, creditedAmount, rate, request.Currency, packageID, packageBonus, validUntil, expires, request.IdempotencyKey)
	if err != nil {
		if isUniqueViolation(err) {
			return s.getExistingByIdempotency(ctx, request.TenantID, request.IdempotencyKey)
		}
		return CreateResult{}, err
	}
	order, err := s.getOrderByID(ctx, request.TenantID, id)
	if err != nil {
		return CreateResult{}, err
	}
	providerResult, err := s.createProviderOrder(ctx, order, config, request.ReturnURL)
	if err != nil {
		_ = s.markFailed(ctx, id, err.Error())
		return CreateResult{}, err
	}
	updated, err := s.updateProviderOrder(ctx, order, providerResult)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{Order: updated}, nil
}

func (s *SQLService) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *SQLService) getExistingByIdempotency(ctx context.Context, tenantID, key string) (CreateResult, error) {
	var order Order
	err := s.scanOrder(s.db.QueryRowContext(ctx, orderSelect+` WHERE po.tenant_id = $1::uuid AND po.idempotency_key = $2`, tenantID, key), &order)
	if errors.Is(err, sql.ErrNoRows) {
		return CreateResult{}, ErrInvalidRequest
	}
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{Order: order}, nil
}

func (s *SQLService) GetOrder(ctx context.Context, tenantID, orderID string) (Order, error) {
	if s == nil || s.db == nil || !ids.Valid(strings.TrimSpace(tenantID)) || !ids.Valid(strings.TrimSpace(orderID)) {
		return Order{}, ErrInvalidRequest
	}
	return s.getOrderByID(ctx, tenantID, orderID)
}

func (s *SQLService) getOrderByID(ctx context.Context, tenantID, orderID string) (Order, error) {
	var order Order
	err := s.scanOrder(s.db.QueryRowContext(ctx, orderSelect+` WHERE po.id = $1::uuid AND po.tenant_id = $2::uuid`, orderID, tenantID), &order)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}
	return order, err
}

func (s *SQLService) ListOrders(ctx context.Context, tenantID string, query OrderQuery) (OrderList, error) {
	if s == nil || s.db == nil || !ids.Valid(strings.TrimSpace(tenantID)) {
		return OrderList{}, ErrInvalidRequest
	}
	if query.Limit <= 0 {
		query.Limit = 20
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM payment_orders WHERE tenant_id = $1::uuid`, tenantID).Scan(&total); err != nil {
		return OrderList{}, err
	}
	rows, err := s.db.QueryContext(ctx, orderSelect+` WHERE po.tenant_id = $1::uuid ORDER BY po.created_at DESC LIMIT $2 OFFSET $3`, tenantID, query.Limit, query.Offset)
	if err != nil {
		return OrderList{}, err
	}
	defer rows.Close()
	orders := make([]Order, 0, query.Limit)
	for rows.Next() {
		var order Order
		if err := s.scanOrder(rows, &order); err != nil {
			return OrderList{}, err
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		return OrderList{}, err
	}
	return OrderList{Orders: orders, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

const orderSelect = `SELECT po.id::text, po.tenant_id::text, po.user_id::text, po.provider, po.merchant_order_no, COALESCE(po.provider_order_id, ''), po.amount::text, po.credited_amount::text, po.recharge_rate::text, po.currency, COALESCE(po.package_id::text, ''), COALESCE(po.bonus_amount::text, '0'), po.valid_until, po.status, COALESCE(po.checkout_url, ''), COALESCE(po.checkout_client_secret, ''), COALESCE(po.qr_code, ''), COALESCE(po.failure_reason, ''), po.paid_at, po.expires_at, po.created_at, po.updated_at FROM payment_orders po`

type scanner interface{ Scan(...any) error }

func (s *SQLService) scanOrder(row scanner, order *Order) error {
	var paid, validUntil sql.NullTime
	err := row.Scan(&order.ID, &order.TenantID, &order.UserID, &order.Provider, &order.MerchantOrderNo, &order.ProviderOrderID, &order.Amount, &order.CreditedAmount, &order.RechargeRate, &order.Currency, &order.PackageID, &order.BonusAmount, &validUntil, &order.Status, &order.CheckoutURL, &order.CheckoutClientSecret, &order.QRCode, &order.FailureReason, &paid, &order.ExpiresAt, &order.CreatedAt, &order.UpdatedAt)
	if validUntil.Valid {
		order.ValidUntil = &validUntil.Time
	}
	if paid.Valid {
		order.PaidAt = &paid.Time
	}
	return err
}

type providerOrder struct {
	ProviderOrderID, CheckoutURL, QRCode, CheckoutClientSecret string
	Raw                                                        json.RawMessage
}

func (s *SQLService) createProviderOrder(ctx context.Context, order Order, config map[string]string, returnURL string) (providerOrder, error) {
	switch order.Provider {
	case ProviderStripe:
		return s.createStripeOrder(ctx, order, config, returnURL)
	case ProviderPayPal:
		return s.createPayPalOrder(ctx, order, config, returnURL)
	case ProviderWechat:
		return s.createWechatOrder(ctx, order, config)
	case ProviderAlipay:
		return s.createAlipayOrder(ctx, order, config)
	default:
		return providerOrder{}, ErrInvalidRequest
	}
}

func (s *SQLService) updateProviderOrder(ctx context.Context, order Order, result providerOrder) (Order, error) {
	_, err := s.db.ExecContext(ctx, `UPDATE payment_orders SET provider_order_id = NULLIF($2, ''), checkout_url = NULLIF($3, ''), qr_code = NULLIF($4, ''), checkout_client_secret = NULLIF($5, ''), response_payload_json = $6::jsonb, updated_at = now() WHERE id = $1::uuid`, order.ID, result.ProviderOrderID, result.CheckoutURL, result.QRCode, result.CheckoutClientSecret, []byte(orEmptyJSON(result.Raw)))
	if err != nil {
		return Order{}, err
	}
	return s.getOrderByID(ctx, order.TenantID, order.ID)
}

func orEmptyJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return `{}`
	}
	return string(raw)
}

func (s *SQLService) markFailed(ctx context.Context, orderID, reason string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE payment_orders SET status = 'failed', failure_reason = LEFT($2, 1000), updated_at = now() WHERE id = $1::uuid AND status = 'pending'`, orderID, reason)
	return err
}

func (s *SQLService) HandleWebhook(ctx context.Context, provider string, headers http.Header, body []byte) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if !supportedProvider(provider) || len(body) == 0 || len(body) > 2<<20 {
		return ErrCallbackInvalid
	}
	config, err := s.readRawConfig(ctx, provider)
	if err != nil {
		return err
	}
	if !hasRequiredFields(provider, config) {
		return ErrProviderUnconfig
	}
	var payment callbackPayment
	switch provider {
	case ProviderStripe:
		payment, err = verifyStripeWebhook(config, headers, body)
	case ProviderPayPal:
		payment, err = s.verifyPayPalWebhook(ctx, config, headers, body)
	case ProviderWechat:
		payment, err = verifyWechatWebhook(config, headers, body)
	case ProviderAlipay:
		payment, err = verifyAlipayWebhook(config, body)
	}
	if err != nil {
		return err
	}
	payment.Provider = provider
	if !payment.Paid {
		if payment.MerchantOrderNo != "" || payment.ProviderOrderID != "" {
			return nil
		}
		return ErrCallbackInvalid
	}
	return s.settleCallback(ctx, payment)
}

type callbackPayment struct {
	Provider, MerchantOrderNo, ProviderOrderID, Currency, Amount string
	Paid                                                         bool
}

func (s *SQLService) settleCallback(ctx context.Context, payment callbackPayment) error {
	var order Order
	if payment.MerchantOrderNo == "" && payment.ProviderOrderID == "" {
		return ErrCallbackInvalid
	}
	query := orderSelect + ` WHERE po.merchant_order_no = NULLIF($1, '')`
	args := []any{payment.MerchantOrderNo}
	if payment.MerchantOrderNo == "" {
		query = orderSelect + ` WHERE po.provider_order_id = NULLIF($1, '')`
		args = []any{payment.ProviderOrderID}
	}
	if err := s.scanOrder(s.db.QueryRowContext(ctx, query, args...), &order); errors.Is(err, sql.ErrNoRows) {
		return ErrOrderNotFound
	} else if err != nil {
		return err
	}
	if payment.Provider != "" && order.Provider != payment.Provider {
		return ErrCallbackInvalid
	}
	if order.Status == "paid" {
		return nil
	}
	if order.Status != "pending" {
		return ErrOrderClosed
	}
	if payment.Currency != "" && !strings.EqualFold(order.Currency, payment.Currency) {
		return ErrCurrencyMismatch
	}
	if !sameAmount(order.Amount, payment.Amount, order.Currency) {
		return ErrAmountMismatch
	}
	if s.clock().After(order.ExpiresAt) {
		_, _ = s.db.ExecContext(ctx, `UPDATE payment_orders SET status = 'expired', updated_at = now() WHERE id = $1::uuid AND status = 'pending'`, order.ID)
		return ErrOrderClosed
	}
	if _, err := s.biller.Credit(ctx, "payment:"+order.Provider, billing.CreditRequest{TenantID: order.TenantID, Currency: order.Currency, Amount: order.CreditedAmount, IdempotencyKey: "payment:credit:" + order.ID, Reason: "online recharge " + order.MerchantOrderNo}); err != nil && !errors.Is(err, billing.ErrDuplicateTransaction) {
		return err
	}
	now := s.clock()
	_, err := s.db.ExecContext(ctx, `UPDATE payment_orders SET status = 'paid', provider_order_id = COALESCE(NULLIF($2, ''), provider_order_id), paid_at = $3, updated_at = $3 WHERE id = $1::uuid AND status = 'pending'`, order.ID, payment.ProviderOrderID, now)
	return err
}

func (s *SQLService) CapturePayPal(ctx context.Context, tenantID, orderID string) error {
	order, err := s.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return err
	}
	if order.Provider != ProviderPayPal {
		return ErrInvalidRequest
	}
	if order.Status == "paid" {
		return nil
	}
	if order.ProviderOrderID == "" {
		return ErrInvalidRequest
	}
	config, err := s.readRawConfig(ctx, ProviderPayPal)
	if err != nil {
		return err
	}
	token, err := paypalAccessToken(ctx, s.httpClient(), config)
	if err != nil {
		return err
	}
	base := paypalBaseURL(config)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v2/checkout/orders/"+url.PathEscape(order.ProviderOrderID)+"/capture", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("paypal capture returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		ID            string `json:"id"`
		Status        string `json:"status"`
		PurchaseUnits []struct {
			Payments struct {
				Captures []struct {
					Amount struct {
						Value        string `json:"value"`
						CurrencyCode string `json:"currency_code"`
					} `json:"amount"`
					Status string `json:"status"`
				} `json:"captures"`
			} `json:"payments"`
		} `json:"purchase_units"`
	}
	if json.Unmarshal(raw, &payload) != nil || !strings.EqualFold(payload.Status, "COMPLETED") {
		return ErrCallbackInvalid
	}
	amount, currency, captureStatus := "", "", ""
	if len(payload.PurchaseUnits) > 0 && len(payload.PurchaseUnits[0].Payments.Captures) > 0 {
		capture := payload.PurchaseUnits[0].Payments.Captures[0]
		amount, currency, captureStatus = capture.Amount.Value, capture.Amount.CurrencyCode, capture.Status
	}
	if captureStatus != "COMPLETED" || amount == "" || currency == "" {
		return ErrCallbackInvalid
	}
	return s.settleCallback(ctx, callbackPayment{Provider: ProviderPayPal, MerchantOrderNo: order.MerchantOrderNo, ProviderOrderID: order.ProviderOrderID, Currency: currency, Amount: amount, Paid: true})
}

func normalizeAmount(value, currency string) (string, error) {
	rat, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	if !ok || rat.Sign() <= 0 {
		return "", ErrInvalidRequest
	}
	decimals := decimalPlaces(currency)
	scaled := new(big.Rat).Mul(rat, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)))
	if scaled.Denom().Cmp(big.NewInt(1)) != 0 {
		return "", ErrInvalidRequest
	}
	return trimDecimal(rat.FloatString(12)), nil
}
func sameAmount(left, right, currency string) bool {
	a, err := normalizeAmount(left, currency)
	if err != nil {
		return false
	}
	b, err := normalizeAmount(right, currency)
	return err == nil && a == b
}
func decimalPlaces(currency string) int {
	switch strings.ToUpper(currency) {
	case "JPY", "KRW", "VND":
		return 0
	default:
		return 2
	}
}
func trimDecimal(value string) string {
	value = strings.TrimRight(strings.TrimRight(value, "0"), ".")
	if value == "" || value == "-0" {
		return "0"
	}
	return value
}
func minorAmount(value, currency string) (string, error) {
	normalized, err := normalizeAmount(value, currency)
	if err != nil {
		return "", err
	}
	rat, _ := new(big.Rat).SetString(normalized)
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimalPlaces(currency))), nil)
	return new(big.Int).Quo(new(big.Int).Mul(rat.Num(), scale), rat.Denom()).String(), nil
}
func validHTTPSURL(value string) bool {
	u, err := url.Parse(strings.TrimSpace(value))
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil
}

func providerSupportsCurrency(provider, currency string) bool {
	switch provider {
	case ProviderWechat, ProviderAlipay:
		return strings.EqualFold(strings.TrimSpace(currency), "CNY")
	default:
		return strings.TrimSpace(currency) != ""
	}
}

func isUniqueViolation(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "duplicate key") || strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}

var _ Service = (*SQLService)(nil)
