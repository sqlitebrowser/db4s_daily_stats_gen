package main

// This generates basic status to answer two questions:
//
//   1) How many DB4S clients (unique ip address) are checking the '/currentrelease' version each day?
//
//      This should give us a rough idea of the size of our active userbase
//
//   2) How many of each version of DB4S are checking the '/currentrelease' version each day?
//
//      This should give us a rough idea of the mix of versions being used

// NOTE: While much of this processing could instead be done using SQL directly in PG, it's not worth the time for
//       me to learn/refresh my knowledge of the appropriate PG bits at this point.  So, just going to do it using
//       Go instead, even though it's less efficient processing-wise. ;)

import (
	"context"
	"crypto/md5"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/jackc/pgx"
	"github.com/jackc/pgx/pgtype"
	"github.com/minio/go-homedir"
	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/config"
)

// Configuration file
type TomlConfig struct {
	Jaeger JaegerInfo
	Pg     PGInfo
}
type JaegerInfo struct {
	CollectorEndPoint string
}
type PGInfo struct {
	Database       string
	NumConnections int `toml:"num_connections"`
	Port           int
	Password       string
	Server         string
	SSL            bool
	Username       string
}

var (
	// Application config
	Conf TomlConfig

	// Toggle for display of debugging info
	debug = true

	// PostgreSQL Connection pool
	pg *pgx.ConnPool

	// Use Jaeger?
	enableJaeger = false
)

func main() {
	// Override config file location via environment variables
	var err error
	var configFile string
	tempString := os.Getenv("CONFIG_FILE")
	if tempString == "" {
		// TODO: Might be a good idea to add permission checks of the dir & conf file, to ensure they're not
		//       world readable.  Similar in concept to what ssh does for its config files.
		userHome, err := homedir.Dir()
		if err != nil {
			log.Fatalf("User home directory couldn't be determined: %s", "\n")
		}
		configFile = filepath.Join(userHome, ".db4s", "daily_stats_gen.toml")
	} else {
		configFile = tempString
	}

	// Read our configuration settings
	if _, err = toml.DecodeFile(configFile, &Conf); err != nil {
		log.Fatal(err)
	}

	// Set up initial Jaeger service and span
	tracer, closer := initJaeger("db4s stats generator")
	defer closer.Close()
	opentracing.SetGlobalTracer(tracer)

	// * Connect to PG database *
	pgSpan := tracer.StartSpan("connect postgres")

	// Setup the PostgreSQL config
	pgConfig := new(pgx.ConnConfig)
	pgConfig.Host = Conf.Pg.Server
	pgConfig.Port = uint16(Conf.Pg.Port)
	pgConfig.User = Conf.Pg.Username
	pgConfig.Password = Conf.Pg.Password
	pgConfig.Database = Conf.Pg.Database
	clientTLSConfig := tls.Config{InsecureSkipVerify: true}
	if Conf.Pg.SSL {
		// TODO: Likely need to add the PG TLS cert file info here
		pgConfig.TLSConfig = &clientTLSConfig
	} else {
		pgConfig.TLSConfig = nil
	}

	// Connect to PG
	pgPoolConfig := pgx.ConnPoolConfig{*pgConfig, Conf.Pg.NumConnections, nil, 5 * time.Second}
	pg, err = pgx.NewConnPool(pgPoolConfig)
	if err != nil {
		log.Fatal(err)
	}

	// Log successful connection
	log.Printf("Connected to PostgreSQL server: %v:%v\n", Conf.Pg.Server, uint16(Conf.Pg.Port))
	pgSpan.Finish()

	// Add any new user agents to the db4s_release_info table
	uaSpan := tracer.StartSpan("update user agents")
	uaCtx := opentracing.ContextWithSpan(context.Background(), uaSpan)
	err = updateUserAgents(uaCtx)
	if err != nil {
		log.Fatalf(err.Error())
	}
	uaSpan.Finish()

	// * Daily users *

	// The earliest date with entries is 2018-08-13, so we start with that.  We repeatedly call the function for
	// getting IP addresses, incrementing the date each time until we exceed time.Now()
	startDate := time.Date(2018, 8, 13, 0, 0, 0, 0, time.UTC)
	endDate := startDate.Add(time.Hour * 24)
	dailySpan := tracer.StartSpan("calculate daily users")
	for endDate.Before(time.Now()) {
		numIPs, IPsPerUserAgent, err := getIPs(startDate, endDate)
		err = saveDailyStats(startDate, numIPs, IPsPerUserAgent)
		if err != nil {
			log.Fatalf(err.Error())
		}

		// Display debug info if appropriate
		if debug {
			log.Printf("Unique IP addresses for %v: %v\n", startDate.Format("2006 Jan 2"), numIPs)
		}

		startDate = startDate.AddDate(0, 0, 1)
		endDate = startDate.AddDate(0, 0, 1)
	}
	dailySpan.Finish()

	// * Weekly users *
	// TODO: Determine the "week of year" for 2018-08-13, and use that as the starting date for weekly stats instead
	startDate = time.Date(2018, 8, 7, 0, 0, 0, 0, time.UTC)
	endDate = startDate.AddDate(0, 0, 7)
	wkSpan := tracer.StartSpan("calculate weekly users")
	for endDate.Before(time.Now()) {
		numIPs, IPsPerUserAgent, err := getIPs(startDate, endDate)
		err = saveWeeklyStats(startDate, numIPs, IPsPerUserAgent)
		if err != nil {
			log.Fatalf(err.Error())
		}

		// Display debug info if appropriate
		if debug {
			yr, wk := startDate.ISOWeek()
			log.Printf("Unique IP addresses for week %v, %v: %v\n", yr, wk, numIPs)
		}

		startDate = startDate.AddDate(0, 0, 7)
		endDate = startDate.AddDate(0, 0, 7)
	}
	wkSpan.Finish()

	// * Monthly users *
	startDate = time.Date(2018, 8, 1, 0, 0, 0, 0, time.UTC)
	endDate = startDate.AddDate(0, 1, 0)
	mthSpan := tracer.StartSpan("calculate monthly users")
	for endDate.Before(time.Now().AddDate(0, 1, 0)) {
		numIPs, IPsPerUserAgent, err := getIPs(startDate, endDate)
		err = saveMonthlyStats(startDate, numIPs, IPsPerUserAgent)
		if err != nil {
			log.Fatalf(err.Error())
		}

		// Display debug info if appropriate
		if debug {
			log.Printf("Unique IP addresses for month %v: %v\n", startDate.Format("2006 Jan"), numIPs)
		}

		startDate = startDate.AddDate(0, 1, 0)
		endDate = startDate.AddDate(0, 1, 0)
	}
	mthSpan.Finish()

	// Close the PG connection gracefully
	pg.Close()
	log.Println("Done")
}

// getIPs() returns the number of DB4S instances doing a version check in the given date range, plus a count of the
// quantity per DB4S version
func getIPs(startDate time.Time, endDate time.Time) (IPs int, userAgentIPs map[string]int, err error) {
	// This nested map approach (inside of a combined key) should allow for counting the # of unique IP's per user agent
	IPsPerUserAgent := make(map[string]map[[16]byte]int)

	// Retrieve entire result set of valid `/currentrelease` requests for the desired time range
	uniqueIPs := make(map[[16]byte]int)
	dbQuery := `
		SELECT http_user_agent, client_ipv4, client_ipv6, client_ip_strange
		FROM download_log
		WHERE request = '/currentrelease'
			AND http_user_agent LIKE 'sqlitebrowser %' AND http_user_agent NOT LIKE '%AppEngine%'
			AND request_time > $1
			AND request_time < $2`
	rows, err := pg.Query(dbQuery, &startDate, &endDate)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return
	}
	defer rows.Close()
	rowCount := 0
	for rows.Next() {
		rowCount++
		var userAgent pgtype.Text
		var IPv4, IPv6, IPStrange pgtype.Text
		err = rows.Scan(&userAgent, &IPv4, &IPv6, &IPStrange)
		if err != nil {
			log.Printf("Error retrieving rows: %v\n", err)
			return
		}

		// Work out the key to use.  We use a hash of the IP address, to stop weird characters in the IP Strange field
		// being a problem
		var IPHash [16]byte
		if IPStrange.Status == pgtype.Present {
			IPHash = md5.Sum([]byte(IPStrange.String))
		} else if IPv6.Status == pgtype.Present {
			IPHash = md5.Sum([]byte(IPv6.String))
		} else if IPv4.Status == pgtype.Present {
			IPHash = md5.Sum([]byte(IPv4.String))
		} else {
			// This shouldn't happen, but check for it just in case
			log.Fatalf("Doesn't seem to be any non-NULL client IP field for one of the rows")
		}

		// Update the unique IP address counter as appropriate
		uniqueIPs[IPHash]++

		// Increment the counter for the user agent + IP address combination
		ipMap, ok := IPsPerUserAgent[userAgent.String]
		if !ok {
			ipMap = make(map[[16]byte]int)
			IPsPerUserAgent[userAgent.String] = ipMap
		}
		ipMap[IPHash]++
	}

	// Unique IP addresses
	IPs = len(uniqueIPs)

	// Number of unique IP addresses per user agent
	userAgentIPs = make(map[string]int)
	for i, j := range IPsPerUserAgent {
		userAgentIPs[i] = len(j)
	}

	return
}

// initJaeger returns an instance of Jaeger Tracer
func initJaeger(service string) (opentracing.Tracer, io.Closer) {
	samplerConst := 1.0
	if !enableJaeger {
		samplerConst = 0.0
	}
	cfg := &config.Configuration{
		ServiceName: service,
		Sampler: &config.SamplerConfig{
			Type:  "const",
			Param: samplerConst,
		},
		Reporter: &config.ReporterConfig{
			CollectorEndpoint: Conf.Jaeger.CollectorEndPoint,
		},
	}
	tracer, closer, err := cfg.NewTracer(config.Logger(jaeger.StdLogger))
	if err != nil {
		panic(fmt.Sprintf("ERROR: cannot init Jaeger: %v\n", err))
	}
	return tracer, closer
}

// saveDailyStats() inserts new or updated daily stats counts into the db4s_stats_daily table
func saveDailyStats(date time.Time, count int, IPsPerUserAgent map[string]int) error {
	// Update the non-version-specific daily stats
	// NOTE - The hard coded 1 value for the release version corresponds to the manually added "Unique IPs" entry in
	// the release version table
	dbQuery := `
		INSERT INTO db4s_stats_daily (stats_date, db4s_release, unique_ips)
		VALUES ($1, 1, $2)
		ON CONFLICT (stats_date, db4s_release)
			DO UPDATE
				SET unique_ips = $2
				WHERE db4s_stats_daily.stats_date = $1
					AND db4s_stats_daily.db4s_release IS NULL`
	commandTag, err := pg.Exec(dbQuery, date, count)
	if err != nil {
		// For now, don't bother logging a failure here.  This *might* need changing later on
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows > 1 {
		log.Printf("Wrong number of rows (%v) affected when adding a daily stats row: %v\n", numRows, date)
	}

	// Update the version-specific daily stats
	for i, verCount := range IPsPerUserAgent {
		// Strip the leading 'sqlitebrowser ' string from the version number
		versionString := strings.TrimPrefix(i, "sqlitebrowser ")
		dbQuery = `
		WITH ver AS (
			SELECT release_id
			FROM db4s_release_info
			WHERE version_number = $2
		)
		INSERT INTO db4s_stats_daily (stats_date, db4s_release, unique_ips)
		SELECT $1, (SELECT release_id FROM ver), $3
		ON CONFLICT (stats_date, db4s_release)
			DO UPDATE
				SET unique_ips = $3
				WHERE db4s_stats_daily.stats_date = $1
					AND db4s_stats_daily.db4s_release = (SELECT release_id FROM ver)`
		commandTag, err := pg.Exec(dbQuery, date, versionString, verCount)
		if err != nil {
			// For now, don't bother logging a failure here.  This *might* need changing later on
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows > 1 {
			log.Printf("Wrong number of rows (%v) affected when adding a daily stats row: %v\n", numRows, date)
		}
	}
	return nil
}

// saveMonthlyStats() inserts new or updated weekly stats counts into the db4s_stats_monthly table
func saveMonthlyStats(date time.Time, count int, IPsPerUserAgent map[string]int) error {
	// Update the non-version-specific monthly stats
	// NOTE - The hard coded 1 value for the release version corresponds to the manually added "Unique IPs" entry in
	// the release version table
	dbQuery := `
		INSERT INTO db4s_stats_monthly (stats_date, db4s_release, unique_ips)
		VALUES ($1, 1, $2)
		ON CONFLICT (stats_date, db4s_release)
			DO UPDATE
				SET unique_ips = $2
				WHERE db4s_stats_monthly.stats_date = $1
					AND db4s_stats_monthly.db4s_release IS NULL`
	commandTag, err := pg.Exec(dbQuery, date, count)
	if err != nil {
		// For now, don't bother logging a failure here.  This *might* need changing later on
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows > 1 {
		log.Printf("Wrong number of rows (%v) affected when adding a monthly stats row: %v\n", numRows, date)
	}

	// Update the version-specific monthly stats
	for i, verCount := range IPsPerUserAgent {
		// Strip the leading 'sqlitebrowser ' string from the version number
		versionString := strings.TrimPrefix(i, "sqlitebrowser ")
		dbQuery = `
		WITH ver AS (
			SELECT release_id
			FROM db4s_release_info
			WHERE version_number = $2
		)
		INSERT INTO db4s_stats_monthly (stats_date, db4s_release, unique_ips)
		SELECT $1, (SELECT release_id FROM ver), $3
		ON CONFLICT (stats_date, db4s_release)
			DO UPDATE
				SET unique_ips = $3
				WHERE db4s_stats_monthly.stats_date = $1
					AND db4s_stats_monthly.db4s_release = (SELECT release_id FROM ver)`
		commandTag, err := pg.Exec(dbQuery, date, versionString, verCount)
		if err != nil {
			// For now, don't bother logging a failure here.  This *might* need changing later on
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows > 1 {
			log.Printf("Wrong number of rows (%v) affected when adding a monthly stats row: %v\n", numRows, date)
		}
	}
	return nil
}

// saveWeeklyStats() inserts new or updated weekly stats counts into the db4s_stats_weekly table
func saveWeeklyStats(date time.Time, count int, IPsPerUserAgent map[string]int) error {
	// Update the non-version-specific weekly stats
	// NOTE - The hard coded 1 value for the release version corresponds to the manually added "Unique IPs" entry in
	// the release version table
	dbQuery := `
		INSERT INTO db4s_stats_weekly (stats_date, db4s_release, unique_ips)
		VALUES ($1, 1, $2)
		ON CONFLICT (stats_date, db4s_release)
			DO UPDATE
				SET unique_ips = $2
				WHERE db4s_stats_weekly.stats_date = $1
					AND db4s_stats_weekly.db4s_release IS NULL`
	commandTag, err := pg.Exec(dbQuery, date, count)
	if err != nil {
		// For now, don't bother logging a failure here.  This *might* need changing later on
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows > 1 {
		log.Printf("Wrong number of rows (%v) affected when adding a weekly stats row: %v\n", numRows, date)
	}

	// Update the version-specific weekly stats
	for i, verCount := range IPsPerUserAgent {
		// Strip the leading 'sqlitebrowser ' string from the version number
		versionString := strings.TrimPrefix(i, "sqlitebrowser ")
		dbQuery = `
		WITH ver AS (
			SELECT release_id
			FROM db4s_release_info
			WHERE version_number = $2
		)
		INSERT INTO db4s_stats_weekly (stats_date, db4s_release, unique_ips)
		SELECT $1, (SELECT release_id FROM ver), $3
		ON CONFLICT (stats_date, db4s_release)
			DO UPDATE
				SET unique_ips = $3
				WHERE db4s_stats_weekly.stats_date = $1
					AND db4s_stats_weekly.db4s_release = (SELECT release_id FROM ver)`
		commandTag, err := pg.Exec(dbQuery, date, versionString, verCount)
		if err != nil {
			// For now, don't bother logging a failure here.  This *might* need changing later on
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows > 1 {
			log.Printf("Wrong number of rows (%v) affected when adding a weekly stats row: %v\n", numRows, date)
		}
	}
	return nil
}

// updateUserAgents() retrieves the full list of user agents present in the daily request logs, then ensures there's an
// entry for each one in the main stats processing reference table
func updateUserAgents(ctx context.Context) error {
	span, _ := opentracing.StartSpanFromContext(ctx, "update user agents")
	defer span.Finish()

	log.Printf("Updating DB4S user agents list in the database...")

	// Get list of all (valid) user agents in the logs.  The ORDER BY clause here gives an alphabetical sorting rather
	// than numerical, but it'll do for now.
	dbQuery := `
		SELECT DISTINCT (http_user_agent)
		FROM download_log
		WHERE request = '/currentrelease'
			AND http_user_agent LIKE 'sqlitebrowser %' AND http_user_agent NOT LIKE '%AppEngine%'
		ORDER BY http_user_agent ASC`
	rows, err := pg.Query(dbQuery)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return err
	}
	defer rows.Close()
	var userAgents []string
	for rows.Next() {
		var userAgent pgtype.Text
		err = rows.Scan(&userAgent)
		if err != nil {
			log.Printf("Error retrieving rows: %v\n", err)
			return err
		}
		if userAgent.Status == pgtype.Present {
			v := strings.TrimPrefix(userAgent.String, "sqlitebrowser ")
			userAgents = append(userAgents, v)
		}
	}

	// Insert any missing user agents into the db4s_release_info table
	for _, j := range userAgents {
		dbQuery = `
			INSERT INTO db4s_release_info (version_number)
			VALUES ($1)
			ON CONFLICT DO NOTHING`
		commandTag, err := pg.Exec(dbQuery, j)
		if err != nil {
			// For now, don't bother logging a failure here.  This *might* need changing later on
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows > 1 {
			log.Printf("Wrong number of rows (%v) affected when adding release: %v\n", numRows, j)
		}
	}

	return nil
}
