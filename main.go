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
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/jackc/pgx/v5/pgtype"
	pgpool "github.com/jackc/pgx/v5/pgxpool"
)

// Configuration file
type TomlConfig struct {
	Pg PGInfo
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
	DB *pgpool.Pool
)

func main() {
	// Override config file location via environment variables
	var err error
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		// TODO: Might be a good idea to add permission checks of the dir & conf file, to ensure they're not
		//       world readable.  Similar in concept to what ssh does for its config files.
		userHome, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("User home directory couldn't be determined: %s", "\n")
		}
		configFile = filepath.Join(userHome, ".db4s", "daily_stats_gen.toml")
	}

	// Read our configuration settings
	if _, err = toml.DecodeFile(configFile, &Conf); err != nil {
		log.Fatal(err)
	}

	// Check if an environment variable override for debug mode was present
	debugEnv := os.Getenv("DB4S_DAILY_STATS_DEBUG")
	if debugEnv != "" {
		debug, err = strconv.ParseBool(debugEnv)
		if err != nil {
			log.Fatalf("Couldn't parse DB4S_DAILY_STATS_DEBUG environment variable")
		}
	}
	if debug {
		log.Println("Running with debug output enabled")
	}

	// If a command line argument of "-d" was given (the only thing we check for), then enable "daily" mode
	if len(os.Args) > 1 && os.Args[1] == "-d" {
		dailyMode = true
		if debug {
			log.Println("Running in daily mode")
		}
	}

	// * Connect to PG database *

	// Prepare TLS configuration
	tlsConfig := tls.Config{}
	if Conf.Pg.SSL {
		tlsConfig.ServerName = Conf.Pg.Server
		tlsConfig.InsecureSkipVerify = false
	} else {
		tlsConfig.InsecureSkipVerify = true
	}

	// Set the main PostgreSQL database configuration values
	pgConfig, err := pgpool.ParseConfig(fmt.Sprintf("host=%s port=%d user= %s password = %s dbname=%s pool_max_conns=%d connect_timeout=10", Conf.Pg.Server, uint16(Conf.Pg.Port), Conf.Pg.Username, Conf.Pg.Password, Conf.Pg.Database, Conf.Pg.NumConnections))
	if err != nil {
		return
	}

	// Gorm connection string
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s connect_timeout=10 sslmode=", Conf.Pg.Server, uint16(Conf.Pg.Port), Conf.Pg.Username, Conf.Pg.Password, Conf.Pg.Database)

	// Enable encrypted connections where needed
	if Conf.Pg.SSL {
		pgConfig.ConnConfig.TLSConfig = &tlsConfig
		dsn += "require"
	} else {
		dsn += "disable"
	}

	// Connect to database
	DB, err = pgpool.New(context.Background(), pgConfig.ConnString())
	if err != nil {
		log.Fatal(err)
	}

	// Log successful connection if appropriate
	if debug {
		log.Printf("Connected to PostgreSQL server: %v:%v\n", Conf.Pg.Server, uint16(Conf.Pg.Port))
	}

	// Add any new user agents to the db4s_release_info table
	err = updateUserAgents(context.Background())
	if err != nil {
		log.Fatalf(err.Error())
	}

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

	// Close the PG connection gracefully
	DB.Close()

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
			OR request = '/SQLiteDatabaseBrowserPortable_3.12.0_English.paf.exe'
			OR request = '/DB.Browser.for.SQLite-3.12.2-win32.msi'
			OR request = '/DB.Browser.for.SQLite-3.12.2-win32.zip'
			OR request = '/DB.Browser.for.SQLite-3.12.2-win64.msi'
			OR request = '/DB.Browser.for.SQLite-3.12.2-win64.zip'
			OR request = '/DB.Browser.for.SQLite-3.12.2.dmg'
			OR request = '/DB.Browser.for.SQLite-arm64-3.12.2.dmg'
			OR request = '/SQLiteDatabaseBrowserPortable_3.12.2_English.paf.exe'
			OR request = '/DB.Browser.for.SQLite-v3.13.0.dmg'
			OR request = '/DB.Browser.for.SQLite-v3.13.0-win32.msi'
			OR request = '/DB.Browser.for.SQLite-v3.13.0-win32.zip'
			OR request = '/DB.Browser.for.SQLite-v3.13.0-win64.msi'
			OR request = '/DB.Browser.for.SQLite-v3.13.0-win64.zip'
			OR request = '/DB.Browser.for.SQLite-v3.13.0-x86.64.AppImage'
	    )
		AND request_time > $1
		AND request_time < $2
		AND status = 200`
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&DLs)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
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
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[27] = a // 27 is "DB4S 3.12.0 Portable" (as per the db4s_download_info table)

	// 3.12.2
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.12.2-win32.msi'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[28] = a // 28 is "DB4S 3.12.2 win32 msi" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.12.2-win32.zip'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[29] = a // 29 is "DB4S 3.12.2 win32 zip" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.12.2-win64.msi'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[30] = a // 30 is "DB4S 3.12.2 win64 msi" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.12.2-win64.zip'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[31] = a // 31 is "DB4S 3.12.2 win64 zip" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-3.12.2.dmg'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[32] = a // 32 is "DB4S 3.12.2 macOS" (as per the db4s_download_info table)
	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/SQLiteDatabaseBrowserPortable_3.12.2_English.paf.exe'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[33] = a // 33 is "DB4S 3.12.2 Portable" (as per the db4s_download_info table)

	// 3.13.0

	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-arm64-3.12.2.dmg'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[34] = a // 34 is "DB.Browser.for.SQLite-arm64-3.12.2.dmg" (as per the db4s_download_info table)

	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-v3.13.0.dmg'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[35] = a // 35 is "DB.Browser.for.SQLite-v3.13.0.dmg" (as per the db4s_download_info table)

	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-v3.13.0-win32.msi'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[36] = a // 36 is "DB.Browser.for.SQLite-v3.13.0-win32.msi" (as per the db4s_download_info table)

	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-v3.13.0-win32.zip'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[37] = a // 37 is "DB.Browser.for.SQLite-v3.13.0-win32.zip" (as per the db4s_download_info table)

	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-v3.13.0-win64.msi'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[38] = a // 38 is "DB.Browser.for.SQLite-v3.13.0-win64.msi" (as per the db4s_download_info table)

	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-v3.13.0-win64.zip'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[39] = a // 39 is "DB.Browser.for.SQLite-v3.13.0-win64.zip" (as per the db4s_download_info table)

	dbQuery = `
		SELECT count(*)
		FROM download_log
		WHERE request = '/DB.Browser.for.SQLite-v3.13.0-x86.64.AppImage'
			AND request_time > $1
			AND request_time < $2
			AND status = 200`
	err = DB.QueryRow(context.Background(), dbQuery, &startDate, &endDate).Scan(&a)
	if err != nil {
		log.Fatalf("Database query failed: %v\n", err)
		return
	}
	DLsPerVersion[40] = a // 40 is "DB.Browser.for.SQLite-v3.13.0-x86.64.AppImage" (as per the db4s_download_info table)
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
	rows, err := DB.Query(context.Background(), dbQuery, &startDate, &endDate)
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
		if IPStrange.String != "" && IPStrange.Valid {
			IPHash = md5.Sum([]byte(IPStrange.String))
		} else if IPv6.String != "" && IPv6.Valid {
			IPHash = md5.Sum([]byte(IPv6.String))
		} else if IPv4.String != "" && IPv4.Valid {
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
	commandTag, err := DB.Exec(context.Background(), dbQuery, date, count)
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
		commandTag, err := DB.Exec(context.Background(), dbQuery, date, version, DLCount)
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
	commandTag, err := DB.Exec(context.Background(), dbQuery, date, count)
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
		commandTag, err := DB.Exec(context.Background(), dbQuery, date, versionString, verCount)
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
	commandTag, err := DB.Exec(context.Background(), dbQuery, date, count)
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
		commandTag, err := DB.Exec(context.Background(), dbQuery, date, version, DLCount)
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
	commandTag, err := DB.Exec(context.Background(), dbQuery, date, count)
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
		commandTag, err := DB.Exec(context.Background(), dbQuery, date, versionString, verCount)
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
	commandTag, err := DB.Exec(context.Background(), dbQuery, date, count)
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
		commandTag, err := DB.Exec(context.Background(), dbQuery, date, version, DLCount)
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
	commandTag, err := DB.Exec(context.Background(), dbQuery, date, count)
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
		commandTag, err := DB.Exec(context.Background(), dbQuery, date, versionString, verCount)
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
	rows, err := DB.Query(context.Background(), dbQuery)
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
		if userAgent.String != "" && userAgent.Valid {
			v := strings.TrimPrefix(userAgent.String, "sqlitebrowser ")
			userAgents = append(userAgents, v)
		}
	}

	// Insert any missing user agents into the db4s_release_info table
	for _, j := range userAgents {
		if debug {
			log.Printf("Adding user agent '%v'", j)
		}

		dbQuery = `
			INSERT INTO db4s_release_info (version_number)
			VALUES ($1)
			ON CONFLICT DO NOTHING`
		commandTag, err := DB.Exec(context.Background(), dbQuery, j)
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
