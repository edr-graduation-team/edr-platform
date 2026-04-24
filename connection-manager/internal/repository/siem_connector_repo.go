package repository

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SiemConnectorRow is a persisted SIEM / webhook destination.
type SiemConnectorRow struct {
	ID             uuid.UUID       `json:"id"`
	Name           string          `json:"name"`
	ConnectorType  string          `json:"connector_type"`
	EndpointURL    string          `json:"endpoint_url"`
	Enabled        bool            `json:"enabled"`
	Status         string          `json:"status"`
	LastTestAt     *time.Time      `json:"last_test_at,omitempty"`
	LastError      *string         `json:"last_error,omitempty"`
	Notes          string          `json:"notes"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// SiemConnectorRepository persists SIEM connector definitions.
type SiemConnectorRepository interface {
	List(ctx context.Context) ([]SiemConnectorRow, error)
	GetByID(ctx context.Context, id uuid.UUID) (*SiemConnectorRow, error)
	Create(ctx context.Context, row *SiemConnectorRow) error
	Update(ctx context.Context, row *SiemConnectorRow) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// PostgresSiemConnectorRepository implements SiemConnectorRepository.
type PostgresSiemConnectorRepository struct {
	db *pgxpool.Pool
}

// NewPostgresSiemConnectorRepository creates the repository.
func NewPostgresSiemConnectorRepository(db *pgxpool.Pool) *PostgresSiemConnectorRepository {
	return &PostgresSiemConnectorRepository{db: db}
}

func (r *PostgresSiemConnectorRepository) List(ctx context.Context) ([]SiemConnectorRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, connector_type, endpoint_url, enabled, status, last_test_at, last_error, notes, metadata, created_at, updated_at
		FROM siem_connectors
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SiemConnectorRow
	for rows.Next() {
		var row SiemConnectorRow
		if err := rows.Scan(
			&row.ID, &row.Name, &row.ConnectorType, &row.EndpointURL, &row.Enabled, &row.Status,
			&row.LastTestAt, &row.LastError, &row.Notes, &row.Metadata, &row.CreatedAt, &row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *PostgresSiemConnectorRepository) GetByID(ctx context.Context, id uuid.UUID) (*SiemConnectorRow, error) {
	var row SiemConnectorRow
	err := r.db.QueryRow(ctx, `
		SELECT id, name, connector_type, endpoint_url, enabled, status, last_test_at, last_error, notes, metadata, created_at, updated_at
		FROM siem_connectors WHERE id = $1`, id,
	).Scan(
		&row.ID, &row.Name, &row.ConnectorType, &row.EndpointURL, &row.Enabled, &row.Status,
		&row.LastTestAt, &row.LastError, &row.Notes, &row.Metadata, &row.CreatedAt, &row.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *PostgresSiemConnectorRepository) Create(ctx context.Context, row *SiemConnectorRow) error {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	if len(row.Metadata) == 0 {
		row.Metadata = []byte(`{}`)
	}
	return r.db.QueryRow(ctx, `
		INSERT INTO siem_connectors (id, name, connector_type, endpoint_url, enabled, status, notes, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`,
		row.ID, row.Name, row.ConnectorType, row.EndpointURL, row.Enabled, row.Status, row.Notes, row.Metadata,
	).Scan(&row.CreatedAt, &row.UpdatedAt)
}

func (r *PostgresSiemConnectorRepository) Update(ctx context.Context, row *SiemConnectorRow) error {
	if len(row.Metadata) == 0 {
		row.Metadata = []byte(`{}`)
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE siem_connectors SET
			name = $2, connector_type = $3, endpoint_url = $4, enabled = $5, status = $6,
			notes = $7, metadata = $8, updated_at = NOW()
		WHERE id = $1`,
		row.ID, row.Name, row.ConnectorType, row.EndpointURL, row.Enabled, row.Status, row.Notes, row.Metadata,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresSiemConnectorRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM siem_connectors WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ValidSiemConnectorType returns true if t is allowed.
func ValidSiemConnectorType(t string) bool {
	switch strings.TrimSpace(t) {
	case "splunk_hec", "azure_sentinel", "elastic_webhook", "generic_webhook", "syslog_tls":
		return true
	default:
		return false
	}
}
