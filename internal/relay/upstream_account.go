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
	rateMultiplier string
}

var (
	errUpstreamAccountUnsupported = errors.New("upstream account information is not supported")
	errUpstreamAccountNotFound    = errors.New("upstream account information was not returned")
)

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

	var provider, baseURL, integration, credentialRef string
	err := r.db.QueryRowContext(ctx, `
		SELECT provider, base_url, upstream_integration, upstream_account_credential_ref
		FROM channels
		WHERE id = $1::uuid
		  AND deleted_at IS NULL
	`, channelID).Scan(&provider, &baseURL, &integration, &credentialRef)
	if errors.Is(err, sql.ErrNoRows) {
		return ChannelSummary{}, ErrChannelNotFound
	}
	if err != nil {
		return ChannelSummary{}, err
	}

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

	integration = normalizeUpstreamIntegration(integration)
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

	snapshot, fetchErr := fetchUpstreamAccount(ctx, integration, provider, baseURL, secret)
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
		        WHEN NULLIF($2, '') IS NULL THEN upstream_balance
		        ELSE $2::numeric
		    END,
		    upstream_balance_unit = CASE
		        WHEN NULLIF($2, '') IS NULL THEN upstream_balance_unit
		        ELSE $3
		    END,
		    upstream_rate_multiplier = CASE
		        WHEN NULLIF($4, '') IS NULL THEN upstream_rate_multiplier
		        ELSE $4::numeric
		    END,
		    upstream_account_sync_status = 'success',
		    upstream_account_sync_error = '',
		    upstream_account_synced_at = now()
		WHERE id = $1::uuid
		  AND deleted_at IS NULL
	`, channelID, snapshot.balance, snapshot.balanceUnit, snapshot.rateMultiplier)
	return err
}

func fetchUpstreamAccount(ctx context.Context, integration, provider, baseURL, credential string) (upstreamAccountSnapshot, error) {
	client, err := providerHTTPClient(baseURL)
	if err != nil {
		return upstreamAccountSnapshot{}, err
	}
	client.Timeout = 10 * time.Second

	switch integration {
	case UpstreamIntegrationNewAPI:
		body, err := requestUpstreamAccountJSON(ctx, client, baseURL, "/api/user/self", credential)
		if err != nil {
			return upstreamAccountSnapshot{}, err
		}
		return parseNewAPIAccount(body)
	case UpstreamIntegrationSub2API:
		body, err := requestUpstreamAccountJSON(ctx, client, baseURL, "/api/v1/auth/me", credential)
		if err != nil {
			return upstreamAccountSnapshot{}, err
		}
		snapshot, err := parseSub2APIAccount(body)
		if err != nil {
			return upstreamAccountSnapshot{}, err
		}

		// Sub2API exposes rates separately. A single value is stored only when
		// the upstream response is unambiguous; multiple group rates stay empty.
		ratesBody, ratesErr := requestUpstreamAccountJSON(ctx, client, baseURL, "/api/v1/groups/rates", credential)
		if ratesErr == nil {
			snapshot.rateMultiplier = singleRateMultiplier(ratesBody)
		}
		return snapshot, nil
	default:
		return upstreamAccountSnapshot{}, errUpstreamAccountUnsupported
	}
}

func requestUpstreamAccountJSON(ctx context.Context, client *http.Client, baseURL, path, credential string) (json.RawMessage, error) {
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
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("upstream account endpoint returned HTTP %d", response.StatusCode)
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
	payload, err := decodeAccountPayload(body)
	if err != nil {
		return upstreamAccountSnapshot{}, err
	}
	balance, balanceKey := findNumber(payload, []string{
		"balance", "remaining_balance", "quota", "remain_quota", "remaining_quota",
	})
	if balance == "" {
		return upstreamAccountSnapshot{}, errUpstreamAccountNotFound
	}
	unit := "USD"
	if strings.Contains(strings.ToLower(balanceKey), "quota") {
		unit = "quota"
	}
	rate, _ := findNumber(payload, []string{
		"rate_multiplier", "group_ratio", "ratio", "multiplier",
	})
	return upstreamAccountSnapshot{
		balance:        balance,
		balanceUnit:    unit,
		rateMultiplier: rate,
	}, nil
}

func parseSub2APIAccount(body json.RawMessage) (upstreamAccountSnapshot, error) {
	payload, err := decodeAccountPayload(body)
	if err != nil {
		return upstreamAccountSnapshot{}, err
	}
	balance, balanceKey := findNumber(payload, []string{
		"balance", "available_balance", "remaining_balance", "quota",
	})
	if balance == "" {
		return upstreamAccountSnapshot{}, errUpstreamAccountNotFound
	}
	unit := "USD"
	if strings.Contains(strings.ToLower(balanceKey), "quota") {
		unit = "quota"
	}
	rate, _ := findNumber(payload, []string{"rate_multiplier", "multiplier", "ratio"})
	return upstreamAccountSnapshot{
		balance:        balance,
		balanceUnit:    unit,
		rateMultiplier: rate,
	}, nil
}

func decodeAccountPayload(body json.RawMessage) (map[string]any, error) {
	var raw any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return nil, err
	}
	root, ok := raw.(map[string]any)
	if !ok {
		return nil, errUpstreamAccountNotFound
	}
	if success, ok := root["success"].(bool); ok && !success {
		return nil, errors.New("upstream account response was unsuccessful")
	}
	if data, ok := root["data"].(map[string]any); ok {
		return data, nil
	}
	return root, nil
}

func findNumber(payload map[string]any, keys []string) (string, string) {
	wanted := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		wanted[strings.ToLower(key)] = struct{}{}
	}
	var walk func(any) (string, string)
	walk = func(value any) (string, string) {
		switch item := value.(type) {
		case map[string]any:
			for key, candidate := range item {
				if _, ok := wanted[strings.ToLower(strings.TrimSpace(key))]; ok {
					if number := accountNumber(candidate); number != "" {
						return number, key
					}
				}
			}
			for _, candidate := range item {
				if number, key := walk(candidate); number != "" {
					return number, key
				}
			}
		case []any:
			for _, candidate := range item {
				if number, key := walk(candidate); number != "" {
					return number, key
				}
			}
		}
		return "", ""
	}
	return walk(payload)
}

func singleRateMultiplier(body json.RawMessage) string {
	var raw any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if decoder.Decode(&raw) != nil {
		return ""
	}
	values := make([]string, 0, 4)
	var walk func(any)
	walk = func(value any) {
		switch item := value.(type) {
		case map[string]any:
			for key, candidate := range item {
				lower := strings.ToLower(key)
				if strings.Contains(lower, "rate") || strings.Contains(lower, "ratio") || strings.Contains(lower, "multiplier") {
					if number := accountNumber(candidate); number != "" {
						values = append(values, number)
						continue
					}
				}
				walk(candidate)
			}
		case []any:
			for _, candidate := range item {
				walk(candidate)
			}
		}
	}
	walk(raw)
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		unique[value] = struct{}{}
	}
	if len(unique) != 1 {
		return ""
	}
	for value := range unique {
		return value
	}
	return ""
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
		return "上游账户信息查询失败"
	}
}
