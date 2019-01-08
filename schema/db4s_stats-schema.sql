--
-- PostgreSQL database dump
--

-- Dumped from database version 10.6
-- Dumped by pg_dump version 10.6

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET client_min_messages = warning;
SET row_security = off;


SET default_tablespace = '';

SET default_with_oids = false;

---

DROP TABLE public.db4s_users_daily CASCADE;
DROP TABLE public.db4s_users_weekly CASCADE;
DROP TABLE public.db4s_users_monthly CASCADE;
DROP SEQUENCE public.db4s_release_info_release_id_seq CASCADE;
DROP SEQUENCE public.db4s_users_daily_daily_id_seq CASCADE;
DROP SEQUENCE public.db4s_users_weekly_weekly_id_seq CASCADE;
DROP SEQUENCE public.db4s_users_monthly_monthly_id_seq CASCADE;
DROP TABLE public.db4s_release_info CASCADE;


--
-- Name: db4s_release_info; Type: TABLE; Schema: public; Owner: db4s
--

CREATE TABLE public.db4s_release_info (
    release_id integer NOT NULL,
    version_number text,
    friendly_name text
);


ALTER TABLE public.db4s_release_info OWNER TO db4s;

--
-- Name: db4s_release_info_release_id_seq; Type: SEQUENCE; Schema: public; Owner: db4s
--

CREATE SEQUENCE public.db4s_release_info_release_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.db4s_release_info_release_id_seq OWNER TO db4s;

--
-- Name: db4s_release_info_release_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: db4s
--

ALTER SEQUENCE public.db4s_release_info_release_id_seq OWNED BY public.db4s_release_info.release_id;


--
-- Name: db4s_users_daily; Type: TABLE; Schema: public; Owner: db4s
--

CREATE TABLE public.db4s_users_daily (
    daily_id integer NOT NULL,
    stats_date timestamp without time zone,
    db4s_release integer,
    unique_ips integer
);


ALTER TABLE public.db4s_users_daily OWNER TO db4s;

--
-- Name: db4s_users_daily_daily_id_seq; Type: SEQUENCE; Schema: public; Owner: db4s
--

CREATE SEQUENCE public.db4s_users_daily_daily_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.db4s_users_daily_daily_id_seq OWNER TO db4s;

--
-- Name: db4s_users_daily_daily_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: db4s
--

ALTER SEQUENCE public.db4s_users_daily_daily_id_seq OWNED BY public.db4s_users_daily.daily_id;


--
-- Name: db4s_users_monthly; Type: TABLE; Schema: public; Owner: db4s
--

CREATE TABLE public.db4s_users_monthly (
    monthly_id integer NOT NULL,
    stats_date timestamp without time zone,
    db4s_release integer,
    unique_ips integer
);


ALTER TABLE public.db4s_users_monthly OWNER TO db4s;

--
-- Name: db4s_users_monthly_monthly_id_seq; Type: SEQUENCE; Schema: public; Owner: db4s
--

CREATE SEQUENCE public.db4s_users_monthly_monthly_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.db4s_users_monthly_monthly_id_seq OWNER TO db4s;

--
-- Name: db4s_users_monthly_monthly_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: db4s
--

ALTER SEQUENCE public.db4s_users_monthly_monthly_id_seq OWNED BY public.db4s_users_monthly.monthly_id;


--
-- Name: db4s_users_weekly; Type: TABLE; Schema: public; Owner: db4s
--

CREATE TABLE public.db4s_users_weekly (
    weekly_id integer NOT NULL,
    stats_date timestamp without time zone,
    db4s_release integer,
    unique_ips integer
);


ALTER TABLE public.db4s_users_weekly OWNER TO db4s;

--
-- Name: db4s_users_weekly_weekly_id_seq; Type: SEQUENCE; Schema: public; Owner: db4s
--

CREATE SEQUENCE public.db4s_users_weekly_weekly_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.db4s_users_weekly_weekly_id_seq OWNER TO db4s;

--
-- Name: db4s_users_weekly_weekly_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: db4s
--

ALTER SEQUENCE public.db4s_users_weekly_weekly_id_seq OWNED BY public.db4s_users_weekly.weekly_id;


--
-- Name: db4s_release_info release_id; Type: DEFAULT; Schema: public; Owner: db4s
--

ALTER TABLE ONLY public.db4s_release_info ALTER COLUMN release_id SET DEFAULT nextval('public.db4s_release_info_release_id_seq'::regclass);


--
-- Name: db4s_users_daily daily_id; Type: DEFAULT; Schema: public; Owner: db4s
--

ALTER TABLE ONLY public.db4s_users_daily ALTER COLUMN daily_id SET DEFAULT nextval('public.db4s_users_daily_daily_id_seq'::regclass);


--
-- Name: db4s_users_monthly monthly_id; Type: DEFAULT; Schema: public; Owner: db4s
--

ALTER TABLE ONLY public.db4s_users_monthly ALTER COLUMN monthly_id SET DEFAULT nextval('public.db4s_users_monthly_monthly_id_seq'::regclass);


--
-- Name: db4s_users_weekly weekly_id; Type: DEFAULT; Schema: public; Owner: db4s
--

ALTER TABLE ONLY public.db4s_users_weekly ALTER COLUMN weekly_id SET DEFAULT nextval('public.db4s_users_weekly_weekly_id_seq'::regclass);


--
-- Data for Name: db4s_release_info; Type: TABLE DATA; Schema: public; Owner: db4s
--

COPY public.db4s_release_info (release_id, version_number, friendly_name) FROM stdin;
3	3.10.100	3.11.0-alpha1
4	3.10.200	3.11.0-beta1
5	3.10.201	3.11.0-beta2
6	3.10.202	3.11.0-beta3
2	3.10.99	3.10-nightlies
7	3.11.99	3.11-nightlies
1	Unique IPs	Unique IPs
\.


--
-- Data for Name: db4s_users_daily; Type: TABLE DATA; Schema: public; Owner: db4s
--

COPY public.db4s_users_daily (daily_id, stats_date, db4s_release, unique_ips) FROM stdin;
\.


--
-- Data for Name: db4s_users_monthly; Type: TABLE DATA; Schema: public; Owner: db4s
--

COPY public.db4s_users_monthly (monthly_id, stats_date, db4s_release, unique_ips) FROM stdin;
\.


--
-- Data for Name: db4s_users_weekly; Type: TABLE DATA; Schema: public; Owner: db4s
--

COPY public.db4s_users_weekly (weekly_id, stats_date, db4s_release, unique_ips) FROM stdin;
\.


--
-- Name: db4s_release_info_release_id_seq; Type: SEQUENCE SET; Schema: public; Owner: db4s
--

SELECT pg_catalog.setval('public.db4s_release_info_release_id_seq', 8, true);


--
-- Name: db4s_users_daily_daily_id_seq; Type: SEQUENCE SET; Schema: public; Owner: db4s
--

SELECT pg_catalog.setval('public.db4s_users_daily_daily_id_seq', 1, true);


--
-- Name: db4s_users_monthly_monthly_id_seq; Type: SEQUENCE SET; Schema: public; Owner: db4s
--

SELECT pg_catalog.setval('public.db4s_users_monthly_monthly_id_seq', 1, true);


--
-- Name: db4s_users_weekly_weekly_id_seq; Type: SEQUENCE SET; Schema: public; Owner: db4s
--

SELECT pg_catalog.setval('public.db4s_users_weekly_weekly_id_seq', 1, true);


--
-- Name: db4s_release_info_release_id_uindex; Type: INDEX; Schema: public; Owner: db4s
--

CREATE UNIQUE INDEX db4s_release_info_release_id_uindex ON public.db4s_release_info USING btree (release_id);


--
-- Name: db4s_release_info_version_number_uindex; Type: INDEX; Schema: public; Owner: db4s
--

CREATE UNIQUE INDEX db4s_release_info_version_number_uindex ON public.db4s_release_info USING btree (version_number);


--
-- Name: db4s_users_daily_stats_date_db4s_release_uindex; Type: INDEX; Schema: public; Owner: db4s
--

CREATE UNIQUE INDEX db4s_users_daily_stats_date_db4s_release_uindex ON public.db4s_users_daily USING btree (stats_date, db4s_release);


--
-- Name: db4s_users_monthly_stats_date_db4s_release_uindex; Type: INDEX; Schema: public; Owner: db4s
--

CREATE UNIQUE INDEX db4s_users_monthly_stats_date_db4s_release_uindex ON public.db4s_users_monthly USING btree (stats_date, db4s_release);


--
-- Name: db4s_users_weekly_stats_date_db4s_release_uindex; Type: INDEX; Schema: public; Owner: db4s
--

CREATE UNIQUE INDEX db4s_users_weekly_stats_date_db4s_release_uindex ON public.db4s_users_weekly USING btree (stats_date, db4s_release);


--
-- Name: db4s_users_daily db4s_users_daily_db4s_release_info_release_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: db4s
--

ALTER TABLE ONLY public.db4s_users_daily
    ADD CONSTRAINT db4s_users_daily_db4s_release_info_release_id_fk FOREIGN KEY (db4s_release) REFERENCES public.db4s_release_info(release_id) ON UPDATE CASCADE ON DELETE SET NULL;


--
-- Name: db4s_users_monthly db4s_users_monthly_db4s_release_info_release_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: db4s
--

ALTER TABLE ONLY public.db4s_users_monthly
    ADD CONSTRAINT db4s_users_monthly_db4s_release_info_release_id_fk FOREIGN KEY (db4s_release) REFERENCES public.db4s_release_info(release_id) ON UPDATE CASCADE ON DELETE SET NULL;


--
-- Name: db4s_users_weekly db4s_users_weekly_db4s_release_info_release_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: db4s
--

ALTER TABLE ONLY public.db4s_users_weekly
    ADD CONSTRAINT db4s_users_weekly_db4s_release_info_release_id_fk FOREIGN KEY (db4s_release) REFERENCES public.db4s_release_info(release_id) ON UPDATE CASCADE ON DELETE SET NULL;


--
-- PostgreSQL database dump complete
--

