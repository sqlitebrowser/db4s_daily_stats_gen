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
	"crypto/md5"
	"crypto/tls"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/jackc/pgx"
	"github.com/jackc/pgx/pgtype"
	"github.com/minio/go-homedir"
	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/config"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
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

	// PostgreSQL Connection pool
	pg *pgx.ConnPool

	tracer opentracing.Tracer
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
	var closer io.Closer
	tracer, closer = initJaeger("db4s stats generator")
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

	// Open connection to PG

	// * Daily users *
	var dbQuery string

	type entry struct {
		IPv4        pgtype.Text
		IPv6        pgtype.Text
		IPStrange   pgtype.Text
	}

	// This nested map approach (inside of a combined key) should allow for counting the # of unique IP's per user agent
	IPsPerUserAgent := make(map[string]map[[16]byte]int)

	uniqueIPs := make(map[[16]byte]int)

	// Retrieve entire result set of valid `/currentrelease` requests for the desired time range
	dbQuery = `
		SELECT http_user_agent, client_ipv4, client_ipv6, client_ip_strange
		FROM download_log
		WHERE request = '/currentrelease'
  			AND http_user_agent LIKE 'sqlitebrowser %' AND http_user_agent NOT LIKE '%AppEngine%'
			AND request_time > '2018-08-13 00:00 UTC'
			AND request_time < '2018-08-14 00:00 UTC'`
	rows, err := pg.Query(dbQuery)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return
	}
	defer rows.Close()
    rowCount := 0
	for rows.Next() {
		rowCount++
		var userAgent pgtype.Text
		var row entry
		err = rows.Scan(&userAgent, &row.IPv4, &row.IPv6, &row.IPStrange)
		if err != nil {
			log.Printf("Error retrieving rows: %v\n", err)
			return
		}

		// Work out the key to use.  We use a hash of the IP address, to stop weird characters in the IP Strange field
		// being a problem
		var IPHash [16]byte
		if row.IPStrange.Status == pgtype.Present {
			IPHash = md5.Sum([]byte(row.IPStrange.String))
		} else if row.IPv6.Status == pgtype.Present {
			IPHash = md5.Sum([]byte(row.IPv6.String))
		} else if row.IPv4.Status == pgtype.Present {
			IPHash = md5.Sum([]byte(row.IPv4.String))
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

	// Info while developing
	log.Printf("Number of rows for 2018-08-13: %v\n", rowCount)
	log.Printf("IP addresses for 2018-08-13: %v\n", len(uniqueIPs))

    // Number of unique IP addresses per user agent
    for i, j := range IPsPerUserAgent {
		log.Printf("User agent: %v  Unique IP addresses: %v\n", i, len(j))
	}


	// * Weekly users *

	// * Monthly users *

	// Close the PG connection gracefully
	pg.Close()
}

// initJaeger returns an instance of Jaeger Tracer
func initJaeger(service string) (opentracing.Tracer, io.Closer) {
	cfg := &config.Configuration{
		ServiceName: service,
		Sampler: &config.SamplerConfig{
			Type:  "const",
			Param: 1,
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