package enterprise

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ai-token/internal/ids"
	"ai-token/internal/mfa"
)

const MaxLicenseSize = 10 * 1024 * 1024

var (
	ErrUnavailable      = errors.New("enterprise verification service is unavailable")
	ErrInvalidRequest   = errors.New("invalid enterprise verification request")
	ErrNotFound         = errors.New("enterprise verification is not found")
	ErrPending          = errors.New("enterprise verification is already pending")
	ErrAlreadyApproved  = errors.New("enterprise verification is already approved")
	ErrReviewInvalid    = errors.New("enterprise verification cannot be reviewed")
	ErrDocumentInvalid  = errors.New("invalid business license document")
	ErrDocumentTooLarge = errors.New("business license document is too large")
	ErrDocumentType     = errors.New("business license document type is not supported")
)

type Submission struct {
	ID                 string     `json:"id"`
	TenantID           string     `json:"tenant_id"`
	SubmittedBy        string     `json:"submitted_by"`
	EnterpriseName     string     `json:"enterprise_name"`
	UnifiedCreditCode  string     `json:"unified_credit_code"`
	LicenseFilename    string     `json:"license_filename"`
	LicenseContentType string     `json:"license_content_type"`
	LicenseSize        int64      `json:"license_size"`
	LicenseSHA256      string     `json:"license_sha256"`
	BankAccountName    string     `json:"bank_account_name"`
	BankName           string     `json:"bank_name"`
	BankAccount        string     `json:"bank_account,omitempty"`
	BankAccountMasked  string     `json:"bank_account_masked,omitempty"`
	Status             string     `json:"status"`
	RejectionReason    string     `json:"rejection_reason,omitempty"`
	ReviewedBy         string     `json:"reviewed_by,omitempty"`
	ReviewedAt         *time.Time `json:"reviewed_at,omitempty"`
	SubmittedAt        time.Time  `json:"submitted_at"`
	CreatedAt          time.Time  `json:"created_at"`
}

type SubmitRequest struct {
	EnterpriseName    string
	UnifiedCreditCode string
	LicenseFilename   string
	LicenseType       string
	License           []byte
	BankAccountName   string
	BankName          string
	BankAccount       string
}

type Document struct {
	Filename    string
	ContentType string
	SHA256      string
	Content     []byte
}

type ConsoleService interface {
	GetCurrent(context.Context, string, string) (Submission, error)
	Submit(context.Context, string, string, SubmitRequest) (Submission, error)
}

type AdminService interface {
	List(context.Context, string) ([]Submission, error)
	Get(context.Context, string) (Submission, error)
	Review(context.Context, string, string, string, string) (Submission, error)
	License(context.Context, string) (Document, error)
}

type Service struct {
	db  *sql.DB
	box *mfa.SecretBox
}

func NewSQLService(db *sql.DB, box *mfa.SecretBox) (*Service, error) {
	if db == nil || box == nil {
		return nil, errors.New("database and secret box are required")
	}
	return &Service{db: db, box: box}, nil
}

func (s *Service) GetCurrent(ctx context.Context, tenantID, userID string) (Submission, error) {
	if s == nil || s.db == nil || !validIDs(tenantID, userID) {
		return Submission{}, ErrInvalidRequest
	}
	var item Submission
	var bankCipher []byte
	var reviewedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, submissionSelect+`
		WHERE ev.tenant_id = $1::uuid
		  AND EXISTS (
		      SELECT 1
		      FROM tenant_members tm
		      WHERE tm.tenant_id = ev.tenant_id
		        AND tm.user_id = $2::uuid
		        AND tm.status = 'active'
		  )
		ORDER BY ev.submitted_at DESC, ev.created_at DESC
		LIMIT 1`, tenantID, userID).Scan(submissionArgs(&item, &bankCipher, &reviewedAt)...)
	if errors.Is(err, sql.ErrNoRows) {
		return Submission{}, ErrNotFound
	}
	if err != nil {
		return Submission{}, err
	}
	return s.finishSubmission(item, bankCipher, reviewedAt, false)
}

func (s *Service) Submit(ctx context.Context, userID, tenantID string, request SubmitRequest) (Submission, error) {
	if s == nil || s.db == nil || s.box == nil {
		return Submission{}, ErrUnavailable
	}
	request, err := normalizeSubmitRequest(request)
	if err != nil {
		return Submission{}, err
	}
	if !validIDs(userID, tenantID) {
		return Submission{}, ErrInvalidRequest
	}
	var activeMember bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM tenant_members
			WHERE tenant_id = $1::uuid
			  AND user_id = $2::uuid
			  AND status = 'active'
		)
	`, tenantID, userID).Scan(&activeMember); err != nil {
		return Submission{}, err
	}
	if !activeMember {
		return Submission{}, ErrInvalidRequest
	}
	licenseCipher, err := s.box.Seal(request.License)
	if err != nil {
		return Submission{}, err
	}
	bankCipher, err := s.box.Seal([]byte(request.BankAccount))
	if err != nil {
		return Submission{}, err
	}
	id, err := ids.New()
	if err != nil {
		return Submission{}, err
	}
	hash := sha256.Sum256(request.License)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Submission{}, err
	}
	defer func() { _ = tx.Rollback() }()
	// Lock the tenant key even when no previous submission exists. A row-level
	// lock on the latest submission cannot prevent two first-time submissions
	// from racing each other.
	if _, err := tx.ExecContext(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended('ai-token:enterprise-verification:' || $1, 0))
	`, tenantID); err != nil {
		return Submission{}, err
	}
	var latestStatus string
	err = tx.QueryRowContext(ctx, `
		SELECT status FROM enterprise_verifications
		WHERE tenant_id = $1::uuid
		ORDER BY submitted_at DESC, created_at DESC
		LIMIT 1 FOR UPDATE`, tenantID).Scan(&latestStatus)
	if err == nil {
		switch latestStatus {
		case "pending":
			return Submission{}, ErrPending
		case "approved":
			return Submission{}, ErrAlreadyApproved
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Submission{}, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO enterprise_verifications (
			id, tenant_id, submitted_by, enterprise_name, unified_credit_code,
			license_filename, license_content_type, license_size, license_sha256,
			license_ciphertext, bank_account_name, bank_name, bank_account_ciphertext,
			status
		) VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, 'pending')
	`, id, tenantID, userID, request.EnterpriseName, request.UnifiedCreditCode,
		request.LicenseFilename, request.LicenseType, len(request.License), hex.EncodeToString(hash[:]),
		[]byte(licenseCipher), request.BankAccountName, request.BankName, bankCipher)
	if err != nil {
		return Submission{}, err
	}
	if err := tx.Commit(); err != nil {
		return Submission{}, err
	}
	return s.get(ctx, id, false)
}

func (s *Service) List(ctx context.Context, status string) ([]Submission, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "" && !validStatus(status) {
		return nil, ErrInvalidRequest
	}
	query := submissionSelect
	args := []any{}
	if status != "" {
		query += ` WHERE ev.status = $1`
		args = append(args, status)
	}
	query += ` ORDER BY CASE ev.status WHEN 'pending' THEN 0 WHEN 'rejected' THEN 1 ELSE 2 END, ev.submitted_at DESC, ev.id DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Submission, 0)
	for rows.Next() {
		var item Submission
		var bankCipher []byte
		var reviewedAt sql.NullTime
		if err := rows.Scan(submissionArgs(&item, &bankCipher, &reviewedAt)...); err != nil {
			return nil, err
		}
		item, err = s.finishSubmission(item, bankCipher, reviewedAt, false)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) Get(ctx context.Context, id string) (Submission, error) {
	return s.get(ctx, id, true)
}

func (s *Service) get(ctx context.Context, id string, revealBank bool) (Submission, error) {
	if s == nil || s.db == nil {
		return Submission{}, ErrUnavailable
	}
	if !ids.Valid(strings.TrimSpace(id)) {
		return Submission{}, ErrInvalidRequest
	}
	var item Submission
	var bankCipher []byte
	var reviewedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, submissionSelect+` WHERE ev.id = $1::uuid`, id).Scan(submissionArgs(&item, &bankCipher, &reviewedAt)...)
	if errors.Is(err, sql.ErrNoRows) {
		return Submission{}, ErrNotFound
	}
	if err != nil {
		return Submission{}, err
	}
	return s.finishSubmission(item, bankCipher, reviewedAt, revealBank)
}

func (s *Service) Review(ctx context.Context, reviewerID, id, status, reason string) (Submission, error) {
	if s == nil || s.db == nil || !validIDs(reviewerID, id) {
		return Submission{}, ErrInvalidRequest
	}
	status = strings.ToLower(strings.TrimSpace(status))
	reason = strings.TrimSpace(reason)
	if status != "approved" && status != "rejected" || len(reason) > 2000 || status == "rejected" && reason == "" {
		return Submission{}, ErrReviewInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Submission{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var current string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM enterprise_verifications WHERE id = $1::uuid FOR UPDATE`, id).Scan(&current); errors.Is(err, sql.ErrNoRows) {
		return Submission{}, ErrNotFound
	} else if err != nil {
		return Submission{}, err
	}
	if current != "pending" {
		return Submission{}, ErrReviewInvalid
	}
	if _, err := tx.ExecContext(ctx, `UPDATE enterprise_verifications SET status = $2, rejection_reason = NULLIF($3, ''), reviewed_by = $4::uuid, reviewed_at = now(), updated_at = now() WHERE id = $1::uuid`, id, status, reason, reviewerID); err != nil {
		return Submission{}, err
	}
	if err := tx.Commit(); err != nil {
		return Submission{}, err
	}
	return s.get(ctx, id, true)
}

func (s *Service) License(ctx context.Context, id string) (Document, error) {
	if s == nil || s.db == nil || s.box == nil || !ids.Valid(strings.TrimSpace(id)) {
		return Document{}, ErrInvalidRequest
	}
	var filename, contentType, hash, encoded string
	if err := s.db.QueryRowContext(ctx, `SELECT license_filename, license_content_type, license_sha256, license_ciphertext FROM enterprise_verifications WHERE id = $1::uuid`, id).Scan(&filename, &contentType, &hash, &encoded); errors.Is(err, sql.ErrNoRows) {
		return Document{}, ErrNotFound
	} else if err != nil {
		return Document{}, err
	}
	content, err := s.box.Open(encoded)
	if err != nil {
		return Document{}, ErrDocumentInvalid
	}
	return Document{Filename: filename, ContentType: contentType, SHA256: hash, Content: content}, nil
}

func (s *Service) finishSubmission(item Submission, bankCipher []byte, reviewedAt sql.NullTime, revealBank bool) (Submission, error) {
	if len(bankCipher) > 0 {
		plain, err := s.box.Open(string(bankCipher))
		if err != nil {
			return Submission{}, ErrDocumentInvalid
		}
		item.BankAccount = string(plain)
		item.BankAccountMasked = maskAccount(item.BankAccount)
		if !revealBank {
			item.BankAccount = ""
		}
	}
	if reviewedAt.Valid {
		value := reviewedAt.Time
		item.ReviewedAt = &value
	}
	return item, nil
}

func normalizeSubmitRequest(request SubmitRequest) (SubmitRequest, error) {
	request.EnterpriseName = strings.TrimSpace(request.EnterpriseName)
	request.UnifiedCreditCode = strings.ToUpper(strings.TrimSpace(request.UnifiedCreditCode))
	request.LicenseFilename = filepath.Base(strings.TrimSpace(request.LicenseFilename))
	request.LicenseType = strings.ToLower(strings.TrimSpace(request.LicenseType))
	request.BankAccountName = strings.TrimSpace(request.BankAccountName)
	request.BankName = strings.TrimSpace(request.BankName)
	request.BankAccount = strings.TrimSpace(request.BankAccount)
	if request.EnterpriseName == "" || len(request.EnterpriseName) > 200 || !validUnifiedCreditCode(request.UnifiedCreditCode) || request.BankAccountName == "" || len(request.BankAccountName) > 200 || request.BankName == "" || len(request.BankName) > 300 || !bankAccountPattern.MatchString(request.BankAccount) {
		return SubmitRequest{}, ErrInvalidRequest
	}
	if len(request.License) == 0 {
		return SubmitRequest{}, ErrDocumentInvalid
	}
	if len(request.License) > MaxLicenseSize {
		return SubmitRequest{}, ErrDocumentTooLarge
	}
	contentType, ok := normalizeDocumentType(request.LicenseFilename, request.LicenseType, request.License)
	if !ok {
		return SubmitRequest{}, ErrDocumentType
	}
	request.LicenseType = contentType
	return request, nil
}

func normalizeDocumentType(filename, contentType string, content []byte) (string, bool) {
	ext := strings.ToLower(filepath.Ext(filename))
	if filename == "" || strings.ContainsAny(filename, "\r\n") {
		return "", false
	}
	if strings.HasPrefix(string(content), "%PDF-") && (contentType == "" || contentType == "application/pdf" || ext == ".pdf") {
		return "application/pdf", true
	}
	if len(content) >= 3 && content[0] == 0xff && content[1] == 0xd8 && content[2] == 0xff && (contentType == "" || contentType == "image/jpeg" || ext == ".jpg" || ext == ".jpeg") {
		return "image/jpeg", true
	}
	if len(content) >= 8 && string(content[:8]) == "\x89PNG\r\n\x1a\n" && (contentType == "" || contentType == "image/png" || ext == ".png") {
		return "image/png", true
	}
	return "", false
}

func maskAccount(value string) string {
	if len(value) <= 4 {
		return strings.Repeat("*", len(value))
	}
	return strings.Repeat("*", len(value)-4) + value[len(value)-4:]
}

func validIDs(values ...string) bool {
	for _, value := range values {
		if !ids.Valid(strings.TrimSpace(value)) {
			return false
		}
	}
	return true
}

func validStatus(value string) bool {
	return value == "pending" || value == "approved" || value == "rejected"
}

var creditCodePattern = regexp.MustCompile(`^[0-9A-HJ-NPQRTUWXY]{18}$`)
var bankAccountPattern = regexp.MustCompile(`^[0-9]{6,32}$`)

const unifiedCreditCodeAlphabet = "0123456789ABCDEFGHJKLMNPQRTUWXY"

var unifiedCreditCodeWeights = [...]int{1, 3, 9, 27, 19, 26, 16, 17, 20, 29, 25, 13, 8, 24, 10, 30, 28}

func validUnifiedCreditCode(value string) bool {
	if !creditCodePattern.MatchString(value) {
		return false
	}
	sum := 0
	for index, char := range value[:17] {
		position := strings.IndexRune(unifiedCreditCodeAlphabet, char)
		if position < 0 {
			return false
		}
		sum += position * unifiedCreditCodeWeights[index]
	}
	check := (31 - sum%31) % 31
	return unifiedCreditCodeAlphabet[check] == value[17]
}

const submissionSelect = `
	SELECT ev.id::text, ev.tenant_id::text, ev.submitted_by::text, ev.enterprise_name,
	       ev.unified_credit_code, ev.license_filename, ev.license_content_type,
	       ev.license_size, ev.license_sha256, ev.bank_account_name, ev.bank_name,
	       ev.bank_account_ciphertext, ev.status, COALESCE(ev.rejection_reason, ''),
	       COALESCE(ev.reviewed_by::text, ''), ev.reviewed_at, ev.submitted_at, ev.created_at
	FROM enterprise_verifications ev`

func submissionArgs(item *Submission, bankCipher *[]byte, reviewedAt *sql.NullTime) []any {
	return []any{&item.ID, &item.TenantID, &item.SubmittedBy, &item.EnterpriseName, &item.UnifiedCreditCode, &item.LicenseFilename, &item.LicenseContentType, &item.LicenseSize, &item.LicenseSHA256, &item.BankAccountName, &item.BankName, bankCipher, &item.Status, &item.RejectionReason, &item.ReviewedBy, reviewedAt, &item.SubmittedAt, &item.CreatedAt}
}

func (s *Service) String() string {
	return fmt.Sprintf("enterprise service configured=%t", s != nil && s.db != nil)
}
