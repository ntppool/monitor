--
-- PostgreSQL database dump
--

\restrict JrXmdpwfgrZ3hPsF1HhDavlypKeXBg7Tbu8oGE4HaBPH4PJsOgK0pVSUbzYpnXZ

-- Dumped from database version 18.1 (Postgres.app)
-- Dumped by pg_dump version 18.1 (Postgres.app)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: account_invites_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.account_invites_status AS ENUM (
    'pending',
    'accepted',
    'expired'
);


--
-- Name: account_subscriptions_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.account_subscriptions_status AS ENUM (
    'incomplete',
    'incomplete_expired',
    'trialing',
    'active',
    'past_due',
    'canceled',
    'unpaid',
    'ended'
);


--
-- Name: monitor_registrations_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.monitor_registrations_status AS ENUM (
    'pending',
    'accepted',
    'completed',
    'rejected',
    'cancelled'
);


--
-- Name: monitors_ip_version; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.monitors_ip_version AS ENUM (
    'v4',
    'v6'
);


--
-- Name: monitors_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.monitors_status AS ENUM (
    'pending',
    'testing',
    'active',
    'paused',
    'deleted'
);


--
-- Name: monitors_type; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.monitors_type AS ENUM (
    'monitor',
    'score'
);


--
-- Name: server_scores_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.server_scores_status AS ENUM (
    'candidate',
    'testing',
    'active',
    'paused'
);


--
-- Name: servers_ip_version; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.servers_ip_version AS ENUM (
    'v4',
    'v6'
);


--
-- Name: user_equipment_applications_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.user_equipment_applications_status AS ENUM (
    'New',
    'Pending',
    'Maybe',
    'No',
    'Approved'
);


--
-- Name: user_tasks_task; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.user_tasks_task AS ENUM (
    'download',
    'delete'
);


--
-- Name: vendor_zones_client_type; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.vendor_zones_client_type AS ENUM (
    'ntp',
    'sntp',
    'legacy'
);


--
-- Name: vendor_zones_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.vendor_zones_status AS ENUM (
    'New',
    'Pending',
    'Approved',
    'Rejected'
);


--
-- Name: zone_server_counts_ip_version; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.zone_server_counts_ip_version AS ENUM (
    'v4',
    'v6'
);


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: account_invites; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_invites (
    id bigint NOT NULL,
    account_id bigint NOT NULL,
    email character varying(255) NOT NULL,
    status public.account_invites_status,
    user_id bigint,
    sent_by_id bigint NOT NULL,
    code character varying(25) NOT NULL,
    expires_on timestamp with time zone NOT NULL,
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: account_invites_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.account_invites_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: account_invites_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.account_invites_id_seq OWNED BY public.account_invites.id;


--
-- Name: account_subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_subscriptions (
    id bigint NOT NULL,
    account_id bigint NOT NULL,
    stripe_subscription_id character varying(255),
    status public.account_subscriptions_status,
    name character varying(255) NOT NULL,
    max_zones bigint NOT NULL,
    max_devices bigint NOT NULL,
    created_on timestamp with time zone NOT NULL,
    ended_on timestamp with time zone,
    modified_on timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: account_subscriptions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.account_subscriptions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: account_subscriptions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.account_subscriptions_id_seq OWNED BY public.account_subscriptions.id;


--
-- Name: account_users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_users (
    account_id bigint NOT NULL,
    user_id bigint NOT NULL
);


--
-- Name: accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.accounts (
    id bigint NOT NULL,
    id_token character varying(36),
    name character varying(255),
    organization_name character varying(150),
    organization_url character varying(150),
    public_profile boolean DEFAULT false NOT NULL,
    url_slug character varying(150),
    flags json,
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    stripe_customer_id character varying(255)
);


--
-- Name: accounts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.accounts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: accounts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.accounts_id_seq OWNED BY public.accounts.id;


--
-- Name: api_keys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.api_keys (
    id bigint NOT NULL,
    account_id bigint,
    user_id bigint,
    api_key character varying(255),
    grants text,
    audience text NOT NULL,
    token_lookup character varying(16) NOT NULL,
    token_hashed character varying(256) NOT NULL,
    last_seen timestamp with time zone,
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: api_keys_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.api_keys_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: api_keys_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.api_keys_id_seq OWNED BY public.api_keys.id;


--
-- Name: api_keys_monitors; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.api_keys_monitors (
    api_key_id bigint NOT NULL,
    monitor_id bigint NOT NULL
);


--
-- Name: combust_cache; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.combust_cache (
    id character varying(64) NOT NULL,
    type character varying(20) DEFAULT ''::character varying NOT NULL,
    created timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    purge_key character varying(16),
    data bytea NOT NULL,
    metadata bytea,
    serialized boolean DEFAULT false NOT NULL,
    expire timestamp with time zone DEFAULT '1969-12-31 16:00:00-08'::timestamp with time zone NOT NULL
);


--
-- Name: combust_secrets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.combust_secrets (
    secret_ts bigint NOT NULL,
    expires_ts bigint NOT NULL,
    type character varying(32) NOT NULL,
    secret character(32)
);


--
-- Name: dns_roots; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.dns_roots (
    id bigint NOT NULL,
    origin character varying(255) NOT NULL,
    vendor_available smallint DEFAULT '0'::smallint NOT NULL,
    general_use smallint DEFAULT '0'::smallint NOT NULL,
    ns_list character varying(255) NOT NULL
);


--
-- Name: dns_roots_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.dns_roots_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: dns_roots_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.dns_roots_id_seq OWNED BY public.dns_roots.id;


--
-- Name: emails; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.emails (
    id bigint NOT NULL,
    message_id character varying(255) NOT NULL,
    to_addresses text[] NOT NULL,
    from_address character varying(255) NOT NULL,
    reply_to character varying(255),
    subject text NOT NULL,
    body_text text NOT NULL,
    body_html text,
    account_id bigint,
    user_id bigint,
    server_id bigint,
    sent_at timestamp with time zone,
    error text,
    email_type character varying(50) NOT NULL,
    metadata jsonb,
    created_on timestamp with time zone DEFAULT now() NOT NULL,
    modified_on timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: emails_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.emails_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: emails_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.emails_id_seq OWNED BY public.emails.id;


--
-- Name: goose_db_version; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.goose_db_version (
    id integer NOT NULL,
    version_id bigint NOT NULL,
    is_applied boolean NOT NULL,
    tstamp timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: goose_db_version_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.goose_db_version ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.goose_db_version_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: log_scores; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.log_scores (
    id bigint NOT NULL,
    monitor_id bigint,
    server_id bigint NOT NULL,
    ts timestamp with time zone NOT NULL,
    score double precision DEFAULT '0'::double precision NOT NULL,
    step double precision DEFAULT '0'::double precision NOT NULL,
    "offset" double precision,
    rtt integer,
    attributes text
);


--
-- Name: log_scores_archive_status; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.log_scores_archive_status (
    id bigint NOT NULL,
    archiver character varying(255) NOT NULL,
    log_score_id bigint,
    modified_on timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: log_scores_archive_status_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.log_scores_archive_status_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: log_scores_archive_status_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.log_scores_archive_status_id_seq OWNED BY public.log_scores_archive_status.id;


--
-- Name: log_scores_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.log_scores_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: log_scores_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.log_scores_id_seq OWNED BY public.log_scores.id;


--
-- Name: logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.logs (
    id bigint NOT NULL,
    account_id bigint,
    server_id bigint,
    user_id bigint,
    vendor_zone_id bigint,
    type character varying(50),
    message text,
    changes text,
    created_on timestamp with time zone NOT NULL
);


--
-- Name: logs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: logs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.logs_id_seq OWNED BY public.logs.id;


--
-- Name: monitor_registrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.monitor_registrations (
    id bigint NOT NULL,
    monitor_id bigint,
    request_token character varying(128) NOT NULL,
    verification_token character varying(32) NOT NULL,
    ip4 character varying(15) DEFAULT ''::character varying NOT NULL,
    ip6 character varying(39) DEFAULT ''::character varying NOT NULL,
    tls_name character varying(255) DEFAULT ''::character varying,
    hostname character varying(256) DEFAULT ''::character varying NOT NULL,
    location_code character varying(5) DEFAULT ''::character varying NOT NULL,
    account_id bigint,
    client character varying(256) DEFAULT ''::character varying NOT NULL,
    status public.monitor_registrations_status NOT NULL,
    last_seen timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    created_on timestamp with time zone NOT NULL
);


--
-- Name: monitor_registrations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.monitor_registrations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: monitor_registrations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.monitor_registrations_id_seq OWNED BY public.monitor_registrations.id;


--
-- Name: monitors; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.monitors (
    id bigint NOT NULL,
    id_token character varying(36),
    type public.monitors_type DEFAULT 'monitor'::public.monitors_type NOT NULL,
    user_id bigint,
    account_id bigint,
    hostname character varying(255) DEFAULT ''::character varying NOT NULL,
    location character varying(255) DEFAULT ''::character varying NOT NULL,
    ip character varying(40),
    ip_version public.monitors_ip_version,
    tls_name character varying(255),
    api_key character varying(64),
    status public.monitors_status NOT NULL,
    config text NOT NULL,
    client_version character varying(255) DEFAULT ''::character varying NOT NULL,
    last_seen timestamp with time zone,
    last_submit timestamp with time zone,
    created_on timestamp with time zone NOT NULL,
    deleted_on timestamp with time zone,
    is_current boolean DEFAULT true
);


--
-- Name: monitors_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.monitors_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: monitors_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.monitors_id_seq OWNED BY public.monitors.id;


--
-- Name: oidc_public_keys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.oidc_public_keys (
    id bigint NOT NULL,
    kid character varying(255) NOT NULL,
    public_key text NOT NULL,
    algorithm character varying(20) NOT NULL,
    created_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone,
    active boolean DEFAULT true NOT NULL
);


--
-- Name: oidc_public_keys_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.oidc_public_keys_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: oidc_public_keys_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.oidc_public_keys_id_seq OWNED BY public.oidc_public_keys.id;


--
-- Name: schema_revision; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.schema_revision (
    revision integer DEFAULT 0 NOT NULL,
    schema_name character varying(30) NOT NULL
);


--
-- Name: scorer_status; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.scorer_status (
    id bigint NOT NULL,
    scorer_id bigint NOT NULL,
    log_score_id bigint NOT NULL,
    modified_on timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: scorer_status_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.scorer_status_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: scorer_status_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.scorer_status_id_seq OWNED BY public.scorer_status.id;


--
-- Name: server_alerts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.server_alerts (
    server_id bigint NOT NULL,
    last_score double precision NOT NULL,
    first_email_time timestamp with time zone NOT NULL,
    last_email_time timestamp with time zone
);


--
-- Name: TABLE server_alerts; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.server_alerts IS 'Tracks bad server notification history per server';


--
-- Name: COLUMN server_alerts.server_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.server_alerts.server_id IS 'Server that has been notified about';


--
-- Name: COLUMN server_alerts.last_score; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.server_alerts.last_score IS 'Server score at last notification (for deterioration detection)';


--
-- Name: COLUMN server_alerts.first_email_time; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.server_alerts.first_email_time IS 'When server was first flagged as bad';


--
-- Name: COLUMN server_alerts.last_email_time; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.server_alerts.last_email_time IS 'When last notification was sent';


--
-- Name: server_notes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.server_notes (
    id bigint NOT NULL,
    server_id bigint NOT NULL,
    name character varying(255) DEFAULT ''::character varying NOT NULL,
    note text NOT NULL,
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: server_notes_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.server_notes_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: server_notes_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.server_notes_id_seq OWNED BY public.server_notes.id;


--
-- Name: server_precheck_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.server_precheck_tokens (
    id bigint NOT NULL,
    token text NOT NULL,
    account_id bigint NOT NULL,
    precheck_data jsonb NOT NULL,
    created_on timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    expires_on timestamp with time zone DEFAULT (CURRENT_TIMESTAMP + '00:05:00'::interval) NOT NULL,
    consumed boolean DEFAULT false NOT NULL
);


--
-- Name: server_precheck_tokens_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.server_precheck_tokens_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: server_precheck_tokens_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.server_precheck_tokens_id_seq OWNED BY public.server_precheck_tokens.id;


--
-- Name: server_scores; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.server_scores (
    id bigint NOT NULL,
    monitor_id bigint NOT NULL,
    server_id bigint NOT NULL,
    score_ts timestamp with time zone,
    score_raw double precision DEFAULT '0'::double precision NOT NULL,
    stratum smallint,
    status public.server_scores_status DEFAULT 'candidate'::public.server_scores_status NOT NULL,
    queue_ts timestamp with time zone,
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    constraint_violation_type character varying(50),
    constraint_violation_since timestamp with time zone,
    last_constraint_check timestamp with time zone,
    pause_reason character varying(20)
);


--
-- Name: server_scores_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.server_scores_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: server_scores_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.server_scores_id_seq OWNED BY public.server_scores.id;


--
-- Name: server_urls; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.server_urls (
    id bigint NOT NULL,
    server_id bigint NOT NULL,
    url character varying(255) NOT NULL
);


--
-- Name: server_urls_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.server_urls_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: server_urls_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.server_urls_id_seq OWNED BY public.server_urls.id;


--
-- Name: server_verifications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.server_verifications (
    id bigint NOT NULL,
    server_id bigint NOT NULL,
    user_id bigint,
    user_ip character varying(45) DEFAULT ''::character varying NOT NULL,
    indirect_ip character varying(45) DEFAULT ''::character varying NOT NULL,
    verified_on timestamp with time zone,
    token character varying(36),
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: server_verifications_history; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.server_verifications_history (
    id bigint NOT NULL,
    server_id bigint NOT NULL,
    user_id bigint,
    user_ip character varying(45) DEFAULT ''::character varying NOT NULL,
    indirect_ip character varying(45) DEFAULT ''::character varying NOT NULL,
    verified_on timestamp with time zone,
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: server_verifications_history_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.server_verifications_history_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: server_verifications_history_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.server_verifications_history_id_seq OWNED BY public.server_verifications_history.id;


--
-- Name: server_verifications_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.server_verifications_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: server_verifications_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.server_verifications_id_seq OWNED BY public.server_verifications.id;


--
-- Name: server_zones; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.server_zones (
    server_id bigint NOT NULL,
    zone_id bigint NOT NULL
);


--
-- Name: servers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.servers (
    id bigint NOT NULL,
    ip character varying(40) NOT NULL,
    ip_version public.servers_ip_version DEFAULT 'v4'::public.servers_ip_version NOT NULL,
    user_id bigint,
    account_id bigint,
    hostname character varying(255),
    stratum smallint,
    in_pool smallint DEFAULT '0'::smallint NOT NULL,
    in_server_list smallint DEFAULT '0'::smallint NOT NULL,
    netspeed bigint DEFAULT '10000'::bigint NOT NULL,
    netspeed_target bigint DEFAULT '10000'::bigint NOT NULL,
    created_on timestamp with time zone NOT NULL,
    updated_on timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    score_ts timestamp with time zone,
    score_raw double precision DEFAULT '0'::double precision NOT NULL,
    deletion_on date,
    flags character varying(4096) DEFAULT '{}'::character varying NOT NULL
);


--
-- Name: servers_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.servers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: servers_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.servers_id_seq OWNED BY public.servers.id;


--
-- Name: servers_monitor_review; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.servers_monitor_review (
    server_id bigint NOT NULL,
    last_review timestamp with time zone,
    next_review timestamp with time zone,
    last_change timestamp with time zone,
    config character varying(4096) DEFAULT ''::character varying NOT NULL
);


--
-- Name: system_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.system_settings (
    id bigint NOT NULL,
    key character varying(255) NOT NULL,
    value text NOT NULL,
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: system_settings_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.system_settings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: system_settings_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.system_settings_id_seq OWNED BY public.system_settings.id;


--
-- Name: user_equipment_applications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_equipment_applications (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    application text,
    contact_information text,
    status public.user_equipment_applications_status DEFAULT 'New'::public.user_equipment_applications_status NOT NULL
);


--
-- Name: user_equipment_applications_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_equipment_applications_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_equipment_applications_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_equipment_applications_id_seq OWNED BY public.user_equipment_applications.id;


--
-- Name: user_identities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_identities (
    id bigint NOT NULL,
    profile_id character varying(255) NOT NULL,
    user_id bigint NOT NULL,
    provider character varying(255) NOT NULL,
    data text,
    email character varying(255),
    created_on timestamp with time zone DEFAULT '2003-01-26 16:00:00-08'::timestamp with time zone NOT NULL,
    modified_on timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: user_identities_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_identities_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_identities_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_identities_id_seq OWNED BY public.user_identities.id;


--
-- Name: user_privileges; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_privileges (
    user_id bigint NOT NULL,
    see_all_servers boolean DEFAULT false,
    vendor_admin boolean DEFAULT false,
    equipment_admin boolean DEFAULT false,
    support_staff boolean DEFAULT false,
    monitor_admin boolean DEFAULT false
);


--
-- Name: user_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_sessions (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    token_lookup character varying(16) NOT NULL,
    token_hashed character varying(256) NOT NULL,
    last_seen timestamp with time zone,
    created_on timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    csrf_token character varying(64) DEFAULT NULL::character varying
);


--
-- Name: user_sessions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_sessions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_sessions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_sessions_id_seq OWNED BY public.user_sessions.id;


--
-- Name: user_tasks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_tasks (
    id bigint NOT NULL,
    user_id bigint,
    task public.user_tasks_task NOT NULL,
    status text NOT NULL,
    traceid uuid DEFAULT uuidv7() NOT NULL,
    execute_on timestamp with time zone,
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: user_tasks_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_tasks_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_tasks_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_tasks_id_seq OWNED BY public.user_tasks.id;


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id bigint NOT NULL,
    id_token character varying(36),
    email character varying(255) NOT NULL,
    name character varying(255),
    username character varying(40),
    public_profile boolean DEFAULT false NOT NULL,
    deletion_on timestamp with time zone
);


--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: users_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.users_id_seq OWNED BY public.users.id;


--
-- Name: vendor_zones; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vendor_zones (
    id bigint NOT NULL,
    id_token character varying(36),
    zone_name character varying(90) NOT NULL,
    status public.vendor_zones_status DEFAULT 'New'::public.vendor_zones_status NOT NULL,
    user_id bigint,
    organization_name character varying(255),
    client_type public.vendor_zones_client_type DEFAULT 'sntp'::public.vendor_zones_client_type NOT NULL,
    contact_information text,
    request_information text,
    device_information text,
    device_count bigint,
    opensource boolean DEFAULT false NOT NULL,
    opensource_info text,
    rt_ticket integer,
    approved_on timestamp with time zone,
    created_on timestamp with time zone NOT NULL,
    modified_on timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    dns_root_id bigint NOT NULL,
    account_id bigint
);


--
-- Name: vendor_zones_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.vendor_zones_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: vendor_zones_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.vendor_zones_id_seq OWNED BY public.vendor_zones.id;


--
-- Name: zone_server_counts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.zone_server_counts (
    id bigint NOT NULL,
    zone_id bigint NOT NULL,
    ip_version public.zone_server_counts_ip_version NOT NULL,
    date date NOT NULL,
    count_active integer NOT NULL,
    count_registered integer NOT NULL,
    netspeed_active bigint NOT NULL
);


--
-- Name: zone_server_counts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.zone_server_counts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: zone_server_counts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.zone_server_counts_id_seq OWNED BY public.zone_server_counts.id;


--
-- Name: zones; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.zones (
    id bigint NOT NULL,
    name character varying(255) NOT NULL,
    description character varying(255),
    parent_id bigint,
    dns boolean DEFAULT true NOT NULL
);


--
-- Name: zones_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.zones_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: zones_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.zones_id_seq OWNED BY public.zones.id;


--
-- Name: account_invites id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_invites ALTER COLUMN id SET DEFAULT nextval('public.account_invites_id_seq'::regclass);


--
-- Name: account_subscriptions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_subscriptions ALTER COLUMN id SET DEFAULT nextval('public.account_subscriptions_id_seq'::regclass);


--
-- Name: accounts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.accounts ALTER COLUMN id SET DEFAULT nextval('public.accounts_id_seq'::regclass);


--
-- Name: api_keys id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys ALTER COLUMN id SET DEFAULT nextval('public.api_keys_id_seq'::regclass);


--
-- Name: dns_roots id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dns_roots ALTER COLUMN id SET DEFAULT nextval('public.dns_roots_id_seq'::regclass);


--
-- Name: emails id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.emails ALTER COLUMN id SET DEFAULT nextval('public.emails_id_seq'::regclass);


--
-- Name: log_scores id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.log_scores ALTER COLUMN id SET DEFAULT nextval('public.log_scores_id_seq'::regclass);


--
-- Name: log_scores_archive_status id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.log_scores_archive_status ALTER COLUMN id SET DEFAULT nextval('public.log_scores_archive_status_id_seq'::regclass);


--
-- Name: logs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.logs ALTER COLUMN id SET DEFAULT nextval('public.logs_id_seq'::regclass);


--
-- Name: monitor_registrations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.monitor_registrations ALTER COLUMN id SET DEFAULT nextval('public.monitor_registrations_id_seq'::regclass);


--
-- Name: monitors id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.monitors ALTER COLUMN id SET DEFAULT nextval('public.monitors_id_seq'::regclass);


--
-- Name: oidc_public_keys id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.oidc_public_keys ALTER COLUMN id SET DEFAULT nextval('public.oidc_public_keys_id_seq'::regclass);


--
-- Name: scorer_status id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scorer_status ALTER COLUMN id SET DEFAULT nextval('public.scorer_status_id_seq'::regclass);


--
-- Name: server_notes id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_notes ALTER COLUMN id SET DEFAULT nextval('public.server_notes_id_seq'::regclass);


--
-- Name: server_precheck_tokens id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_precheck_tokens ALTER COLUMN id SET DEFAULT nextval('public.server_precheck_tokens_id_seq'::regclass);


--
-- Name: server_scores id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_scores ALTER COLUMN id SET DEFAULT nextval('public.server_scores_id_seq'::regclass);


--
-- Name: server_urls id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_urls ALTER COLUMN id SET DEFAULT nextval('public.server_urls_id_seq'::regclass);


--
-- Name: server_verifications id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_verifications ALTER COLUMN id SET DEFAULT nextval('public.server_verifications_id_seq'::regclass);


--
-- Name: server_verifications_history id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_verifications_history ALTER COLUMN id SET DEFAULT nextval('public.server_verifications_history_id_seq'::regclass);


--
-- Name: servers id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.servers ALTER COLUMN id SET DEFAULT nextval('public.servers_id_seq'::regclass);


--
-- Name: system_settings id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.system_settings ALTER COLUMN id SET DEFAULT nextval('public.system_settings_id_seq'::regclass);


--
-- Name: user_equipment_applications id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_equipment_applications ALTER COLUMN id SET DEFAULT nextval('public.user_equipment_applications_id_seq'::regclass);


--
-- Name: user_identities id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_identities ALTER COLUMN id SET DEFAULT nextval('public.user_identities_id_seq'::regclass);


--
-- Name: user_sessions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_sessions ALTER COLUMN id SET DEFAULT nextval('public.user_sessions_id_seq'::regclass);


--
-- Name: user_tasks id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_tasks ALTER COLUMN id SET DEFAULT nextval('public.user_tasks_id_seq'::regclass);


--
-- Name: users id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users ALTER COLUMN id SET DEFAULT nextval('public.users_id_seq'::regclass);


--
-- Name: vendor_zones id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vendor_zones ALTER COLUMN id SET DEFAULT nextval('public.vendor_zones_id_seq'::regclass);


--
-- Name: zone_server_counts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.zone_server_counts ALTER COLUMN id SET DEFAULT nextval('public.zone_server_counts_id_seq'::regclass);


--
-- Name: zones id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.zones ALTER COLUMN id SET DEFAULT nextval('public.zones_id_seq'::regclass);


--
-- Name: account_invites account_invites_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_invites
    ADD CONSTRAINT account_invites_pkey PRIMARY KEY (id);


--
-- Name: account_subscriptions account_subscriptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_subscriptions
    ADD CONSTRAINT account_subscriptions_pkey PRIMARY KEY (id);


--
-- Name: account_users account_users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_users
    ADD CONSTRAINT account_users_pkey PRIMARY KEY (account_id, user_id);


--
-- Name: accounts accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.accounts
    ADD CONSTRAINT accounts_pkey PRIMARY KEY (id);


--
-- Name: api_keys_monitors api_keys_monitors_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys_monitors
    ADD CONSTRAINT api_keys_monitors_pkey PRIMARY KEY (api_key_id, monitor_id);


--
-- Name: api_keys api_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_pkey PRIMARY KEY (id);


--
-- Name: combust_cache combust_cache_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.combust_cache
    ADD CONSTRAINT combust_cache_pkey PRIMARY KEY (id, type);


--
-- Name: combust_secrets combust_secrets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.combust_secrets
    ADD CONSTRAINT combust_secrets_pkey PRIMARY KEY (type, secret_ts);


--
-- Name: dns_roots dns_roots_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dns_roots
    ADD CONSTRAINT dns_roots_pkey PRIMARY KEY (id);


--
-- Name: emails emails_message_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.emails
    ADD CONSTRAINT emails_message_id_key UNIQUE (message_id);


--
-- Name: emails emails_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.emails
    ADD CONSTRAINT emails_pkey PRIMARY KEY (id);


--
-- Name: goose_db_version goose_db_version_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.goose_db_version
    ADD CONSTRAINT goose_db_version_pkey PRIMARY KEY (id);


--
-- Name: log_scores_archive_status log_scores_archive_status_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.log_scores_archive_status
    ADD CONSTRAINT log_scores_archive_status_pkey PRIMARY KEY (id);


--
-- Name: logs logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.logs
    ADD CONSTRAINT logs_pkey PRIMARY KEY (id);


--
-- Name: monitor_registrations monitor_registrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.monitor_registrations
    ADD CONSTRAINT monitor_registrations_pkey PRIMARY KEY (id);


--
-- Name: monitors monitors_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.monitors
    ADD CONSTRAINT monitors_pkey PRIMARY KEY (id);


--
-- Name: oidc_public_keys oidc_public_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.oidc_public_keys
    ADD CONSTRAINT oidc_public_keys_pkey PRIMARY KEY (id);


--
-- Name: schema_revision schema_revision_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schema_revision
    ADD CONSTRAINT schema_revision_pkey PRIMARY KEY (schema_name);


--
-- Name: scorer_status scorer_status_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scorer_status
    ADD CONSTRAINT scorer_status_pkey PRIMARY KEY (id);


--
-- Name: server_alerts server_alerts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_alerts
    ADD CONSTRAINT server_alerts_pkey PRIMARY KEY (server_id);


--
-- Name: server_notes server_notes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_notes
    ADD CONSTRAINT server_notes_pkey PRIMARY KEY (id);


--
-- Name: server_precheck_tokens server_precheck_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_precheck_tokens
    ADD CONSTRAINT server_precheck_tokens_pkey PRIMARY KEY (id);


--
-- Name: server_precheck_tokens server_precheck_tokens_token_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_precheck_tokens
    ADD CONSTRAINT server_precheck_tokens_token_key UNIQUE (token);


--
-- Name: server_scores server_scores_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_scores
    ADD CONSTRAINT server_scores_pkey PRIMARY KEY (id);


--
-- Name: server_urls server_urls_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_urls
    ADD CONSTRAINT server_urls_pkey PRIMARY KEY (id);


--
-- Name: server_verifications_history server_verifications_history_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_verifications_history
    ADD CONSTRAINT server_verifications_history_pkey PRIMARY KEY (id);


--
-- Name: server_verifications server_verifications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_verifications
    ADD CONSTRAINT server_verifications_pkey PRIMARY KEY (id);


--
-- Name: server_zones server_zones_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_zones
    ADD CONSTRAINT server_zones_pkey PRIMARY KEY (server_id, zone_id);


--
-- Name: servers_monitor_review servers_monitor_review_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.servers_monitor_review
    ADD CONSTRAINT servers_monitor_review_pkey PRIMARY KEY (server_id);


--
-- Name: servers servers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.servers
    ADD CONSTRAINT servers_pkey PRIMARY KEY (id);


--
-- Name: system_settings system_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.system_settings
    ADD CONSTRAINT system_settings_pkey PRIMARY KEY (id);


--
-- Name: user_equipment_applications user_equipment_applications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_equipment_applications
    ADD CONSTRAINT user_equipment_applications_pkey PRIMARY KEY (id);


--
-- Name: user_identities user_identities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_identities
    ADD CONSTRAINT user_identities_pkey PRIMARY KEY (id);


--
-- Name: user_privileges user_privileges_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_privileges
    ADD CONSTRAINT user_privileges_pkey PRIMARY KEY (user_id);


--
-- Name: user_sessions user_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_sessions
    ADD CONSTRAINT user_sessions_pkey PRIMARY KEY (id);


--
-- Name: user_tasks user_tasks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_tasks
    ADD CONSTRAINT user_tasks_pkey PRIMARY KEY (id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: vendor_zones vendor_zones_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vendor_zones
    ADD CONSTRAINT vendor_zones_pkey PRIMARY KEY (id);


--
-- Name: zone_server_counts zone_server_counts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.zone_server_counts
    ADD CONSTRAINT zone_server_counts_pkey PRIMARY KEY (id);


--
-- Name: zones zones_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.zones
    ADD CONSTRAINT zones_pkey PRIMARY KEY (id);


--
-- Name: idx_18134_account_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18134_account_id ON public.account_invites USING btree (account_id, email);


--
-- Name: idx_18134_account_invites_sent_by_fk; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18134_account_invites_sent_by_fk ON public.account_invites USING btree (sent_by_id);


--
-- Name: idx_18134_account_invites_user_fk; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18134_account_invites_user_fk ON public.account_invites USING btree (user_id);


--
-- Name: idx_18134_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18134_code ON public.account_invites USING btree (code);


--
-- Name: idx_18140_account_subscriptions_account_fk; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18140_account_subscriptions_account_fk ON public.account_subscriptions USING btree (account_id);


--
-- Name: idx_18140_stripe_subscription_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18140_stripe_subscription_id ON public.account_subscriptions USING btree (stripe_subscription_id);


--
-- Name: idx_18147_account_users_user_fk; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18147_account_users_user_fk ON public.account_users USING btree (user_id);


--
-- Name: idx_18151_id_token; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18151_id_token ON public.accounts USING btree (id_token);


--
-- Name: idx_18151_stripe_customer_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18151_stripe_customer_id ON public.accounts USING btree (stripe_customer_id);


--
-- Name: idx_18151_url_slug_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18151_url_slug_idx ON public.accounts USING btree (url_slug);


--
-- Name: idx_18160_api_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18160_api_key ON public.api_keys USING btree (api_key);


--
-- Name: idx_18160_api_keys_account_fk; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18160_api_keys_account_fk ON public.api_keys USING btree (account_id);


--
-- Name: idx_18160_api_keys_user_fk; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18160_api_keys_user_fk ON public.api_keys USING btree (user_id);


--
-- Name: idx_18167_api_keys_monitors_monitors_fk; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18167_api_keys_monitors_monitors_fk ON public.api_keys_monitors USING btree (monitor_id);


--
-- Name: idx_18170_expire_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18170_expire_idx ON public.combust_cache USING btree (expire);


--
-- Name: idx_18170_purge_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18170_purge_idx ON public.combust_cache USING btree (purge_key);


--
-- Name: idx_18179_expires_ts; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18179_expires_ts ON public.combust_secrets USING btree (expires_ts);


--
-- Name: idx_18183_origin; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18183_origin ON public.dns_roots USING btree (origin);


--
-- Name: idx_18192_log_scores_server_ts_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18192_log_scores_server_ts_idx ON public.log_scores USING btree (server_id, ts);


--
-- Name: idx_18192_ts; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18192_ts ON public.log_scores USING btree (ts);


--
-- Name: idx_18201_archiver; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18201_archiver ON public.log_scores_archive_status USING btree (archiver);


--
-- Name: idx_18201_log_score_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18201_log_score_id ON public.log_scores_archive_status USING btree (log_score_id);


--
-- Name: idx_18207_account_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18207_account_id_idx ON public.logs USING btree (account_id);


--
-- Name: idx_18207_logs_vendor_zone_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18207_logs_vendor_zone_id ON public.logs USING btree (vendor_zone_id);


--
-- Name: idx_18207_server_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18207_server_id ON public.logs USING btree (server_id, type);


--
-- Name: idx_18207_server_logs_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18207_server_logs_user_id ON public.logs USING btree (user_id);


--
-- Name: idx_18214_monitor_registrations_account_fk; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18214_monitor_registrations_account_fk ON public.monitor_registrations USING btree (account_id);


--
-- Name: idx_18214_monitor_registrations_monitor_id_fk; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18214_monitor_registrations_monitor_id_fk ON public.monitor_registrations USING btree (monitor_id);


--
-- Name: idx_18214_request_token; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18214_request_token ON public.monitor_registrations USING btree (request_token);


--
-- Name: idx_18214_verification_token; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18214_verification_token ON public.monitor_registrations USING btree (verification_token);


--
-- Name: idx_18228_api_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18228_api_key ON public.monitors USING btree (api_key);


--
-- Name: idx_18228_id_token; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18228_id_token ON public.monitors USING btree (id_token);


--
-- Name: idx_18228_ip; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18228_ip ON public.monitors USING btree (ip, is_current);


--
-- Name: idx_18228_monitors_account_fk; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18228_monitors_account_fk ON public.monitors USING btree (account_id);


--
-- Name: idx_18228_monitors_tls_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18228_monitors_tls_name ON public.monitors USING btree (tls_name, ip_version);


--
-- Name: idx_18228_monitors_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18228_monitors_user_id ON public.monitors USING btree (user_id);


--
-- Name: idx_18228_token_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18228_token_id ON public.monitors USING btree (id_token);


--
-- Name: idx_18228_type_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18228_type_status ON public.monitors USING btree (type, status);


--
-- Name: idx_18240_idx_active_expires; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18240_idx_active_expires ON public.oidc_public_keys USING btree (active, expires_at);


--
-- Name: idx_18240_idx_kid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18240_idx_kid ON public.oidc_public_keys USING btree (kid);


--
-- Name: idx_18240_kid; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18240_kid ON public.oidc_public_keys USING btree (kid);


--
-- Name: idx_18252_scorer_log_score_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18252_scorer_log_score_id ON public.scorer_status USING btree (log_score_id);


--
-- Name: idx_18252_scores_status_monitor_id_fk; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18252_scores_status_monitor_id_fk ON public.scorer_status USING btree (scorer_id);


--
-- Name: idx_18261_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18261_name ON public.server_notes USING btree (name);


--
-- Name: idx_18261_server; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18261_server ON public.server_notes USING btree (server_id, name);


--
-- Name: idx_18270_idx_constraint_violation; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18270_idx_constraint_violation ON public.server_scores USING btree (constraint_violation_type, constraint_violation_since);


--
-- Name: idx_18270_idx_paused_monitors; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18270_idx_paused_monitors ON public.server_scores USING btree (status, last_constraint_check, pause_reason);


--
-- Name: idx_18270_monitor_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18270_monitor_id ON public.server_scores USING btree (monitor_id, server_id);


--
-- Name: idx_18270_monitor_id_2; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18270_monitor_id_2 ON public.server_scores USING btree (monitor_id, score_ts);


--
-- Name: idx_18270_server_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18270_server_id ON public.server_scores USING btree (server_id, monitor_id);


--
-- Name: idx_18278_server; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18278_server ON public.server_urls USING btree (server_id);


--
-- Name: idx_18283_server; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18283_server ON public.server_verifications USING btree (server_id);


--
-- Name: idx_18283_server_verifications_ibfk_2; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18283_server_verifications_ibfk_2 ON public.server_verifications USING btree (user_id);


--
-- Name: idx_18283_token; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18283_token ON public.server_verifications USING btree (token);


--
-- Name: idx_18291_server_verifications_history_ibfk_1; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18291_server_verifications_history_ibfk_1 ON public.server_verifications_history USING btree (server_id);


--
-- Name: idx_18291_server_verifications_history_ibfk_2; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18291_server_verifications_history_ibfk_2 ON public.server_verifications_history USING btree (user_id);


--
-- Name: idx_18298_locations_zone; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18298_locations_zone ON public.server_zones USING btree (zone_id);


--
-- Name: idx_18302_admin; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18302_admin ON public.servers USING btree (user_id);


--
-- Name: idx_18302_deletion_on; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18302_deletion_on ON public.servers USING btree (deletion_on);


--
-- Name: idx_18302_ip; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18302_ip ON public.servers USING btree (ip);


--
-- Name: idx_18302_score_ts; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18302_score_ts ON public.servers USING btree (score_ts);


--
-- Name: idx_18302_server_account_fk; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18302_server_account_fk ON public.servers USING btree (account_id);


--
-- Name: idx_18316_next_review; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18316_next_review ON public.servers_monitor_review USING btree (next_review);


--
-- Name: idx_18323_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18323_key ON public.system_settings USING btree (key);


--
-- Name: idx_18331_user_equipment_applications_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18331_user_equipment_applications_user_id ON public.user_equipment_applications USING btree (user_id);


--
-- Name: idx_18339_profile_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18339_profile_id ON public.user_identities USING btree (profile_id);


--
-- Name: idx_18339_user_identities_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18339_user_identities_user_id ON public.user_identities USING btree (user_id);


--
-- Name: idx_18356_token_lookup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18356_token_lookup ON public.user_sessions USING btree (token_lookup);


--
-- Name: idx_18356_user_sessions_user_fk; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18356_user_sessions_user_fk ON public.user_sessions USING btree (user_id);


--
-- Name: idx_18362_user_tasks_user_fk; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18362_user_tasks_user_fk ON public.user_tasks USING btree (user_id);


--
-- Name: idx_18371_email; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18371_email ON public.users USING btree (email);


--
-- Name: idx_18371_id_token; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18371_id_token ON public.users USING btree (id_token);


--
-- Name: idx_18371_username; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18371_username ON public.users USING btree (username);


--
-- Name: idx_18379_dns_root_fk; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18379_dns_root_fk ON public.vendor_zones USING btree (dns_root_id);


--
-- Name: idx_18379_id_token; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18379_id_token ON public.vendor_zones USING btree (id_token);


--
-- Name: idx_18379_vendor_zone_account_fk; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18379_vendor_zone_account_fk ON public.vendor_zones USING btree (account_id);


--
-- Name: idx_18379_vendor_zones_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18379_vendor_zones_user_id ON public.vendor_zones USING btree (user_id);


--
-- Name: idx_18379_zone_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18379_zone_name ON public.vendor_zones USING btree (zone_name, dns_root_id);


--
-- Name: idx_18390_date_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18390_date_idx ON public.zone_server_counts USING btree (date, zone_id);


--
-- Name: idx_18390_zone_date; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18390_zone_date ON public.zone_server_counts USING btree (zone_id, date, ip_version);


--
-- Name: idx_18395_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_18395_name ON public.zones USING btree (name);


--
-- Name: idx_18395_parent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_18395_parent ON public.zones USING btree (parent_id);


--
-- Name: idx_emails_account_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_emails_account_id ON public.emails USING btree (account_id);


--
-- Name: idx_emails_created_on; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_emails_created_on ON public.emails USING btree (created_on);


--
-- Name: idx_emails_email_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_emails_email_type ON public.emails USING btree (email_type);


--
-- Name: idx_emails_message_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_emails_message_id ON public.emails USING btree (message_id);


--
-- Name: idx_emails_sent_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_emails_sent_at ON public.emails USING btree (sent_at);


--
-- Name: idx_emails_server_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_emails_server_id ON public.emails USING btree (server_id);


--
-- Name: idx_emails_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_emails_user_id ON public.emails USING btree (user_id);


--
-- Name: idx_precheck_tokens_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_precheck_tokens_active ON public.server_precheck_tokens USING btree (token) WHERE (NOT consumed);


--
-- Name: idx_precheck_tokens_cleanup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_precheck_tokens_cleanup ON public.server_precheck_tokens USING btree (expires_on);


--
-- Name: users_username_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX users_username_unique ON public.users USING btree (username) WHERE (username IS NOT NULL);


--
-- Name: account_invites account_invites_account_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_invites
    ADD CONSTRAINT account_invites_account_fk FOREIGN KEY (account_id) REFERENCES public.accounts(id);


--
-- Name: account_invites account_invites_sent_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_invites
    ADD CONSTRAINT account_invites_sent_by_fk FOREIGN KEY (sent_by_id) REFERENCES public.users(id);


--
-- Name: account_invites account_invites_user_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_invites
    ADD CONSTRAINT account_invites_user_fk FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: account_subscriptions account_subscriptions_account_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_subscriptions
    ADD CONSTRAINT account_subscriptions_account_fk FOREIGN KEY (account_id) REFERENCES public.accounts(id);


--
-- Name: account_users account_users_account_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_users
    ADD CONSTRAINT account_users_account_fk FOREIGN KEY (account_id) REFERENCES public.accounts(id);


--
-- Name: account_users account_users_user_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_users
    ADD CONSTRAINT account_users_user_fk FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: api_keys api_keys_account_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_account_fk FOREIGN KEY (account_id) REFERENCES public.accounts(id);


--
-- Name: api_keys_monitors api_keys_monitors_api_keys_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys_monitors
    ADD CONSTRAINT api_keys_monitors_api_keys_fk FOREIGN KEY (api_key_id) REFERENCES public.api_keys(id) ON DELETE CASCADE;


--
-- Name: api_keys_monitors api_keys_monitors_monitors_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys_monitors
    ADD CONSTRAINT api_keys_monitors_monitors_fk FOREIGN KEY (monitor_id) REFERENCES public.monitors(id);


--
-- Name: api_keys api_keys_user_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_user_fk FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: vendor_zones dns_root_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vendor_zones
    ADD CONSTRAINT dns_root_fk FOREIGN KEY (dns_root_id) REFERENCES public.dns_roots(id);


--
-- Name: emails emails_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.emails
    ADD CONSTRAINT emails_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id);


--
-- Name: emails emails_server_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.emails
    ADD CONSTRAINT emails_server_id_fkey FOREIGN KEY (server_id) REFERENCES public.servers(id);


--
-- Name: emails emails_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.emails
    ADD CONSTRAINT emails_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: log_scores log_score_monitor_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.log_scores
    ADD CONSTRAINT log_score_monitor_id_fk FOREIGN KEY (monitor_id) REFERENCES public.monitors(id);


--
-- Name: log_scores log_scores_server; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.log_scores
    ADD CONSTRAINT log_scores_server FOREIGN KEY (server_id) REFERENCES public.servers(id) ON DELETE CASCADE;


--
-- Name: logs logs_vendor_zone_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.logs
    ADD CONSTRAINT logs_vendor_zone_id FOREIGN KEY (vendor_zone_id) REFERENCES public.vendor_zones(id) ON DELETE CASCADE;


--
-- Name: monitor_registrations monitor_registrations_account_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.monitor_registrations
    ADD CONSTRAINT monitor_registrations_account_fk FOREIGN KEY (account_id) REFERENCES public.accounts(id);


--
-- Name: monitor_registrations monitor_registrations_monitor_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.monitor_registrations
    ADD CONSTRAINT monitor_registrations_monitor_id_fk FOREIGN KEY (monitor_id) REFERENCES public.monitors(id) ON DELETE CASCADE;


--
-- Name: monitors monitors_account_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.monitors
    ADD CONSTRAINT monitors_account_fk FOREIGN KEY (account_id) REFERENCES public.accounts(id);


--
-- Name: monitors monitors_user_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.monitors
    ADD CONSTRAINT monitors_user_id FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: scorer_status scores_status_monitor_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scorer_status
    ADD CONSTRAINT scores_status_monitor_id_fk FOREIGN KEY (scorer_id) REFERENCES public.monitors(id);


--
-- Name: servers server_account_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.servers
    ADD CONSTRAINT server_account_fk FOREIGN KEY (account_id) REFERENCES public.accounts(id);


--
-- Name: server_alerts server_alerts_server; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_alerts
    ADD CONSTRAINT server_alerts_server FOREIGN KEY (server_id) REFERENCES public.servers(id) ON DELETE CASCADE;


--
-- Name: servers_monitor_review server_monitor_review_server_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.servers_monitor_review
    ADD CONSTRAINT server_monitor_review_server_id_fk FOREIGN KEY (server_id) REFERENCES public.servers(id) ON DELETE CASCADE;


--
-- Name: server_notes server_notes_ibfk_1; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_notes
    ADD CONSTRAINT server_notes_ibfk_1 FOREIGN KEY (server_id) REFERENCES public.servers(id) ON DELETE CASCADE;


--
-- Name: server_precheck_tokens server_precheck_tokens_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_precheck_tokens
    ADD CONSTRAINT server_precheck_tokens_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id);


--
-- Name: server_scores server_score_monitor_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_scores
    ADD CONSTRAINT server_score_monitor_fk FOREIGN KEY (monitor_id) REFERENCES public.monitors(id);


--
-- Name: server_scores server_score_server_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_scores
    ADD CONSTRAINT server_score_server_id FOREIGN KEY (server_id) REFERENCES public.servers(id) ON DELETE CASCADE;


--
-- Name: server_urls server_urls_server; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_urls
    ADD CONSTRAINT server_urls_server FOREIGN KEY (server_id) REFERENCES public.servers(id) ON DELETE CASCADE;


--
-- Name: server_verifications_history server_verifications_history_ibfk_1; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_verifications_history
    ADD CONSTRAINT server_verifications_history_ibfk_1 FOREIGN KEY (server_id) REFERENCES public.servers(id) ON DELETE CASCADE;


--
-- Name: server_verifications_history server_verifications_history_ibfk_2; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_verifications_history
    ADD CONSTRAINT server_verifications_history_ibfk_2 FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: server_verifications server_verifications_ibfk_1; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_verifications
    ADD CONSTRAINT server_verifications_ibfk_1 FOREIGN KEY (server_id) REFERENCES public.servers(id) ON DELETE CASCADE;


--
-- Name: server_verifications server_verifications_ibfk_2; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.server_verifications
    ADD CONSTRAINT server_verifications_ibfk_2 FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: servers servers_user_ibfk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.servers
    ADD CONSTRAINT servers_user_ibfk FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: user_equipment_applications user_equipment_applications_user_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_equipment_applications
    ADD CONSTRAINT user_equipment_applications_user_id FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: user_identities user_identities_user_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_identities
    ADD CONSTRAINT user_identities_user_id FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_privileges user_privileges_user; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_privileges
    ADD CONSTRAINT user_privileges_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_sessions user_sessions_user_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_sessions
    ADD CONSTRAINT user_sessions_user_fk FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: user_tasks user_tasks_user_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_tasks
    ADD CONSTRAINT user_tasks_user_fk FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: vendor_zones vendor_zone_account_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vendor_zones
    ADD CONSTRAINT vendor_zone_account_fk FOREIGN KEY (account_id) REFERENCES public.accounts(id);


--
-- Name: vendor_zones vendor_zones_user_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vendor_zones
    ADD CONSTRAINT vendor_zones_user_id FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: zone_server_counts zone_server_counts; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.zone_server_counts
    ADD CONSTRAINT zone_server_counts FOREIGN KEY (zone_id) REFERENCES public.zones(id) ON DELETE CASCADE;


--
-- Name: zones zones_parent; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.zones
    ADD CONSTRAINT zones_parent FOREIGN KEY (parent_id) REFERENCES public.zones(id);


--
-- Name: SCHEMA public; Type: ACL; Schema: -; Owner: -
--

GRANT ALL ON SCHEMA public TO ntppool;


--
-- Name: DEFAULT PRIVILEGES FOR SEQUENCES; Type: DEFAULT ACL; Schema: public; Owner: -
--

ALTER DEFAULT PRIVILEGES FOR ROLE postgres IN SCHEMA public GRANT ALL ON SEQUENCES TO ntppool;


--
-- Name: DEFAULT PRIVILEGES FOR TABLES; Type: DEFAULT ACL; Schema: public; Owner: -
--

ALTER DEFAULT PRIVILEGES FOR ROLE postgres IN SCHEMA public GRANT ALL ON TABLES TO ntppool;


--
-- PostgreSQL database dump complete
--

\unrestrict JrXmdpwfgrZ3hPsF1HhDavlypKeXBg7Tbu8oGE4HaBPH4PJsOgK0pVSUbzYpnXZ
