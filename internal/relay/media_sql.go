package relay

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"ai-token/internal/ids"
)

func (r *SQLChannelRouter) CreateMediaJob(ctx context.Context, job MediaJob) error {
	if r == nil || r.db == nil {
		return ErrUnavailable
	}
	if !ids.Valid(strings.TrimSpace(job.ID)) || !ids.Valid(strings.TrimSpace(job.ModelRequestID)) ||
		!ids.Valid(strings.TrimSpace(job.TenantID)) || !ids.Valid(strings.TrimSpace(job.ProjectID)) ||
		!ids.Valid(strings.TrimSpace(job.TokenID)) || !ids.Valid(strings.TrimSpace(job.Channel.ID)) ||
		strings.TrimSpace(job.Provider) == "" || strings.TrimSpace(job.Model) == "" ||
		strings.TrimSpace(job.UpstreamJobID) == "" {
		return ErrInvalidRequest
	}
	response := job.Response
	if len(response) == 0 {
		response = []byte(`{}`)
	}
	metrics, err := json.Marshal(job.EstimatedMetrics)
	if err != nil {
		return ErrInvalidRequest
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO media_jobs (
			id, model_request_id, reservation_id, tenant_id, project_id, token_id, group_id,
			channel_id, provider, model_name, upstream_model_name, upstream_job_id, status,
			output_uri, response_json, estimated_metrics_json, failure_reason
		) VALUES ($1::uuid, $2::uuid, NULLIF($3, '')::uuid, $4::uuid, $5::uuid, $6::uuid, NULLIF($7, '')::uuid,
		          $8::uuid, $9, $10, $11, $12, $13, NULLIF($14, ''), $15::jsonb, $16::jsonb, NULLIF($17, ''))
	`, job.ID, job.ModelRequestID, job.ReservationID, job.TenantID, job.ProjectID, job.TokenID, job.GroupID,
		job.Channel.ID, canonicalProvider(job.Provider), job.Model, firstNonEmpty(job.UpstreamModelName, job.Model), job.UpstreamJobID,
		normalizeVideoStatus(job.Status), job.OutputURI, response, metrics, job.FailureReason)
	return err
}

func (r *SQLChannelRouter) GetMediaJob(ctx context.Context, id, tenantID, tokenID string) (MediaJob, error) {
	if r == nil || r.db == nil {
		return MediaJob{}, ErrUnavailable
	}
	id, tenantID, tokenID = strings.TrimSpace(id), strings.TrimSpace(tenantID), strings.TrimSpace(tokenID)
	if !ids.Valid(id) || !ids.Valid(tenantID) || !ids.Valid(tokenID) {
		return MediaJob{}, ErrInvalidRequest
	}
	var (
		job                      MediaJob
		reservationID            sql.NullString
		channelID, channelName   string
		baseURL, credentialRef   string
		priority, weight         int
		response, metrics        []byte
		completedAt              sql.NullTime
		failureReason, outputURI sql.NullString
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT mj.id::text, mj.tenant_id::text, mj.project_id::text, mj.token_id::text,
		       COALESCE(mj.reservation_id::text, ''),
		       COALESCE(mj.group_id::text, ''), mj.model_name, mj.upstream_model_name, mj.provider,
		       mj.upstream_job_id, mj.status, COALESCE(mj.output_uri, ''),
		       mj.response_json, mj.estimated_metrics_json, COALESCE(mj.failure_reason, ''),
		       mj.created_at, mj.updated_at, mj.completed_at,
		       c.id::text, c.name, c.base_url, c.credential_ref,
		       c.upstream_cost_discount::text, c.priority, c.weight
		FROM media_jobs mj
		JOIN channels c ON c.id = mj.channel_id
		WHERE mj.id = $1::uuid
		  AND mj.tenant_id = $2::uuid
		  AND mj.token_id = $3::uuid
	`, id, tenantID, tokenID).Scan(
		&job.ID, &job.TenantID, &job.ProjectID, &job.TokenID, &reservationID, &job.GroupID,
		&job.Model, &job.UpstreamModelName, &job.Provider, &job.UpstreamJobID, &job.Status, &outputURI,
		&response, &metrics, &failureReason, &job.CreatedAt, &job.UpdatedAt, &completedAt,
		&channelID, &channelName, &baseURL, &credentialRef, &job.Channel.UpstreamCostDiscount, &priority, &weight,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return MediaJob{}, ErrModelNotFound
	}
	if err != nil {
		return MediaJob{}, err
	}
	job.OutputURI, job.FailureReason = outputURI.String, failureReason.String
	job.ReservationID = reservationID.String
	if completedAt.Valid {
		value := completedAt.Time
		job.CompletedAt = &value
	}
	_ = json.Unmarshal(response, &job.Response)
	_ = json.Unmarshal(metrics, &job.EstimatedMetrics)
	job.Channel = Channel{ID: channelID, Name: channelName, BaseURL: baseURL, CredentialRef: credentialRef, Provider: job.Provider, Priority: priority, Weight: weight, UpstreamModelName: job.UpstreamModelName, UpstreamCostDiscount: job.Channel.UpstreamCostDiscount}
	if strings.TrimSpace(job.UpstreamModelName) == "" {
		job.UpstreamModelName = job.Model
		job.Channel.UpstreamModelName = job.Model
	}
	return job, nil
}

func (r *SQLChannelRouter) UpdateMediaJob(ctx context.Context, id, status, outputURI string, response json.RawMessage, failureReason string) error {
	if r == nil || r.db == nil {
		return ErrUnavailable
	}
	id, status = strings.TrimSpace(id), normalizeVideoStatus(status)
	if id == "" || status == "" {
		return ErrInvalidRequest
	}
	if len(response) == 0 {
		response = []byte(`{}`)
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE media_jobs
		SET status = $2,
		    output_uri = NULLIF($3, ''),
		    response_json = $4::jsonb,
		    failure_reason = NULLIF($5, ''),
		    updated_at = now(),
		    completed_at = CASE WHEN $2 IN ('completed', 'failed', 'cancelled') THEN COALESCE(completed_at, now()) ELSE completed_at END
		WHERE id = $1::uuid
	`, id, status, strings.TrimSpace(outputURI), response, strings.TrimSpace(failureReason))
	return err
}

func (r *SQLChannelRouter) ListPendingMediaJobs(ctx context.Context, limit int) ([]MediaJob, error) {
	if r == nil || r.db == nil {
		return nil, ErrUnavailable
	}
	if limit < 1 || limit > 500 {
		return nil, ErrInvalidRequest
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT mj.id::text, mj.tenant_id::text, mj.project_id::text, mj.token_id::text,
		       COALESCE(mj.reservation_id::text, ''), COALESCE(mj.group_id::text, ''),
		       mj.model_name, mj.upstream_model_name, mj.provider, mj.upstream_job_id,
		       mj.status, COALESCE(mj.output_uri, ''), mj.response_json,
		       mj.estimated_metrics_json, COALESCE(mj.failure_reason, ''),
		       mj.created_at, mj.updated_at, mj.completed_at,
		       c.id::text, c.name, c.base_url, c.credential_ref,
		       c.upstream_cost_discount::text, c.priority, c.weight
		FROM media_jobs mj
		JOIN channels c ON c.id = mj.channel_id
		WHERE mj.status IN ('queued', 'processing')
		   OR (mj.status IN ('completed', 'failed', 'cancelled') AND mj.reservation_id IS NOT NULL AND EXISTS (
			SELECT 1 FROM billing_reservations br
			WHERE br.id = mj.reservation_id AND br.status = 'pending'
		   ))
		ORDER BY mj.updated_at ASC, mj.id ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]MediaJob, 0, limit)
	for rows.Next() {
		var (
			job                                     MediaJob
			reservationID, outputURI, failureReason sql.NullString
			response, metrics                       []byte
			completedAt                             sql.NullTime
			channelID, channelName, baseURL         string
			credentialRef                           string
			upstreamCostDiscount                    string
			priority, weight                        int
		)
		if err := rows.Scan(
			&job.ID, &job.TenantID, &job.ProjectID, &job.TokenID,
			&reservationID, &job.GroupID, &job.Model, &job.UpstreamModelName,
			&job.Provider, &job.UpstreamJobID, &job.Status, &outputURI,
			&response, &metrics, &failureReason, &job.CreatedAt, &job.UpdatedAt,
			&completedAt, &channelID, &channelName, &baseURL, &credentialRef,
			&upstreamCostDiscount,
			&priority, &weight,
		); err != nil {
			return nil, err
		}
		job.ReservationID, job.OutputURI, job.FailureReason = reservationID.String, outputURI.String, failureReason.String
		if completedAt.Valid {
			value := completedAt.Time
			job.CompletedAt = &value
		}
		_ = json.Unmarshal(response, &job.Response)
		_ = json.Unmarshal(metrics, &job.EstimatedMetrics)
		job.Channel = Channel{ID: channelID, Name: channelName, BaseURL: baseURL, CredentialRef: credentialRef, Provider: job.Provider, Priority: priority, Weight: weight, UpstreamModelName: job.UpstreamModelName, UpstreamCostDiscount: upstreamCostDiscount}
		if strings.TrimSpace(job.UpstreamModelName) == "" {
			job.UpstreamModelName = job.Model
			job.Channel.UpstreamModelName = job.Model
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

var _ MediaJobStore = (*SQLChannelRouter)(nil)
