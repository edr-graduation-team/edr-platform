--
-- PostgreSQL database dump
--

\restrict iyM2yrw0N97vod3R2UmZqeaabresZAGfpxFNTpkvTG4Ylv1ofY2qAImghwl6dL1

-- Dumped from database version 16.13
-- Dumped by pg_dump version 16.13

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: add_to_command_queue(); Type: FUNCTION; Schema: public; Owner: edr
--

CREATE FUNCTION public.add_to_command_queue() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    INSERT INTO command_queue (command_id, agent_id, priority, scheduled_at)
    VALUES (NEW.id, NEW.agent_id, NEW.priority, NOW())
    ON CONFLICT (command_id) DO NOTHING;
    RETURN NEW;
END;
$$;


ALTER FUNCTION public.add_to_command_queue() OWNER TO edr;

--
-- Name: cleanup_expired_csrs(); Type: FUNCTION; Schema: public; Owner: edr
--

CREATE FUNCTION public.cleanup_expired_csrs() RETURNS integer
    LANGUAGE plpgsql
    AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM csrs 
    WHERE expires_at < NOW() AND approved = FALSE;
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$;


ALTER FUNCTION public.cleanup_expired_csrs() OWNER TO edr;

--
-- Name: cleanup_expired_installation_tokens(); Type: FUNCTION; Schema: public; Owner: edr
--

CREATE FUNCTION public.cleanup_expired_installation_tokens() RETURNS integer
    LANGUAGE plpgsql
    AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM installation_tokens 
    WHERE expires_at < NOW() AND used = FALSE;
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$;


ALTER FUNCTION public.cleanup_expired_installation_tokens() OWNER TO edr;

--
-- Name: FUNCTION cleanup_expired_installation_tokens(); Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON FUNCTION public.cleanup_expired_installation_tokens() IS 'Cleanup function for expired unused tokens';


--
-- Name: increment_policy_version(); Type: FUNCTION; Schema: public; Owner: edr
--

CREATE FUNCTION public.increment_policy_version() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.version = OLD.version + 1;
    RETURN NEW;
END;
$$;


ALTER FUNCTION public.increment_policy_version() OWNER TO edr;

--
-- Name: process_alert_on_create(); Type: FUNCTION; Schema: public; Owner: edr
--

CREATE FUNCTION public.process_alert_on_create() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    alert_record RECORD;
    matching_rules INTEGER;
BEGIN
    -- الحصول على بيانات التنبيه الجديد
    SELECT * INTO alert_record FROM alerts WHERE id = NEW.id;
    
    -- تسجيل التنبيه للمعالجة
    RAISE NOTICE 'Processing alert %: severity=%, rule=%, agent=%', 
        NEW.id, NEW.severity, NEW.rule_name, NEW.agent_id;
    
    -- تحديث إحصائيات التنبيهات
    INSERT INTO alert_stats (date, alert_count, critical_count, high_count, medium_count, low_count)
    VALUES (CURRENT_DATE, 1, 
        CASE WHEN NEW.severity = 'critical' THEN 1 ELSE 0 END,
        CASE WHEN NEW.severity = 'high' THEN 1 ELSE 0 END,
        CASE WHEN NEW.severity = 'medium' THEN 1 ELSE 0 END,
        CASE WHEN NEW.severity = 'low' THEN 1 ELSE 0 END)
    ON CONFLICT (date) DO UPDATE SET
        alert_count = alert_stats.alert_count + 1,
        critical_count = alert_stats.critical_count + 
            CASE WHEN NEW.severity = 'critical' THEN 1 ELSE 0 END,
        high_count = alert_stats.high_count + 
            CASE WHEN NEW.severity = 'high' THEN 1 ELSE 0 END,
        medium_count = alert_stats.medium_count + 
            CASE WHEN NEW.severity = 'medium' THEN 1 ELSE 0 END,
        low_count = alert_stats.low_count + 
            CASE WHEN NEW.severity = 'low' THEN 1 ELSE 0 END,
        updated_at = now();
    
    RETURN NEW;
END;
$$;


ALTER FUNCTION public.process_alert_on_create() OWNER TO edr;

--
-- Name: recompute_all_vuln_priorities(); Type: FUNCTION; Schema: public; Owner: edr
--

CREATE FUNCTION public.recompute_all_vuln_priorities() RETURNS integer
    LANGUAGE plpgsql
    AS $$
DECLARE
    n INTEGER;
BEGIN
    UPDATE vulnerability_findings vf
    SET priority_score = vuln_priority_score(
            vf.cvss, vf.kev_listed, vf.exploit_available, a.criticality
        ),
        updated_at = NOW()
    FROM agents a
    WHERE vf.agent_id = a.id;
    GET DIAGNOSTICS n = ROW_COUNT;
    RETURN n;
END$$;


ALTER FUNCTION public.recompute_all_vuln_priorities() OWNER TO edr;

--
-- Name: recompute_vuln_priority_for_agent(uuid); Type: FUNCTION; Schema: public; Owner: edr
--

CREATE FUNCTION public.recompute_vuln_priority_for_agent(p_agent_id uuid) RETURNS integer
    LANGUAGE plpgsql
    AS $$
DECLARE
    n INTEGER;
BEGIN
    UPDATE vulnerability_findings vf
    SET priority_score = vuln_priority_score(
            vf.cvss, vf.kev_listed, vf.exploit_available, a.criticality
        ),
        updated_at = NOW()
    FROM agents a
    WHERE vf.agent_id = a.id
      AND vf.agent_id = p_agent_id;
    GET DIAGNOSTICS n = ROW_COUNT;
    RETURN n;
END$$;


ALTER FUNCTION public.recompute_vuln_priority_for_agent(p_agent_id uuid) OWNER TO edr;

--
-- Name: set_command_expires(); Type: FUNCTION; Schema: public; Owner: edr
--

CREATE FUNCTION public.set_command_expires() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.expires_at IS NULL THEN
        NEW.expires_at = NEW.issued_at + (NEW.timeout_seconds || ' seconds')::INTERVAL;
    END IF;
    RETURN NEW;
END;
$$;


ALTER FUNCTION public.set_command_expires() OWNER TO edr;

--
-- Name: trg_agent_criticality_changed(); Type: FUNCTION; Schema: public; Owner: edr
--

CREATE FUNCTION public.trg_agent_criticality_changed() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.criticality IS DISTINCT FROM OLD.criticality THEN
        PERFORM recompute_vuln_priority_for_agent(NEW.id);
    END IF;
    RETURN NEW;
END$$;


ALTER FUNCTION public.trg_agent_criticality_changed() OWNER TO edr;

--
-- Name: update_enrollment_tokens_updated_at(); Type: FUNCTION; Schema: public; Owner: edr
--

CREATE FUNCTION public.update_enrollment_tokens_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;


ALTER FUNCTION public.update_enrollment_tokens_updated_at() OWNER TO edr;

--
-- Name: update_process_baselines_updated_at(); Type: FUNCTION; Schema: public; Owner: edr
--

CREATE FUNCTION public.update_process_baselines_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$;


ALTER FUNCTION public.update_process_baselines_updated_at() OWNER TO edr;

--
-- Name: update_sigma_alerts_updated_at(); Type: FUNCTION; Schema: public; Owner: edr
--

CREATE FUNCTION public.update_sigma_alerts_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$;


ALTER FUNCTION public.update_sigma_alerts_updated_at() OWNER TO edr;

--
-- Name: update_sigma_rules_updated_at(); Type: FUNCTION; Schema: public; Owner: edr
--

CREATE FUNCTION public.update_sigma_rules_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$;


ALTER FUNCTION public.update_sigma_rules_updated_at() OWNER TO edr;

--
-- Name: update_updated_at_column(); Type: FUNCTION; Schema: public; Owner: edr
--

CREATE FUNCTION public.update_updated_at_column() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;


ALTER FUNCTION public.update_updated_at_column() OWNER TO edr;

--
-- Name: vuln_priority_score(double precision, boolean, boolean, character varying); Type: FUNCTION; Schema: public; Owner: edr
--

CREATE FUNCTION public.vuln_priority_score(p_cvss double precision, p_kev boolean, p_exploit boolean, p_criticality character varying) RETURNS double precision
    LANGUAGE plpgsql IMMUTABLE
    AS $$
DECLARE
    base   DOUBLE PRECISION := COALESCE(p_cvss, 0) * 10;     -- 0..100
    bonus  DOUBLE PRECISION := 0;
    crit   DOUBLE PRECISION := 0;
    final  DOUBLE PRECISION;
BEGIN
    IF p_kev     THEN bonus := bonus + 25; END IF;
    IF p_exploit THEN bonus := bonus + 10; END IF;

    -- Criticality adjustment: weights the asset's business value into the score.
    crit := CASE COALESCE(p_criticality, 'medium')
        WHEN 'low'      THEN -15
        WHEN 'medium'   THEN   0
        WHEN 'high'     THEN  10
        WHEN 'critical' THEN  20
        ELSE                   0
    END;

    final := base + bonus + crit;
    IF final > 100 THEN final := 100; END IF;
    IF final < 0   THEN final := 0;   END IF;
    RETURN final;
END$$;


ALTER FUNCTION public.vuln_priority_score(p_cvss double precision, p_kev boolean, p_exploit boolean, p_criticality character varying) OWNER TO edr;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: agent_packages; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.agent_packages (
    id uuid NOT NULL,
    sha256 text NOT NULL,
    filename text DEFAULT 'edr-agent.exe'::text NOT NULL,
    storage_path text NOT NULL,
    build_params jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    agent_id uuid,
    consumed_at timestamp with time zone
);


ALTER TABLE public.agent_packages OWNER TO edr;

--
-- Name: agent_patch_profiles; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.agent_patch_profiles (
    agent_id uuid NOT NULL,
    profile jsonb DEFAULT '{}'::jsonb NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.agent_patch_profiles OWNER TO edr;

--
-- Name: agent_quarantine_items; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.agent_quarantine_items (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    agent_id uuid NOT NULL,
    event_id text,
    original_path text NOT NULL,
    quarantine_path text NOT NULL,
    sha256 text,
    threat_name text,
    source text DEFAULT 'auto_responder'::text NOT NULL,
    state text DEFAULT 'quarantined'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT agent_quarantine_items_source_check CHECK ((source = ANY (ARRAY['auto_responder'::text, 'manual_c2'::text]))),
    CONSTRAINT agent_quarantine_items_state_check CHECK ((state = ANY (ARRAY['quarantined'::text, 'acknowledged'::text, 'restored'::text, 'deleted'::text])))
);


ALTER TABLE public.agent_quarantine_items OWNER TO edr;

--
-- Name: agents; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.agents (
    id uuid NOT NULL,
    hostname character varying(255) NOT NULL,
    status character varying(50) DEFAULT 'pending'::character varying NOT NULL,
    os_type character varying(50),
    os_version character varying(100),
    cpu_count integer,
    memory_mb bigint,
    agent_version character varying(50),
    installed_date timestamp with time zone,
    last_seen timestamp with time zone,
    events_collected bigint DEFAULT 0,
    events_delivered bigint DEFAULT 0,
    queue_depth integer DEFAULT 0,
    cpu_usage double precision DEFAULT 0,
    memory_used_mb bigint DEFAULT 0,
    health_score double precision DEFAULT 100.0,
    current_cert_id uuid,
    cert_expires_at timestamp with time zone,
    tags jsonb DEFAULT '{}'::jsonb,
    metadata jsonb DEFAULT '{}'::jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    is_isolated boolean DEFAULT false NOT NULL,
    events_dropped bigint DEFAULT 0,
    ip_addresses jsonb DEFAULT '[]'::jsonb,
    hardware_id character varying(128),
    sysmon_installed boolean DEFAULT false NOT NULL,
    sysmon_running boolean DEFAULT false NOT NULL,
    criticality character varying(16) DEFAULT 'medium'::character varying NOT NULL,
    business_unit text DEFAULT 'unknown'::text NOT NULL,
    environment character varying(32) DEFAULT 'unknown'::character varying NOT NULL,
    CONSTRAINT agents_criticality_chk CHECK (((criticality)::text = ANY ((ARRAY['low'::character varying, 'medium'::character varying, 'high'::character varying, 'critical'::character varying])::text[])))
);


ALTER TABLE public.agents OWNER TO edr;

--
-- Name: TABLE agents; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON TABLE public.agents IS 'Registered EDR agents with their status and metrics';


--
-- Name: COLUMN agents.id; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.agents.id IS 'Unique agent identifier (UUID)';


--
-- Name: COLUMN agents.hostname; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.agents.hostname IS 'Agent hostname (must be unique)';


--
-- Name: COLUMN agents.status; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.agents.status IS 'Agent status: pending, online, offline, degraded, suspended';


--
-- Name: COLUMN agents.health_score; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.agents.health_score IS 'Calculated health score (0-100)';


--
-- Name: COLUMN agents.is_isolated; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.agents.is_isolated IS 'Whether the agent network is currently isolated (firewall-blocked except C2)';


--
-- Name: COLUMN agents.events_dropped; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.agents.events_dropped IS 'Cumulative events filtered/rate-limited at agent edge (potential blinding indicator)';


--
-- Name: COLUMN agents.ip_addresses; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.agents.ip_addresses IS 'JSONB array of agent non-loopback IP addresses from last heartbeat';


--
-- Name: COLUMN agents.sysmon_installed; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.agents.sysmon_installed IS 'True when the Sysmon64 service is present on the endpoint (binary exists + service registered).';


--
-- Name: COLUMN agents.sysmon_running; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.agents.sysmon_running IS 'True when the Sysmon64 service is in RUNNING state at last heartbeat.';


--
-- Name: alert_stats; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.alert_stats (
    date date NOT NULL,
    alert_count integer DEFAULT 0,
    critical_count integer DEFAULT 0,
    high_count integer DEFAULT 0,
    medium_count integer DEFAULT 0,
    low_count integer DEFAULT 0,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


ALTER TABLE public.alert_stats OWNER TO edr;

--
-- Name: alerts; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.alerts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    severity character varying(20) NOT NULL,
    title character varying(500) NOT NULL,
    description text,
    agent_id uuid NOT NULL,
    rule_id character varying(100),
    rule_name character varying(255),
    status character varying(20) DEFAULT 'open'::character varying NOT NULL,
    assigned_to uuid,
    resolution character varying(50),
    resolution_notes text,
    event_count integer DEFAULT 1,
    first_event_at timestamp with time zone,
    last_event_at timestamp with time zone,
    detected_at timestamp with time zone DEFAULT now() NOT NULL,
    acknowledged_at timestamp with time zone,
    resolved_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    tags jsonb DEFAULT '{}'::jsonb,
    metadata jsonb DEFAULT '{}'::jsonb,
    notes text,
    risk_score integer DEFAULT 0 NOT NULL,
    context_snapshot jsonb DEFAULT '{}'::jsonb NOT NULL,
    score_breakdown jsonb DEFAULT '{}'::jsonb NOT NULL,
    false_positive_risk numeric(4,3) DEFAULT 0.000 NOT NULL,
    CONSTRAINT alerts_resolution_check CHECK (((resolution)::text = ANY ((ARRAY['false_positive'::character varying, 'remediated'::character varying, 'escalated'::character varying, 'accepted_risk'::character varying, 'duplicate'::character varying])::text[]))),
    CONSTRAINT alerts_severity_check CHECK (((severity)::text = ANY ((ARRAY['critical'::character varying, 'high'::character varying, 'medium'::character varying, 'low'::character varying, 'informational'::character varying])::text[]))),
    CONSTRAINT alerts_status_check CHECK (((status)::text = ANY ((ARRAY['open'::character varying, 'in_progress'::character varying, 'resolved'::character varying, 'closed'::character varying, 'false_positive'::character varying])::text[])))
);


ALTER TABLE public.alerts OWNER TO edr;

--
-- Name: TABLE alerts; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON TABLE public.alerts IS 'Security alerts generated from event analysis';


--
-- Name: COLUMN alerts.severity; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.alerts.severity IS 'Alert severity: critical, high, medium, low, informational';


--
-- Name: COLUMN alerts.status; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.alerts.status IS 'Alert status: open, in_progress, resolved, closed, false_positive';


--
-- Name: COLUMN alerts.event_count; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.alerts.event_count IS 'Number of correlated events';


--
-- Name: COLUMN alerts.risk_score; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.alerts.risk_score IS 'Context-aware risk score (0-100) computed by the sigma-engine RiskScorer.';


--
-- Name: COLUMN alerts.context_snapshot; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.alerts.context_snapshot IS 'Full forensic evidence snapshot: ancestor chain, privilege context, burst count.';


--
-- Name: COLUMN alerts.score_breakdown; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.alerts.score_breakdown IS 'Component-level breakdown of the risk_score formula for SOC analyst transparency.';


--
-- Name: COLUMN alerts.false_positive_risk; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.alerts.false_positive_risk IS 'False positive probability estimate (0.000-1.000) based on signature and known-good path signals.';


--
-- Name: audit_logs; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.audit_logs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid,
    username character varying(255),
    action character varying(100) NOT NULL,
    resource_type character varying(100),
    resource_id uuid,
    old_value jsonb,
    new_value jsonb,
    result character varying(20) DEFAULT 'success'::character varying NOT NULL,
    error_message text,
    ip_address text DEFAULT ''::text,
    user_agent text DEFAULT ''::text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.audit_logs OWNER TO edr;

--
-- Name: TABLE audit_logs; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON TABLE public.audit_logs IS 'Immutable security audit trail';


--
-- Name: COLUMN audit_logs.ip_address; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.audit_logs.ip_address IS 'Client IP as TEXT (not INET) to accept empty strings';


--
-- Name: automation_metrics; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.automation_metrics (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    rule_id uuid,
    date date NOT NULL,
    executions_count integer DEFAULT 0,
    successful_executions integer DEFAULT 0,
    failed_executions integer DEFAULT 0,
    avg_execution_time_ms integer DEFAULT 0,
    created_at timestamp with time zone DEFAULT now()
);


ALTER TABLE public.automation_metrics OWNER TO edr;

--
-- Name: automation_rules; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.automation_rules (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    trigger_conditions jsonb DEFAULT '{}'::jsonb NOT NULL,
    playbook_id uuid NOT NULL,
    priority integer DEFAULT 100,
    auto_execute boolean DEFAULT false,
    cooldown_minutes integer DEFAULT 30,
    enabled boolean DEFAULT true,
    success_rate double precision DEFAULT 0.0,
    last_execution timestamp with time zone,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


ALTER TABLE public.automation_rules OWNER TO edr;

--
-- Name: certificates; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.certificates (
    id uuid NOT NULL,
    agent_id uuid NOT NULL,
    cert_fingerprint character varying(64) NOT NULL,
    public_key text NOT NULL,
    serial_number character varying(100),
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    issued_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    revoked_by uuid,
    revoke_reason character varying(255),
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.certificates OWNER TO edr;

--
-- Name: TABLE certificates; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON TABLE public.certificates IS 'Agent TLS certificates with revocation tracking';


--
-- Name: COLUMN certificates.cert_fingerprint; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.certificates.cert_fingerprint IS 'SHA256 fingerprint of the certificate';


--
-- Name: COLUMN certificates.status; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.certificates.status IS 'Certificate status: active, expired, revoked, superseded';


--
-- Name: command_queue; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.command_queue (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    command_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    priority integer NOT NULL,
    scheduled_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.command_queue OWNER TO edr;

--
-- Name: TABLE command_queue; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON TABLE public.command_queue IS 'Priority queue for pending commands';


--
-- Name: commands; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.commands (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    agent_id uuid NOT NULL,
    command_type character varying(50) NOT NULL,
    parameters jsonb DEFAULT '{}'::jsonb,
    priority integer DEFAULT 5 NOT NULL,
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    result jsonb,
    error_message text,
    exit_code integer,
    timeout_seconds integer DEFAULT 300 NOT NULL,
    issued_at timestamp with time zone DEFAULT now() NOT NULL,
    sent_at timestamp with time zone,
    acknowledged_at timestamp with time zone,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    expires_at timestamp with time zone,
    issued_by uuid,
    metadata jsonb DEFAULT '{}'::jsonb,
    CONSTRAINT commands_command_type_check CHECK (((command_type)::text = ANY ((ARRAY['kill_process'::character varying, 'terminate_process'::character varying, 'quarantine_file'::character varying, 'restore_quarantine_file'::character varying, 'delete_quarantine_file'::character varying, 'collect_logs'::character varying, 'collect_forensics'::character varying, 'isolate_network'::character varying, 'isolate'::character varying, 'restore_network'::character varying, 'unisolate_network'::character varying, 'unisolate'::character varying, 'restart_agent'::character varying, 'restart_service'::character varying, 'start_agent'::character varying, 'start_service'::character varying, 'stop_agent'::character varying, 'stop_service'::character varying, 'restart_machine'::character varying, 'restart'::character varying, 'shutdown_machine'::character varying, 'shutdown'::character varying, 'scan_file'::character varying, 'scan_memory'::character varying, 'update_agent'::character varying, 'uninstall_agent'::character varying, 'update_policy'::character varying, 'update_config'::character varying, 'update_filter_policy'::character varying, 'update_vuln_config'::character varying, 'adjust_rate'::character varying, 'run_cmd'::character varying, 'custom'::character varying, 'block_ip'::character varying, 'unblock_ip'::character varying, 'block_domain'::character varying, 'unblock_domain'::character varying, 'update_signatures'::character varying, 'enable_sysmon'::character varying, 'disable_sysmon'::character varying, 'post_isolation_triage'::character varying, 'process_tree_snapshot'::character varying, 'persistence_scan'::character varying, 'lsass_access_audit'::character varying, 'filesystem_timeline'::character varying, 'network_last_seen'::character varying, 'agent_integrity_check'::character varying, 'memory_dump'::character varying])::text[]))),
    CONSTRAINT commands_priority_check CHECK (((priority >= 1) AND (priority <= 10))),
    CONSTRAINT commands_status_check CHECK (((status)::text = ANY ((ARRAY['pending'::character varying, 'sent'::character varying, 'acknowledged'::character varying, 'executing'::character varying, 'completed'::character varying, 'failed'::character varying, 'timeout'::character varying, 'cancelled'::character varying])::text[])))
);


ALTER TABLE public.commands OWNER TO edr;

--
-- Name: TABLE commands; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON TABLE public.commands IS 'Remote commands to be executed on agents';


--
-- Name: COLUMN commands.command_type; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.commands.command_type IS 'Type of command to execute';


--
-- Name: COLUMN commands.parameters; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.commands.parameters IS 'Command-specific parameters as JSON';


--
-- Name: COLUMN commands.priority; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.commands.priority IS 'Execution priority (1=lowest, 10=highest)';


--
-- Name: COLUMN commands.status; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.commands.status IS 'Current execution status';


--
-- Name: context_policies; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.context_policies (
    id bigint NOT NULL,
    name text NOT NULL,
    scope_type text NOT NULL,
    scope_value text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    user_role_weight double precision DEFAULT 1.0 NOT NULL,
    device_criticality_weight double precision DEFAULT 1.0 NOT NULL,
    network_anomaly_factor double precision DEFAULT 1.0 NOT NULL,
    trusted_networks jsonb DEFAULT '[]'::jsonb NOT NULL,
    notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT context_policies_scope_type_check CHECK ((scope_type = ANY (ARRAY['global'::text, 'agent'::text, 'user'::text])))
);


ALTER TABLE public.context_policies OWNER TO edr;

--
-- Name: context_policies_id_seq; Type: SEQUENCE; Schema: public; Owner: edr
--

CREATE SEQUENCE public.context_policies_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.context_policies_id_seq OWNER TO edr;

--
-- Name: context_policies_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: edr
--

ALTER SEQUENCE public.context_policies_id_seq OWNED BY public.context_policies.id;


--
-- Name: csrs; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.csrs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    agent_id uuid NOT NULL,
    csr_data text NOT NULL,
    approved boolean DEFAULT false,
    approved_by uuid,
    approved_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone DEFAULT (now() + '24:00:00'::interval) NOT NULL
);


ALTER TABLE public.csrs OWNER TO edr;

--
-- Name: TABLE csrs; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON TABLE public.csrs IS 'Pending Certificate Signing Requests awaiting approval';


--
-- Name: enrollment_token_consumptions; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.enrollment_token_consumptions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    token_id uuid NOT NULL,
    hardware_id character varying(128) NOT NULL,
    agent_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.enrollment_token_consumptions OWNER TO edr;

--
-- Name: enrollment_tokens; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.enrollment_tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    token character varying(128) NOT NULL,
    description character varying(255) DEFAULT ''::character varying NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    expires_at timestamp with time zone,
    use_count integer DEFAULT 0 NOT NULL,
    max_uses integer,
    created_by character varying(255) DEFAULT 'system'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    revoked_at timestamp with time zone,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.enrollment_tokens OWNER TO edr;

--
-- Name: TABLE enrollment_tokens; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON TABLE public.enrollment_tokens IS 'Dynamic enrollment tokens for agent registration, managed via Dashboard';


--
-- Name: COLUMN enrollment_tokens.token; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.enrollment_tokens.token IS 'Cryptographically-secure random token string (hex, 64 chars)';


--
-- Name: COLUMN enrollment_tokens.is_active; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.enrollment_tokens.is_active IS 'FALSE = revoked; enrollment requests using this token will be rejected';


--
-- Name: COLUMN enrollment_tokens.max_uses; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.enrollment_tokens.max_uses IS 'NULL = unlimited uses; otherwise token is auto-deactivated after max_uses enrollments';


--
-- Name: event_batches_fallback; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.event_batches_fallback (
    id bigint NOT NULL,
    batch_id text NOT NULL,
    agent_id text NOT NULL,
    payload bytea NOT NULL,
    metadata jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    replayed boolean DEFAULT false NOT NULL,
    replayed_at timestamp with time zone
);


ALTER TABLE public.event_batches_fallback OWNER TO edr;

--
-- Name: event_batches_fallback_id_seq; Type: SEQUENCE; Schema: public; Owner: edr
--

CREATE SEQUENCE public.event_batches_fallback_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.event_batches_fallback_id_seq OWNER TO edr;

--
-- Name: event_batches_fallback_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: edr
--

ALTER SEQUENCE public.event_batches_fallback_id_seq OWNED BY public.event_batches_fallback.id;


--
-- Name: events; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    agent_id uuid NOT NULL,
    batch_id uuid,
    event_type character varying(100) NOT NULL,
    severity character varying(50) DEFAULT 'informational'::character varying NOT NULL,
    ts timestamp with time zone NOT NULL,
    summary text DEFAULT ''::text NOT NULL,
    raw jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.events OWNER TO edr;

--
-- Name: TABLE events; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON TABLE public.events IS 'Durable event telemetry (searchable subset + raw JSONB)';


--
-- Name: COLUMN events.ts; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.events.ts IS 'Event timestamp (parsed from event.timestamp)';


--
-- Name: COLUMN events.raw; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.events.raw IS 'Validated event JSON object from ingestion stream';


--
-- Name: forensic_collections; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.forensic_collections (
    command_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    command_type character varying(64) NOT NULL,
    issued_at timestamp with time zone NOT NULL,
    completed_at timestamp with time zone,
    time_range text,
    log_types text,
    summary jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.forensic_collections OWNER TO edr;

--
-- Name: forensic_events; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.forensic_events (
    id bigint NOT NULL,
    command_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    log_type text NOT NULL,
    ts timestamp with time zone,
    event_id text,
    level text,
    provider text,
    message text,
    raw jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.forensic_events OWNER TO edr;

--
-- Name: forensic_events_id_seq; Type: SEQUENCE; Schema: public; Owner: edr
--

CREATE SEQUENCE public.forensic_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.forensic_events_id_seq OWNER TO edr;

--
-- Name: forensic_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: edr
--

ALTER SEQUENCE public.forensic_events_id_seq OWNED BY public.forensic_events.id;


--
-- Name: installation_tokens; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.installation_tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    token_value character varying(255) NOT NULL,
    agent_id uuid,
    used boolean DEFAULT false,
    used_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone DEFAULT (now() + '24:00:00'::interval) NOT NULL
);


ALTER TABLE public.installation_tokens OWNER TO edr;

--
-- Name: TABLE installation_tokens; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON TABLE public.installation_tokens IS 'One-time tokens for agent registration (24h validity)';


--
-- Name: ioc_enrichment; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.ioc_enrichment (
    id bigint NOT NULL,
    agent_id uuid,
    run_id bigint,
    ioc_type text NOT NULL,
    ioc_value text NOT NULL,
    provider text NOT NULL,
    verdict text,
    score integer,
    raw jsonb,
    fetched_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.ioc_enrichment OWNER TO edr;

--
-- Name: ioc_enrichment_id_seq; Type: SEQUENCE; Schema: public; Owner: edr
--

CREATE SEQUENCE public.ioc_enrichment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.ioc_enrichment_id_seq OWNER TO edr;

--
-- Name: ioc_enrichment_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: edr
--

ALTER SEQUENCE public.ioc_enrichment_id_seq OWNED BY public.ioc_enrichment.id;


--
-- Name: kev_catalog; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.kev_catalog (
    cve character varying(64) NOT NULL,
    vendor_project text DEFAULT ''::text NOT NULL,
    product text DEFAULT ''::text NOT NULL,
    vulnerability_name text DEFAULT ''::text NOT NULL,
    date_added timestamp with time zone,
    short_description text DEFAULT ''::text NOT NULL,
    required_action text DEFAULT ''::text NOT NULL,
    due_date timestamp with time zone,
    known_ransomware text DEFAULT ''::text NOT NULL,
    notes text DEFAULT ''::text NOT NULL,
    synced_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.kev_catalog OWNER TO edr;

--
-- Name: malware_hashes; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.malware_hashes (
    id integer NOT NULL,
    sha256 character varying(64) NOT NULL,
    name text,
    family text,
    severity text,
    source text,
    first_seen timestamp with time zone DEFAULT now() NOT NULL,
    version bigint NOT NULL
);


ALTER TABLE public.malware_hashes OWNER TO edr;

--
-- Name: malware_hashes_id_seq; Type: SEQUENCE; Schema: public; Owner: edr
--

CREATE SEQUENCE public.malware_hashes_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.malware_hashes_id_seq OWNER TO edr;

--
-- Name: malware_hashes_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: edr
--

ALTER SEQUENCE public.malware_hashes_id_seq OWNED BY public.malware_hashes.id;


--
-- Name: permissions; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.permissions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    resource character varying(50) NOT NULL,
    action character varying(50) NOT NULL,
    description text DEFAULT ''::text NOT NULL
);


ALTER TABLE public.permissions OWNER TO edr;

--
-- Name: TABLE permissions; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON TABLE public.permissions IS 'Granular permissions: resource:action pairs';


--
-- Name: playbook_executions; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.playbook_executions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    alert_id uuid NOT NULL,
    playbook_id uuid NOT NULL,
    rule_id uuid,
    agent_id uuid NOT NULL,
    status character varying(50) DEFAULT 'pending'::character varying,
    started_at timestamp with time zone DEFAULT now(),
    completed_at timestamp with time zone,
    commands_executed integer DEFAULT 0,
    commands_total integer DEFAULT 0,
    result jsonb DEFAULT '{}'::jsonb,
    error_message text,
    created_by uuid,
    execution_time_ms integer,
    CONSTRAINT playbook_executions_status_check CHECK (((status)::text = ANY ((ARRAY['pending'::character varying, 'running'::character varying, 'completed'::character varying, 'failed'::character varying, 'cancelled'::character varying])::text[])))
);


ALTER TABLE public.playbook_executions OWNER TO edr;

--
-- Name: playbook_runs; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.playbook_runs (
    id bigint NOT NULL,
    agent_id uuid NOT NULL,
    playbook text DEFAULT 'default_post_isolation'::text NOT NULL,
    trigger text DEFAULT 'isolation.succeeded'::text NOT NULL,
    status text DEFAULT 'running'::text NOT NULL,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    finished_at timestamp with time zone,
    summary jsonb DEFAULT '{}'::jsonb NOT NULL
);


ALTER TABLE public.playbook_runs OWNER TO edr;

--
-- Name: playbook_runs_id_seq; Type: SEQUENCE; Schema: public; Owner: edr
--

CREATE SEQUENCE public.playbook_runs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.playbook_runs_id_seq OWNER TO edr;

--
-- Name: playbook_runs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: edr
--

ALTER SEQUENCE public.playbook_runs_id_seq OWNED BY public.playbook_runs.id;


--
-- Name: playbook_steps; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.playbook_steps (
    id bigint NOT NULL,
    run_id bigint NOT NULL,
    step_name text NOT NULL,
    command_type text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    command_id uuid,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    error text
);


ALTER TABLE public.playbook_steps OWNER TO edr;

--
-- Name: playbook_steps_id_seq; Type: SEQUENCE; Schema: public; Owner: edr
--

CREATE SEQUENCE public.playbook_steps_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.playbook_steps_id_seq OWNER TO edr;

--
-- Name: playbook_steps_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: edr
--

ALTER SEQUENCE public.playbook_steps_id_seq OWNED BY public.playbook_steps.id;


--
-- Name: playbook_suggestions; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.playbook_suggestions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    alert_id uuid NOT NULL,
    playbook_id uuid NOT NULL,
    confidence double precision,
    reason text,
    mitre_match character varying[],
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT playbook_suggestions_confidence_check CHECK (((confidence >= (0)::double precision) AND (confidence <= (1)::double precision)))
);


ALTER TABLE public.playbook_suggestions OWNER TO edr;

--
-- Name: policies; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.policies (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    rules jsonb DEFAULT '[]'::jsonb NOT NULL,
    targets jsonb DEFAULT '{"agents": [], "groups": [], "apply_to_all": false}'::jsonb,
    enabled boolean DEFAULT true NOT NULL,
    priority integer DEFAULT 100 NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    created_by uuid NOT NULL,
    updated_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    tags jsonb DEFAULT '{}'::jsonb,
    metadata jsonb DEFAULT '{}'::jsonb
);


ALTER TABLE public.policies OWNER TO edr;

--
-- Name: TABLE policies; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON TABLE public.policies IS 'Security policies for agent configuration';


--
-- Name: COLUMN policies.rules; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.policies.rules IS 'JSON array of policy rules';


--
-- Name: COLUMN policies.targets; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.policies.targets IS 'JSON object defining target agents/groups';


--
-- Name: policy_agent_assignments; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.policy_agent_assignments (
    policy_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    assigned_at timestamp with time zone DEFAULT now() NOT NULL,
    assigned_by uuid
);


ALTER TABLE public.policy_agent_assignments OWNER TO edr;

--
-- Name: TABLE policy_agent_assignments; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON TABLE public.policy_agent_assignments IS 'Policy to agent assignments';


--
-- Name: policy_versions; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.policy_versions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    policy_id uuid NOT NULL,
    version integer NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    rules jsonb NOT NULL,
    targets jsonb,
    enabled boolean NOT NULL,
    priority integer NOT NULL,
    changed_by uuid,
    change_reason text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.policy_versions OWNER TO edr;

--
-- Name: TABLE policy_versions; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON TABLE public.policy_versions IS 'Historical versions of policies for audit';


--
-- Name: process_baselines; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.process_baselines (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    agent_id character varying(255) NOT NULL,
    process_name character varying(512) NOT NULL,
    process_path character varying(1024),
    hour_of_day smallint NOT NULL,
    avg_executions_per_hour numeric(10,4) DEFAULT 0.0 NOT NULL,
    max_executions_per_hour integer DEFAULT 0 NOT NULL,
    min_executions_per_hour integer DEFAULT 0 NOT NULL,
    stddev_executions numeric(10,4) DEFAULT 0.0,
    observation_days integer DEFAULT 0 NOT NULL,
    typical_signature_status character varying(50),
    typical_integrity_level character varying(20),
    typically_elevated boolean DEFAULT false,
    common_parents jsonb DEFAULT '[]'::jsonb NOT NULL,
    confidence_score numeric(3,2) DEFAULT 0.00 NOT NULL,
    last_observed_at timestamp with time zone,
    baseline_window_days integer DEFAULT 14 NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT process_baselines_hour_of_day_check CHECK (((hour_of_day >= 0) AND (hour_of_day <= 23)))
);


ALTER TABLE public.process_baselines OWNER TO edr;

--
-- Name: TABLE process_baselines; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON TABLE public.process_baselines IS 'Behavioral baseline for process execution frequency per agent per hour-of-day. Used by RiskScorer Sprint 4 to compute contextual false-positive discounts for statistically normal behavior.';


--
-- Name: COLUMN process_baselines.hour_of_day; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.process_baselines.hour_of_day IS 'Hour in UTC (0-23) for circadian behavioral profiling.';


--
-- Name: COLUMN process_baselines.avg_executions_per_hour; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.process_baselines.avg_executions_per_hour IS 'Rolling 14-day average number of times this process starts per hour on this agent.';


--
-- Name: COLUMN process_baselines.common_parents; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.process_baselines.common_parents IS 'JSON array of most frequent parent process names observed spawning this process.';


--
-- Name: COLUMN process_baselines.confidence_score; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.process_baselines.confidence_score IS 'Model confidence [0.00-1.00]. Formula: 1 - exp(-observation_days/7). Reaches ~0.86 after 14 days.';


--
-- Name: response_playbooks; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.response_playbooks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    category character varying(100) NOT NULL,
    severity_filter character varying[] DEFAULT '{}'::character varying[],
    rule_pattern character varying(255),
    commands jsonb DEFAULT '[]'::jsonb NOT NULL,
    mitre_techniques character varying[] DEFAULT '{}'::character varying[],
    enabled boolean DEFAULT true,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    CONSTRAINT response_playbooks_category_check CHECK (((category)::text = ANY ((ARRAY['containment'::character varying, 'investigation'::character varying, 'remediation'::character varying, 'validation'::character varying])::text[])))
);


ALTER TABLE public.response_playbooks OWNER TO edr;

--
-- Name: role_permissions; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.role_permissions (
    role_id uuid NOT NULL,
    permission_id uuid NOT NULL
);


ALTER TABLE public.role_permissions OWNER TO edr;

--
-- Name: TABLE role_permissions; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON TABLE public.role_permissions IS 'Maps roles to their granted permissions';


--
-- Name: roles; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.roles (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name character varying(50) NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    is_built_in boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.roles OWNER TO edr;

--
-- Name: TABLE roles; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON TABLE public.roles IS 'RBAC roles — both built-in and custom';


--
-- Name: schema_migrations; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.schema_migrations (
    version bigint NOT NULL,
    dirty boolean NOT NULL
);


ALTER TABLE public.schema_migrations OWNER TO edr;

--
-- Name: siem_connectors; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.siem_connectors (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name character varying(255) NOT NULL,
    connector_type character varying(64) NOT NULL,
    endpoint_url text DEFAULT ''::text NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    status character varying(32) DEFAULT 'never_tested'::character varying NOT NULL,
    last_test_at timestamp with time zone,
    last_error text,
    notes text DEFAULT ''::text NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT siem_connectors_status_chk CHECK (((status)::text = ANY ((ARRAY['never_tested'::character varying, 'ok'::character varying, 'degraded'::character varying, 'error'::character varying, 'disabled'::character varying])::text[]))),
    CONSTRAINT siem_connectors_type_chk CHECK (((connector_type)::text = ANY ((ARRAY['splunk_hec'::character varying, 'azure_sentinel'::character varying, 'elastic_webhook'::character varying, 'generic_webhook'::character varying, 'syslog_tls'::character varying])::text[])))
);


ALTER TABLE public.siem_connectors OWNER TO edr;

--
-- Name: sigma_alert_correlations; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.sigma_alert_correlations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    alert_low_id text NOT NULL,
    alert_high_id text NOT NULL,
    relation_type character varying(32) NOT NULL,
    correlation_score double precision NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


ALTER TABLE public.sigma_alert_correlations OWNER TO edr;

--
-- Name: sigma_alerts; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.sigma_alerts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    "timestamp" timestamp with time zone NOT NULL,
    agent_id character varying(255) DEFAULT ''::character varying NOT NULL,
    rule_id character varying(255) NOT NULL,
    rule_title character varying(512),
    severity character varying(20) NOT NULL,
    category character varying(100),
    event_count integer DEFAULT 1,
    event_ids text[] DEFAULT ARRAY[]::text[],
    mitre_tactics text[] DEFAULT ARRAY[]::text[],
    mitre_techniques text[] DEFAULT ARRAY[]::text[],
    matched_fields jsonb DEFAULT '{}'::jsonb,
    matched_selections text[] DEFAULT ARRAY[]::text[],
    context_data jsonb DEFAULT '{}'::jsonb,
    status character varying(20) DEFAULT 'open'::character varying,
    assigned_to character varying(255),
    resolution_notes text,
    confidence numeric(3,2) DEFAULT 0.80,
    false_positive_risk numeric(3,2) DEFAULT 0.00,
    match_count integer DEFAULT 1,
    related_rules text[] DEFAULT ARRAY[]::text[],
    combined_confidence numeric(3,2),
    severity_promoted boolean DEFAULT false,
    original_severity character varying(20),
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    risk_score integer DEFAULT 0 NOT NULL,
    context_snapshot jsonb DEFAULT '{}'::jsonb NOT NULL,
    score_breakdown jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT sigma_alerts_severity_check CHECK (((severity)::text = ANY ((ARRAY['critical'::character varying, 'high'::character varying, 'medium'::character varying, 'low'::character varying, 'informational'::character varying])::text[]))),
    CONSTRAINT sigma_alerts_status_check CHECK (((status)::text = ANY ((ARRAY['open'::character varying, 'acknowledged'::character varying, 'investigating'::character varying, 'resolved'::character varying, 'false_positive'::character varying, 'suppressed'::character varying])::text[])))
);


ALTER TABLE public.sigma_alerts OWNER TO edr;

--
-- Name: TABLE sigma_alerts; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON TABLE public.sigma_alerts IS 'Security alerts generated by Sigma rule engine. 30-day retention policy.';


--
-- Name: COLUMN sigma_alerts.id; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.sigma_alerts.id IS 'Unique alert identifier (UUID)';


--
-- Name: COLUMN sigma_alerts."timestamp"; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.sigma_alerts."timestamp" IS 'When the alert was triggered';


--
-- Name: COLUMN sigma_alerts.agent_id; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.sigma_alerts.agent_id IS 'UUID of the EDR agent that generated the event';


--
-- Name: COLUMN sigma_alerts.rule_id; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.sigma_alerts.rule_id IS 'Sigma rule ID that matched';


--
-- Name: COLUMN sigma_alerts.severity; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.sigma_alerts.severity IS 'Alert severity: critical, high, medium, low, informational';


--
-- Name: COLUMN sigma_alerts.event_count; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.sigma_alerts.event_count IS 'Number of events aggregated into this alert';


--
-- Name: COLUMN sigma_alerts.matched_fields; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.sigma_alerts.matched_fields IS 'Fields that matched the Sigma rule (JSON)';


--
-- Name: COLUMN sigma_alerts.status; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.sigma_alerts.status IS 'Alert workflow status';


--
-- Name: COLUMN sigma_alerts.confidence; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.sigma_alerts.confidence IS 'Detection confidence score (0.00-1.00)';


--
-- Name: COLUMN sigma_alerts.risk_score; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.sigma_alerts.risk_score IS 'Context-aware risk score (0-100). Computed by RiskScorer: base(severity) + lineage_bonus + privilege_bonus + burst_bonus - fp_discount.';


--
-- Name: COLUMN sigma_alerts.context_snapshot; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.sigma_alerts.context_snapshot IS 'Full forensic evidence snapshot at scoring time. Contains reconstructed ancestor chain, privilege context, burst count, and component score breakdown. Stored as JSONB for flexible querying.';


--
-- Name: COLUMN sigma_alerts.score_breakdown; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.sigma_alerts.score_breakdown IS 'Scalar breakdown of the risk_score formula components: base_score, lineage_bonus, privilege_bonus, burst_bonus, fp_discount, raw_score, final_score.';


--
-- Name: sigma_rules; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.sigma_rules (
    id character varying(255) NOT NULL,
    title character varying(512) NOT NULL,
    description text,
    author character varying(255),
    content text NOT NULL,
    enabled boolean DEFAULT true,
    status character varying(20) DEFAULT 'stable'::character varying,
    product character varying(100) DEFAULT 'windows'::character varying,
    category character varying(100),
    service character varying(100),
    severity character varying(20),
    mitre_tactics text[] DEFAULT ARRAY[]::text[],
    mitre_techniques text[] DEFAULT ARRAY[]::text[],
    tags text[] DEFAULT ARRAY[]::text[],
    "references" text[] DEFAULT ARRAY[]::text[],
    version integer DEFAULT 1,
    date_created date,
    date_modified date,
    source character varying(100) DEFAULT 'official'::character varying,
    source_url text,
    custom_metadata jsonb DEFAULT '{}'::jsonb,
    false_positives text[] DEFAULT ARRAY[]::text[],
    avg_match_time_ms numeric(10,3),
    total_matches bigint DEFAULT 0,
    last_matched_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT sigma_rules_severity_check CHECK (((severity)::text = ANY ((ARRAY['critical'::character varying, 'high'::character varying, 'medium'::character varying, 'low'::character varying, 'informational'::character varying])::text[]))),
    CONSTRAINT sigma_rules_source_check CHECK (((source)::text = ANY ((ARRAY['official'::character varying, 'custom'::character varying, 'community'::character varying, 'imported'::character varying])::text[]))),
    CONSTRAINT sigma_rules_status_check CHECK (((status)::text = ANY ((ARRAY['stable'::character varying, 'test'::character varying, 'experimental'::character varying, 'deprecated'::character varying])::text[])))
);


ALTER TABLE public.sigma_rules OWNER TO edr;

--
-- Name: TABLE sigma_rules; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON TABLE public.sigma_rules IS 'Sigma detection rules stored for dynamic rule management';


--
-- Name: COLUMN sigma_rules.id; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.sigma_rules.id IS 'Unique rule identifier (from Sigma YAML id field)';


--
-- Name: COLUMN sigma_rules.title; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.sigma_rules.title IS 'Human-readable rule title';


--
-- Name: COLUMN sigma_rules.content; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.sigma_rules.content IS 'Full Sigma rule YAML content';


--
-- Name: COLUMN sigma_rules.enabled; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.sigma_rules.enabled IS 'Whether the rule is active for detection';


--
-- Name: COLUMN sigma_rules.product; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.sigma_rules.product IS 'Target product (windows, linux, etc.)';


--
-- Name: COLUMN sigma_rules.category; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.sigma_rules.category IS 'Event category (process_creation, network_connection, etc.)';


--
-- Name: COLUMN sigma_rules.source; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.sigma_rules.source IS 'Rule origin: official (SigmaHQ), custom, community, imported';


--
-- Name: signature_sync_epochs; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.signature_sync_epochs (
    id bigint NOT NULL,
    synced_at timestamp with time zone DEFAULT now() NOT NULL,
    hashes_inserted bigint DEFAULT 0 NOT NULL,
    generation bigint
);


ALTER TABLE public.signature_sync_epochs OWNER TO edr;

--
-- Name: signature_sync_epochs_id_seq; Type: SEQUENCE; Schema: public; Owner: edr
--

CREATE SEQUENCE public.signature_sync_epochs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.signature_sync_epochs_id_seq OWNER TO edr;

--
-- Name: signature_sync_epochs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: edr
--

ALTER SEQUENCE public.signature_sync_epochs_id_seq OWNED BY public.signature_sync_epochs.id;


--
-- Name: triage_snapshots; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.triage_snapshots (
    id bigint NOT NULL,
    agent_id uuid NOT NULL,
    run_id bigint,
    kind text NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.triage_snapshots OWNER TO edr;

--
-- Name: triage_snapshots_id_seq; Type: SEQUENCE; Schema: public; Owner: edr
--

CREATE SEQUENCE public.triage_snapshots_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.triage_snapshots_id_seq OWNER TO edr;

--
-- Name: triage_snapshots_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: edr
--

ALTER SEQUENCE public.triage_snapshots_id_seq OWNED BY public.triage_snapshots.id;


--
-- Name: users; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.users (
    id uuid NOT NULL,
    username character varying(255) NOT NULL,
    email character varying(255) NOT NULL,
    password_hash character varying(255) NOT NULL,
    full_name character varying(255),
    role character varying(50) DEFAULT 'viewer'::character varying NOT NULL,
    status character varying(50) DEFAULT 'active'::character varying NOT NULL,
    last_login timestamp with time zone,
    login_attempts integer DEFAULT 0,
    locked_until timestamp with time zone,
    mfa_enabled boolean DEFAULT false,
    mfa_secret character varying(255),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.users OWNER TO edr;

--
-- Name: TABLE users; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON TABLE public.users IS 'Dashboard user accounts with RBAC';


--
-- Name: COLUMN users.role; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.users.role IS 'User role: admin, security, analyst, operations, viewer';


--
-- Name: COLUMN users.status; Type: COMMENT; Schema: public; Owner: edr
--

COMMENT ON COLUMN public.users.status IS 'Account status: active, inactive, locked';


--
-- Name: vulnerability_findings; Type: TABLE; Schema: public; Owner: edr
--

CREATE TABLE public.vulnerability_findings (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    agent_id uuid NOT NULL,
    cve character varying(64) DEFAULT ''::character varying NOT NULL,
    title text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    severity character varying(32) DEFAULT 'medium'::character varying NOT NULL,
    cvss double precision,
    status character varying(32) DEFAULT 'open'::character varying NOT NULL,
    source character varying(128) DEFAULT 'import'::character varying NOT NULL,
    package_name text DEFAULT ''::text NOT NULL,
    fixed_version text DEFAULT ''::text NOT NULL,
    detected_at timestamp with time zone DEFAULT now() NOT NULL,
    published_at timestamp with time zone,
    due_at timestamp with time zone,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    kev_listed boolean DEFAULT false NOT NULL,
    kev_added_date timestamp with time zone,
    kev_due_date timestamp with time zone,
    exploit_available boolean DEFAULT false NOT NULL,
    priority_score double precision DEFAULT 0 NOT NULL,
    installed_version text DEFAULT ''::text NOT NULL,
    reference_url text DEFAULT ''::text NOT NULL,
    CONSTRAINT vulnerability_findings_severity_chk CHECK (((severity)::text = ANY ((ARRAY['critical'::character varying, 'high'::character varying, 'medium'::character varying, 'low'::character varying, 'informational'::character varying])::text[]))),
    CONSTRAINT vulnerability_findings_status_chk CHECK (((status)::text = ANY ((ARRAY['open'::character varying, 'acknowledged'::character varying, 'resolved'::character varying, 'risk_accepted'::character varying])::text[])))
);


ALTER TABLE public.vulnerability_findings OWNER TO edr;

--
-- Name: context_policies id; Type: DEFAULT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.context_policies ALTER COLUMN id SET DEFAULT nextval('public.context_policies_id_seq'::regclass);


--
-- Name: event_batches_fallback id; Type: DEFAULT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.event_batches_fallback ALTER COLUMN id SET DEFAULT nextval('public.event_batches_fallback_id_seq'::regclass);


--
-- Name: forensic_events id; Type: DEFAULT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.forensic_events ALTER COLUMN id SET DEFAULT nextval('public.forensic_events_id_seq'::regclass);


--
-- Name: ioc_enrichment id; Type: DEFAULT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.ioc_enrichment ALTER COLUMN id SET DEFAULT nextval('public.ioc_enrichment_id_seq'::regclass);


--
-- Name: malware_hashes id; Type: DEFAULT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.malware_hashes ALTER COLUMN id SET DEFAULT nextval('public.malware_hashes_id_seq'::regclass);


--
-- Name: playbook_runs id; Type: DEFAULT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.playbook_runs ALTER COLUMN id SET DEFAULT nextval('public.playbook_runs_id_seq'::regclass);


--
-- Name: playbook_steps id; Type: DEFAULT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.playbook_steps ALTER COLUMN id SET DEFAULT nextval('public.playbook_steps_id_seq'::regclass);


--
-- Name: signature_sync_epochs id; Type: DEFAULT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.signature_sync_epochs ALTER COLUMN id SET DEFAULT nextval('public.signature_sync_epochs_id_seq'::regclass);


--
-- Name: triage_snapshots id; Type: DEFAULT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.triage_snapshots ALTER COLUMN id SET DEFAULT nextval('public.triage_snapshots_id_seq'::regclass);


--
-- Name: agent_packages agent_packages_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.agent_packages
    ADD CONSTRAINT agent_packages_pkey PRIMARY KEY (id);


--
-- Name: agent_patch_profiles agent_patch_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.agent_patch_profiles
    ADD CONSTRAINT agent_patch_profiles_pkey PRIMARY KEY (agent_id);


--
-- Name: agent_quarantine_items agent_quarantine_items_agent_qpath_unique; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.agent_quarantine_items
    ADD CONSTRAINT agent_quarantine_items_agent_qpath_unique UNIQUE (agent_id, quarantine_path);


--
-- Name: agent_quarantine_items agent_quarantine_items_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.agent_quarantine_items
    ADD CONSTRAINT agent_quarantine_items_pkey PRIMARY KEY (id);


--
-- Name: agents agents_hostname_key; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_hostname_key UNIQUE (hostname);


--
-- Name: agents agents_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_pkey PRIMARY KEY (id);


--
-- Name: alert_stats alert_stats_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.alert_stats
    ADD CONSTRAINT alert_stats_pkey PRIMARY KEY (date);


--
-- Name: alerts alerts_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.alerts
    ADD CONSTRAINT alerts_pkey PRIMARY KEY (id);


--
-- Name: audit_logs audit_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.audit_logs
    ADD CONSTRAINT audit_logs_pkey PRIMARY KEY (id);


--
-- Name: automation_metrics automation_metrics_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.automation_metrics
    ADD CONSTRAINT automation_metrics_pkey PRIMARY KEY (id);


--
-- Name: automation_rules automation_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.automation_rules
    ADD CONSTRAINT automation_rules_pkey PRIMARY KEY (id);


--
-- Name: certificates certificates_cert_fingerprint_key; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.certificates
    ADD CONSTRAINT certificates_cert_fingerprint_key UNIQUE (cert_fingerprint);


--
-- Name: certificates certificates_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.certificates
    ADD CONSTRAINT certificates_pkey PRIMARY KEY (id);


--
-- Name: command_queue command_queue_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.command_queue
    ADD CONSTRAINT command_queue_pkey PRIMARY KEY (id);


--
-- Name: commands commands_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.commands
    ADD CONSTRAINT commands_pkey PRIMARY KEY (id);


--
-- Name: context_policies context_policies_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.context_policies
    ADD CONSTRAINT context_policies_pkey PRIMARY KEY (id);


--
-- Name: csrs csrs_agent_id_key; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.csrs
    ADD CONSTRAINT csrs_agent_id_key UNIQUE (agent_id);


--
-- Name: csrs csrs_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.csrs
    ADD CONSTRAINT csrs_pkey PRIMARY KEY (id);


--
-- Name: enrollment_token_consumptions enrollment_token_consumptions_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.enrollment_token_consumptions
    ADD CONSTRAINT enrollment_token_consumptions_pkey PRIMARY KEY (id);


--
-- Name: enrollment_tokens enrollment_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.enrollment_tokens
    ADD CONSTRAINT enrollment_tokens_pkey PRIMARY KEY (id);


--
-- Name: enrollment_tokens enrollment_tokens_token_key; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.enrollment_tokens
    ADD CONSTRAINT enrollment_tokens_token_key UNIQUE (token);


--
-- Name: event_batches_fallback event_batches_fallback_batch_id_key; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.event_batches_fallback
    ADD CONSTRAINT event_batches_fallback_batch_id_key UNIQUE (batch_id);


--
-- Name: event_batches_fallback event_batches_fallback_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.event_batches_fallback
    ADD CONSTRAINT event_batches_fallback_pkey PRIMARY KEY (id);


--
-- Name: events events_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.events
    ADD CONSTRAINT events_pkey PRIMARY KEY (id);


--
-- Name: forensic_collections forensic_collections_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.forensic_collections
    ADD CONSTRAINT forensic_collections_pkey PRIMARY KEY (command_id);


--
-- Name: forensic_events forensic_events_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.forensic_events
    ADD CONSTRAINT forensic_events_pkey PRIMARY KEY (id);


--
-- Name: installation_tokens installation_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.installation_tokens
    ADD CONSTRAINT installation_tokens_pkey PRIMARY KEY (id);


--
-- Name: installation_tokens installation_tokens_token_value_key; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.installation_tokens
    ADD CONSTRAINT installation_tokens_token_value_key UNIQUE (token_value);


--
-- Name: ioc_enrichment ioc_enrichment_ioc_type_ioc_value_provider_key; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.ioc_enrichment
    ADD CONSTRAINT ioc_enrichment_ioc_type_ioc_value_provider_key UNIQUE (ioc_type, ioc_value, provider);


--
-- Name: ioc_enrichment ioc_enrichment_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.ioc_enrichment
    ADD CONSTRAINT ioc_enrichment_pkey PRIMARY KEY (id);


--
-- Name: kev_catalog kev_catalog_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.kev_catalog
    ADD CONSTRAINT kev_catalog_pkey PRIMARY KEY (cve);


--
-- Name: malware_hashes malware_hashes_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.malware_hashes
    ADD CONSTRAINT malware_hashes_pkey PRIMARY KEY (id);


--
-- Name: malware_hashes malware_hashes_sha256_key; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.malware_hashes
    ADD CONSTRAINT malware_hashes_sha256_key UNIQUE (sha256);


--
-- Name: permissions permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_pkey PRIMARY KEY (id);


--
-- Name: permissions permissions_resource_action_key; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_resource_action_key UNIQUE (resource, action);


--
-- Name: playbook_executions playbook_executions_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.playbook_executions
    ADD CONSTRAINT playbook_executions_pkey PRIMARY KEY (id);


--
-- Name: playbook_runs playbook_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.playbook_runs
    ADD CONSTRAINT playbook_runs_pkey PRIMARY KEY (id);


--
-- Name: playbook_steps playbook_steps_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.playbook_steps
    ADD CONSTRAINT playbook_steps_pkey PRIMARY KEY (id);


--
-- Name: playbook_suggestions playbook_suggestions_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.playbook_suggestions
    ADD CONSTRAINT playbook_suggestions_pkey PRIMARY KEY (id);


--
-- Name: policies policies_name_key; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.policies
    ADD CONSTRAINT policies_name_key UNIQUE (name);


--
-- Name: policies policies_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.policies
    ADD CONSTRAINT policies_pkey PRIMARY KEY (id);


--
-- Name: policy_agent_assignments policy_agent_assignments_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.policy_agent_assignments
    ADD CONSTRAINT policy_agent_assignments_pkey PRIMARY KEY (policy_id, agent_id);


--
-- Name: policy_versions policy_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.policy_versions
    ADD CONSTRAINT policy_versions_pkey PRIMARY KEY (id);


--
-- Name: process_baselines process_baselines_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.process_baselines
    ADD CONSTRAINT process_baselines_pkey PRIMARY KEY (id);


--
-- Name: response_playbooks response_playbooks_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.response_playbooks
    ADD CONSTRAINT response_playbooks_pkey PRIMARY KEY (id);


--
-- Name: role_permissions role_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_pkey PRIMARY KEY (role_id, permission_id);


--
-- Name: roles roles_name_key; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_name_key UNIQUE (name);


--
-- Name: roles roles_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_pkey PRIMARY KEY (id);


--
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);


--
-- Name: siem_connectors siem_connectors_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.siem_connectors
    ADD CONSTRAINT siem_connectors_pkey PRIMARY KEY (id);


--
-- Name: sigma_alert_correlations sigma_alert_correlations_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.sigma_alert_correlations
    ADD CONSTRAINT sigma_alert_correlations_pkey PRIMARY KEY (id);


--
-- Name: sigma_alerts sigma_alerts_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.sigma_alerts
    ADD CONSTRAINT sigma_alerts_pkey PRIMARY KEY (id);


--
-- Name: sigma_rules sigma_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.sigma_rules
    ADD CONSTRAINT sigma_rules_pkey PRIMARY KEY (id);


--
-- Name: signature_sync_epochs signature_sync_epochs_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.signature_sync_epochs
    ADD CONSTRAINT signature_sync_epochs_pkey PRIMARY KEY (id);


--
-- Name: triage_snapshots triage_snapshots_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.triage_snapshots
    ADD CONSTRAINT triage_snapshots_pkey PRIMARY KEY (id);


--
-- Name: command_queue unique_command_queue; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.command_queue
    ADD CONSTRAINT unique_command_queue UNIQUE (command_id);


--
-- Name: policy_versions unique_policy_version; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.policy_versions
    ADD CONSTRAINT unique_policy_version UNIQUE (policy_id, version);


--
-- Name: sigma_alert_correlations uq_sigma_alert_correlation_pair; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.sigma_alert_correlations
    ADD CONSTRAINT uq_sigma_alert_correlation_pair UNIQUE (alert_low_id, alert_high_id);


--
-- Name: users users_email_key; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_email_key UNIQUE (email);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: users users_username_key; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_username_key UNIQUE (username);


--
-- Name: vulnerability_findings vulnerability_findings_pkey; Type: CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.vulnerability_findings
    ADD CONSTRAINT vulnerability_findings_pkey PRIMARY KEY (id);


--
-- Name: idx_agent_packages_agent_id; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_agent_packages_agent_id ON public.agent_packages USING btree (agent_id);


--
-- Name: idx_agent_packages_expires_at; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_agent_packages_expires_at ON public.agent_packages USING btree (expires_at);


--
-- Name: idx_agent_quarantine_agent_state; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_agent_quarantine_agent_state ON public.agent_quarantine_items USING btree (agent_id, state);


--
-- Name: idx_agents_cert_expires; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_agents_cert_expires ON public.agents USING btree (cert_expires_at);


--
-- Name: idx_agents_criticality; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_agents_criticality ON public.agents USING btree (criticality);


--
-- Name: idx_agents_events_dropped; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_agents_events_dropped ON public.agents USING btree (events_dropped) WHERE (events_dropped > 0);


--
-- Name: idx_agents_hardware_id_unique; Type: INDEX; Schema: public; Owner: edr
--

CREATE UNIQUE INDEX idx_agents_hardware_id_unique ON public.agents USING btree (hardware_id) WHERE ((hardware_id IS NOT NULL) AND ((hardware_id)::text <> ''::text));


--
-- Name: idx_agents_health_score; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_agents_health_score ON public.agents USING btree (health_score);


--
-- Name: idx_agents_hostname; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_agents_hostname ON public.agents USING btree (hostname);


--
-- Name: idx_agents_last_seen; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_agents_last_seen ON public.agents USING btree (last_seen);


--
-- Name: idx_agents_os_type; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_agents_os_type ON public.agents USING btree (os_type);


--
-- Name: idx_agents_status; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_agents_status ON public.agents USING btree (status);


--
-- Name: idx_agents_sysmon_installed; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_agents_sysmon_installed ON public.agents USING btree (sysmon_installed);


--
-- Name: idx_agents_sysmon_running; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_agents_sysmon_running ON public.agents USING btree (sysmon_running);


--
-- Name: idx_alerts_agent_id; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_alerts_agent_id ON public.alerts USING btree (agent_id);


--
-- Name: idx_alerts_agent_risk_score; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_alerts_agent_risk_score ON public.alerts USING btree (agent_id, risk_score DESC);


--
-- Name: idx_alerts_assigned_to; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_alerts_assigned_to ON public.alerts USING btree (assigned_to);


--
-- Name: idx_alerts_context_snapshot; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_alerts_context_snapshot ON public.alerts USING gin (context_snapshot);


--
-- Name: idx_alerts_detected_at; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_alerts_detected_at ON public.alerts USING btree (detected_at DESC);


--
-- Name: idx_alerts_open_by_severity; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_alerts_open_by_severity ON public.alerts USING btree (severity) WHERE ((status)::text = 'open'::text);


--
-- Name: idx_alerts_risk_score; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_alerts_risk_score ON public.alerts USING btree (risk_score DESC);


--
-- Name: idx_alerts_rule_id; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_alerts_rule_id ON public.alerts USING btree (rule_id);


--
-- Name: idx_alerts_search; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_alerts_search ON public.alerts USING gin (to_tsvector('english'::regconfig, (((title)::text || ' '::text) || COALESCE(description, ''::text))));


--
-- Name: idx_alerts_severity; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_alerts_severity ON public.alerts USING btree (severity);


--
-- Name: idx_alerts_status; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_alerts_status ON public.alerts USING btree (status);


--
-- Name: idx_alerts_status_severity; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_alerts_status_severity ON public.alerts USING btree (status, severity);


--
-- Name: idx_audit_logs_action; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_audit_logs_action ON public.audit_logs USING btree (action);


--
-- Name: idx_audit_logs_created_at; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_audit_logs_created_at ON public.audit_logs USING btree (created_at DESC);


--
-- Name: idx_audit_logs_resource_type; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_audit_logs_resource_type ON public.audit_logs USING btree (resource_type);


--
-- Name: idx_audit_logs_result; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_audit_logs_result ON public.audit_logs USING btree (result);


--
-- Name: idx_audit_logs_user_id; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_audit_logs_user_id ON public.audit_logs USING btree (user_id);


--
-- Name: idx_automation_metrics_date; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_automation_metrics_date ON public.automation_metrics USING btree (date);


--
-- Name: idx_automation_metrics_rule_id; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_automation_metrics_rule_id ON public.automation_metrics USING btree (rule_id);


--
-- Name: idx_automation_rules_enabled_priority; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_automation_rules_enabled_priority ON public.automation_rules USING btree (enabled, priority);


--
-- Name: idx_certificates_agent_id; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_certificates_agent_id ON public.certificates USING btree (agent_id);


--
-- Name: idx_certificates_agent_status; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_certificates_agent_status ON public.certificates USING btree (agent_id, status);


--
-- Name: idx_certificates_expires_at; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_certificates_expires_at ON public.certificates USING btree (expires_at);


--
-- Name: idx_certificates_fingerprint; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_certificates_fingerprint ON public.certificates USING btree (cert_fingerprint);


--
-- Name: idx_certificates_status; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_certificates_status ON public.certificates USING btree (status);


--
-- Name: idx_command_queue_agent_priority; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_command_queue_agent_priority ON public.command_queue USING btree (agent_id, priority DESC, scheduled_at);


--
-- Name: idx_commands_agent_id; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_commands_agent_id ON public.commands USING btree (agent_id);


--
-- Name: idx_commands_command_type; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_commands_command_type ON public.commands USING btree (command_type);


--
-- Name: idx_commands_issued_at; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_commands_issued_at ON public.commands USING btree (issued_at DESC);


--
-- Name: idx_commands_issued_by; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_commands_issued_by ON public.commands USING btree (issued_by);


--
-- Name: idx_commands_pending; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_commands_pending ON public.commands USING btree (agent_id, priority DESC) WHERE ((status)::text = 'pending'::text);


--
-- Name: idx_commands_status; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_commands_status ON public.commands USING btree (status);


--
-- Name: idx_context_policies_enabled_scope; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_context_policies_enabled_scope ON public.context_policies USING btree (enabled, scope_type);


--
-- Name: idx_csrs_agent_id; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_csrs_agent_id ON public.csrs USING btree (agent_id);


--
-- Name: idx_csrs_approved; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_csrs_approved ON public.csrs USING btree (approved);


--
-- Name: idx_csrs_expires; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_csrs_expires ON public.csrs USING btree (expires_at);


--
-- Name: idx_enrollment_token_consumptions_token_hw_unique; Type: INDEX; Schema: public; Owner: edr
--

CREATE UNIQUE INDEX idx_enrollment_token_consumptions_token_hw_unique ON public.enrollment_token_consumptions USING btree (token_id, hardware_id);


--
-- Name: idx_enrollment_token_consumptions_token_id; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_enrollment_token_consumptions_token_id ON public.enrollment_token_consumptions USING btree (token_id);


--
-- Name: idx_enrollment_tokens_active; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_enrollment_tokens_active ON public.enrollment_tokens USING btree (is_active);


--
-- Name: idx_enrollment_tokens_expires; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_enrollment_tokens_expires ON public.enrollment_tokens USING btree (expires_at);


--
-- Name: idx_enrollment_tokens_token; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_enrollment_tokens_token ON public.enrollment_tokens USING btree (token);


--
-- Name: idx_events_agent_ts; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_events_agent_ts ON public.events USING btree (agent_id, ts DESC);


--
-- Name: idx_events_raw_gin; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_events_raw_gin ON public.events USING gin (raw);


--
-- Name: idx_events_severity_ts; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_events_severity_ts ON public.events USING btree (severity, ts DESC);


--
-- Name: idx_events_ts; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_events_ts ON public.events USING btree (ts DESC);


--
-- Name: idx_events_type_ts; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_events_type_ts ON public.events USING btree (event_type, ts DESC);


--
-- Name: idx_fallback_unreplayed; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_fallback_unreplayed ON public.event_batches_fallback USING btree (replayed) WHERE (NOT replayed);


--
-- Name: idx_forensic_collections_agent_time; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_forensic_collections_agent_time ON public.forensic_collections USING btree (agent_id, issued_at DESC);


--
-- Name: idx_forensic_events_command_type_id; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_forensic_events_command_type_id ON public.forensic_events USING btree (command_id, log_type, id);


--
-- Name: idx_installation_tokens_expires; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_installation_tokens_expires ON public.installation_tokens USING btree (expires_at);


--
-- Name: idx_installation_tokens_token; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_installation_tokens_token ON public.installation_tokens USING btree (token_value);


--
-- Name: idx_installation_tokens_used; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_installation_tokens_used ON public.installation_tokens USING btree (used);


--
-- Name: idx_ioc_enrichment_agent; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_ioc_enrichment_agent ON public.ioc_enrichment USING btree (agent_id, fetched_at DESC);


--
-- Name: idx_ioc_enrichment_value; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_ioc_enrichment_value ON public.ioc_enrichment USING btree (ioc_type, ioc_value);


--
-- Name: idx_kev_catalog_synced_at; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_kev_catalog_synced_at ON public.kev_catalog USING btree (synced_at DESC);


--
-- Name: idx_mh_source; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_mh_source ON public.malware_hashes USING btree (source);


--
-- Name: idx_mh_version; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_mh_version ON public.malware_hashes USING btree (version);


--
-- Name: idx_playbook_executions_agent_id; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_playbook_executions_agent_id ON public.playbook_executions USING btree (agent_id);


--
-- Name: idx_playbook_executions_alert_id; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_playbook_executions_alert_id ON public.playbook_executions USING btree (alert_id);


--
-- Name: idx_playbook_executions_playbook_id; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_playbook_executions_playbook_id ON public.playbook_executions USING btree (playbook_id);


--
-- Name: idx_playbook_executions_status; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_playbook_executions_status ON public.playbook_executions USING btree (status);


--
-- Name: idx_playbook_runs_agent; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_playbook_runs_agent ON public.playbook_runs USING btree (agent_id, started_at DESC);


--
-- Name: idx_playbook_runs_status; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_playbook_runs_status ON public.playbook_runs USING btree (status) WHERE (status = 'running'::text);


--
-- Name: idx_playbook_steps_command; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_playbook_steps_command ON public.playbook_steps USING btree (command_id) WHERE (command_id IS NOT NULL);


--
-- Name: idx_playbook_steps_run; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_playbook_steps_run ON public.playbook_steps USING btree (run_id);


--
-- Name: idx_playbook_suggestions_alert_id; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_playbook_suggestions_alert_id ON public.playbook_suggestions USING btree (alert_id);


--
-- Name: idx_policies_created_by; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_policies_created_by ON public.policies USING btree (created_by);


--
-- Name: idx_policies_enabled; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_policies_enabled ON public.policies USING btree (enabled);


--
-- Name: idx_policies_name; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_policies_name ON public.policies USING btree (name);


--
-- Name: idx_policies_priority; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_policies_priority ON public.policies USING btree (priority DESC);


--
-- Name: idx_policy_agent_assignments_agent; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_policy_agent_assignments_agent ON public.policy_agent_assignments USING btree (agent_id);


--
-- Name: idx_policy_versions_policy_id; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_policy_versions_policy_id ON public.policy_versions USING btree (policy_id);


--
-- Name: idx_policy_versions_version; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_policy_versions_version ON public.policy_versions USING btree (policy_id, version DESC);


--
-- Name: idx_process_baselines_agent; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_process_baselines_agent ON public.process_baselines USING btree (agent_id);


--
-- Name: idx_process_baselines_common_parents; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_process_baselines_common_parents ON public.process_baselines USING gin (common_parents);


--
-- Name: idx_process_baselines_lookup; Type: INDEX; Schema: public; Owner: edr
--

CREATE UNIQUE INDEX idx_process_baselines_lookup ON public.process_baselines USING btree (agent_id, process_name, hour_of_day);


--
-- Name: idx_process_baselines_process_name; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_process_baselines_process_name ON public.process_baselines USING btree (process_name);


--
-- Name: idx_response_playbooks_category; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_response_playbooks_category ON public.response_playbooks USING btree (category);


--
-- Name: idx_response_playbooks_enabled; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_response_playbooks_enabled ON public.response_playbooks USING btree (enabled);


--
-- Name: idx_role_permissions_perm; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_role_permissions_perm ON public.role_permissions USING btree (permission_id);


--
-- Name: idx_role_permissions_role; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_role_permissions_role ON public.role_permissions USING btree (role_id);


--
-- Name: idx_siem_connectors_enabled; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_siem_connectors_enabled ON public.siem_connectors USING btree (enabled);


--
-- Name: idx_siem_connectors_type; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_siem_connectors_type ON public.siem_connectors USING btree (connector_type);


--
-- Name: idx_sigma_alert_correlations_created; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alert_correlations_created ON public.sigma_alert_correlations USING btree (created_at DESC);


--
-- Name: idx_sigma_alert_correlations_high; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alert_correlations_high ON public.sigma_alert_correlations USING btree (alert_high_id);


--
-- Name: idx_sigma_alert_correlations_low; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alert_correlations_low ON public.sigma_alert_correlations USING btree (alert_low_id);


--
-- Name: idx_sigma_alerts_agent_id; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_agent_id ON public.sigma_alerts USING btree (agent_id);


--
-- Name: idx_sigma_alerts_agent_risk_score; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_agent_risk_score ON public.sigma_alerts USING btree (agent_id, risk_score DESC);


--
-- Name: idx_sigma_alerts_agent_timestamp; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_agent_timestamp ON public.sigma_alerts USING btree (agent_id, "timestamp" DESC);


--
-- Name: idx_sigma_alerts_context_snapshot; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_context_snapshot ON public.sigma_alerts USING gin (context_snapshot);


--
-- Name: idx_sigma_alerts_dedup; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_dedup ON public.sigma_alerts USING btree (agent_id, rule_id, "timestamp" DESC);


--
-- Name: idx_sigma_alerts_dedup_lookup; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_dedup_lookup ON public.sigma_alerts USING btree (agent_id, rule_id, "timestamp" DESC) WHERE ((status)::text <> ALL (ARRAY[('resolved'::character varying)::text, ('false_positive'::character varying)::text]));


--
-- Name: idx_sigma_alerts_mitre_tactics; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_mitre_tactics ON public.sigma_alerts USING gin (mitre_tactics);


--
-- Name: idx_sigma_alerts_mitre_techniques; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_mitre_techniques ON public.sigma_alerts USING gin (mitre_techniques);


--
-- Name: idx_sigma_alerts_open_risk; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_open_risk ON public.sigma_alerts USING btree (risk_score DESC) WHERE ((status)::text = ANY ((ARRAY['open'::character varying, 'investigating'::character varying])::text[]));


--
-- Name: idx_sigma_alerts_open_ts; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_open_ts ON public.sigma_alerts USING btree ("timestamp" DESC) WHERE ((status)::text = ANY ((ARRAY['open'::character varying, 'investigating'::character varying])::text[]));


--
-- Name: idx_sigma_alerts_risk_score; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_risk_score ON public.sigma_alerts USING btree (risk_score DESC);


--
-- Name: idx_sigma_alerts_risk_status; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_risk_status ON public.sigma_alerts USING btree (risk_score DESC, status);


--
-- Name: idx_sigma_alerts_rule_id; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_rule_id ON public.sigma_alerts USING btree (rule_id);


--
-- Name: idx_sigma_alerts_rule_timestamp; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_rule_timestamp ON public.sigma_alerts USING btree (rule_id, "timestamp" DESC);


--
-- Name: idx_sigma_alerts_rule_title_search; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_rule_title_search ON public.sigma_alerts USING gin (to_tsvector('english'::regconfig, (rule_title)::text));


--
-- Name: idx_sigma_alerts_score_breakdown; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_score_breakdown ON public.sigma_alerts USING gin (score_breakdown);


--
-- Name: idx_sigma_alerts_severity; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_severity ON public.sigma_alerts USING btree (severity);


--
-- Name: idx_sigma_alerts_severity_timestamp; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_severity_timestamp ON public.sigma_alerts USING btree (severity, "timestamp" DESC);


--
-- Name: idx_sigma_alerts_status; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_status ON public.sigma_alerts USING btree (status);


--
-- Name: idx_sigma_alerts_status_timestamp; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_status_timestamp ON public.sigma_alerts USING btree (status, "timestamp" DESC);


--
-- Name: idx_sigma_alerts_timestamp; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_alerts_timestamp ON public.sigma_alerts USING btree ("timestamp" DESC);


--
-- Name: idx_sigma_rules_category; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_rules_category ON public.sigma_rules USING btree (category);


--
-- Name: idx_sigma_rules_description_search; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_rules_description_search ON public.sigma_rules USING gin (to_tsvector('english'::regconfig, COALESCE(description, ''::text)));


--
-- Name: idx_sigma_rules_enabled; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_rules_enabled ON public.sigma_rules USING btree (enabled);


--
-- Name: idx_sigma_rules_enabled_product; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_rules_enabled_product ON public.sigma_rules USING btree (enabled, product);


--
-- Name: idx_sigma_rules_mitre_tactics; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_rules_mitre_tactics ON public.sigma_rules USING gin (mitre_tactics);


--
-- Name: idx_sigma_rules_mitre_techniques; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_rules_mitre_techniques ON public.sigma_rules USING gin (mitre_techniques);


--
-- Name: idx_sigma_rules_product; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_rules_product ON public.sigma_rules USING btree (product);


--
-- Name: idx_sigma_rules_product_category; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_rules_product_category ON public.sigma_rules USING btree (product, category);


--
-- Name: idx_sigma_rules_severity; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_rules_severity ON public.sigma_rules USING btree (severity);


--
-- Name: idx_sigma_rules_source; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_rules_source ON public.sigma_rules USING btree (source);


--
-- Name: idx_sigma_rules_status; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_rules_status ON public.sigma_rules USING btree (status);


--
-- Name: idx_sigma_rules_tags; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_rules_tags ON public.sigma_rules USING gin (tags);


--
-- Name: idx_sigma_rules_title_search; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_sigma_rules_title_search ON public.sigma_rules USING gin (to_tsvector('english'::regconfig, (title)::text));


--
-- Name: idx_triage_snapshots_agent_kind; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_triage_snapshots_agent_kind ON public.triage_snapshots USING btree (agent_id, kind, created_at DESC);


--
-- Name: idx_triage_snapshots_run; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_triage_snapshots_run ON public.triage_snapshots USING btree (run_id) WHERE (run_id IS NOT NULL);


--
-- Name: idx_users_email; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_users_email ON public.users USING btree (email);


--
-- Name: idx_users_role; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_users_role ON public.users USING btree (role);


--
-- Name: idx_users_status; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_users_status ON public.users USING btree (status);


--
-- Name: idx_users_username; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_users_username ON public.users USING btree (username);


--
-- Name: idx_vuln_findings_kev; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_vuln_findings_kev ON public.vulnerability_findings USING btree (kev_listed) WHERE (kev_listed = true);


--
-- Name: idx_vuln_findings_priority; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_vuln_findings_priority ON public.vulnerability_findings USING btree (priority_score DESC);


--
-- Name: idx_vulnerability_findings_agent_id; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_vulnerability_findings_agent_id ON public.vulnerability_findings USING btree (agent_id);


--
-- Name: idx_vulnerability_findings_cve; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_vulnerability_findings_cve ON public.vulnerability_findings USING btree (cve);


--
-- Name: idx_vulnerability_findings_detected_at; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_vulnerability_findings_detected_at ON public.vulnerability_findings USING btree (detected_at DESC);


--
-- Name: idx_vulnerability_findings_severity; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_vulnerability_findings_severity ON public.vulnerability_findings USING btree (severity);


--
-- Name: idx_vulnerability_findings_status; Type: INDEX; Schema: public; Owner: edr
--

CREATE INDEX idx_vulnerability_findings_status ON public.vulnerability_findings USING btree (status);


--
-- Name: uq_vuln_findings_agent_cve_pkg; Type: INDEX; Schema: public; Owner: edr
--

CREATE UNIQUE INDEX uq_vuln_findings_agent_cve_pkg ON public.vulnerability_findings USING btree (agent_id, cve, package_name) WHERE ((cve)::text <> ''::text);


--
-- Name: ux_context_policies_scope; Type: INDEX; Schema: public; Owner: edr
--

CREATE UNIQUE INDEX ux_context_policies_scope ON public.context_policies USING btree (scope_type, scope_value);


--
-- Name: agents agents_criticality_changed; Type: TRIGGER; Schema: public; Owner: edr
--

CREATE TRIGGER agents_criticality_changed AFTER UPDATE OF criticality ON public.agents FOR EACH ROW EXECUTE FUNCTION public.trg_agent_criticality_changed();


--
-- Name: enrollment_tokens trg_enrollment_tokens_updated_at; Type: TRIGGER; Schema: public; Owner: edr
--

CREATE TRIGGER trg_enrollment_tokens_updated_at BEFORE UPDATE ON public.enrollment_tokens FOR EACH ROW EXECUTE FUNCTION public.update_enrollment_tokens_updated_at();


--
-- Name: alerts trigger_alerts_updated_at; Type: TRIGGER; Schema: public; Owner: edr
--

CREATE TRIGGER trigger_alerts_updated_at BEFORE UPDATE ON public.alerts FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: commands trigger_commands_queue; Type: TRIGGER; Schema: public; Owner: edr
--

CREATE TRIGGER trigger_commands_queue AFTER INSERT ON public.commands FOR EACH ROW WHEN (((new.status)::text = 'pending'::text)) EXECUTE FUNCTION public.add_to_command_queue();


--
-- Name: commands trigger_commands_set_expires; Type: TRIGGER; Schema: public; Owner: edr
--

CREATE TRIGGER trigger_commands_set_expires BEFORE INSERT ON public.commands FOR EACH ROW EXECUTE FUNCTION public.set_command_expires();


--
-- Name: policies trigger_policies_updated_at; Type: TRIGGER; Schema: public; Owner: edr
--

CREATE TRIGGER trigger_policies_updated_at BEFORE UPDATE ON public.policies FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: policies trigger_policies_version; Type: TRIGGER; Schema: public; Owner: edr
--

CREATE TRIGGER trigger_policies_version BEFORE UPDATE ON public.policies FOR EACH ROW WHEN (((old.rules IS DISTINCT FROM new.rules) OR (old.targets IS DISTINCT FROM new.targets))) EXECUTE FUNCTION public.increment_policy_version();


--
-- Name: alerts trigger_process_alert_creation; Type: TRIGGER; Schema: public; Owner: edr
--

CREATE TRIGGER trigger_process_alert_creation AFTER INSERT ON public.alerts FOR EACH ROW EXECUTE FUNCTION public.process_alert_on_create();


--
-- Name: process_baselines trigger_process_baselines_updated_at; Type: TRIGGER; Schema: public; Owner: edr
--

CREATE TRIGGER trigger_process_baselines_updated_at BEFORE UPDATE ON public.process_baselines FOR EACH ROW EXECUTE FUNCTION public.update_process_baselines_updated_at();


--
-- Name: sigma_alerts trigger_sigma_alerts_updated_at; Type: TRIGGER; Schema: public; Owner: edr
--

CREATE TRIGGER trigger_sigma_alerts_updated_at BEFORE UPDATE ON public.sigma_alerts FOR EACH ROW EXECUTE FUNCTION public.update_sigma_alerts_updated_at();


--
-- Name: sigma_rules trigger_sigma_rules_updated_at; Type: TRIGGER; Schema: public; Owner: edr
--

CREATE TRIGGER trigger_sigma_rules_updated_at BEFORE UPDATE ON public.sigma_rules FOR EACH ROW EXECUTE FUNCTION public.update_sigma_rules_updated_at();


--
-- Name: agents update_agents_updated_at; Type: TRIGGER; Schema: public; Owner: edr
--

CREATE TRIGGER update_agents_updated_at BEFORE UPDATE ON public.agents FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: alert_stats update_alert_stats_updated_at; Type: TRIGGER; Schema: public; Owner: edr
--

CREATE TRIGGER update_alert_stats_updated_at BEFORE UPDATE ON public.alert_stats FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: automation_rules update_automation_rules_updated_at; Type: TRIGGER; Schema: public; Owner: edr
--

CREATE TRIGGER update_automation_rules_updated_at BEFORE UPDATE ON public.automation_rules FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: response_playbooks update_response_playbooks_updated_at; Type: TRIGGER; Schema: public; Owner: edr
--

CREATE TRIGGER update_response_playbooks_updated_at BEFORE UPDATE ON public.response_playbooks FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: roles update_roles_updated_at; Type: TRIGGER; Schema: public; Owner: edr
--

CREATE TRIGGER update_roles_updated_at BEFORE UPDATE ON public.roles FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: users update_users_updated_at; Type: TRIGGER; Schema: public; Owner: edr
--

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON public.users FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: agent_packages agent_packages_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.agent_packages
    ADD CONSTRAINT agent_packages_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- Name: agent_patch_profiles agent_patch_profiles_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.agent_patch_profiles
    ADD CONSTRAINT agent_patch_profiles_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- Name: agent_quarantine_items agent_quarantine_items_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.agent_quarantine_items
    ADD CONSTRAINT agent_quarantine_items_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- Name: alerts alerts_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.alerts
    ADD CONSTRAINT alerts_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- Name: alerts alerts_assigned_to_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.alerts
    ADD CONSTRAINT alerts_assigned_to_fkey FOREIGN KEY (assigned_to) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: audit_logs audit_logs_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.audit_logs
    ADD CONSTRAINT audit_logs_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: automation_metrics automation_metrics_rule_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.automation_metrics
    ADD CONSTRAINT automation_metrics_rule_id_fkey FOREIGN KEY (rule_id) REFERENCES public.automation_rules(id) ON DELETE CASCADE;


--
-- Name: automation_rules automation_rules_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.automation_rules
    ADD CONSTRAINT automation_rules_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: automation_rules automation_rules_playbook_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.automation_rules
    ADD CONSTRAINT automation_rules_playbook_id_fkey FOREIGN KEY (playbook_id) REFERENCES public.response_playbooks(id) ON DELETE CASCADE;


--
-- Name: command_queue command_queue_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.command_queue
    ADD CONSTRAINT command_queue_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- Name: command_queue command_queue_command_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.command_queue
    ADD CONSTRAINT command_queue_command_id_fkey FOREIGN KEY (command_id) REFERENCES public.commands(id) ON DELETE CASCADE;


--
-- Name: commands commands_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.commands
    ADD CONSTRAINT commands_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- Name: commands commands_issued_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.commands
    ADD CONSTRAINT commands_issued_by_fkey FOREIGN KEY (issued_by) REFERENCES public.users(id) ON DELETE RESTRICT;


--
-- Name: enrollment_token_consumptions enrollment_token_consumptions_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.enrollment_token_consumptions
    ADD CONSTRAINT enrollment_token_consumptions_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- Name: enrollment_token_consumptions enrollment_token_consumptions_token_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.enrollment_token_consumptions
    ADD CONSTRAINT enrollment_token_consumptions_token_id_fkey FOREIGN KEY (token_id) REFERENCES public.enrollment_tokens(id) ON DELETE CASCADE;


--
-- Name: events events_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.events
    ADD CONSTRAINT events_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- Name: certificates fk_certificates_agent; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.certificates
    ADD CONSTRAINT fk_certificates_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- Name: csrs fk_csrs_agent; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.csrs
    ADD CONSTRAINT fk_csrs_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- Name: csrs fk_csrs_approved_by; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.csrs
    ADD CONSTRAINT fk_csrs_approved_by FOREIGN KEY (approved_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: installation_tokens fk_installation_tokens_agent; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.installation_tokens
    ADD CONSTRAINT fk_installation_tokens_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;


--
-- Name: forensic_collections forensic_collections_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.forensic_collections
    ADD CONSTRAINT forensic_collections_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- Name: forensic_events forensic_events_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.forensic_events
    ADD CONSTRAINT forensic_events_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- Name: forensic_events forensic_events_command_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.forensic_events
    ADD CONSTRAINT forensic_events_command_id_fkey FOREIGN KEY (command_id) REFERENCES public.forensic_collections(command_id) ON DELETE CASCADE;


--
-- Name: ioc_enrichment ioc_enrichment_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.ioc_enrichment
    ADD CONSTRAINT ioc_enrichment_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;


--
-- Name: ioc_enrichment ioc_enrichment_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.ioc_enrichment
    ADD CONSTRAINT ioc_enrichment_run_id_fkey FOREIGN KEY (run_id) REFERENCES public.playbook_runs(id) ON DELETE SET NULL;


--
-- Name: playbook_executions playbook_executions_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.playbook_executions
    ADD CONSTRAINT playbook_executions_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- Name: playbook_executions playbook_executions_alert_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.playbook_executions
    ADD CONSTRAINT playbook_executions_alert_id_fkey FOREIGN KEY (alert_id) REFERENCES public.alerts(id) ON DELETE CASCADE;


--
-- Name: playbook_executions playbook_executions_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.playbook_executions
    ADD CONSTRAINT playbook_executions_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: playbook_executions playbook_executions_playbook_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.playbook_executions
    ADD CONSTRAINT playbook_executions_playbook_id_fkey FOREIGN KEY (playbook_id) REFERENCES public.response_playbooks(id) ON DELETE CASCADE;


--
-- Name: playbook_executions playbook_executions_rule_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.playbook_executions
    ADD CONSTRAINT playbook_executions_rule_id_fkey FOREIGN KEY (rule_id) REFERENCES public.automation_rules(id) ON DELETE SET NULL;


--
-- Name: playbook_runs playbook_runs_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.playbook_runs
    ADD CONSTRAINT playbook_runs_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- Name: playbook_steps playbook_steps_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.playbook_steps
    ADD CONSTRAINT playbook_steps_run_id_fkey FOREIGN KEY (run_id) REFERENCES public.playbook_runs(id) ON DELETE CASCADE;


--
-- Name: playbook_suggestions playbook_suggestions_alert_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.playbook_suggestions
    ADD CONSTRAINT playbook_suggestions_alert_id_fkey FOREIGN KEY (alert_id) REFERENCES public.alerts(id) ON DELETE CASCADE;


--
-- Name: playbook_suggestions playbook_suggestions_playbook_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.playbook_suggestions
    ADD CONSTRAINT playbook_suggestions_playbook_id_fkey FOREIGN KEY (playbook_id) REFERENCES public.response_playbooks(id) ON DELETE CASCADE;


--
-- Name: policies policies_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.policies
    ADD CONSTRAINT policies_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE RESTRICT;


--
-- Name: policies policies_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.policies
    ADD CONSTRAINT policies_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: policy_agent_assignments policy_agent_assignments_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.policy_agent_assignments
    ADD CONSTRAINT policy_agent_assignments_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- Name: policy_agent_assignments policy_agent_assignments_assigned_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.policy_agent_assignments
    ADD CONSTRAINT policy_agent_assignments_assigned_by_fkey FOREIGN KEY (assigned_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: policy_agent_assignments policy_agent_assignments_policy_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.policy_agent_assignments
    ADD CONSTRAINT policy_agent_assignments_policy_id_fkey FOREIGN KEY (policy_id) REFERENCES public.policies(id) ON DELETE CASCADE;


--
-- Name: policy_versions policy_versions_changed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.policy_versions
    ADD CONSTRAINT policy_versions_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: policy_versions policy_versions_policy_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.policy_versions
    ADD CONSTRAINT policy_versions_policy_id_fkey FOREIGN KEY (policy_id) REFERENCES public.policies(id) ON DELETE CASCADE;


--
-- Name: response_playbooks response_playbooks_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.response_playbooks
    ADD CONSTRAINT response_playbooks_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: role_permissions role_permissions_permission_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_permission_id_fkey FOREIGN KEY (permission_id) REFERENCES public.permissions(id) ON DELETE CASCADE;


--
-- Name: role_permissions role_permissions_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.roles(id) ON DELETE CASCADE;


--
-- Name: triage_snapshots triage_snapshots_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.triage_snapshots
    ADD CONSTRAINT triage_snapshots_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- Name: triage_snapshots triage_snapshots_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.triage_snapshots
    ADD CONSTRAINT triage_snapshots_run_id_fkey FOREIGN KEY (run_id) REFERENCES public.playbook_runs(id) ON DELETE SET NULL;


--
-- Name: vulnerability_findings vulnerability_findings_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: edr
--

ALTER TABLE ONLY public.vulnerability_findings
    ADD CONSTRAINT vulnerability_findings_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--

\unrestrict iyM2yrw0N97vod3R2UmZqeaabresZAGfpxFNTpkvTG4Ylv1ofY2qAImghwl6dL1

