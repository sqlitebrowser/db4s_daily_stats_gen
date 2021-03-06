package main

// This generates basic stats to answer two questions:
//
//   1) How many DB4S clients (unique ip address) are checking the '/currentrelease' version each day?
//
//      This should give us a rough idea of the size of our active userbase
//
//   2) How many of each version of DB4S are checking the '/currentrelease' version each day?
//
//      This should give us a rough idea of the mix of versions being used

// In the default mode (with no command line arguments), this will process all entries from the first day (2018-08-13)
// onwards.  In "daily" mode (enabled by "-d" on the command line), this only processes entries for the current time
// period and the time period immediately preceding it.  eg today and yesterday, this week and last week, this month
// and last month

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
	"github.com/mitchellh/go-homedir"
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
	Enable            bool // Should Jaeger be used?
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

	// Is this being run in daily/hourly mode from cron (or similar)?
	dailyMode = false

	// Toggle for display of debugging info
	debug = false

	// PostgreSQL Connection pool
	pg *pgx.ConnPool
)

func main() {
	// Override config file location via environment variables
	var err error
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		// TODO: Might be a good idea to add permission checks of the dir & conf file, to ensure they're not
		//       world readable.  Similar in concept to what ssh does for its config files.
		userHome, err := homedir.Dir()
		if err != nil {
			log.Fatalf("User home directory couldn't be determined: %s", "\n")
		}
		configFile = filepath.Join(userHome, ".db4s", "daily_stats_gen.toml")
	}

	// Read our configuration settings
	if _, err = toml.DecodeFile(configFile, &Conf); err != nil {
		log.Fatal(err)
	}

	// If a command line argument of "-d" was given (the only thing we check for), then enable "daily" mode
	if len(os.Args) > 1 && os.Args[1] == "-d" {
		dailyMode = true
		if debug {
			log.Println("Running in daily mode")
		}
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

	// Log successful connection if appropriate
	if debug {
		log.Printf("Connected to PostgreSQL server: %v:%v\n", Conf.Pg.Server, uint16(Conf.Pg.Port))
	}
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

	var startDate time.Time
	if dailyMode {
		// We're running in daily mode, so we start with yesterday's date and then proceed through to today
		now := time.Now()
		yr := now.Year()
		mth := now.Month()
		day := now.Day()
		today := time.Date(yr, mth, day, 0, 0, 0, 0, time.UTC)
		startDate = today.AddDate(0, 0, -1)
	} else {
		// The earliest date with entries is 2018-08-13, so we start with that.  We repeatedly call the function for
		// getting IP addresses, incrementing the date each time until we exceed time.Now()
		startDate = time.Date(2018, 8, 13, 0, 0, 0, 0, time.UTC)
	}
	endDate := startDate.Add(time.Hour * 24)
	dailySpan := tracer.StartSpan("calculate daily users")
	for endDate.Before(time.Now().AddDate(0, 0, 1)) {
		numIPs, IPsPerUserAgent, err := getIPs(startDate, endDate)
		if err != nil {
			log.Fatalf(err.Error())
		}
		err = saveDailyUsersStats(startDate, numIPs, IPsPerUserAgent)
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

	var wk int
	if dailyMode {
		// * Running in daily mode, so we just need to process the last two weeks of entries *

		// Determine which week we're in from 2018-01-01, with that being week #1.  For reference, 2018-08-13 is week #33
		now := time.Now()
		date := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
		count := 1
		for date.Before(now) {
			date = date.AddDate(0, 0, 7) // Add a week
			count++
		}

		// Wind the start date back two weeks, just to ensure we have complete coverage
		startDate = date.AddDate(0, 0, -14)

	} else {
		// Not running in daily mode, so we process all the entries in the database

		// Determine the "week of year" for 2018-08-13 (the first day with data), and use that as the starting date for
		// weekly stats.  Reference note, it should be week 33. ;)
		_, wk = time.Date(2018, 8, 13, 0, 0, 0, 0, time.UTC).ISOWeek()
		startDate = time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
		for _, w := startDate.ISOWeek(); w < wk; {
			startDate = startDate.AddDate(0, 0, 7)
			_, w = startDate.ISOWeek()
		}
	}
	endDate = startDate.AddDate(0, 0, 7)
	wkSpan := tracer.StartSpan("calculate weekly users")
	for endDate.Before(time.Now().AddDate(0, 0, 7)) {
		numIPs, IPsPerUserAgent, err := getIPs(startDate, endDate)
		if err != nil {
			log.Fatalf(err.Error())
		}
		err = saveWeeklyUsersStats(startDate, numIPs, IPsPerUserAgent)
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

	if dailyMode {
		// We're running in daily mode, so the start date is the 1st day of last month
		now := time.Now()
		yr := now.Year()
		mth := now.Month()
		thisMonth := time.Date(yr, mth, 1, 0, 0, 0, 0, time.UTC) // First date of this month
		startDate = thisMonth.AddDate(0, -1, 0)                  // Wind the start date back one month
	} else {
		// We're not running in daily mode, so we start at the beginning of the data
		startDate = time.Date(2018, 8, 1, 0, 0, 0, 0, time.UTC)
	}
	endDate = startDate.AddDate(0, 1, 0)
	mthSpan := tracer.StartSpan("calculate monthly users")
	for endDate.Before(time.Now().AddDate(0, 1, 0)) {
		numIPs, IPsPerUserAgent, err := getIPs(startDate, endDate)
		if err != nil {
			log.Fatalf(err.Error())
		}
		err = saveMonthlyUsersStats(startDate, numIPs, IPsPerUserAgent)
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

	// * Daily downloads *

	if dailyMode {
		// We're running in daily mode, so we start with yesterday's date and then proceed through to today
		now := time.Now()
		yr := now.Year()
		mth := now.Month()
		day := now.Day()
		today := time.Date(yr, mth, day, 0, 0, 0, 0, time.UTC)
		startDate = today.AddDate(0, 0, -1)
	} else {
		// The earliest date with entries is 2018-08-09, so we start with that.  We repeatedly call the function for
		// getting IP addresses, incrementing the date each time until we exceed time.Now()
		startDate = time.Date(2018, 8, 9, 0, 0, 0, 0, time.UTC)
	}
	endDate = startDate.Add(time.Hour * 24)
	dailyDLSpan := tracer.StartSpan("calculate daily downloads")
	for endDate.Before(time.Now().AddDate(0, 0, 1)) {
		numDLs, DLsPerVersion, err := getDownloads(startDate, endDate)
		if err != nil {
			log.Fatalf(err.Error())
		}
		err = saveDailyDownloadsStats(startDate, numDLs, DLsPerVersion)
		if err != nil {
			log.Fatalf(err.Error())
		}

		// Display debug info if appropriate
		if debug {
			log.Printf("Downloads for %v: %v\n", startDate.Format("2006 Jan 2"), numDLs)
		}

		startDate = startDate.AddDate(0, 0, 1)
		endDate = startDate.AddDate(0, 0, 1)
	}
	dailyDLSpan.Finish()

	// * Weekly downloads *

	if dailyMode {
		// * Running in daily mode, so we just need to process the last two weeks of entries *

		// Determine which week we're in from 2018-01-01, with that being week #1.  For reference, 2018-08-13 is week #33
		now := time.Now()
		date := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
		count := 1
		for date.Before(now) {
			date = date.AddDate(0, 0, 7) // Add a week
			count++
		}

		// Wind the start date back two weeks, just to ensure we have complete coverage
		startDate = date.AddDate(0, 0, -14)

	} else {
		// Not running in daily mode, so we process all the entries in the database

		// Determine the "week of year" for 2018-08-09 (the first day with data), and use that as the starting date for
		// weekly stats.  Reference note, it should be week 32. ;)
		_, wk = time.Date(2018, 8, 9, 0, 0, 0, 0, time.UTC).ISOWeek()
		startDate = time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
		for _, w := startDate.ISOWeek(); w < wk; {
			startDate = startDate.AddDate(0, 0, 7)
			_, w = startDate.ISOWeek()
		}
	}
	endDate = startDate.AddDate(0, 0, 7)
	wkDLSpan := tracer.StartSpan("calculate weekly downloads")
	for endDate.Before(time.Now().AddDate(0, 0, 7)) {
		numDLs, DLsPerVersion, err := getDownloads(startDate, endDate)
		if err != nil {
			log.Fatalf(err.Error())
		}
		err = saveWeeklyDownloadsStats(startDate, numDLs, DLsPerVersion)
		if err != nil {
			log.Fatalf(err.Error())
		}

		// Display debug info if appropriate
		if debug {
			yr, wk := startDate.ISOWeek()
			log.Printf("Downloads for week %v, %v: %v\n", yr, wk, numDLs)
		}

		startDate = startDate.AddDate(0, 0, 7)
		endDate = startDate.AddDate(0, 0, 7)
	}
	wkDLSpan.Finish()

	// * Monthly downloads *

	if dailyMode {
		// We're running in daily mode, so the start date is the 1st day of last month
		now := time.Now()
		yr := now.Year()
		mth := now.Month()
		thisMonth := time.Date(yr, mth, 1, 0, 0, 0, 0, time.UTC) // First date of this month
		startDate = thisMonth.AddDate(0, -1, 0)                  // Wind the start date back one month
	} else {
		// We're not running in daily mode, so we start at the beginning of the data
		startDate = time.Date(2018, 8, 1, 0, 0, 0, 0, time.UTC)
	}
	endDate = startDate.AddDate(0, 1, 0)
	mthDLSpan := tracer.StartSpan("calculate monthly downloads")
	for endDate.Before(time.Now().AddDate(0, 1, 0)) {
		numDLs, DLsPerVersion, err := getDownloads(startDate, endDate)
		if err != nil {
			log.Fatalf(err.Error())
		}
		err = saveMonthlyDownloadsStats(startDate, numDLs, DLsPerVersion)
		if err != nil {
			log.Fatalf(err.Error())
		}

		// Display debug info if appropriate
		if debug {
			log.Printf("Downloads for month %v: %v\n", startDate.Format("2006 Jan"), numDLs)
		}

		startDate = startDate.AddDate(0, 1, 0)
		endDate = startDate.AddDate(0, 1, 0)
	}
	mthDLSpan.Finish()

	// Close the PG connection gracefully
	pg.Close()

	// Display debug info if appropriate
	if debug {
		log.Println("Done")
	}
}

// getDownloads() returns the total number of DB4S downloads in the given date range, plus a breakdown per DB4S version
func getDownloads(startDate time.Time, endDate time.Time) (DLs int32, DLsPerVersion map[int]int32, err error) {
	// Retrieve count of all valid download requests for the desired time range
	DLsPerVersion = make(map[int]int32)
	dbQuery := `
		SELECT count(*)
		FROM download_log
		WHERE (request = '/DB.Browser.for.SQLite-3.10.1.dmg'
			OR request = '/DB.Browser.for.SQLite-3.10.1-win32.exe'
			OR request = '/DB.Browser.for.SQLite-3.10.1-win64.exe'
			OR request = '/SQLiteDatabaseBrowserPortable_3.10.1_English.paf.exe'
			OR request = '/DB.Browser.for.SQLite-3.11.0-win32.msi'
			OR request = '/DB.Browser.for.SQLite-3.11.0-win32.zip'
			OR request = '/DB.Browser.for.SQLite-3.11.0-win64.msi'
			OR request = '/DB.Browser.for.SQLite-3.11.0-win64.zip'
			OR request = '/DB.Browser.for.SQLite-3.11.0.dmg'
			OR request = '/DB.Browser.for.SQLite-3.11.1-win32.msi'
			OR request = '/DB.Browser.for.SQLite-3.11.1-win32.zip'
			OR request = '/DB.Browser.for.SQLite-3.11.1-win64.msi'
			OR request = '/DB.Browser.for.SQLite-3.11.1-win64.zip'
			OR request = '/DB.Browser.for.SQLite-3.11.1.dmg'
			OR request = '/DB.Browser.for.SQLite-3.11.1v2.dmg'
			OR request = '/DB.Browser.for.SQLite-3.11.2-win32.msi'
			OR request = '/DB.Browser.for.SQLite-3.11.2-win32.zip'
			OR request = '/DB.Browser.for.SQLite-3.11.2-win64.msi'
			OR request = '/DB.Browser.for.SQLite-3.11.2-win64.zip'
			OR request = '/DB.Browser.for.SQLite-3.11.2.dmg'
			OR request = '/SQLiteDatabaseBrowserPortable_3.11.2_English.paf.exe'
			OR request = '/SQLiteDatabaseBrowserPortable_3.11.2_Rev_2_English.paf.exe'
			OR request = '/DB.Browser.for.SQLite-3.12.0-win32.msi'
			OR request = '/DB.Browser.for.SQLite-3.12.0-win32.zip'
			OR request = '/DB.Browser.for.SQLite-3.12.0-win64.msi'
			OR request = '/DB.Browser.for.SQLite-3.12.0-win64.zip'
			OR request = '/DB.Browser.for.SQLite-3.12.0.dmg'
			OR request = '/SQLiteDatabaseBrowserPortable_3.12.0_English.paf.exe')
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&DLs)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}

	// * Counts specific downloads for the desired time range *

	// 3.10.1
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.10.1.dmg'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	var a int32
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[1] = a // 1 is "3.10.1 macOS" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.10.1-win32.exe'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[2] = a // 2 is "3.10.1 win32" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.10.1-win64.exe'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[3] = a // 3 is "3.10.1 win64" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/SQLiteDatabaseBrowserPortable_3.10.1_English.paf.exe'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[4] = a // 4 is "3.10.1 Portable" (as per the db4s_download_info table)

	// 3.11.0
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.11.0-win32.msi'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[5] = a // 5 is "3.11.0 Win32 MSI" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.11.0-win32.zip'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[6] = a // 6 is "3.11.0 Win32 .zip" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.11.0-win64.msi'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[7] = a // 7 is "3.11.0 Win64 MSI" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.11.0-win64.zip'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[8] = a // 8 is "3.11.0 Win64 .zip" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.11.0.dmg'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[9] = a // 9 is "3.11.0 macOS" (as per the db4s_download_info table)

	// 3.11.1
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.11.1-win32.msi'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[10] = a // 10 is "3.11.1 Win32 MSI" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.11.1-win32.zip'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[11] = a // 11 is "3.11.1 Win32 .zip" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.11.1-win64.msi'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[12] = a // 12 is "3.11.1 Win64 MSI" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.11.1-win64.zip'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[13] = a // 13 is "3.11.1 Win64 .zip" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE (request = '/DB.Browser.for.SQLite-3.11.1.dmg'
			OR request = '/DB.Browser.for.SQLite-3.11.1v2.dmg')
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[14] = a // 14 is "3.11.1 macOS" (as per the db4s_download_info table)

	// 3.11.2
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.11.2-win32.msi'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[15] = a // 15 is "3.11.2 Win32 MSI" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.11.2-win32.zip'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[16] = a // 16 is "3.11.2 Win32 .zip" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.11.2-win64.msi'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[17] = a // 17 is "3.11.2 Win64 MSI" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.11.2-win64.zip'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[18] = a // 18 is "3.11.2 Win64 .zip" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.11.2.dmg'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[19] = a // 19 is "3.11.2 macOS" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/SQLiteDatabaseBrowserPortable_3.11.2_English.paf.exe'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[20] = a // 20 is "3.11.2 Portable" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/SQLiteDatabaseBrowserPortable_3.11.2_Rev_2_English.paf.exe'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[21] = a // 21 is "3.11.2 Portable v2" (as per the db4s_download_info table)

	// 3.12.0
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.12.0-win32.msi'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[22] = a // 22 is "DB4S 3.12.0 win32 msi" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.12.0-win32.zip'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[23] = a // 23 is "DB4S 3.12.0 win32 zip" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.12.0-win64.msi'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[24] = a // 24 is "DB4S 3.12.0 win64 msi" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.12.0-win64.zip'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[25] = a // 25 is "DB4S 3.12.0 win64 zip" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.12.0.dmg'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[26] = a // 26 is "DB4S 3.12.0 macOS" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/SQLiteDatabaseBrowserPortable_3.12.0_English.paf.exe'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = pg.QueryRow(dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[27] = a // 27 is "DB4S 3.12.0 Portable" (as per the db4s_download_info table)

	return
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
			AND request_time < $2
			AND status = 200`
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
	if !Conf.Jaeger.Enable {
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

// saveDailyDownloadsStats() inserts new or updated daily download stats counts into the db4s_downloads_daily table
func saveDailyDownloadsStats(date time.Time, count int32, DLsPerVersion map[int]int32) error {
	// Update the non-version-specific daily stats
	// NOTE - The hard coded 0 value for the db4s download corresponds to the manually added "Total downloads" entry in
	// the DB4S download info table
	dbQuery := `
		INSERT INTO db4s_downloads_daily (stats_date, db4s_download, num_downloads)
		VALUES ($1, 0, $2)
		ON CONFLICT (stats_date, db4s_download)
			DO UPDATE
				SET num_downloads = $2
				WHERE db4s_downloads_daily.stats_date = $1
					AND db4s_downloads_daily.db4s_download = 0`
	commandTag, err := pg.Exec(dbQuery, date, count)
	if err != nil {
		// For now, don't bother logging a failure here.  This *might* need changing later on
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected when adding a daily download stats row: %v\n", numRows, date)
	}

	// Update the version-specific daily download stats
	for version, DLCount := range DLsPerVersion {
		dbQuery = `
		INSERT INTO db4s_downloads_daily (stats_date, db4s_download, num_downloads)
		VALUES ($1, $2, $3)
		ON CONFLICT (stats_date, db4s_download)
			DO UPDATE
				SET num_downloads = $3
				WHERE db4s_downloads_daily.stats_date = $1
					AND db4s_downloads_daily.db4s_download = $2`
		commandTag, err := pg.Exec(dbQuery, date, version, DLCount)
		if err != nil {
			// For now, don't bother logging a failure here.  This *might* need changing later on
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows > 1 {
			log.Printf("Wrong number of rows (%v) affected when adding a daily download stats row: %v\n", numRows, date)
		}
	}
	return nil
}

// saveDailyUsersStats() inserts new or updated daily stats counts into the db4s_users_daily table
func saveDailyUsersStats(date time.Time, count int, IPsPerUserAgent map[string]int) error {
	// Update the non-version-specific daily stats
	// NOTE - The hard coded 1 value for the release version corresponds to the manually added "Unique IPs" entry in
	// the DB4S release info table
	dbQuery := `
		INSERT INTO db4s_users_daily (stats_date, db4s_release, unique_ips)
		VALUES ($1, 1, $2)
		ON CONFLICT (stats_date, db4s_release)
			DO UPDATE
				SET unique_ips = $2
				WHERE db4s_users_daily.stats_date = $1
					AND db4s_users_daily.db4s_release = 1`
	commandTag, err := pg.Exec(dbQuery, date, count)
	if err != nil {
		// For now, don't bother logging a failure here.  This *might* need changing later on
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
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
		INSERT INTO db4s_users_daily (stats_date, db4s_release, unique_ips)
		SELECT $1, (SELECT release_id FROM ver), $3
		ON CONFLICT (stats_date, db4s_release)
			DO UPDATE
				SET unique_ips = $3
				WHERE db4s_users_daily.stats_date = $1
					AND db4s_users_daily.db4s_release = (SELECT release_id FROM ver)`
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

// saveMonthlyDownloadsStats() inserts new or updated monthly download stats counts into the db4s_downloads_monthly table
func saveMonthlyDownloadsStats(date time.Time, count int32, DLsPerVersion map[int]int32) error {
	// Update the non-version-specific monthly stats
	// NOTE - The hard coded 0 value for the db4s download corresponds to the manually added "Total downloads" entry in
	// the DB4S download info table
	dbQuery := `
		INSERT INTO db4s_downloads_monthly (stats_date, db4s_download, num_downloads)
		VALUES ($1, 0, $2)
		ON CONFLICT (stats_date, db4s_download)
			DO UPDATE
				SET num_downloads = $2
				WHERE db4s_downloads_monthly.stats_date = $1
					AND db4s_downloads_monthly.db4s_download = 0`
	commandTag, err := pg.Exec(dbQuery, date, count)
	if err != nil {
		// For now, don't bother logging a failure here.  This *might* need changing later on
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected when adding a monthly download stats row: %v\n", numRows, date)
	}

	// Update the version-specific monthly download stats
	for version, DLCount := range DLsPerVersion {
		dbQuery = `
		INSERT INTO db4s_downloads_monthly (stats_date, db4s_download, num_downloads)
		VALUES ($1, $2, $3)
		ON CONFLICT (stats_date, db4s_download)
			DO UPDATE
				SET num_downloads = $3
				WHERE db4s_downloads_monthly.stats_date = $1
					AND db4s_downloads_monthly.db4s_download = $2`
		commandTag, err := pg.Exec(dbQuery, date, version, DLCount)
		if err != nil {
			// For now, don't bother logging a failure here.  This *might* need changing later on
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows > 1 {
			log.Printf("Wrong number of rows (%v) affected when adding a monthly download stats row: %v\n", numRows, date)
		}
	}
	return nil
}

// saveMonthlyUsersStats() inserts new or updated weekly stats counts into the db4s_users_monthly table
func saveMonthlyUsersStats(date time.Time, count int, IPsPerUserAgent map[string]int) error {
	// Update the non-version-specific monthly stats
	// NOTE - The hard coded 1 value for the release version corresponds to the manually added "Unique IPs" entry in
	// the release version table
	dbQuery := `
		INSERT INTO db4s_users_monthly (stats_date, db4s_release, unique_ips)
		VALUES ($1, 1, $2)
		ON CONFLICT (stats_date, db4s_release)
			DO UPDATE
				SET unique_ips = $2
				WHERE db4s_users_monthly.stats_date = $1
					AND db4s_users_monthly.db4s_release = 1`
	commandTag, err := pg.Exec(dbQuery, date, count)
	if err != nil {
		// For now, don't bother logging a failure here.  This *might* need changing later on
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
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
		INSERT INTO db4s_users_monthly (stats_date, db4s_release, unique_ips)
		SELECT $1, (SELECT release_id FROM ver), $3
		ON CONFLICT (stats_date, db4s_release)
			DO UPDATE
				SET unique_ips = $3
				WHERE db4s_users_monthly.stats_date = $1
					AND db4s_users_monthly.db4s_release = (SELECT release_id FROM ver)`
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

// saveWeeklyDownloadsStats() inserts new or updated weekly download stats counts into the db4s_downloads_weekly table
func saveWeeklyDownloadsStats(date time.Time, count int32, DLsPerVersion map[int]int32) error {
	// Update the non-version-specific weekly stats
	// NOTE - The hard coded 0 value for the db4s download corresponds to the manually added "Total downloads" entry in
	// the DB4S download info table
	dbQuery := `
		INSERT INTO db4s_downloads_weekly (stats_date, db4s_download, num_downloads)
		VALUES ($1, 0, $2)
		ON CONFLICT (stats_date, db4s_download)
			DO UPDATE
				SET num_downloads = $2
				WHERE db4s_downloads_weekly.stats_date = $1
					AND db4s_downloads_weekly.db4s_download = 0`
	commandTag, err := pg.Exec(dbQuery, date, count)
	if err != nil {
		// For now, don't bother logging a failure here.  This *might* need changing later on
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected when adding a weekly download stats row: %v\n", numRows, date)
	}

	// Update the version-specific weekly download stats
	for version, DLCount := range DLsPerVersion {
		dbQuery = `
		INSERT INTO db4s_downloads_weekly (stats_date, db4s_download, num_downloads)
		VALUES ($1, $2, $3)
		ON CONFLICT (stats_date, db4s_download)
			DO UPDATE
				SET num_downloads = $3
				WHERE db4s_downloads_weekly.stats_date = $1
					AND db4s_downloads_weekly.db4s_download = $2`
		commandTag, err := pg.Exec(dbQuery, date, version, DLCount)
		if err != nil {
			// For now, don't bother logging a failure here.  This *might* need changing later on
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows > 1 {
			log.Printf("Wrong number of rows (%v) affected when adding a weekly download stats row: %v\n", numRows, date)
		}
	}
	return nil
}

// saveWeeklyUsersStats() inserts new or updated weekly stats counts into the db4s_users_weekly table
func saveWeeklyUsersStats(date time.Time, count int, IPsPerUserAgent map[string]int) error {
	// Update the non-version-specific weekly stats
	// NOTE - The hard coded 1 value for the release version corresponds to the manually added "Unique IPs" entry in
	// the release version table
	dbQuery := `
		INSERT INTO db4s_users_weekly (stats_date, db4s_release, unique_ips)
		VALUES ($1, 1, $2)
		ON CONFLICT (stats_date, db4s_release)
			DO UPDATE
				SET unique_ips = $2
				WHERE db4s_users_weekly.stats_date = $1
					AND db4s_users_weekly.db4s_release = 1`
	commandTag, err := pg.Exec(dbQuery, date, count)
	if err != nil {
		// For now, don't bother logging a failure here.  This *might* need changing later on
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
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
		INSERT INTO db4s_users_weekly (stats_date, db4s_release, unique_ips)
		SELECT $1, (SELECT release_id FROM ver), $3
		ON CONFLICT (stats_date, db4s_release)
			DO UPDATE
				SET unique_ips = $3
				WHERE db4s_users_weekly.stats_date = $1
					AND db4s_users_weekly.db4s_release = (SELECT release_id FROM ver)`
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

	if debug {
		log.Printf("Updating DB4S user agents list in the database...")
	}

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
