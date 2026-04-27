package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/edr-platform/connection-manager/pkg/models"
)

// PostgresResponsePlaybookRepository implements ResponsePlaybookRepository
type PostgresResponsePlaybookRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresResponsePlaybookRepository(pool *pgxpool.Pool) *PostgresResponsePlaybookRepository {
	return &PostgresResponsePlaybookRepository{pool: pool}
}

func (r *PostgresResponsePlaybookRepository) Create(ctx context.Context, p *models.ResponsePlaybook) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	now := time.Now()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO response_playbooks (id, name, description, category, severity_filter, rule_pattern, commands, mitre_techniques, enabled, created_at, updated_at) 
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		p.ID, p.Name, p.Description, p.Category, p.SeverityFilter, p.RulePattern, p.Commands, p.MITRETechiques, p.Enabled, now, now)
	return err
}

func (r *PostgresResponsePlaybookRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.ResponsePlaybook, error) {
	p := &models.ResponsePlaybook{}
	err := r.pool.QueryRow(ctx, `SELECT id, name, COALESCE(description, ''), category, severity_filter, COALESCE(rule_pattern, ''), commands, mitre_techniques, enabled, created_at, updated_at FROM response_playbooks WHERE id = $1`, id).
		Scan(&p.ID, &p.Name, &p.Description, &p.Category, &p.SeverityFilter, &p.RulePattern, &p.Commands, &p.MITRETechiques, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	return p, err
}

func (r *PostgresResponsePlaybookRepository) Update(ctx context.Context, p *models.ResponsePlaybook) error {
	now := time.Now()
	_, err := r.pool.Exec(ctx,
		`UPDATE response_playbooks SET name=$1, description=$2, category=$3, severity_filter=$4, rule_pattern=$5, commands=$6, mitre_techniques=$7, enabled=$8, updated_at=$9 WHERE id=$10`,
		p.Name, p.Description, p.Category, p.SeverityFilter, p.RulePattern, p.Commands, p.MITRETechiques, p.Enabled, now, p.ID)
	return err
}

func (r *PostgresResponsePlaybookRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM response_playbooks WHERE id=$1`, id)
	return err
}

func (r *PostgresResponsePlaybookRepository) List(ctx context.Context, filter PlaybookFilter) ([]*models.ResponsePlaybook, error) {
	// Simplified list ignoring filters for brevity
	limit := filter.Limit
	if limit == 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `SELECT id, name, COALESCE(description, ''), category, severity_filter, COALESCE(rule_pattern, ''), commands, mitre_techniques, enabled, created_at, updated_at FROM response_playbooks LIMIT $1 OFFSET $2`, limit, filter.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var playbooks []*models.ResponsePlaybook
	for rows.Next() {
		p := &models.ResponsePlaybook{}
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Category, &p.SeverityFilter, &p.RulePattern, &p.Commands, &p.MITRETechiques, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		playbooks = append(playbooks, p)
	}
	return playbooks, nil
}

func (r *PostgresResponsePlaybookRepository) Count(ctx context.Context, filter PlaybookFilter) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM response_playbooks`).Scan(&count)
	return count, err
}

// PostgresAutomationRuleRepository implements AutomationRuleRepository
type PostgresAutomationRuleRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresAutomationRuleRepository(pool *pgxpool.Pool) *PostgresAutomationRuleRepository {
	return &PostgresAutomationRuleRepository{pool: pool}
}

func (r *PostgresAutomationRuleRepository) Create(ctx context.Context, rule *models.AutomationRule) error {
	if rule.ID == uuid.Nil {
		rule.ID = uuid.New()
	}
	now := time.Now()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO automation_rules (id, name, description, trigger_conditions, playbook_id, priority, auto_execute, cooldown_minutes, enabled, success_rate, created_at, updated_at) 
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		rule.ID, rule.Name, rule.Description, rule.TriggerConditions, rule.PlaybookID, rule.Priority, rule.AutoExecute, rule.CooldownMinutes, rule.Enabled, rule.SuccessRate, now, now)
	return err
}

func (r *PostgresAutomationRuleRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.AutomationRule, error) {
	rule := &models.AutomationRule{}
	err := r.pool.QueryRow(ctx, `SELECT id, name, COALESCE(description, ''), trigger_conditions, playbook_id, priority, auto_execute, cooldown_minutes, enabled, success_rate, last_execution, created_at, updated_at FROM automation_rules WHERE id = $1`, id).
		Scan(&rule.ID, &rule.Name, &rule.Description, &rule.TriggerConditions, &rule.PlaybookID, &rule.Priority, &rule.AutoExecute, &rule.CooldownMinutes, &rule.Enabled, &rule.SuccessRate, &rule.LastExecution, &rule.CreatedAt, &rule.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	return rule, err
}

func (r *PostgresAutomationRuleRepository) Update(ctx context.Context, rule *models.AutomationRule) error {
	now := time.Now()
	_, err := r.pool.Exec(ctx,
		`UPDATE automation_rules SET name=$1, description=$2, trigger_conditions=$3, playbook_id=$4, priority=$5, auto_execute=$6, cooldown_minutes=$7, enabled=$8, success_rate=$9, last_execution=$10, updated_at=$11 WHERE id=$12`,
		rule.Name, rule.Description, rule.TriggerConditions, rule.PlaybookID, rule.Priority, rule.AutoExecute, rule.CooldownMinutes, rule.Enabled, rule.SuccessRate, rule.LastExecution, now, rule.ID)
	return err
}

func (r *PostgresAutomationRuleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM automation_rules WHERE id=$1`, id)
	return err
}

func (r *PostgresAutomationRuleRepository) List(ctx context.Context) ([]*models.AutomationRule, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name, COALESCE(description, ''), trigger_conditions, playbook_id, priority, auto_execute, cooldown_minutes, enabled, success_rate, last_execution, created_at, updated_at FROM automation_rules`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []*models.AutomationRule
	for rows.Next() {
		rule := &models.AutomationRule{}
		if err := rows.Scan(&rule.ID, &rule.Name, &rule.Description, &rule.TriggerConditions, &rule.PlaybookID, &rule.Priority, &rule.AutoExecute, &rule.CooldownMinutes, &rule.Enabled, &rule.SuccessRate, &rule.LastExecution, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func (r *PostgresAutomationRuleRepository) GetMatchingRules(ctx context.Context, alert *models.Alert) ([]*models.AutomationRule, error) {
	// Simplified logic for fetching matching rules. Ideally done in DB via jsonb querying.
	// For now, fetch all enabled rules and let service filter them.
	rows, err := r.pool.Query(ctx, `SELECT id, name, COALESCE(description, ''), trigger_conditions, playbook_id, priority, auto_execute, cooldown_minutes, enabled, success_rate, last_execution, created_at, updated_at FROM automation_rules WHERE enabled=true`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []*models.AutomationRule
	for rows.Next() {
		rule := &models.AutomationRule{}
		if err := rows.Scan(&rule.ID, &rule.Name, &rule.Description, &rule.TriggerConditions, &rule.PlaybookID, &rule.Priority, &rule.AutoExecute, &rule.CooldownMinutes, &rule.Enabled, &rule.SuccessRate, &rule.LastExecution, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// PostgresPlaybookExecutionRepository implements PlaybookExecutionRepository
type PostgresPlaybookExecutionRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresPlaybookExecutionRepository(pool *pgxpool.Pool) *PostgresPlaybookExecutionRepository {
	return &PostgresPlaybookExecutionRepository{pool: pool}
}

func (r *PostgresPlaybookExecutionRepository) Create(ctx context.Context, execution *models.PlaybookExecution) error {
	if execution.ID == uuid.Nil {
		execution.ID = uuid.New()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO playbook_executions (id, alert_id, playbook_id, rule_id, agent_id, status, started_at) 
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		execution.ID, execution.AlertID, execution.PlaybookID, execution.RuleID, execution.AgentID, execution.Status, execution.StartedAt)
	return err
}

func (r *PostgresPlaybookExecutionRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.PlaybookExecution, error) {
	exec := &models.PlaybookExecution{}
	err := r.pool.QueryRow(ctx, `SELECT id, alert_id, playbook_id, rule_id, agent_id, status, started_at, completed_at, error_message FROM playbook_executions WHERE id = $1`, id).
		Scan(&exec.ID, &exec.AlertID, &exec.PlaybookID, &exec.RuleID, &exec.AgentID, &exec.Status, &exec.StartedAt, &exec.CompletedAt, &exec.ErrorMessage)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	return exec, err
}

func (r *PostgresPlaybookExecutionRepository) Update(ctx context.Context, execution *models.PlaybookExecution) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE playbook_executions SET status=$1, completed_at=$2, execution_time_ms=$3, error_message=$4 WHERE id=$5`,
		execution.Status, execution.CompletedAt, execution.ExecutionTimeMs, execution.ErrorMessage, execution.ID)
	return err
}

func (r *PostgresPlaybookExecutionRepository) List(ctx context.Context, filter ExecutionFilter) ([]*models.PlaybookExecution, error) {
	limit := filter.Limit
	if limit == 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `SELECT id, alert_id, playbook_id, rule_id, agent_id, status, started_at, completed_at, error_message FROM playbook_executions LIMIT $1 OFFSET $2`, limit, filter.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var execs []*models.PlaybookExecution
	for rows.Next() {
		e := &models.PlaybookExecution{}
		if err := rows.Scan(&e.ID, &e.AlertID, &e.PlaybookID, &e.RuleID, &e.AgentID, &e.Status, &e.StartedAt, &e.CompletedAt, &e.ErrorMessage); err != nil {
			return nil, err
		}
		execs = append(execs, e)
	}
	return execs, nil
}

func (r *PostgresPlaybookExecutionRepository) GetByAlertID(ctx context.Context, alertID uuid.UUID) ([]*models.PlaybookExecution, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, alert_id, playbook_id, rule_id, agent_id, status, started_at, completed_at, error_message FROM playbook_executions WHERE alert_id = $1`, alertID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var execs []*models.PlaybookExecution
	for rows.Next() {
		e := &models.PlaybookExecution{}
		if err := rows.Scan(&e.ID, &e.AlertID, &e.PlaybookID, &e.RuleID, &e.AgentID, &e.Status, &e.StartedAt, &e.CompletedAt, &e.ErrorMessage); err != nil {
			return nil, err
		}
		execs = append(execs, e)
	}
	return execs, nil
}

func (r *PostgresPlaybookExecutionRepository) CreateSuggestion(ctx context.Context, suggestion *models.PlaybookSuggestion) error {
	if suggestion.ID == uuid.Nil {
		suggestion.ID = uuid.New()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO playbook_suggestions (id, alert_id, playbook_id, confidence, reason, mitre_match) 
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		suggestion.ID, suggestion.AlertID, suggestion.PlaybookID, suggestion.Confidence, suggestion.Reason, suggestion.MITREMatch)
	return err
}

func (r *PostgresPlaybookExecutionRepository) GetSuggestions(ctx context.Context, alertID uuid.UUID) ([]models.PlaybookSuggestion, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, alert_id, playbook_id, confidence, reason, mitre_match FROM playbook_suggestions WHERE alert_id = $1`, alertID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var suggestions []models.PlaybookSuggestion
	for rows.Next() {
		s := models.PlaybookSuggestion{}
		if err := rows.Scan(&s.ID, &s.AlertID, &s.PlaybookID, &s.Confidence, &s.Reason, &s.MITREMatch); err != nil {
			return nil, err
		}
		suggestions = append(suggestions, s)
	}
	return suggestions, nil
}

// PostgresAutomationMetricsRepository implements AutomationMetricsRepository
type PostgresAutomationMetricsRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresAutomationMetricsRepository(pool *pgxpool.Pool) *PostgresAutomationMetricsRepository {
	return &PostgresAutomationMetricsRepository{pool: pool}
}

func (r *PostgresAutomationMetricsRepository) GetRuleMetrics(ctx context.Context, ruleID uuid.UUID, since time.Time) (*models.AutomationMetrics, error) {
	m := &models.AutomationMetrics{}
	err := r.pool.QueryRow(ctx, `SELECT rule_id, SUM(executions_count), SUM(successful_executions), SUM(failed_executions), AVG(avg_execution_time_ms) FROM automation_metrics WHERE rule_id = $1 AND date >= $2 GROUP BY rule_id`, ruleID, since).
		Scan(&m.RuleID, &m.ExecutionsCount, &m.SuccessfulExecutions, &m.FailedExecutions, &m.AvgExecutionTimeMs)
	if err == pgx.ErrNoRows {
		return &models.AutomationMetrics{RuleID: ruleID}, nil // Return empty stats instead of error
	}
	return m, err
}

func (r *PostgresAutomationMetricsRepository) RecordRuleExecution(ctx context.Context, ruleID uuid.UUID, success bool, executionTime time.Duration) error {
	today := time.Now().Format("2006-01-02")
	var successInc, failInc int
	if success {
		successInc = 1
	} else {
		failInc = 1
	}
	
	_, err := r.pool.Exec(ctx, `
		INSERT INTO automation_metrics (rule_id, date, executions_count, successful_executions, failed_executions, avg_execution_time_ms) 
		VALUES ($1, $2, 1, $3, $4, $5)
		ON CONFLICT (rule_id, date) DO UPDATE SET
			executions_count = automation_metrics.executions_count + 1,
			successful_executions = automation_metrics.successful_executions + $3,
			failed_executions = automation_metrics.failed_executions + $4,
			avg_execution_time_ms = ((automation_metrics.avg_execution_time_ms * automation_metrics.executions_count) + $5) / (automation_metrics.executions_count + 1)
	`, ruleID, today, successInc, failInc, executionTime.Milliseconds())
	return err
}

func (r *PostgresAutomationMetricsRepository) GetMetrics(ctx context.Context, timeRange string) (*models.AutomationMetricsSummary, error) {
	// Simplified mock implementation to satisfy interface
	return &models.AutomationMetricsSummary{}, nil
}
