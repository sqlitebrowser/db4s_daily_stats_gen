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

DROP TABLE public.db4s_downloads_daily CASCADE;
DROP TABLE public.db4s_downloads_weekly CASCADE;
DROP TABLE public.db4s_downloads_monthly CASCADE;
DROP TABLE public.db4s_users_daily CASCADE;
DROP TABLE public.db4s_users_weekly CASCADE;
DROP TABLE public.db4s_users_monthly CASCADE;
DROP SEQUENCE public.db4s_release_info_release_id_seq CASCADE;
DROP SEQUENCE public.db4s_users_daily_daily_id_seq CASCADE;
DROP SEQUENCE public.db4s_users_weekly_weekly_id_seq CASCADE;
DROP SEQUENCE public.db4s_users_monthly_monthly_id_seq CASCADE;
DROP TABLE public.db4s_release_info CASCADE;
DROP TABLE public.db4s_download_info CASCADE;

--
-- Name: db4s_download_info; Type: TABLE; Schema: public; Owner: db4s
--

CREATE TABLE public.db4s_download_info (
    download_id integer NOT NULL,
    friendly_name text
);


ALTER TABLE public.db4s_download_info OWNER TO db4s;


--
-- Name: db4s_download_info_download_id_seq; Type: SEQUENCE; Schema: public; Owner: db4s
--

CREATE SEQUENCE public.db4s_download_info_download_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.db4s_download_info_download_id_seq OWNER TO db4s;

--
-- Name: db4s_download_info_download_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: db4s
--

ALTER SEQUENCE public.db4s_download_info_download_id_seq OWNED BY public.db4s_download_info.download_id;


--
-- Name: db4s_downloads_daily; Type: TABLE; Schema: public; Owner: db4s
--

CREATE TABLE public.db4s_downloads_daily (
    daily_id integer NOT NULL,
    stats_date timestamp without time zone,
    db4s_download integer,
    num_downloads integer
);


ALTER TABLE public.db4s_downloads_daily OWNER TO db4s;

--
-- Name: db4s_downloads_daily_daily_id_seq; Type: SEQUENCE; Schema: public; Owner: db4s
--

CREATE SEQUENCE public.db4s_downloads_daily_daily_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.db4s_downloads_daily_daily_id_seq OWNER TO db4s;

--
-- Name: db4s_downloads_daily_daily_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: db4s
--

ALTER SEQUENCE public.db4s_downloads_daily_daily_id_seq OWNED BY public.db4s_downloads_daily.daily_id;


--
-- Name: db4s_downloads_monthly; Type: TABLE; Schema: public; Owner: db4s
--

CREATE TABLE public.db4s_downloads_monthly (
    monthly_id integer NOT NULL,
    stats_date timestamp without time zone,
    db4s_download integer,
    num_downloads integer
);


ALTER TABLE public.db4s_downloads_monthly OWNER TO db4s;

--
-- Name: db4s_downloads_monthly_monthly_id_seq; Type: SEQUENCE; Schema: public; Owner: db4s
--

CREATE SEQUENCE public.db4s_downloads_monthly_monthly_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.db4s_downloads_monthly_monthly_id_seq OWNER TO db4s;

--
-- Name: db4s_downloads_monthly_monthly_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: db4s
--

ALTER SEQUENCE public.db4s_downloads_monthly_monthly_id_seq OWNED BY public.db4s_downloads_monthly.monthly_id;


--
-- Name: db4s_downloads_weekly; Type: TABLE; Schema: public; Owner: db4s
--

CREATE TABLE public.db4s_downloads_weekly (
    weekly_id integer NOT NULL,
    stats_date timestamp without time zone,
    db4s_download integer,
    num_downloads integer
);


ALTER TABLE public.db4s_downloads_weekly OWNER TO db4s;

--
-- Name: db4s_downloads_weekly_weekly_id_seq; Type: SEQUENCE; Schema: public; Owner: db4s
--

CREATE SEQUENCE public.db4s_downloads_weekly_weekly_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.db4s_downloads_weekly_weekly_id_seq OWNER TO db4s;

--
-- Name: db4s_downloads_weekly_weekly_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: db4s
--

ALTER SEQUENCE public.db4s_downloads_weekly_weekly_id_seq OWNED BY public.db4s_downloads_weekly.weekly_id;


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
-- Name: db4s_download_info download_id; Type: DEFAULT; Schema: public; Owner: db4s
--

ALTER TABLE ONLY public.db4s_download_info ALTER COLUMN download_id SET DEFAULT nextval('public.db4s_download_info_download_id_seq'::regclass);


--
-- Name: db4s_downloads_daily daily_id; Type: DEFAULT; Schema: public; Owner: db4s
--

ALTER TABLE ONLY public.db4s_downloads_daily ALTER COLUMN daily_id SET DEFAULT nextval('public.db4s_downloads_daily_daily_id_seq'::regclass);


--
-- Name: db4s_downloads_monthly monthly_id; Type: DEFAULT; Schema: public; Owner: db4s
--

ALTER TABLE ONLY public.db4s_downloads_monthly ALTER COLUMN monthly_id SET DEFAULT nextval('public.db4s_downloads_monthly_monthly_id_seq'::regclass);


--
-- Name: db4s_downloads_weekly weekly_id; Type: DEFAULT; Schema: public; Owner: db4s
--

ALTER TABLE ONLY public.db4s_downloads_weekly ALTER COLUMN weekly_id SET DEFAULT nextval('public.db4s_downloads_weekly_weekly_id_seq'::regclass);


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
-- Data for Name: db4s_download_info; Type: TABLE DATA; Schema: public; Owner: db4s
--

COPY public.db4s_download_info (download_id, friendly_name) FROM stdin;
1	DB4S 3.10.1 macOS
2	DB4S 3.10.1 win32
3	DB4S 3.10.1 win64
4	DB4S 3.10.1 Portable
0	Total downloads
\.


--
-- Data for Name: db4s_downloads_daily; Type: TABLE DATA; Schema: public; Owner: db4s
--

COPY public.db4s_downloads_daily (daily_id, stats_date, db4s_download, num_downloads) FROM stdin;
\.


--
-- Data for Name: db4s_downloads_monthly; Type: TABLE DATA; Schema: public; Owner: db4s
--

COPY public.db4s_downloads_monthly (monthly_id, stats_date, db4s_download, num_downloads) FROM stdin;
\.


--
-- Data for Name: db4s_downloads_weekly; Type: TABLE DATA; Schema: public; Owner: db4s
--

COPY public.db4s_downloads_weekly (weekly_id, stats_date, db4s_download, num_downloads) FROM stdin;
\.


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
-- Name: db4s_download_info_download_id_seq; Type: SEQUENCE SET; Schema: public; Owner: db4s
--

SELECT pg_catalog.setval('public.db4s_download_info_download_id_seq', 5, true);


--
-- Name: db4s_downloads_daily_daily_id_seq; Type: SEQUENCE SET; Schema: public; Owner: db4s
--

SELECT pg_catalog.setval('public.db4s_downloads_daily_daily_id_seq', 1, true);


--
-- Name: db4s_downloads_monthly_monthly_id_seq; Type: SEQUENCE SET; Schema: public; Owner: db4s
--

SELECT pg_catalog.setval('public.db4s_downloads_monthly_monthly_id_seq', 1, true);


--
-- Name: db4s_downloads_weekly_weekly_id_seq; Type: SEQUENCE SET; Schema: public; Owner: db4s
--

SELECT pg_catalog.setval('public.db4s_downloads_weekly_weekly_id_seq', 1, true);


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
-- Name: db4s_download_info db4s_download_info_pk; Type: CONSTRAINT; Schema: public; Owner: db4s
--

ALTER TABLE ONLY public.db4s_download_info
    ADD CONSTRAINT db4s_download_info_pk PRIMARY KEY (download_id);


--
-- Name: db4s_downloads_daily db4s_downloads_daily_pk; Type: CONSTRAINT; Schema: public; Owner: db4s
--

ALTER TABLE ONLY public.db4s_downloads_daily
    ADD CONSTRAINT db4s_downloads_daily_pk PRIMARY KEY (daily_id);


--
-- Name: db4s_downloads_monthly db4s_downloads_monthly_pk; Type: CONSTRAINT; Schema: public; Owner: db4s
--

ALTER TABLE ONLY public.db4s_downloads_monthly
    ADD CONSTRAINT db4s_downloads_monthly_pk PRIMARY KEY (monthly_id);


--
-- Name: db4s_downloads_weekly db4s_downloads_weekly_pk; Type: CONSTRAINT; Schema: public; Owner: db4s
--

ALTER TABLE ONLY public.db4s_downloads_weekly
    ADD CONSTRAINT db4s_downloads_weekly_pk PRIMARY KEY (weekly_id);


--
-- Name: db4s_downloads_daily_stats_date_db4s_download_uindex; Type: INDEX; Schema: public; Owner: db4s
--

CREATE UNIQUE INDEX db4s_downloads_daily_stats_date_db4s_download_uindex ON public.db4s_downloads_daily USING btree (stats_date, db4s_download);


--
-- Name: db4s_downloads_monthly_stats_date_db4s_download_uindex; Type: INDEX; Schema: public; Owner: db4s
--

CREATE UNIQUE INDEX db4s_downloads_monthly_stats_date_db4s_download_uindex ON public.db4s_downloads_monthly USING btree (stats_date, db4s_download);


--
-- Name: db4s_downloads_weekly_stats_date_db4s_download_uindex; Type: INDEX; Schema: public; Owner: db4s
--

CREATE UNIQUE INDEX db4s_downloads_weekly_stats_date_db4s_download_uindex ON public.db4s_downloads_weekly USING btree (stats_date, db4s_download);


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
-- Name: db4s_downloads_daily db4s_downloads_daily_db4s_download_info_download_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: db4s
--

ALTER TABLE ONLY public.db4s_downloads_daily
    ADD CONSTRAINT db4s_downloads_daily_db4s_download_info_download_id_fk FOREIGN KEY (db4s_download) REFERENCES public.db4s_download_info(download_id) ON UPDATE CASCADE ON DELETE SET NULL;


--
-- Name: db4s_downloads_monthly db4s_downloads_monthly_db4s_download_info_download_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: db4s
--

ALTER TABLE ONLY public.db4s_downloads_monthly
    ADD CONSTRAINT db4s_downloads_monthly_db4s_download_info_download_id_fk FOREIGN KEY (db4s_download) REFERENCES public.db4s_download_info(download_id) ON UPDATE CASCADE ON DELETE SET NULL;


--
-- Name: db4s_downloads_weekly db4s_downloads_weekly_db4s_download_info_download_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: db4s
--

ALTER TABLE ONLY public.db4s_downloads_weekly
    ADD CONSTRAINT db4s_downloads_weekly_db4s_download_info_download_id_fk FOREIGN KEY (db4s_download) REFERENCES public.db4s_download_info(download_id) ON UPDATE CASCADE ON DELETE SET NULL;


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

