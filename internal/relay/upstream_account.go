package relay

import (
	"context"
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

	"ai-token/internal/ids"
)

type upstreamAccountSnapshot struct {
	balance        string
	balanceUnit    string
	balanceTotal   string
	balanceUsed    string
	planName       string
	rateMultiplier string
}

var (
	errUpstreamAccountUnsupported = errors.New("upstream account information is not supported")
	errUpstreamAccountNotFound    = errors.New("upstream account information was not returned")
)

type upstreamAccountHTTPError struct {
	status int
}

func (e *upstreamAccountHTTPError) Error() string {
	return fmt.Sprintf("upstream account endpoint returned HTTP %d", e.status)
}

// SyncChannelAccount refreshes only the administrator-facing account snapshot.
// It intentionally does not call any channel health recorder and does not
// touch the fields used by SelectCandidates or billing.
func (r *SQLChannelRouter) SyncChannelAccount(ctx context.Context, actorID, channelID string) (ChannelSummary, error) {
	if r == nil || r.db == nil {
		return ChannelSummary{}, ErrUnavailable
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" || !ids.Valid(channelID) {
		return ChannelSummary{}, ErrInvalidRequest
	}
	_ = actorID

	var provider, baseURL, integration, credentialRef, accountUserID string
	err := r.db.QueryRowContext(ctx, `
		SELECT provider, base_url, upstream_integration,
		       upstream_account_credential_ref, upstream_account_user_id
		FROM channels
		WHERE id = $1::uuid
		  AND deleted_at IS NULL
	`, channelID).Scan(&provider, &baseURL, &integration, &credentialRef, &accountUserID)
	if errors.Is(err, sql.ErrNoRows) {
		return ChannelSummary{}, ErrChannelNotFound
	}
	if err != nil {
		return ChannelSummary{}, err
	}
	integration = normalizeUpstreamIntegration(integration)

	if _, err := r.db.ExecContext(ctx, `
		UPDATE channels
		SET upstream_account_last_attempt_at = now(),
		    upstream_account_sync_status = 'pending',
		    upstream_account_sync_error = ''
		WHERE id = $1::uuid
		  AND deleted_at IS NULL
	`, channelID); err != nil {
		return ChannelSummary{}, err
	}

	if integration == UpstreamIntegrationOfficial || integration == UpstreamIntegrationOther || integration == "" {
		if err := r.finishUnsupportedAccountSync(ctx, channelID); err != nil {
			return ChannelSummary{}, err
		}
		return r.GetChannel(ctx, channelID)
	}
	secret, err := r.resolveAccountSecret(ctx, credentialRef)
	if err != nil {
		if updateErr := r.finishFailedAccountSync(ctx, channelID, "未配置独立的上游账户查询凭据"); updateErr != nil {
			return ChannelSummary{}, updateErr
		}
		return r.GetChannel(ctx, channelID)
	}

	snapshot, fetchErr := fetchUpstreamAccount(ctx, integration, provider, baseURL, secret, accountUserID)
	if fetchErr != nil {
		if updateErr := r.finishFailedAccountSync(ctx, channelID, safeAccountSyncError(fetchErr)); updateErr != nil {
			return ChannelSummary{}, updateErr
		}
		return r.GetChannel(ctx, channelID)
	}
	if err := r.finishSuccessfulAccountSync(ctx, channelID, snapshot); err != nil {
		return ChannelSummary{}, err
	}
	_ = actorID // kept in the signature for audit-compatible future extensions
	return r.GetChannel(ctx, channelID)
}

func (r *SQLChannelRouter) resolveAccountSecret(ctx context.Context, ref string) (string, error) {
	if r == nil || r.db == nil || r.box == nil {
		return "", ErrCredentialUnavailable
	}
	ref = strings.TrimSpace(ref)
	if !strings.HasPrefix(ref, "account-secret:") {
		return "", ErrCredentialUnavailable
	}
	secretID := strings.TrimSpace(strings.TrimPrefix(ref, "account-secret:"))
	if secretID == "" || !ids.Valid(secretID) {
		return "", ErrCredentialUnavailable
	}
	var encrypted []byte
	if err := r.db.QueryRowContext(ctx, `
		SELECT encrypted_secret
		FROM channel_account_secrets
		WHERE id = $1::uuid
		  AND revoked_at IS NULL
	`, secretID).Scan(&encrypted); err != nil {
		return "", ErrCredentialUnavailable
	}
	secret, err := r.box.Open(string(encrypted))
	if err != nil || strings.TrimSpace(string(secret)) == "" {
		return "", ErrCredentialUnavailable
	}
	return strings.TrimSpace(string(secret)), nil
}

func (r *SQLChannelRouter) finishUnsupportedAccountSync(ctx context.Context, channelID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE channels
		SET upstream_balance = NULL,
		    upstream_balance_unit = '',
		    upstream_balance_total = NULL,
		    upstream_balance_used = NULL,
		    upstream_account_plan_name = '',
		    upstream_rate_multiplier = NULL,
		    upstream_account_sync_status = 'not_supported',
		    upstream_account_sync_error = '',
		    upstream_account_synced_at = NULL,
		    upstream_account_last_attempt_at = COALESCE(upstream_account_last_attempt_at, now())
		WHERE id = $1::uuid
		  AND deleted_at IS NULL
	`, channelID)
	return err
}

func (r *SQLChannelRouter) finishFailedAccountSync(ctx context.Context, channelID, message string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE channels
		SET upstream_account_sync_status = 'failed',
		    upstream_account_sync_error = $2
		WHERE id = $1::uuid
		  AND deleted_at IS NULL
	`, channelID, strings.TrimSpace(message))
	return err
}

func (r *SQLChannelRouter) finishSuccessfulAccountSync(ctx context.Context, channelID string, snapshot upstreamAccountSnapshot) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE channels
		SET upstream_balance = CASE
		        WHEN NULLIF($2, '') IS NULL THEN NULL
		        ELSE $2::numeric
		    END,
		    upstream_balance_unit = CASE
		        WHEN NULLIF($2, '') IS NULL THEN ''
		        ELSE $3
		    END,
		    upstream_balance_total = CASE
		        WHEN NULLIF($4, '') IS NULL THEN NULL
		        ELSE $4::numeric
		    END,
		    upstream_balance_used = CASE
		        WHEN NULLIF($5, '') IS NULL THEN NULL
		        ELSE $5::numeric
		    END,
		    upstream_account_plan_name = $6,
		    upstream_rate_multiplier = CASE
		        WHEN NULLIF($7, '') IS NULL THEN NULL
		        ELSE $7::numeric
		    END,
		    upstream_account_sync_status = 'success',
		    upstream_account_sync_error = '',
		    upstream_account_synced_at = now()
		WHERE id = $1::uuid
		  AND deleted_at IS NULL
	`, channelID, snapshot.balance, snapshot.balanceUnit, snapshot.balanceTotal, snapshot.balanceUsed, snapshot.planName, snapshot.rateMultiplier)
	return err
}

func fetchUpstreamAccount(ctx context.Context, integration, provider, baseURL, credential, accountUserID string) (upstreamAccountSnapshot, error) {
	client, err := providerHTTPClient(baseURL)
	if err != nil {
		return upstreamAccountSnapshot{}, err
	}
	client.Timeout = 10 * time.Second

	switch integration {
	case UpstreamIntegrationNewAPI:
		extraHeaders := map[string]string{}
		// Older NewAPI deployments used this header to select the user. Newer
		// deployments authenticate the user directly from the dashboard PAT and
		// explicitly no longer require New-Api-User. Send it only when configured
		// so both generations remain compatible.
		if strings.TrimSpace(accountUserID) != "" {
			extraHeaders["New-Api-User"] = strings.TrimSpace(accountUserID)
		}
		body, err := requestUpstreamAccountJSON(ctx, client, baseURL, "/api/user/self", credential, extraHeaders)
		if err != nil {
			return upstreamAccountSnapshot{}, err
		}
		snapshot, err := parseNewAPIAccount(body)
		if err != nil {
			return upstreamAccountSnapshot{}, err
		}
		// NewAPI exposes the effective group ratio on the user-groups endpoint,
		// not on /api/user/self. This is best-effort: balance data remains useful
		// when an older deployment does not expose the secondary endpoint.
		groupsBody, groupsErr := requestUpstreamAccountJSON(ctx, client, baseURL, "/api/user/self/groups", credential, extraHeaders)
		if groupsErr == nil {
			if ratio := parseNewAPIGroupRate(groupsBody, snapshot.planName); ratio != "" {
				snapshot.rateMultiplier = ratio
			}
		}
		return snapshot, nil
	case UpstreamIntegrationSub2API:
		body, err := requestUpstreamAccountJSON(ctx, client, baseURL, "/v1/usage", credential, nil)
		if err != nil {
			return upstreamAccountSnapshot{}, err
		}
		snapshot, err := parseSub2APIAccount(body)
		if err != nil {
			return upstreamAccountSnapshot{}, err
		}
		// Sub2API keeps the effective key multiplier on its dedicated billing
		// endpoint; /v1/usage only reports balance and usage.
		billingBody, billingErr := requestUpstreamAccountJSON(ctx, client, baseURL, "/v1/sub2api/billing", credential, nil)
		if billingErr == nil {
			if ratio := parseSub2APIBillingRate(billingBody); ratio != "" {
				snapshot.rateMultiplier = ratio
			}
		}
		return snapshot, nil
	default:
		return upstreamAccountSnapshot{}, errUpstreamAccountUnsupported
	}
}

func requestUpstreamAccountJSON(ctx context.Context, client *http.Client, baseURL, path, credential string, extraHeaders map[string]string) (json.RawMessage, error) {
	endpoint, err := accountEndpointURL(baseURL, path)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(credential))
	request.Header.Set("User-Agent", "ai-token-upstream-account-sync/1")
	for key, value := range extraHeaders {
		if strings.TrimSpace(key) != "" {
			request.Header.Set(key, value)
		}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, &upstreamAccountHTTPError{status: response.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

func accountEndpointURL(rawBaseURL, path string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", ErrInvalidRequest
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	for _, suffix := range []string{"/api/v1", "/v1beta", "/v1", "/api"} {
		if strings.HasSuffix(basePath, suffix) {
			basePath = strings.TrimSuffix(basePath, suffix)
			break
		}
	}
	parsed.Path = strings.TrimRight(basePath, "/") + "/" + strings.TrimLeft(path, "/")
	parsed.RawPath = ""
	return parsed.String(), nil
}

func parseNewAPIAccount(body json.RawMessage) (upstreamAccountSnapshot, error) {
	payload, err := decodeAccountData(body, true)
	if err != nil {
		return upstreamAccountSnapshot{}, err
	}
	quota := rawAccountNumber(payload, "quota")
	if quota == "" {
		return upstreamAccountSnapshot{}, errUpstreamAccountNotFound
	}
	balance := divideAccountNumber(quota, 500000)
	usedQuota := rawAccountNumber(payload, "used_quota")
	used := divideAccountNumber(usedQuota, 500000)
	total := addAccountNumbers(balance, used)
	planName := rawAccountString(payload, "group")
	if planName == "" {
		planName = "默认套餐"
	}
	return upstreamAccountSnapshot{
		balance:        balance,
		balanceUnit:    "USD",
		balanceTotal:   total,
		balanceUsed:    used,
		planName:       planName,
		rateMultiplier: firstAccountNumber(payload, "group_ratio", "rate_multiplier", "ratio", "multiplier"),
	}, nil
}

func parseNewAPIGroupRate(body json.RawMessage, groupName string) string {
	payload, err := decodeAccountData(body, false)
	if err != nil {
		return ""
	}
	groupName = strings.TrimSpace(groupName)
	if groupName != "" {
		if raw, ok := payload[groupName]; ok {
			var value any
			decoder := json.NewDecoder(strings.NewReader(string(raw)))
			decoder.UseNumber()
			if decoder.Decode(&value) == nil {
				if number := accountNumber(value); number != "" {
					return number
				}
				if object, ok := value.(map[string]any); ok {
					for _, key := range []string{"ratio", "group_ratio", "rate_multiplier", "multiplier"} {
						if number := accountNumber(object[key]); number != "" {
							return number
						}
					}
				}
			}
		}
	}
	// Some compatible wrappers return a single group without preserving the
	// group name. Accept that unambiguous form, but never guess among multiple
	// ratios.
	var found string
	for _, raw := range payload {
		var value any
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		if decoder.Decode(&value) != nil {
			continue
		}
		var number string
		if object, ok := value.(map[string]any); ok {
			for _, key := range []string{"ratio", "group_ratio", "rate_multiplier", "multiplier"} {
				if number = accountNumber(object[key]); number != "" {
					break
				}
			}
		} else {
			number = accountNumber(value)
		}
		if number == "" {
			continue
		}
		if found != "" && found != number {
			return ""
		}
		found = number
	}
	return found
}

func parseSub2APIAccount(body json.RawMessage) (upstreamAccountSnapshot, error) {
	payload, err := decodeAccountData(body, false)
	if err != nil {
		return upstreamAccountSnapshot{}, err
	}
	if rawAccountBool(payload, "isValid") == "false" {
		return upstreamAccountSnapshot{}, errors.New("upstream account response was unsuccessful")
	}
	balance := rawAccountNumber(payload, "remaining")
	quota, _ := rawAccountObject(payload, "quota")
	if balance == "" {
		balance = rawAccountNumber(quota, "remaining")
	}
	if balance == "" {
		balance = rawAccountNumber(payload, "balance")
	}
	if balance == "" {
		return upstreamAccountSnapshot{}, errUpstreamAccountNotFound
	}
	total := firstAccountNumber(payload, "total")
	used := firstAccountNumber(payload, "used")
	if total == "" {
		total = rawAccountNumber(quota, "limit")
	}
	if used == "" {
		used = rawAccountNumber(quota, "used")
	}
	unit := firstAccountString(payload, "unit")
	if unit == "" {
		unit = rawAccountString(quota, "unit")
	}
	if unit == "" {
		unit = "USD"
	}
	planName := firstAccountString(payload, "planName", "plan_name")
	if planName == "" {
		if len(quota) > 0 {
			planName = "API Key 额度"
		} else {
			planName = "钱包余额"
		}
	}
	return upstreamAccountSnapshot{
		balance:        balance,
		balanceUnit:    unit,
		balanceTotal:   total,
		balanceUsed:    used,
		planName:       planName,
		rateMultiplier: firstAccountNumber(payload, "rate_multiplier", "multiplier", "ratio"),
	}, nil
}

func parseSub2APIBillingRate(body json.RawMessage) string {
	payload, err := decodeAccountData(body, false)
	if err != nil {
		return ""
	}
	return firstAccountNumber(payload,
		"effective_rate_multiplier",
		"resolved_rate_multiplier",
		"group_rate_multiplier",
		"rate_multiplier",
		"multiplier",
		"ratio",
	)
}

func decodeAccountData(body json.RawMessage, requireSuccess bool) (map[string]json.RawMessage, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	if root == nil {
		return nil, errUpstreamAccountNotFound
	}
	if rawSuccess, ok := root["success"]; ok {
		var success bool
		if err := json.Unmarshal(rawSuccess, &success); err != nil || !success {
			return nil, errors.New("upstream account response was unsuccessful")
		}
	} else if requireSuccess {
		return nil, errors.New("upstream account response was unsuccessful")
	}
	if rawData, ok := root["data"]; ok {
		var data map[string]json.RawMessage
		if err := json.Unmarshal(rawData, &data); err != nil || data == nil {
			return nil, errUpstreamAccountNotFound
		}
		return data, nil
	}
	return root, nil
}

func rawAccountNumber(payload map[string]json.RawMessage, key string) string {
	if payload == nil {
		return ""
	}
	raw, ok := payload[key]
	if !ok {
		return ""
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return ""
	}
	return accountNumber(value)
}

func rawAccountString(payload map[string]json.RawMessage, key string) string {
	if payload == nil {
		return ""
	}
	raw, ok := payload[key]
	if !ok {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func rawAccountBool(payload map[string]json.RawMessage, key string) string {
	if payload == nil {
		return ""
	}
	raw, ok := payload[key]
	if !ok {
		return ""
	}
	var value bool
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	if value {
		return "true"
	}
	return "false"
}

func rawAccountObject(payload map[string]json.RawMessage, key string) (map[string]json.RawMessage, bool) {
	if payload == nil {
		return nil, false
	}
	raw, ok := payload[key]
	if !ok {
		return nil, false
	}
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return nil, false
	}
	return value, true
}

func firstAccountNumber(payload map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		if value := rawAccountNumber(payload, key); value != "" {
			return value
		}
	}
	return ""
}

func firstAccountString(payload map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		if value := rawAccountString(payload, key); value != "" {
			return value
		}
	}
	return ""
}

func divideAccountNumber(value string, divisor int64) string {
	if value == "" || divisor == 0 {
		return ""
	}
	ratio, ok := new(big.Rat).SetString(value)
	if !ok {
		return ""
	}
	ratio.Quo(ratio, new(big.Rat).SetInt64(divisor))
	number := new(big.Float).SetPrec(256).SetRat(ratio)
	return accountNumber(json.Number(number.Text('f', 18)))
}

func addAccountNumbers(left, right string) string {
	if left == "" || right == "" {
		return ""
	}
	first, firstOK := new(big.Rat).SetString(left)
	second, secondOK := new(big.Rat).SetString(right)
	if !firstOK || !secondOK {
		return ""
	}
	first.Add(first, second)
	number := new(big.Float).SetPrec(256).SetRat(first)
	return accountNumber(json.Number(number.Text('f', 18)))
}

func accountNumber(value any) string {
	var raw string
	switch item := value.(type) {
	case json.Number:
		raw = item.String()
	case string:
		raw = strings.TrimSpace(item)
	default:
		return ""
	}
	if raw == "" || len(raw) > 128 {
		return ""
	}
	number, _, err := big.ParseFloat(raw, 10, 256, big.ToNearestEven)
	if err != nil {
		return ""
	}
	normalized := strings.TrimRight(strings.TrimRight(number.Text('f', 18), "0"), ".")
	if normalized == "" || normalized == "-0" {
		return "0"
	}
	integer := strings.TrimPrefix(strings.SplitN(normalized, ".", 2)[0], "-")
	if len(strings.TrimLeft(integer, "0")) > 22 {
		return ""
	}
	return normalized
}

func safeAccountSyncError(err error) string {
	switch {
	case errors.Is(err, errUpstreamAccountUnsupported):
		return "该上游系统不支持账户信息查询"
	case errors.Is(err, errUpstreamAccountNotFound):
		return "上游接口未返回可识别的余额字段"
	case errors.Is(err, ErrCredentialUnavailable):
		return "独立的上游账户查询凭据不可用"
	default:
		var httpErr *upstreamAccountHTTPError
		if errors.As(err, &httpErr) {
			return fmt.Sprintf("上游账户接口返回 HTTP %d", httpErr.status)
		}
		return "上游账户信息查询失败"
	}
}
