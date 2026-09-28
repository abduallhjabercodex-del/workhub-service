--
-- PostgreSQL database dump
--

\restrict dYsQjbud020RqvQxiAljMmQuYcfnvaAa2W26JG9YelylhkFDljmCI1fDFJ9IAXm

-- Dumped from database version 16.14
-- Dumped by pg_dump version 16.14

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

ALTER TABLE IF EXISTS ONLY public.user_quotas DROP CONSTRAINT IF EXISTS user_quotas_user_id_fkey;
ALTER TABLE IF EXISTS ONLY public.user_quotas DROP CONSTRAINT IF EXISTS user_quotas_set_by_fkey;
ALTER TABLE IF EXISTS ONLY public.tasks DROP CONSTRAINT IF EXISTS tasks_project_id_fkey;
ALTER TABLE IF EXISTS ONLY public.tasks DROP CONSTRAINT IF EXISTS tasks_created_by_fkey;
ALTER TABLE IF EXISTS ONLY public.tasks DROP CONSTRAINT IF EXISTS tasks_assignee_id_fkey;
ALTER TABLE IF EXISTS ONLY public.task_tags DROP CONSTRAINT IF EXISTS task_tags_task_id_fkey;
ALTER TABLE IF EXISTS ONLY public.task_tags DROP CONSTRAINT IF EXISTS task_tags_tag_id_fkey;
ALTER TABLE IF EXISTS ONLY public.tags DROP CONSTRAINT IF EXISTS tags_project_id_fkey;
ALTER TABLE IF EXISTS ONLY public.refresh_tokens DROP CONSTRAINT IF EXISTS refresh_tokens_user_id_fkey;
ALTER TABLE IF EXISTS ONLY public.projects DROP CONSTRAINT IF EXISTS projects_owner_user_id_fkey;
ALTER TABLE IF EXISTS ONLY public.memberships DROP CONSTRAINT IF EXISTS memberships_user_id_fkey;
ALTER TABLE IF EXISTS ONLY public.memberships DROP CONSTRAINT IF EXISTS memberships_project_id_fkey;
ALTER TABLE IF EXISTS ONLY public.audit_events DROP CONSTRAINT IF EXISTS audit_events_task_id_fkey;
ALTER TABLE IF EXISTS ONLY public.audit_events DROP CONSTRAINT IF EXISTS audit_events_project_id_fkey;
ALTER TABLE IF EXISTS ONLY public.audit_events DROP CONSTRAINT IF EXISTS audit_events_actor_user_id_fkey;
DROP TRIGGER IF EXISTS users_set_updated_at ON public.users;
DROP TRIGGER IF EXISTS user_quotas_set_updated_at ON public.user_quotas;
DROP TRIGGER IF EXISTS tasks_set_updated_at ON public.tasks;
DROP TRIGGER IF EXISTS projects_set_updated_at ON public.projects;
DROP INDEX IF EXISTS public.users_email_lower_uq;
DROP INDEX IF EXISTS public.user_quotas_set_by_idx;
DROP INDEX IF EXISTS public.tasks_project_summary_idx;
DROP INDEX IF EXISTS public.tasks_project_status_created_idx;
DROP INDEX IF EXISTS public.tasks_global_status_idx;
DROP INDEX IF EXISTS public.tasks_assignee_idx;
DROP INDEX IF EXISTS public.refresh_tokens_user_idx;
DROP INDEX IF EXISTS public.projects_owner_idx;
DROP INDEX IF EXISTS public.outbox_unpublished_idx;
DROP INDEX IF EXISTS public.memberships_user_idx;
DROP INDEX IF EXISTS public.memberships_project_idx;
DROP INDEX IF EXISTS public.audit_events_task_created_idx;
DROP INDEX IF EXISTS public.audit_events_project_created_idx;
DROP INDEX IF EXISTS public.audit_events_actor_idx;
ALTER TABLE IF EXISTS ONLY public.users DROP CONSTRAINT IF EXISTS users_pkey;
ALTER TABLE IF EXISTS ONLY public.user_quotas DROP CONSTRAINT IF EXISTS user_quotas_pkey;
ALTER TABLE IF EXISTS ONLY public.tasks DROP CONSTRAINT IF EXISTS tasks_pkey;
ALTER TABLE IF EXISTS ONLY public.task_tags DROP CONSTRAINT IF EXISTS task_tags_pkey;
ALTER TABLE IF EXISTS ONLY public.tags DROP CONSTRAINT IF EXISTS tags_project_name_uq;
ALTER TABLE IF EXISTS ONLY public.tags DROP CONSTRAINT IF EXISTS tags_pkey;
ALTER TABLE IF EXISTS ONLY public.schema_migrations DROP CONSTRAINT IF EXISTS schema_migrations_pkey;
ALTER TABLE IF EXISTS ONLY public.refresh_tokens DROP CONSTRAINT IF EXISTS refresh_tokens_token_hash_key;
ALTER TABLE IF EXISTS ONLY public.refresh_tokens DROP CONSTRAINT IF EXISTS refresh_tokens_pkey;
ALTER TABLE IF EXISTS ONLY public.projects DROP CONSTRAINT IF EXISTS projects_pkey;
ALTER TABLE IF EXISTS ONLY public.outbox_events DROP CONSTRAINT IF EXISTS outbox_events_pkey;
ALTER TABLE IF EXISTS ONLY public.outbox_events DROP CONSTRAINT IF EXISTS outbox_events_event_id_key;
ALTER TABLE IF EXISTS ONLY public.notification_deliveries DROP CONSTRAINT IF EXISTS notification_deliveries_pkey;
ALTER TABLE IF EXISTS ONLY public.memberships DROP CONSTRAINT IF EXISTS memberships_pkey;
ALTER TABLE IF EXISTS ONLY public.audit_events DROP CONSTRAINT IF EXISTS audit_events_pkey;
DROP TABLE IF EXISTS public.users;
DROP TABLE IF EXISTS public.user_quotas;
DROP TABLE IF EXISTS public.tasks;
DROP TABLE IF EXISTS public.task_tags;
DROP TABLE IF EXISTS public.tags;
DROP TABLE IF EXISTS public.schema_migrations;
DROP TABLE IF EXISTS public.refresh_tokens;
DROP TABLE IF EXISTS public.projects;
DROP TABLE IF EXISTS public.outbox_events;
DROP TABLE IF EXISTS public.notification_deliveries;
DROP TABLE IF EXISTS public.memberships;
DROP TABLE IF EXISTS public.audit_events;
DROP FUNCTION IF EXISTS public.set_updated_at();
--
-- Name: set_updated_at(); Type: FUNCTION; Schema: public; Owner: workhub
--

CREATE FUNCTION public.set_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;


ALTER FUNCTION public.set_updated_at() OWNER TO workhub;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: audit_events; Type: TABLE; Schema: public; Owner: workhub
--

CREATE TABLE public.audit_events (
    id bigint NOT NULL,
    actor_user_id bigint,
    project_id bigint,
    task_id bigint,
    event_type text NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT audit_event_target_exists CHECK (((project_id IS NOT NULL) OR (task_id IS NOT NULL))),
    CONSTRAINT audit_event_type_not_blank CHECK ((btrim(event_type) <> ''::text))
);


ALTER TABLE public.audit_events OWNER TO workhub;

--
-- Name: audit_events_id_seq; Type: SEQUENCE; Schema: public; Owner: workhub
--

ALTER TABLE public.audit_events ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.audit_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: memberships; Type: TABLE; Schema: public; Owner: workhub
--

CREATE TABLE public.memberships (
    project_id bigint NOT NULL,
    user_id bigint NOT NULL,
    role text DEFAULT 'member'::text NOT NULL,
    joined_at timestamp with time zone DEFAULT now() NOT NULL,
    removed_at timestamp with time zone,
    CONSTRAINT memberships_role_valid CHECK ((role = ANY (ARRAY['owner'::text, 'admin'::text, 'member'::text, 'viewer'::text])))
);


ALTER TABLE public.memberships OWNER TO workhub;

--
-- Name: notification_deliveries; Type: TABLE; Schema: public; Owner: workhub
--

CREATE TABLE public.notification_deliveries (
    event_id text NOT NULL,
    delivered_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.notification_deliveries OWNER TO workhub;

--
-- Name: outbox_events; Type: TABLE; Schema: public; Owner: workhub
--

CREATE TABLE public.outbox_events (
    id bigint NOT NULL,
    event_id text NOT NULL,
    event_type text NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id bigint NOT NULL,
    payload jsonb NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    available_at timestamp with time zone DEFAULT now() NOT NULL,
    published_at timestamp with time zone,
    last_error text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT outbox_aggregate_type_not_blank CHECK ((btrim(aggregate_type) <> ''::text)),
    CONSTRAINT outbox_attempts_non_negative CHECK ((attempts >= 0)),
    CONSTRAINT outbox_event_type_not_blank CHECK ((btrim(event_type) <> ''::text))
);


ALTER TABLE public.outbox_events OWNER TO workhub;

--
-- Name: outbox_events_id_seq; Type: SEQUENCE; Schema: public; Owner: workhub
--

ALTER TABLE public.outbox_events ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.outbox_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: projects; Type: TABLE; Schema: public; Owner: workhub
--

CREATE TABLE public.projects (
    id bigint NOT NULL,
    owner_user_id bigint NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    CONSTRAINT projects_name_not_blank CHECK ((btrim(name) <> ''::text))
);


ALTER TABLE public.projects OWNER TO workhub;

--
-- Name: projects_id_seq; Type: SEQUENCE; Schema: public; Owner: workhub
--

ALTER TABLE public.projects ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.projects_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: refresh_tokens; Type: TABLE; Schema: public; Owner: workhub
--

CREATE TABLE public.refresh_tokens (
    id bigint NOT NULL,
    token_hash text NOT NULL,
    user_id bigint NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    replaced_by_token_hash text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT refresh_tokens_hash_not_blank CHECK ((btrim(token_hash) <> ''::text))
);


ALTER TABLE public.refresh_tokens OWNER TO workhub;

--
-- Name: refresh_tokens_id_seq; Type: SEQUENCE; Schema: public; Owner: workhub
--

ALTER TABLE public.refresh_tokens ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.refresh_tokens_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: schema_migrations; Type: TABLE; Schema: public; Owner: workhub
--

CREATE TABLE public.schema_migrations (
    version character varying(255) NOT NULL,
    applied_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.schema_migrations OWNER TO workhub;

--
-- Name: tags; Type: TABLE; Schema: public; Owner: workhub
--

CREATE TABLE public.tags (
    id bigint NOT NULL,
    project_id bigint NOT NULL,
    name text NOT NULL,
    color text DEFAULT '#64748B'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT tags_color_hex CHECK ((color ~ '^#[0-9A-Fa-f]{6}$'::text)),
    CONSTRAINT tags_name_not_blank CHECK ((btrim(name) <> ''::text))
);


ALTER TABLE public.tags OWNER TO workhub;

--
-- Name: tags_id_seq; Type: SEQUENCE; Schema: public; Owner: workhub
--

ALTER TABLE public.tags ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.tags_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: task_tags; Type: TABLE; Schema: public; Owner: workhub
--

CREATE TABLE public.task_tags (
    task_id bigint NOT NULL,
    tag_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.task_tags OWNER TO workhub;

--
-- Name: tasks; Type: TABLE; Schema: public; Owner: workhub
--

CREATE TABLE public.tasks (
    id bigint NOT NULL,
    project_id bigint NOT NULL,
    created_by bigint NOT NULL,
    assignee_id bigint,
    title text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'todo'::text NOT NULL,
    priority smallint DEFAULT 3 NOT NULL,
    due_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    CONSTRAINT tasks_priority_valid CHECK (((priority >= 1) AND (priority <= 5))),
    CONSTRAINT tasks_status_valid CHECK ((status = ANY (ARRAY['todo'::text, 'in_progress'::text, 'done'::text]))),
    CONSTRAINT tasks_title_not_blank CHECK ((btrim(title) <> ''::text))
);


ALTER TABLE public.tasks OWNER TO workhub;

--
-- Name: tasks_id_seq; Type: SEQUENCE; Schema: public; Owner: workhub
--

ALTER TABLE public.tasks ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.tasks_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: user_quotas; Type: TABLE; Schema: public; Owner: workhub
--

CREATE TABLE public.user_quotas (
    user_id bigint NOT NULL,
    max_projects integer,
    max_members_per_project integer,
    max_tasks_per_project integer,
    set_by bigint,
    set_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    reason text,
    CONSTRAINT user_quotas_members_range CHECK (((max_members_per_project IS NULL) OR (max_members_per_project >= '-1'::integer))),
    CONSTRAINT user_quotas_projects_range CHECK (((max_projects IS NULL) OR (max_projects >= '-1'::integer))),
    CONSTRAINT user_quotas_tasks_range CHECK (((max_tasks_per_project IS NULL) OR (max_tasks_per_project >= '-1'::integer)))
);


ALTER TABLE public.user_quotas OWNER TO workhub;

--
-- Name: users; Type: TABLE; Schema: public; Owner: workhub
--

CREATE TABLE public.users (
    id bigint NOT NULL,
    email text NOT NULL,
    display_name text NOT NULL,
    password_hash text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    CONSTRAINT users_display_name_not_blank CHECK ((btrim(display_name) <> ''::text)),
    CONSTRAINT users_email_not_blank CHECK ((btrim(email) <> ''::text)),
    CONSTRAINT users_password_hash_not_blank CHECK ((btrim(password_hash) <> ''::text))
);


ALTER TABLE public.users OWNER TO workhub;

--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: workhub
--

ALTER TABLE public.users ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Data for Name: audit_events; Type: TABLE DATA; Schema: public; Owner: workhub
--

COPY public.audit_events (id, actor_user_id, project_id, task_id, event_type, payload, created_at) FROM stdin;
\.


--
-- Data for Name: memberships; Type: TABLE DATA; Schema: public; Owner: workhub
--

COPY public.memberships (project_id, user_id, role, joined_at, removed_at) FROM stdin;
\.


--
-- Data for Name: notification_deliveries; Type: TABLE DATA; Schema: public; Owner: workhub
--

COPY public.notification_deliveries (event_id, delivered_at) FROM stdin;
\.


--
-- Data for Name: outbox_events; Type: TABLE DATA; Schema: public; Owner: workhub
--

COPY public.outbox_events (id, event_id, event_type, aggregate_type, aggregate_id, payload, attempts, available_at, published_at, last_error, created_at) FROM stdin;
\.


--
-- Data for Name: projects; Type: TABLE DATA; Schema: public; Owner: workhub
--

COPY public.projects (id, owner_user_id, name, description, created_at, updated_at, deleted_at) FROM stdin;
1	1	Day 16 Persistence Drill Project		2026-09-20 12:46:51.932747+00	2026-09-20 12:46:51.932747+00	\N
2	1	Day 18 DR Drill Canary Record - 1789999187		2026-09-21 13:59:47.924667+00	2026-09-21 13:59:47.924667+00	\N
\.


--
-- Data for Name: refresh_tokens; Type: TABLE DATA; Schema: public; Owner: workhub
--

COPY public.refresh_tokens (id, token_hash, user_id, expires_at, revoked_at, replaced_by_token_hash, created_at) FROM stdin;
\.


--
-- Data for Name: schema_migrations; Type: TABLE DATA; Schema: public; Owner: workhub
--

COPY public.schema_migrations (version, applied_at) FROM stdin;
0001_init.up.sql	2026-09-20 12:33:05.508658+00
0002_refresh_tokens.up.sql	2026-09-20 12:33:32.228674+00
0003_outbox.up.sql	2026-09-20 12:33:32.244246+00
0004_membership_roles.up.sql	2026-09-20 12:33:32.260533+00
0005_user_quotas.up.sql	2026-09-20 12:33:32.275025+00
0006_indexes.up.sql	2026-09-20 12:33:32.288236+00
0007_updated_at_triggers.up.sql	2026-09-20 12:33:32.302266+00
\.


--
-- Data for Name: tags; Type: TABLE DATA; Schema: public; Owner: workhub
--

COPY public.tags (id, project_id, name, color, created_at) FROM stdin;
\.


--
-- Data for Name: task_tags; Type: TABLE DATA; Schema: public; Owner: workhub
--

COPY public.task_tags (task_id, tag_id, created_at) FROM stdin;
\.


--
-- Data for Name: tasks; Type: TABLE DATA; Schema: public; Owner: workhub
--

COPY public.tasks (id, project_id, created_by, assignee_id, title, description, status, priority, due_at, created_at, updated_at, deleted_at) FROM stdin;
\.


--
-- Data for Name: user_quotas; Type: TABLE DATA; Schema: public; Owner: workhub
--

COPY public.user_quotas (user_id, max_projects, max_members_per_project, max_tasks_per_project, set_by, set_at, updated_at, reason) FROM stdin;
\.


--
-- Data for Name: users; Type: TABLE DATA; Schema: public; Owner: workhub
--

COPY public.users (id, email, display_name, password_hash, created_at, updated_at, deleted_at) FROM stdin;
1	test@workhub.io	Day16 Test User	hash123	2026-09-20 12:46:45.891434+00	2026-09-20 12:46:45.891434+00	\N
\.


--
-- Name: audit_events_id_seq; Type: SEQUENCE SET; Schema: public; Owner: workhub
--

SELECT pg_catalog.setval('public.audit_events_id_seq', 1, false);


--
-- Name: outbox_events_id_seq; Type: SEQUENCE SET; Schema: public; Owner: workhub
--

SELECT pg_catalog.setval('public.outbox_events_id_seq', 1, false);


--
-- Name: projects_id_seq; Type: SEQUENCE SET; Schema: public; Owner: workhub
--

SELECT pg_catalog.setval('public.projects_id_seq', 2, true);


--
-- Name: refresh_tokens_id_seq; Type: SEQUENCE SET; Schema: public; Owner: workhub
--

SELECT pg_catalog.setval('public.refresh_tokens_id_seq', 1, false);


--
-- Name: tags_id_seq; Type: SEQUENCE SET; Schema: public; Owner: workhub
--

SELECT pg_catalog.setval('public.tags_id_seq', 1, false);


--
-- Name: tasks_id_seq; Type: SEQUENCE SET; Schema: public; Owner: workhub
--

SELECT pg_catalog.setval('public.tasks_id_seq', 1, false);


--
-- Name: users_id_seq; Type: SEQUENCE SET; Schema: public; Owner: workhub
--

SELECT pg_catalog.setval('public.users_id_seq', 1, true);


--
-- Name: audit_events audit_events_pkey; Type: CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_pkey PRIMARY KEY (id);


--
-- Name: memberships memberships_pkey; Type: CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.memberships
    ADD CONSTRAINT memberships_pkey PRIMARY KEY (project_id, user_id);


--
-- Name: notification_deliveries notification_deliveries_pkey; Type: CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.notification_deliveries
    ADD CONSTRAINT notification_deliveries_pkey PRIMARY KEY (event_id);


--
-- Name: outbox_events outbox_events_event_id_key; Type: CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.outbox_events
    ADD CONSTRAINT outbox_events_event_id_key UNIQUE (event_id);


--
-- Name: outbox_events outbox_events_pkey; Type: CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.outbox_events
    ADD CONSTRAINT outbox_events_pkey PRIMARY KEY (id);


--
-- Name: projects projects_pkey; Type: CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.projects
    ADD CONSTRAINT projects_pkey PRIMARY KEY (id);


--
-- Name: refresh_tokens refresh_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.refresh_tokens
    ADD CONSTRAINT refresh_tokens_pkey PRIMARY KEY (id);


--
-- Name: refresh_tokens refresh_tokens_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.refresh_tokens
    ADD CONSTRAINT refresh_tokens_token_hash_key UNIQUE (token_hash);


--
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);


--
-- Name: tags tags_pkey; Type: CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.tags
    ADD CONSTRAINT tags_pkey PRIMARY KEY (id);


--
-- Name: tags tags_project_name_uq; Type: CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.tags
    ADD CONSTRAINT tags_project_name_uq UNIQUE (project_id, name);


--
-- Name: task_tags task_tags_pkey; Type: CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.task_tags
    ADD CONSTRAINT task_tags_pkey PRIMARY KEY (task_id, tag_id);


--
-- Name: tasks tasks_pkey; Type: CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_pkey PRIMARY KEY (id);


--
-- Name: user_quotas user_quotas_pkey; Type: CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.user_quotas
    ADD CONSTRAINT user_quotas_pkey PRIMARY KEY (user_id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: audit_events_actor_idx; Type: INDEX; Schema: public; Owner: workhub
--

CREATE INDEX audit_events_actor_idx ON public.audit_events USING btree (actor_user_id, created_at DESC) WHERE (actor_user_id IS NOT NULL);


--
-- Name: audit_events_project_created_idx; Type: INDEX; Schema: public; Owner: workhub
--

CREATE INDEX audit_events_project_created_idx ON public.audit_events USING btree (project_id, created_at DESC);


--
-- Name: audit_events_task_created_idx; Type: INDEX; Schema: public; Owner: workhub
--

CREATE INDEX audit_events_task_created_idx ON public.audit_events USING btree (task_id, created_at DESC);


--
-- Name: memberships_project_idx; Type: INDEX; Schema: public; Owner: workhub
--

CREATE INDEX memberships_project_idx ON public.memberships USING btree (project_id) WHERE (removed_at IS NULL);


--
-- Name: memberships_user_idx; Type: INDEX; Schema: public; Owner: workhub
--

CREATE INDEX memberships_user_idx ON public.memberships USING btree (user_id) WHERE (removed_at IS NULL);


--
-- Name: outbox_unpublished_idx; Type: INDEX; Schema: public; Owner: workhub
--

CREATE INDEX outbox_unpublished_idx ON public.outbox_events USING btree (available_at, id) WHERE (published_at IS NULL);


--
-- Name: projects_owner_idx; Type: INDEX; Schema: public; Owner: workhub
--

CREATE INDEX projects_owner_idx ON public.projects USING btree (owner_user_id) WHERE (deleted_at IS NULL);


--
-- Name: refresh_tokens_user_idx; Type: INDEX; Schema: public; Owner: workhub
--

CREATE INDEX refresh_tokens_user_idx ON public.refresh_tokens USING btree (user_id);


--
-- Name: tasks_assignee_idx; Type: INDEX; Schema: public; Owner: workhub
--

CREATE INDEX tasks_assignee_idx ON public.tasks USING btree (assignee_id) WHERE (deleted_at IS NULL);


--
-- Name: tasks_global_status_idx; Type: INDEX; Schema: public; Owner: workhub
--

CREATE INDEX tasks_global_status_idx ON public.tasks USING btree (status, created_at DESC) WHERE (deleted_at IS NULL);


--
-- Name: tasks_project_status_created_idx; Type: INDEX; Schema: public; Owner: workhub
--

CREATE INDEX tasks_project_status_created_idx ON public.tasks USING btree (project_id, status, created_at DESC) WHERE (deleted_at IS NULL);


--
-- Name: tasks_project_summary_idx; Type: INDEX; Schema: public; Owner: workhub
--

CREATE INDEX tasks_project_summary_idx ON public.tasks USING btree (project_id) INCLUDE (status) WHERE (deleted_at IS NULL);


--
-- Name: user_quotas_set_by_idx; Type: INDEX; Schema: public; Owner: workhub
--

CREATE INDEX user_quotas_set_by_idx ON public.user_quotas USING btree (set_by) WHERE (set_by IS NOT NULL);


--
-- Name: users_email_lower_uq; Type: INDEX; Schema: public; Owner: workhub
--

CREATE UNIQUE INDEX users_email_lower_uq ON public.users USING btree (lower(email)) WHERE (deleted_at IS NULL);


--
-- Name: projects projects_set_updated_at; Type: TRIGGER; Schema: public; Owner: workhub
--

CREATE TRIGGER projects_set_updated_at BEFORE UPDATE ON public.projects FOR EACH ROW EXECUTE FUNCTION public.set_updated_at();


--
-- Name: tasks tasks_set_updated_at; Type: TRIGGER; Schema: public; Owner: workhub
--

CREATE TRIGGER tasks_set_updated_at BEFORE UPDATE ON public.tasks FOR EACH ROW EXECUTE FUNCTION public.set_updated_at();


--
-- Name: user_quotas user_quotas_set_updated_at; Type: TRIGGER; Schema: public; Owner: workhub
--

CREATE TRIGGER user_quotas_set_updated_at BEFORE UPDATE ON public.user_quotas FOR EACH ROW EXECUTE FUNCTION public.set_updated_at();


--
-- Name: users users_set_updated_at; Type: TRIGGER; Schema: public; Owner: workhub
--

CREATE TRIGGER users_set_updated_at BEFORE UPDATE ON public.users FOR EACH ROW EXECUTE FUNCTION public.set_updated_at();


--
-- Name: audit_events audit_events_actor_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: audit_events audit_events_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: audit_events audit_events_task_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_task_id_fkey FOREIGN KEY (task_id) REFERENCES public.tasks(id) ON DELETE CASCADE;


--
-- Name: memberships memberships_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.memberships
    ADD CONSTRAINT memberships_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: memberships memberships_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.memberships
    ADD CONSTRAINT memberships_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: projects projects_owner_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.projects
    ADD CONSTRAINT projects_owner_user_id_fkey FOREIGN KEY (owner_user_id) REFERENCES public.users(id) ON DELETE RESTRICT;


--
-- Name: refresh_tokens refresh_tokens_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.refresh_tokens
    ADD CONSTRAINT refresh_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: tags tags_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.tags
    ADD CONSTRAINT tags_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: task_tags task_tags_tag_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.task_tags
    ADD CONSTRAINT task_tags_tag_id_fkey FOREIGN KEY (tag_id) REFERENCES public.tags(id) ON DELETE CASCADE;


--
-- Name: task_tags task_tags_task_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.task_tags
    ADD CONSTRAINT task_tags_task_id_fkey FOREIGN KEY (task_id) REFERENCES public.tasks(id) ON DELETE CASCADE;


--
-- Name: tasks tasks_assignee_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_assignee_id_fkey FOREIGN KEY (assignee_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: tasks tasks_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE RESTRICT;


--
-- Name: tasks tasks_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: user_quotas user_quotas_set_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.user_quotas
    ADD CONSTRAINT user_quotas_set_by_fkey FOREIGN KEY (set_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: user_quotas user_quotas_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: workhub
--

ALTER TABLE ONLY public.user_quotas
    ADD CONSTRAINT user_quotas_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--

\unrestrict dYsQjbud020RqvQxiAljMmQuYcfnvaAa2W26JG9YelylhkFDljmCI1fDFJ9IAXm

